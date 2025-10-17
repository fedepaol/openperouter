#!/bin/bash
set -euo pipefail


CONTAINER_ENGINE=${CONTAINER_ENGINE:-"docker"}
CONTAINER_ENGINE_CLI="docker"
KUBECONFIG_PATH=${KUBECONFIG_PATH:-"$(pwd)/kubeconfig"}
KIND=${KIND:-"kind"}
CLAB_VERSION=0.64.0

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-pe-kind}"

RUNTIME_OPTION=""
KIND_COMMAND=$KIND

if [[ $CONTAINER_ENGINE == "podman" ]]; then
    RUNTIME_OPTION="--runtime podman"
    CONTAINER_ENGINE_CLI="sudo podman"
    KIND_COMMAND="sudo KIND_EXPERIMENTAL_PROVIDER=podman $KIND"
    if ! systemctl is-enabled --quiet podman.socket || ! systemctl is-active --quiet podman.socket; then
        echo "Enabling and starting podman.socket service..."
        sudo systemctl enable podman.socket
        sudo systemctl start podman.socket
    fi
fi


