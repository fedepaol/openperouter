#!/usr/bin/env bash
# Add isolated libvirt networks and static node interfaces after installation.
#
# Required configuration (normally config_$USER.sh):
POST_INSTALL_EXTRA_NETWORK_NAMES="toswitch1 toswitch2"
POST_INSTALL_TOSWITCH1_GATEWAY_V4="192.168.11.1/24"
POST_INSTALL_TOSWITCH1_GATEWAY_V6="2001:db8:11::1/64"
POST_INSTALL_TOSWITCH1_NODE_IPS_V4="192.168.11.20/24 192.168.11.21/24 192.168.11.22/24 192.168.11.30/24 192.168.11.31/24"
POST_INSTALL_TOSWITCH1_NODE_IPS_V6="2001:db8:11::14/64 2001:db8:11::15/64 2001:db8:11::16/64 2001:db8:11::1e/64 2001:db8:11::1f/64"
POST_INSTALL_TOSWITCH2_GATEWAY_V4="192.168.12.1/24"
POST_INSTALL_TOSWITCH2_GATEWAY_V6="2001:db8:12::1/64"
POST_INSTALL_TOSWITCH2_NODE_IPS_V4="192.168.12.20/24 192.168.12.21/24 192.168.12.22/24 192.168.12.30/24 192.168.12.31/24"
POST_INSTALL_TOSWITCH2_NODE_IPS_V6="2001:db8:12::14/64 2001:db8:12::15/64 2001:db8:12::16/64 2001:db8:12::1e/64 2001:db8:12::1f/64"

# Node IP order: masters first, then workers. Repeat variables per network.
set -euo pipefail

output_dir="${1:-/tmp/dev-scripts-setup-extra-networks}"
mkdir -p "$output_dir"
exec > >(tee "$output_dir/run.log") 2>&1

config_file="${CONFIG:-$PWD/config_${USER}.sh}"
if [[ ! -r "$config_file" ]]; then
  echo "Missing config: $config_file"
  exit 2
fi
# shellcheck source=/dev/null
source "$config_file"

: "${CLUSTER_NAME:?CLUSTER_NAME must be set}"
: "${NUM_MASTERS:?NUM_MASTERS must be set}"
: "${POST_INSTALL_EXTRA_NETWORK_NAMES:?Set POST_INSTALL_EXTRA_NETWORK_NAMES; do not set EXTRA_NETWORK_NAMES during install}"
KUBECONFIG="${KUBECONFIG:-$PWD/ocp/$CLUSTER_NAME/auth/kubeconfig}"
export KUBECONFIG
BAREMETAL_NETWORK_NAME="${BAREMETAL_NETWORK_NAME:-${CLUSTER_NAME}bm}"

if ! oc get nodes >/dev/null; then
  echo "Cluster unavailable through KUBECONFIG=$KUBECONFIG"
  exit 2
fi

prefix_name() {
  tr '[:lower:]-' '[:upper:]_' <<<"$1"
}

v4_netmask() {
  local prefix="$1" mask= value i
  (( prefix >= 0 && prefix <= 32 )) || return 1
  value=0
  for ((i = 0; i < 4; i++)); do
    if (( prefix >= 8 )); then mask+="255"; prefix=$((prefix - 8))
    elif (( prefix == 0 )); then mask+="0"
    else value=$((256 - 2 ** (8 - prefix))); mask+="$value"; prefix=0
    fi
    (( i < 3 )) && mask+='.'
  done
  printf '%s\n' "$mask"
}

define_network() {
  local network="$1" gateway_v4="$2" gateway_v6="$3" xml="$output_dir/$network.xml"
  local ip prefix mask
  {
    printf '<network>\n  <name>%s</name>\n  <bridge name="%s" stp="on" delay="0"/>\n' "$network" "$network"
    if [[ -n "$gateway_v4" ]]; then
      ip="${gateway_v4%/*}"; prefix="${gateway_v4#*/}"; mask="$(v4_netmask "$prefix")"
      printf '  <ip address="%s" netmask="%s"/>\n' "$ip" "$mask"
    fi
    if [[ -n "$gateway_v6" ]]; then
      ip="${gateway_v6%/*}"; prefix="${gateway_v6#*/}"
      printf '  <ip family="ipv6" address="%s" prefix="%s"/>\n' "$ip" "$prefix"
    fi
    printf '</network>\n'
  } >"$xml"

  if ! sudo virsh net-info "$network" >/dev/null 2>&1; then
    sudo virsh net-define "$xml"
    sudo virsh net-autostart "$network"
  fi
  if ! sudo virsh net-info "$network" | grep -q 'Active:.*yes'; then
    sudo virsh net-start "$network"
  fi
}

