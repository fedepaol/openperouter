#!/bin/bash

function veth_exists {
    ip link show "$1" &> /dev/null
    return $?
}

function container_exists {
    docker ps -a --format '{{.Names}}' | grep -w "$1" &> /dev/null
    return $?
}

function ensure_interface {
  VETH_NAME=$1
  PEER_NAME=$2
  CONTAINER_NAME=$3
  CONTAINER_SIDE_IP=$4

  while true; do
      if ! veth_exists "$VETH_NAME"; then
          if container_exists "$CONTAINER_NAME"; then
              ip link add "$VETH_NAME" type veth peer name "$PEER_NAME"

              pid=$(docker inspect -f '{{.State.Pid}}' "$CONTAINER_NAME")
              ip link set "$PEER_NAME" netns "$pid"

              ip link set "$VETH_NAME" up

	      ip link set "$VETH_NAME" master leaf2-switch
              docker exec "$CONTAINER_NAME" ip address add $CONTAINER_SIDE_IP dev "$VETH_NAME"
              docker exec "$CONTAINER_NAME" ip link set "$VETH_NAME" up
          else
              echo "Container $CONTAINER_NAME does not exist. Exiting."
              exit 1
          fi
      else
	      echo "$VETH_NAME exists"
      fi
      sleep 10
  done
}

