#!/bin/bash
set -euo pipefail

pushd "$(dirname $(readlink -f $0))"

CONTAINER_ENGINE=${CONTAINER_ENGINE:-"docker"}
CONTAINER_ENGINE_CLI="docker"
KUBECONFIG_PATH=${KUBECONFIG_PATH:-"$(pwd)/kubeconfig"}
KIND=${KIND:-"kind"}
CLAB_VERSION=0.64.0

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-pe-kind}"

PODMAN_OPTION=""
PODMAN_KIND_ENV="KIND_EXPERIMENTAL_PROVIDER=podman"
KIND_COMMAND=$KIND

if [[ $CONTAINER_ENGINE == "podman" ]]; then
    PODMAN_OPTION="--runtime podman"
    CONTAINER_ENGINE_CLI="sudo podman"
    KIND_COMMAND="sudo KIND_EXPERIMENTAL_PROVIDER=podman $KIND"
    if ! systemctl is-enabled --quiet podman.socket || ! systemctl is-active --quiet podman.socket; then
        echo "Enabling and starting podman.socket service..."
        sudo systemctl enable podman.socket
        sudo systemctl start podman.socket
    fi
fi

clusters=$(${KIND_COMMAND} get clusters)
for cluster in $clusters; do
  if [[ $cluster == "$KIND_CLUSTER_NAME" ]]; then
    echo "Cluster ${KIND_CLUSTER_NAME} already exists"
    exit 0
  fi
done

if [[ ! -d "/sys/class/net/leafkind-switch" ]]; then
	sudo ip link add name leafkind-switch type bridge
fi

if [[ $(cat /sys/class/net/leafkind-switch/operstate) != "up" ]]; then
sudo ip link set dev leafkind-switch up
fi

if [[ $CONTAINER_ENGINE == "docker" ]]; then
    docker run --rm --privileged \
    --network host \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v /var/run/netns:/var/run/netns \
    -v /etc/hosts:/etc/hosts \
    -v /var/lib/docker/containers:/var/lib/docker/containers \
    --pid="host" \
    -v $(pwd):$(pwd) \
    -w $(pwd) \
    ghcr.io/srl-labs/clab:0.67.0 /usr/bin/clab deploy --reconfigure --topo kind.clab.yml
else
    # We werent able to run clab with podman in podman, installing it and running it
    # from the host.
    if ! command -v clab >/dev/null 2>&1; then
	echo "Clab is not installed, please install it first following https://containerlab.dev/install/"
	exit 1
    fi
    sudo clab deploy --reconfigure --topo kind.clab.yml $RUNTIME_OPTION
fi

${KIND_COMMAND} load docker-image quay.io/frrouting/frr:9.1.0 --name pe-kind
${KIND_COMMAND} load docker-image quay.io/frrouting/frr:10.2.1 --name pe-kind
${KIND_COMMAND} load docker-image gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1 --name pe-kind
${KIND_COMMAND} load docker-image quay.io/metallb/frr-k8s:v0.0.17 --name pe-kind
${KIND_COMMAND} --name pe-kind get kubeconfig > $KUBECONFIG_PATH
export KUBECONFIG=$KUBECONFIG_PATH

kind/frr-k8s/setup.sh

sudo $(which go) run tools/assign_ips.go -file ip_map.txt -engine ${CONTAINER_ENGINE}

${CONTAINER_ENGINE_CLI} exec clab-kind-leafA /setup.sh
${CONTAINER_ENGINE_CLI} exec clab-kind-leafB /setup.sh
${CONTAINER_ENGINE_CLI} exec clab-kind-hostA_red /setup.sh
${CONTAINER_ENGINE_CLI} exec clab-kind-hostA_blue /setup.sh
${CONTAINER_ENGINE_CLI} exec clab-kind-hostB_red /setup.sh
${CONTAINER_ENGINE_CLI} exec clab-kind-hostB_blue /setup.sh

if ! pgrep -f check_veths.sh | xargs -r ps -p | grep -q pe-kind-control-plane; then
	sudo ./check_veths.sh kindctrlpl:toswitch:pe-kind-control-plane:192.168.11.3/24  kindworker:toswitch:pe-kind-worker:192.168.11.4/24 &
fi
sleep 4s

popd
