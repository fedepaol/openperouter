#!/bin/bash
set -euo pipefail

# deploy-vpn-raw.sh - Deploy raw FRR VPN setup to kind nodes
#
# This script deploys the raw FRR configuration approach (bypassing controller)
# to all nodes in a kind cluster.
#
# Usage: ./deploy-vpn-raw.sh <cluster-name>

CLUSTER_NAME="${1:-pe-kind}"

echo "Deploying raw FRR VPN setup to cluster: $CLUSTER_NAME"

# Get all nodes
NODES=$(kind get nodes --name "$CLUSTER_NAME" 2>/dev/null || {
    echo "ERROR: Cluster $CLUSTER_NAME not found"
    echo "Available clusters:"
    kind get clusters
    exit 1
})

if [[ -z "$NODES" ]]; then
    echo "ERROR: No nodes found in cluster $CLUSTER_NAME"
    exit 1
fi

echo "Found nodes:"
echo "$NODES"
echo ""

# Deploy to each node
for NODE in $NODES; do
    echo "=========================================="
    echo "Deploying to node: $NODE"
    echo "=========================================="

    # Create directories
    echo "  Creating directories..."
    docker exec "$NODE" mkdir -p /etc/openperouter/templates /usr/local/bin

    # Copy FRR config template
    echo "  Copying FRR configuration template..."
    docker cp systemdmode/frr-evpn.conf.template "$NODE:/etc/openperouter/templates/" || {
        echo "  ERROR: Failed to copy FRR template"
        continue
    }

    # Copy network setup script
    echo "  Copying network setup script..."
    docker cp systemdmode/setup-network.sh "$NODE:/usr/local/bin/" || {
        echo "  ERROR: Failed to copy network setup script"
        continue
    }
    docker exec "$NODE" chmod +x /usr/local/bin/setup-network.sh

    # Copy main VPN setup script
    echo "  Copying VPN setup script..."
    docker cp systemdmode/setup-vpn-raw.sh "$NODE:/usr/local/bin/" || {
        echo "  ERROR: Failed to copy VPN setup script"
        continue
    }
    docker exec "$NODE" chmod +x /usr/local/bin/setup-vpn-raw.sh

    # Copy common utilities
    echo "  Copying common utilities..."
    docker cp systemdmode/common.sh "$NODE:/usr/local/bin/" || {
        echo "  ERROR: Failed to copy common.sh"
        continue
    }
    docker exec "$NODE" chmod +x /usr/local/bin/common.sh

    # Copy systemd service unit
    echo "  Copying systemd service unit..."
    if [[ -f systemdmode/quadlets/vpn-setup-raw.service ]]; then
        # Use dedicated service file if it exists
        docker cp systemdmode/quadlets/vpn-setup-raw.service "$NODE:/etc/systemd/system/vpn-setup.service"
    else
        # Copy and modify existing service file
        docker cp systemdmode/quadlets/vpn-setup.service "$NODE:/etc/systemd/system/" || {
            echo "  WARNING: Service file not found, creating basic one..."
            docker exec "$NODE" bash -c 'cat > /etc/systemd/system/vpn-setup.service <<EOF
[Unit]
Description=OpenPERouter VPN Setup (Raw FRR Mode)
After=network-online.target routerpod-pod.service frr.service
Requires=routerpod-pod.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
User=root
Group=root
EnvironmentFile=-/etc/openperouter/vpn-setup.env
ExecStart=/usr/local/bin/setup-vpn-raw.sh
StandardOutput=journal
StandardError=journal
SyslogIdentifier=vpn-setup
TimeoutStartSec=180

[Install]
WantedBy=multi-user.target
EOF'
        }
        # Update ExecStart to use raw script
        docker exec "$NODE" sed -i 's|ExecStart=.*setup-vpn\.sh|ExecStart=/usr/local/bin/setup-vpn-raw.sh|' \
            /etc/systemd/system/vpn-setup.service 2>/dev/null || true
    fi

    # Reload systemd
    echo "  Reloading systemd..."
    docker exec "$NODE" systemctl daemon-reload

    # Enable service (but don't start yet)
    echo "  Enabling vpn-setup service..."
    docker exec "$NODE" systemctl enable vpn-setup.service

    echo "  ✓ Deployment to $NODE completed"
    echo ""
done

echo "=========================================="
echo "Deployment Summary"
echo "=========================================="
echo "Deployed to cluster: $CLUSTER_NAME"
echo "Nodes configured: $(echo "$NODES" | wc -w)"
echo ""
echo "Files deployed:"
echo "  - /etc/openperouter/templates/frr-evpn.conf.template"
echo "  - /usr/local/bin/setup-network.sh"
echo "  - /usr/local/bin/setup-vpn-raw.sh"
echo "  - /usr/local/bin/common.sh"
echo "  - /etc/systemd/system/vpn-setup.service"
echo ""
echo "Next steps:"
echo "  1. Configure environment (optional):"
echo "     docker exec <node> vi /etc/openperouter/vpn-setup.env"
echo ""
echo "  2. Start VPN setup on a node:"
echo "     docker exec <node> systemctl start vpn-setup.service"
echo ""
echo "  3. Check status:"
echo "     docker exec <node> systemctl status vpn-setup.service"
echo ""
echo "  4. View logs:"
echo "     docker exec <node> journalctl -u vpn-setup.service --no-pager"
echo ""
echo "  5. Verify BGP:"
echo "     docker exec <node> podman exec frr vtysh -c 'show bgp summary'"
echo ""
echo "  6. Verify EVPN:"
echo "     docker exec <node> podman exec frr vtysh -c 'show evpn vni'"
echo ""
echo "=========================================="
