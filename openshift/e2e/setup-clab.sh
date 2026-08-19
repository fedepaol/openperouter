#!/bin/bash
# Sets up the containerlab fabric and wires it to an OCP cluster deployed via dev-scripts.
#
# Prerequisites:
#   - OCP cluster running via dev-scripts with EXTRA_NETWORK_NAMES="toswitch1 toswitch2"
#     and both _V4 and _V6 subnets configured for dual-stack
#   - OpenPerOuter operator deployed on the cluster
#   - containerlab installed
#   - podman available
#
# Produces:
#   - Running clab topology wired to extra network bridges
#   - nodelink.json for the test suite
#
# Usage:
#   export KUBECONFIG=/root/dev-scripts/ocp/ostest/auth/kubeconfig
#   ./openshift/e2e/setup-clab.sh
#   cd e2etests && CONTAINER_RUNTIME=podman go test -v ./suite/ \
#     --nodelink-config=../openshift/e2e/nodelink.json \
#     --frrk8s-namespace=openshift-frr-k8s --openperouter-namespace=openshift-openperouter-system

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
NODELINK_OUT="${SCRIPT_DIR}/nodelink.json"


CLI="sudo podman"

echo "=== Step 1: Disable DHCP on extra networks ==="
# dev-scripts enables DHCP by default. DHCP IPs compete with our static IPs
# and expire after 60 minutes, breaking VXLAN routing. We assign all IPs
# statically, so DHCP is not needed. Use virsh net-update to remove the DHCP
# range from the live network (--live) and persist across restarts (--config).
for net in toswitch1 toswitch2; do
    DHCP_RANGE=$(virsh net-dumpxml "${net}" 2>/dev/null | grep '<range' | sed 's/^ *//' ) || true
    if [ -n "${DHCP_RANGE}" ]; then
        virsh net-update "${net}" delete ip-dhcp-range "${DHCP_RANGE}" --live --config 2>/dev/null &&             echo "  ${net}: removed DHCP range" || echo "  ${net}: failed to remove DHCP range"
    else
        echo "  ${net}: no DHCP range configured"
    fi
done

echo "=== Step 2: Generate peerLeaf FRR configs ==="
cd "${REPO_ROOT}/clab/tools"
go build -o generate_leaf_config/generate_leaf generate_leaf_config/common.go generate_leaf_config/generate_leaf.go
go build -o generate_leaf_config/generate_leafkind generate_leaf_config/common.go generate_leaf_config/generate_leafkind.go

rm -f ../leafA/frr.conf
./generate_leaf_config/generate_leaf \
    -leaf leafA -neighbor 192.168.1.0 -network 100.64.0.1/32 \
    -template generate_leaf_config/frr_template/frr.conf.template

rm -f ../leafB/frr.conf
./generate_leaf_config/generate_leaf \
    -leaf leafB -neighbor 192.168.1.2 -network 100.64.0.2/32 \
    -template generate_leaf_config/frr_template/frr.conf.template

rm -f ../singlecluster/leafkind1/frr.conf
./generate_leaf_config/generate_leafkind \
    -leaf singlecluster/leafkind1 -asn 64512 -spine-ip 192.168.1.4 \
    -ipv4-listen-range 192.168.11.0/24 -ipv6-listen-range 2001:db8:11::/64 \
    -isis-net 49.0001.0000.0000.0004.00 \
    -toswitch-interface toswitch1 \
    -template generate_leaf_config/frr_template/leafkind.conf.template

rm -f ../singlecluster/leafkind2/frr.conf
./generate_leaf_config/generate_leafkind \
    -leaf singlecluster/leafkind2 -asn 64513 -spine-ip 192.168.1.6 \
    -ipv4-listen-range 192.168.12.0/24 -ipv6-listen-range 2001:db8:12::/64 \
    -isis-net 49.0001.0000.0000.0005.00 \
    -toswitch-interface toswitch2 \
    -template generate_leaf_config/frr_template/leafkind.conf.template

echo "=== Step 3: Enable podman socket ==="
systemctl enable --now podman.socket 2>/dev/null || true

echo "=== Step 4: Deploy clab topology ==="
cd "${REPO_ROOT}"
containerlab deploy --runtime podman \
    --topo "${SCRIPT_DIR}/ocp.clab.yml" --reconfigure

echo "=== Step 5: Assign IPs to clab containers ==="
cd "${REPO_ROOT}/clab"
go run tools/assign_ips/assign_ips.go \
    -file "${SCRIPT_DIR}/ip_map_ocp.txt" -engine "${CLI}"

# Match leafkind bridge-facing MTU to the libvirt bridge (1500)
${CLI} exec clab-kind-leafkind1 ip link set dev toswitch1 mtu 1500
${CLI} exec clab-kind-leafkind2 ip link set dev toswitch2 mtu 1500


