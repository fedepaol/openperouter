#!/bin/bash
set -euo pipefail

pushd "$(dirname $(readlink -f $0))"

KUBECONFIG_PATH=${KUBECONFIG_PATH:-"$(pwd)/kubeconfig"}
KIND_BIN=${KIND_BIN:-"kind"}
CLAB_VERSION=0.64.0

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-pe-kind}"


clusters=$("${KIND_BIN}" get clusters)
for cluster in $clusters; do
  if [[ $cluster == "$KIND_CLUSTER_NAME" ]]; then
    echo "Cluster ${KIND_CLUSTER_NAME} already exists"
    exit 0
  fi
done

if [[ ! -d "/sys/class/net/leafkind-switch" ]]; then
	sudo ip link add name leafkind-switch type bridge
fi

if [[ $(cat /sys/class/net/leaf2-switch/operstate) != "up" ]]; then
sudo ip link set dev leaf2-switch up
fi

docker run --rm -it --privileged \
    --network host \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v /var/run/netns:/var/run/netns \
    -v /etc/hosts:/etc/hosts \
    -v /var/lib/docker/containers:/var/lib/docker/containers \
    --pid="host" \
    -v $(pwd):$(pwd) \
    -w $(pwd) \
    ghcr.io/srl-labs/clab:$CLAB_VERSION /usr/bin/clab deploy --reconfigure --topo kind.clab.yml

docker image pull quay.io/metallb/frr-k8s:v0.0.17
docker image pull quay.io/frrouting/frr:9.1.0
docker image pull gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1
kind load docker-image quay.io/frrouting/frr:9.1.0 --name pe-kind
kind load docker-image gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1 --name pe-kind
kind load docker-image quay.io/metallb/frr-k8s:v0.0.17 --name pe-kind

kind --name pe-kind get kubeconfig > $KUBECONFIG_PATH
export KUBECONFIG=$KUBECONFIG_PATH
kind/frr-k8s/setup.sh

go run tools/assign_ips.go ip_map.txt

docker exec clab-kind-leafA /setup.sh
docker exec clab-kind-leafB /setup.sh

if ! pgrep -f check_veths.sh | xargs -r ps -p | grep -q pe-kind-control-plane; then
	sudo ./check_veths.sh kindctrlpl:toswitch:pe-kind-control-plane:192.168.11.3/24  kindworker:toswitch:pe-kind-worker:192.168.11.4/24 & 
fi
sleep 4s

popd
