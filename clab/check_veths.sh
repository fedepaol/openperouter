#!/bin/bash

set -e
set -u
set -o pipefail

function veth_exists {
    ip link show "$1" &> /dev/null
    return $?
}

function container_exists {
    docker ps -a --format '{{.Names}}' | grep -w "$1" &> /dev/null
    return $?
}

VETH_NAME=$1
PEER_NAME=$2
CONTAINER_NAME=$3
CONTAINER_SIDE_IP=$4

SCRIPT_NAME=$(basename "$0")
if pgrep -f "$SCRIPT_NAME" | xargs -r ps -p | grep -q "$CONTAINER_NAME"; then
	echo "Checker already running for $CONTAINER_NAME, exiting"
fi

echo "keeping $VETH_NAME - $PEER_NAME up in $CONTAINER_NAME"
while true; do
  if ! container_exists "$CONTAINER_NAME"; then
    echo "Container $CONTAINER_NAME does not exist. Exiting."
    exit 1
  fi

  if ! veth_exists "$VETH_NAME"; then
    echo "Veth $VETH_NAME not there, recreating"
    ip link add "$VETH_NAME" type veth peer name "$PEER_NAME"
    echo "Veth $VETH_NAME not there, recreated"
    pid=$(docker inspect -f '{{.State.Pid}}' "$CONTAINER_NAME")
    ip link set "$PEER_NAME" netns "$pid"
    ip link set "$VETH_NAME" up

    ip link set "$VETH_NAME" master leaf2-switch
    echo "Veth $VETH_NAME setting ip"
    docker exec "$CONTAINER_NAME" ip address add $CONTAINER_SIDE_IP dev "$PEER_NAME"
    docker exec "$CONTAINER_NAME" ip link set "$PEER_NAME" up
  fi
  sleep 10
done