echo "=== Step 6: Run container setup scripts ==="
for c in leafA leafB leafSRV6 hostA_red hostA_blue hostA_default hostB_red hostB_blue hostSRV6_red hostSRV6_blue; do
    if ${CLI} exec clab-kind-${c} test -f /setup.sh 2>/dev/null; then
        ${CLI} exec clab-kind-${c} /setup.sh
    fi
done

NODES=$(oc get nodes -l kubernetes.io/os=linux -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')

node_exec() {
    local node=$1; shift
    local pod=$(oc get pods -n openshift-openperouter-system -l app=controller \
        --field-selector spec.nodeName="${node}" -o jsonpath='{.items[0].metadata.name}')
    oc exec -n openshift-openperouter-system "${pod}" -- nsenter -t 1 -m -u -i -n "$@"
}

echo "=== Step 7: Rename NICs + install udev rules ==="
for node in ${NODES}; do
    short_name="${node%%.*}"
    vm_name="ostest_${short_name//-/_}"
    for i in 1 2; do
        BRIDGE_MAC=$(virsh domiflist "${vm_name}" 2>/dev/null | grep "toswitch${i}" | awk '{print $5}')
        if [ -n "${BRIDGE_MAC}" ]; then
            node_exec "${node}" bash -c "
                echo 'SUBSYSTEM==\"net\", ATTR{address}==\"${BRIDGE_MAC}\", NAME=\"toswitch${i}\"' \
                    > /etc/udev/rules.d/70-toswitch${i}.rules
                udevadm control --reload-rules
                IFACE=\$(ip -br link show | grep '${BRIDGE_MAC}' | awk '{print \$1}')
                if [ -n \"\$IFACE\" ] && [ \"\$IFACE\" != \"toswitch${i}\" ]; then
                    nmcli device set \$IFACE managed no 2>/dev/null || true
                    ip link set \$IFACE down
                    ip link set \$IFACE name toswitch${i}
                    ip link set toswitch${i} up
                    echo \"  ${node}: \$IFACE -> toswitch${i}\"
                elif [ \"\$IFACE\" = \"toswitch${i}\" ]; then
                    nmcli device set toswitch${i} managed no 2>/dev/null || true
                    echo \"  ${node}: toswitch${i} already named\"
                else
                    echo \"  ${node}: toswitch${i} not found (MAC ${BRIDGE_MAC})\"
                fi
            " 2>&1
        fi
    done
done

echo "=== Step 8: Assign static IPs (IPv4 + IPv6) ==="
NODE_INDEX=0
NODES_JSON=""
for node in ${NODES}; do
    TS1_V4="192.168.11.$((100 + NODE_INDEX))"
    TS2_V4="192.168.12.$((100 + NODE_INDEX))"
    TS1_V6="2001:db8:11::$((100 + NODE_INDEX))"
    TS2_V6="2001:db8:12::$((100 + NODE_INDEX))"
    NODE_INDEX=$((NODE_INDEX + 1))

    node_exec "${node}" bash -c "
        ip -4 addr flush dev toswitch1 2>/dev/null || true
        ip addr replace ${TS1_V4}/24 dev toswitch1
        ip -6 addr add ${TS1_V6}/64 dev toswitch1 2>/dev/null || true
        ip link set toswitch1 up

        ip -4 addr flush dev toswitch2 2>/dev/null || true
        ip addr replace ${TS2_V4}/24 dev toswitch2
        ip -6 addr add ${TS2_V6}/64 dev toswitch2 2>/dev/null || true
        ip link set toswitch2 up
    " 2>&1
    echo "  ${node}: ts1=${TS1_V4}+${TS1_V6}  ts2=${TS2_V4}+${TS2_V6}"

    if [ -n "${NODES_JSON}" ]; then
        NODES_JSON="${NODES_JSON},"
    fi
    NODES_JSON="${NODES_JSON}
    \"${node}\": {
      \"ipForKindLeaf\": \"${TS1_V4}\",
      \"ipForKindLeaf2\": \"${TS2_V4}\",
      \"ipv6ForKindLeaf\": \"${TS1_V6}\",
      \"ipv6ForKindLeaf2\": \"${TS2_V6}\",
      \"ifaceForKindLeaf\": \"toswitch1\",
      \"ifaceForKindLeaf2\": \"toswitch2\",
      \"leafIfaceForKindLeaf\": \"toswitch1\",
      \"leafIfaceForKindLeaf2\": \"toswitch2\"
    }"
done

cat > "${NODELINK_OUT}" << TOPOEOF
{
  "nodes": {${NODES_JSON}
  }
}
TOPOEOF

echo ""
echo "=== Setup complete ==="
echo "Node links config: ${NODELINK_OUT}"
echo ""
cat "${NODELINK_OUT}"
