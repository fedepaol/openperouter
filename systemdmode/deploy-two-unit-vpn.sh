#!/bin/bash
set -euo pipefail

# deploy-two-unit-vpn.sh - Deploy two-unit VPN setup to kind nodes
#
# This script deploys the two-unit VPN setup approach:
# 1. setup-underlay.service - Underlay infrastructure setup
# 2. generate-config.service - Configuration generation
#
# Usage: ./deploy-two-unit-vpn.sh [cluster-name]

CLUSTER_NAME="${1:-pe-kind}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Deploying two-unit VPN setup to cluster: $CLUSTER_NAME"

# Get all nodes in cluster
NODES=$(kind get nodes --name "$CLUSTER_NAME" 2>/dev/null || true)

if [[ -z "$NODES" ]]; then
    echo "ERROR: No nodes found for cluster $CLUSTER_NAME"
    echo "Create cluster first: kind create cluster --name $CLUSTER_NAME"
    exit 1
fi

echo "Found nodes:"
echo "$NODES"
echo ""

for NODE in $NODES; do
    echo "=== Deploying to $NODE ==="

    # Create directories
    echo "  Creating directories..."
    docker exec "$NODE" mkdir -p /usr/local/bin \
                                 /etc/openperouter/templates \
                                 /etc/systemd/system \
                                 /var/lib/openperouter/configs

    # Copy scripts
    echo "  Copying scripts..."
    docker cp "$SCRIPT_DIR/setup-underlay.sh" "$NODE:/usr/local/bin/"
    docker cp "$SCRIPT_DIR/generate-config.sh" "$NODE:/usr/local/bin/"
    docker cp "$SCRIPT_DIR/common.sh" "$NODE:/usr/local/bin/"

    # Make scripts executable
    echo "  Making scripts executable..."
    docker exec "$NODE" chmod +x /usr/local/bin/setup-underlay.sh \
                                  /usr/local/bin/generate-config.sh \
                                  /usr/local/bin/common.sh

    # Copy template
    echo "  Copying template..."
    docker cp "$SCRIPT_DIR/openpe_evpn.yaml.template" "$NODE:/etc/openperouter/templates/"

    # Copy systemd units
    echo "  Copying systemd units..."
    docker cp "$SCRIPT_DIR/quadlets/setup-underlay.service" "$NODE:/etc/systemd/system/"
    docker cp "$SCRIPT_DIR/quadlets/generate-config.service" "$NODE:/etc/systemd/system/"

    # Create environment file (optional)
    echo "  Creating environment file..."
    docker exec "$NODE" bash -c 'cat > /etc/openperouter/vpn-setup.env <<EOF
# OpenPERouter VPN Setup Environment Variables
# Edit this file to customize configuration

# Underlay Setup
UNDERLAY_NIC=eth1
FRR_READY_TIMEOUT=60
NODE_NAME=$(hostname)

# VPN Configuration
TOR_IP=10.1.1.254
TOR_AS=65000
LOCAL_AS=64514
VRF_NAME=red
L3_VNI=100
L2_VNI=210
VXLAN_PORT=4789
L2_GATEWAY_IP=192.168.110.1/24
EOF
'

    # Reload systemd
    echo "  Reloading systemd..."
    docker exec "$NODE" systemctl daemon-reload

    # Enable services
    echo "  Enabling services..."
    docker exec "$NODE" systemctl enable setup-underlay.service
    docker exec "$NODE" systemctl enable generate-config.service

    echo "  ✓ Deployed to $NODE"
    echo ""
done

echo "=== Deployment Complete ==="
echo ""
echo "To start the services on all nodes:"
echo "  for NODE in \$(kind get nodes --name $CLUSTER_NAME); do"
echo "    docker exec \$NODE systemctl start setup-underlay.service"
echo "    docker exec \$NODE systemctl start generate-config.service"
echo "  done"
echo ""
echo "To check status:"
echo "  docker exec <node-name> systemctl status setup-underlay.service generate-config.service"
echo ""
echo "To view logs:"
echo "  docker exec <node-name> journalctl -u setup-underlay.service -u generate-config.service --no-pager"
echo ""
echo "To verify configuration:"
echo "  docker exec <node-name> cat /var/lib/openperouter/vpn-setup.vars"
echo "  docker exec <node-name> cat /var/lib/openperouter/configs/openpe_evpn.yaml"
