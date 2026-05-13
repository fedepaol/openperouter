#!/bin/bash

set -u

source common.sh

# Override set -e from common.sh: the monitoring loop must survive
# individual command failures and retry on the next iteration.
set +e

function veth_exists {
    ip link show "$1" &> /dev/null
    return $?
}

function container_exists {
    ${CONTAINER_ENGINE_CLI} ps -a --format '{{.Names}}' | grep -w "$1" &> /dev/null
    return $?
}


function peer_exists_in_container {
    "$CONTAINER_ENGINE_CLI" exec "$1" ip link show "$2" &>/dev/null && return 0
    "$CONTAINER_ENGINE_CLI" exec "$1" ip netns exec perouter ip link show "$2" &>/dev/null && return 0
    "$CONTAINER_ENGINE_CLI" exec "$1" bash -c \
        'test -f /etc/perouter/frr/frr.pid && nsenter -t $(cat /etc/perouter/frr/frr.pid) -n ip link show '"$2" &>/dev/null && return 0
    return 1
}

function recreate_veth {
    VETH_NAME=$1
    PEER_NAME=$2
    CONTAINER_NAME=$3
    CONTAINER_SIDE_IP=$4
    TEMP_PEER_NAME="${PEER_NAME}_temp"

    # Clean up stale interfaces
    ip link delete "$VETH_NAME" 2>/dev/null || true
    "$CONTAINER_ENGINE_CLI" exec "$CONTAINER_NAME" ip link delete "$PEER_NAME" 2>/dev/null || true
    "$CONTAINER_ENGINE_CLI" exec "$CONTAINER_NAME" ip link delete "$TEMP_PEER_NAME" 2>/dev/null || true
    ip link delete "$TEMP_PEER_NAME" 2>/dev/null || true

    ip link add "$VETH_NAME" type veth peer name "$TEMP_PEER_NAME"
    echo "Veth $VETH_NAME recreated"
    pid=$("$CONTAINER_ENGINE_CLI" inspect -f '{{.State.Pid}}' "$CONTAINER_NAME")
    ip link set "$TEMP_PEER_NAME" netns "$pid"
    ip link set "$VETH_NAME" up

    ip link set "$VETH_NAME" master "$BRIDGE_NAME"

    MAC_ADDR="02:ed:$[RANDOM%10]$[RANDOM%10]:$[RANDOM%10]$[RANDOM%10]:$[RANDOM%10]$[RANDOM%10]:$[RANDOM%10]$[RANDOM%10]"
    ip link set "$VETH_NAME" address "$MAC_ADDR"
    echo "Veth $VETH_NAME setting ip"
    "$CONTAINER_ENGINE_CLI" exec "$CONTAINER_NAME" ip address add $CONTAINER_SIDE_IP dev "$TEMP_PEER_NAME"
    "$CONTAINER_ENGINE_CLI" exec "$CONTAINER_NAME" ip link set "$TEMP_PEER_NAME" name "$PEER_NAME"
    MAC_ADDR="02:ed:$[RANDOM%10]$[RANDOM%10]:$[RANDOM%10]$[RANDOM%10]:$[RANDOM%10]$[RANDOM%10]:$[RANDOM%10]$[RANDOM%10]"
    "$CONTAINER_ENGINE_CLI" exec "$CONTAINER_NAME" ip link set "$PEER_NAME" address "$MAC_ADDR"
    "$CONTAINER_ENGINE_CLI" exec "$CONTAINER_NAME" ip link set "$PEER_NAME" up
}

function ensure_veth {
  VETH_NAME=$1
  PEER_NAME=$2
  CONTAINER_NAME=$3
  CONTAINER_SIDE_IP=$4
  BRIDGE_NAME=${5:-leafkind1-sw}

  if ! veth_exists "$VETH_NAME"; then
    echo "Veth $VETH_NAME not there, recreating"
    recreate_veth "$VETH_NAME" "$PEER_NAME" "$CONTAINER_NAME" "$CONTAINER_SIDE_IP"
  elif ! peer_exists_in_container "$CONTAINER_NAME" "$PEER_NAME"; then
    echo "Veth $VETH_NAME exists but peer $PEER_NAME missing in container, recreating"
    recreate_veth "$VETH_NAME" "$PEER_NAME" "$CONTAINER_NAME" "$CONTAINER_SIDE_IP"
  fi
}

nodes=("$@")

node_parts=()
while true; do

for node in "${nodes[@]}"; do

    IFS=':' read -ra node_parts <<< "$node"
    veth_name="${node_parts[0]}"
    peer_name="${node_parts[1]}"
    container_name="${node_parts[2]}"
    container_side_ip="${node_parts[3]}"
    bridge_name="${node_parts[4]:-}"

    if ! container_exists "$container_name"; then
      echo "Container $container_name does not exist. Exiting."
      exit 1
    fi

    ensure_veth $veth_name $peer_name $container_name $container_side_ip $bridge_name
done
sleep 5s
done
