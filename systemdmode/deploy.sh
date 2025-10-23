#!/bin/bash
set -euo pipefail

# OpenPerouter Systemd Service Deployment Script for Kind Nodes
# Deploys systemd service files to kind cluster nodes

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYSTEMD_UNIT_DIR="/etc/systemd/system"

log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*"
}

log_error() {
    echo "[ERROR] $*"
}

# Check for cluster name parameter
if [[ $# -lt 1 ]]; then
    log_error "Usage: $0 <kind-cluster-name>"
    log_error "Example: $0 my-cluster"
    exit 1
fi

CLUSTER_NAME="$1"

log_info "Deploying OpenPerouter systemd services to kind cluster: $CLUSTER_NAME"

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

    # Copy service files to the node
    log_info "  Copying service files..."
    for service_file in "$SCRIPT_DIR"/pod-*.service "$SCRIPT_DIR"/container-*.service; do
        if [[ -f "$service_file" ]]; then
            SERVICE_NAME=$(basename "$service_file")
            log_info "    Copying $SERVICE_NAME"
            docker cp "$service_file" "$NODE:$SYSTEMD_UNIT_DIR/$SERVICE_NAME"
        fi
    done

    # Create required directories on the node
    log_info "  Creating required directories..."
    docker exec "$NODE" mkdir -p /etc/perouter/frr
    docker exec "$NODE" mkdir -p /var/lib/hostambassador
    docker exec "$NODE" mkdir -p /etc/openperouter

    # Create configuration file
    log_info "  Creating configuration file with node_index=$NODE_INDEX..."
    docker exec "$NODE" bash -c "cat > /etc/openperouter/config.yaml <<EOF
node_index: $NODE_INDEX
EOF"

    # Increment node index for next node
    NODE_INDEX=$((NODE_INDEX + 1))

    # Reload systemd on the node
    log_info "  Reloading systemd daemon..."
    docker exec "$NODE" systemctl daemon-reload

    # Start the pod services
    log_info "  Starting pod services..."
    docker exec "$NODE" systemctl start pod-routerpod.service || log_warn "Failed to start pod-routerpod.service on $NODE"
    docker exec "$NODE" systemctl start pod-controllerpod.service || log_warn "Failed to start pod-controllerpod.service on $NODE"

    # Enable services for auto-start
    log_info "  Enabling services for auto-start..."
    docker exec "$NODE" systemctl enable pod-routerpod.service pod-controllerpod.service || log_warn "Failed to enable services on $NODE"

    echo ""
done

# Show status for all nodes
log_info "Deployment complete! Showing service status for all nodes:"
echo ""

for NODE in $NODES; do
    echo "========================================"
    log_info "Node: $NODE"
    echo "========================================"
    docker exec "$NODE" systemctl status pod-routerpod.service --no-pager -l 2>&1 || true
    echo ""
    docker exec "$NODE" systemctl status pod-controllerpod.service --no-pager -l 2>&1 || true
    echo ""
done

echo ""
log_info "Useful commands:"
echo "  View logs on a node:     docker exec <node-name> journalctl -u pod-routerpod.service -f"
echo "  View logs on a node:     docker exec <node-name> journalctl -u pod-controllerpod.service -f"
echo "  Restart services:        docker exec <node-name> systemctl restart pod-routerpod.service pod-controllerpod.service"
echo "  Stop services:           docker exec <node-name> systemctl stop pod-routerpod.service pod-controllerpod.service"
echo "  Get node names:          kind get nodes --name $CLUSTER_NAME"
echo ""
