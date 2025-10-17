#/bin/bash
set -x

build_kind_node() {
    IMAGE_NAME="kind-with-podman:latest"
    DOCKERFILE="kind-node-podman.Dockerfile"

    if [ -z "$(docker images -q $IMAGE_NAME)" ]; then
        docker build -f "$DOCKERFILE" -t "$IMAGE_NAME" .
    fi
}

create_registry() {
    REGISTRY_NAME="kind-registry"
    REGISTRY_PORT="5001"

    if [ ! "$(docker ps -q -f name=$REGISTRY_NAME)" ]; then
        if [ "$(docker ps -aq -f status=exited -f name=$REGISTRY_NAME)" ]; then
            docker rm $REGISTRY_NAME
        fi
        docker run -d --restart=always -p "${REGISTRY_PORT}:5000" --name $REGISTRY_NAME registry:2
    fi
}

create_kind_cluster() {
    CLUSTER_NAME="openperouter"

    cat <<EOF | kind create cluster --name $CLUSTER_NAME --image $IMAGE_NAME --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:5001"]
    endpoint = ["http://kind-registry:5000"]
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF

    docker network connect "kind" "kind-registry" || true
}

copy_start_podman() {
    echo "Copying start_podman.sh to all nodes..."

    # Get all node names
    nodes=$(kind get nodes --name $CLUSTER_NAME)

    for node in $nodes; do
        echo "Copying start_podman.sh to node: $node"
        docker cp start_podman.sh $node:/usr/local/bin/start_podman.sh
        docker exec $node chmod +x /usr/local/bin/start_podman.sh
    done

    echo "start_podman.sh copied to all nodes"
}

copy_router_image() {
    echo "Copying router image to all nodes..."

    IMAGE="quay.io/openperouter/router:main"

    # Load to kind's containerd
    echo "Loading router image to kind cluster..."
    kind load docker-image "$IMAGE" --name $CLUSTER_NAME

    # Also load to podman on each node
    nodes=$(kind get nodes --name $CLUSTER_NAME)

    for node in $nodes; do
        echo "Copying router image to node: $node"
        # Try docker cp with a different path
        TEMP_TAR="/tmp/router-image-$node.tar"
        docker save "$IMAGE" -o "$TEMP_TAR"
        docker cp "$TEMP_TAR" "$node:/root/router-image.tar"

        echo "Loading router image into podman on node: $node"
        docker exec "$node" podman load -i "/root/router-image.tar"
        docker exec "$node" rm "/root/router-image.tar"
        rm "$TEMP_TAR"
    done

    echo "Router image copied and loaded on all nodes"
}

copy_frr_image() {
    echo "Copying FRR image to all nodes..."

    IMAGE="quay.io/frrouting/frr:10.2.1"

    # Load to kind's containerd
    echo "Loading FRR image to kind cluster..."
    kind load docker-image "$IMAGE" --name $CLUSTER_NAME

    # Also load to podman on each node
    nodes=$(kind get nodes --name $CLUSTER_NAME)

    for node in $nodes; do
        echo "Copying FRR image to node: $node"
        TEMP_TAR="/tmp/frr-image-$node.tar"
        docker save "$IMAGE" -o "$TEMP_TAR"
        docker cp "$TEMP_TAR" "$node:/root/frr-image.tar"

        echo "Loading FRR image into podman on node: $node"
        docker exec "$node" podman load -i "/root/frr-image.tar"
        docker exec "$node" rm "/root/frr-image.tar"
        rm "$TEMP_TAR"
    done

    echo "FRR image copied and loaded on all nodes"
}


docker rm -f openperouter-control-plane

build_kind_node
create_kind_cluster
copy_start_podman
copy_router_image
copy_frr_image

echo "Kind cluster '$CLUSTER_NAME' created with router and FRR images loaded in podman"

docker exec openperouter-control-plane /usr/local/bin/start_podman.sh

docker exec -it openperouter-control-plane bash
