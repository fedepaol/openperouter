#!/bin/bash
set -euo pipefail

# deploy-vpn-setup.sh - Deploy VPN setup script to all kind nodes
#
# Usage: ./deploy-vpn-setup.sh <kind-cluster-name>
# Example: ./deploy-vpn-setup.sh pe-kind

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log_info() {
    echo "[INFO] $*"
}

log_error() {
    echo "[ERROR] $*"
}

if [[ $# -lt 1 ]]; then
    log_error "Usage: $0 <kind-cluster-name>"
    log_error "Example: $0 pe-kind"
    exit 1
fi

CLUSTER_NAME="$1"

# Get nodes from kind cluster
NODES=$(kind get nodes --name "$CLUSTER_NAME" 2>/dev/null)
if [[ -z "$NODES" ]]; then
    log_error "No nodes found for kind cluster: $CLUSTER_NAME"
    log_error "Please check that the cluster exists with: kind get clusters"
    exit 1
fi

log_info "Deploying VPN setup to cluster: $CLUSTER_NAME"

for NODE in $NODES; do
    log_info "Deploying to node: $NODE"

    # Copy setup script
    log_info "  Copying setup-vpn.sh..."
    docker cp "$SCRIPT_DIR/setup-vpn.sh" "$NODE:/usr/local/bin/"
    docker exec "$NODE" chmod +x /usr/local/bin/setup-vpn.sh

    # Create template directory and copy template
    log_info "  Copying template..."
    docker exec "$NODE" mkdir -p /etc/openperouter/templates
    docker cp "$SCRIPT_DIR/openpe_evpn.yaml.template" "$NODE:/etc/openperouter/templates/"

    # Create config output directory
    log_info "  Creating config directory..."
    docker exec "$NODE" mkdir -p /var/lib/openperouter/configs

    # Copy systemd service unit (via quadlets directory)
    log_info "  Copying systemd service unit..."
    docker exec "$NODE" mkdir -p /etc/containers/systemd
    docker cp "$SCRIPT_DIR/quadlets/vpn-setup.service" "$NODE:/etc/containers/systemd/"

    # Reload systemd daemon
    log_info "  Reloading systemd..."
    docker exec "$NODE" systemctl daemon-reload

    log_info "  Deployment to $NODE complete"
    echo ""
done

log_info "VPN setup deployed to all nodes in cluster: $CLUSTER_NAME"
log_info ""
log_info "To start the service on a node:"
log_info "  docker exec <node-name> systemctl start vpn-setup.service"
log_info ""
log_info "To check status:"
log_info "  docker exec <node-name> systemctl status vpn-setup.service"