configure_node() {
  local network="$1" net_index="$2" role="$3" node_index="$4" ip4="$5" ip6="$6"
  local domain="${CLUSTER_NAME}_${role}_${node_index}"
  local node_ip host_mac mac iface profile mac_node_index="$node_index"
  host_mac="$(sudo virsh domiflist "$domain" | awk -v network="$BAREMETAL_NETWORK_NAME" '$3 == network { print $5; exit }')"
  node_ip="$(sudo virsh net-dhcp-leases "$BAREMETAL_NETWORK_NAME" | awk -v mac="$host_mac" '$3 == mac && $4 == "ipv4" { sub(/\/.*/, "", $5); print $5; exit }')"
  [[ -n "$node_ip" ]] || { echo "No IPv4 lease for $domain on $BAREMETAL_NETWORK_NAME"; return 1; }

  mac="$(sudo virsh domiflist "$domain" | awk -v network="$network" '$3 == network { print $5; exit }')"
  if [[ -z "$mac" ]]; then
    [[ "$role" == worker ]] && mac_node_index=$((node_index + 128))
    printf -v mac '52:54:00:ee:%02x:%02x' "$net_index" "$mac_node_index"
    sudo virsh attach-interface "$domain" network "$network" --model virtio --mac "$mac" --live --config
  fi

  for _ in {1..12}; do
    iface="$(ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "core@$node_ip" \
      "ip -o link | awk -v mac='$mac' 'tolower(\$0) ~ mac { gsub(/:/, \"\", \$2); print \$2; exit }'" 2>/dev/null || true)"
    [[ -n "$iface" ]] && break
    sleep 2
  done
  [[ -n "$iface" ]] || { echo "Cannot find $mac on $node"; return 1; }

  profile="postinstall-$network"
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "core@$node_ip" \
    "sudo nmcli connection delete '$profile' >/dev/null 2>&1 || true; \
     sudo nmcli connection add type ethernet ifname '$iface' con-name '$profile' \
       ipv4.method '$([[ -n "$ip4" ]] && echo manual || echo disabled)' \
       ipv4.addresses '$ip4' ipv4.never-default yes \
       ipv6.method '$([[ -n "$ip6" ]] && echo manual || echo disabled)' \
       ipv6.addresses '$ip6' ipv6.never-default yes; \
     sudo nmcli connection up '$profile'; ip -brief address show dev '$iface'"
}

network_index=0
for network in $POST_INSTALL_EXTRA_NETWORK_NAMES; do
  variable_prefix="$(prefix_name "$network")"
  gateway_v4_var="POST_INSTALL_${variable_prefix}_GATEWAY_V4"
  gateway_v6_var="POST_INSTALL_${variable_prefix}_GATEWAY_V6"
  ips_v4_var="POST_INSTALL_${variable_prefix}_NODE_IPS_V4"
  ips_v6_var="POST_INSTALL_${variable_prefix}_NODE_IPS_V6"
  gateway_v4="${!gateway_v4_var:-}"
  gateway_v6="${!gateway_v6_var:-}"
  read -r -a ips_v4 <<<"${!ips_v4_var:-}"
  read -r -a ips_v6 <<<"${!ips_v6_var:-}"
  [[ -n "$gateway_v4$gateway_v6" ]] || { echo "$network needs gateway IPv4 or IPv6"; exit 2; }

  define_network "$network" "$gateway_v4" "$gateway_v6"
  node_position=0
  for node_index in $(seq 0 $((NUM_MASTERS - 1))); do
    configure_node "$network" "$network_index" master "$node_index" "${ips_v4[$node_index]:-}" "${ips_v6[$node_index]:-}"
    node_position=$((node_position + 1))
  done
  if (( ${NUM_WORKERS:-0} > 0 )); then
    for node_index in $(seq 0 $((NUM_WORKERS - 1))); do
      configure_node "$network" "$network_index" worker "$node_index" "${ips_v4[$node_position]:-}" "${ips_v6[$node_position]:-}"
      node_position=$((node_position + 1))
    done
  fi
  network_index=$((network_index + 1))
done

echo "Done. Output: $output_dir"
