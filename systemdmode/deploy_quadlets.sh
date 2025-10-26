#!/bin/bash
set -euo pipefail

# OpenPerouter Quadlet Deployment Script for Kind Nodes
# Deploys quadlet files to kind cluster nodes

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QUADLET_DIR="/etc/containers/systemd"

log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*"
}

log_error() {
    echo "[ERROR] $*"
}

# Function to load an image into podman on a node
# Args: $1=NODE, $2=IMAGE_NAME
load_image_to_node() {
    local NODE="$1"
    local IMAGE="$2"
    local TEMP_TAR="/tmp/$(basename $IMAGE | tr '/:' '_')-update.tar"

    log_info "    Loading image $IMAGE..."
    docker save "$IMAGE" -o "$TEMP_TAR" 2>/dev/null || {
        log_warn "Failed to save image $IMAGE"
        return 1
    }

    if [[ -f "$TEMP_TAR" ]]; then
        docker cp "$TEMP_TAR" "$NODE:/tmp/image-update.tar"
        docker exec "$NODE" podman load -i /tmp/image-update.tar
        docker exec "$NODE" rm /tmp/image-update.tar
        rm "$TEMP_TAR"
        log_info "    Image $IMAGE loaded successfully"
        return 0
    fi
    return 1
}

update_and_restart_routerpod() {
    local NODE="$1"
    local ROUTER_IMAGE="quay.io/openperouter/router:main"
    local FRR_IMAGE="quay.io/frrouting/frr:10.2.1"

    log_info "  Updating routerpod images..."
    load_image_to_node "$NODE" "$ROUTER_IMAGE"
    load_image_to_node "$NODE" "$FRR_IMAGE"

    log_info "  Restarting routerpod services..."
    docker exec "$NODE" systemctl restart routerpod.service || log_warn "Failed to restart routerpod.service on $NODE"
}

update_and_restart_controllerpod() {
    local NODE="$1"
    local ROUTER_IMAGE="quay.io/openperouter/router:main"

    log_info "  Updating controllerpod images..."
    load_image_to_node "$NODE" "$ROUTER_IMAGE"

    log_info "  Restarting controllerpod services..."
    docker exec "$NODE" systemctl restart controllerpod.service || log_warn "Failed to restart controllerpod.service on $NODE"
}

# Check for cluster name parameter
if [[ $# -lt 1 ]]; then
    log_error "Usage: $0 <kind-cluster-name>"
    log_error "Example: $0 my-cluster"
    exit 1
fi

CLUSTER_NAME="$1"

log_info "Deploying OpenPerouter quadlets to kind cluster: $CLUSTER_NAME"

# Get all nodes in the kind cluster
NODES=$(kind get nodes --name "$CLUSTER_NAME" 2>/dev/null)
if [[ -z "$NODES" ]]; then
    log_error "No nodes found for kind cluster: $CLUSTER_NAME"
    log_error "Please check that the cluster exists with: kind get clusters"
    exit 1
fi

log_info "Found nodes in cluster $CLUSTER_NAME:"
echo "$NODES"
echo ""

# Deploy to each node
NODE_INDEX=0
for NODE in $NODES; do
    log_info "Deploying to node: $NODE"

    # Create quadlet directory on the node
    log_info "  Creating quadlet directory..."
    docker exec "$NODE" mkdir -p "$QUADLET_DIR"

    # Copy quadlet files to the node
    log_info "  Copying quadlet files..."
    for quadlet_file in "$SCRIPT_DIR"/quadlets/*.{pod,container,volume}; do
        if [[ -f "$quadlet_file" ]]; then
            QUADLET_NAME=$(basename "$quadlet_file")
            log_info "    Copying $QUADLET_NAME"
            docker cp "$quadlet_file" "$NODE:$QUADLET_DIR/$QUADLET_NAME"
        fi
    done

    # Create required directories on the node
    log_info "  Creating required directories..."
    docker exec "$NODE" mkdir -p /etc/perouter/frr
    docker exec "$NODE" mkdir -p /var/lib/hostambassador
    docker exec "$NODE" mkdir -p /var/lib/openperouter

    # Create configuration file
    log_info "  Creating configuration file with node_index=$NODE_INDEX..."
    docker exec "$NODE" bash -c "cat > /var/lib/openperouter/config.yaml <<EOF
node_index: $NODE_INDEX
EOF"

    # Increment node index for next node
    NODE_INDEX=$((NODE_INDEX + 1))

    # Reload systemd on the node to pick up quadlet files
    log_info "  Reloading systemd daemon..."
    docker exec "$NODE" systemctl daemon-reload

    # Check if pods are already running and update or start accordingly
    if docker exec "$NODE" systemctl is-active --quiet routerpod.service 2>/dev/null; then
        log_info "  Detected running pods - updating images and restarting..."
        update_and_restart_routerpod "$NODE"
        update_and_restart_controllerpod "$NODE"
    else
        log_info "  Initial deployment - starting pod services..."
        docker exec "$NODE" systemctl start routerpod.service || log_warn "Failed to start routerpod.service on $NODE"
        docker exec "$NODE" systemctl start controllerpod.service || log_warn "Failed to start controllerpod.service on $NODE"
    fi

    # Enable services for auto-start
    log_info "  Enabling services for auto-start..."
    docker exec "$NODE" systemctl enable routerpod.service controllerpod.service || log_warn "Failed to enable services on $NODE"

    echo ""
done

# Show status for all nodes
log_info "Deployment complete! Showing service status for all nodes:"
echo ""

for NODE in $NODES; do
    echo "========================================"
    log_info "Node: $NODE"
    echo "========================================"
    docker exec "$NODE" systemctl status routerpod.service --no-pager -l 2>&1 || true
    echo ""
    docker exec "$NODE" systemctl status controllerpod.service --no-pager -l 2>&1 || true
    echo ""
done

echo ""
log_info "Useful commands:"
echo "  View logs on a node:     docker exec <node-name> journalctl -u routerpod.service -f"
echo "  View logs on a node:     docker exec <node-name> journalctl -u controllerpod.service -f"
echo "  Restart services:        docker exec <node-name> systemctl restart routerpod.service controllerpod.service"
echo "  Stop services:           docker exec <node-name> systemctl stop routerpod.service controllerpod.service"
echo "  Get node names:          kind get nodes --name $CLUSTER_NAME"
echo ""
