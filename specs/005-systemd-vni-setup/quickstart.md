# Quick Start: Systemd-Based VNI Setup

**Feature**: 005-systemd-vni-setup  
**Date**: 2026-04-01  
**Estimated Setup Time**: 10 minutes

## Overview

This guide will help you set up automatic L2 and L3 VPN configuration for OpenPERouter in systemd mode. The setup runs **inside containerlab kind nodes**, not on your host machine.

## Prerequisites

Before you begin, ensure you have:

- [ ] Docker installed
- [ ] Kind installed (Kubernetes in Docker)
- [ ] Make installed
- [ ] Git clone of openperouter repository

### Verify Prerequisites

```bash
# Check docker
docker --version

# Check kind
kind --version

# Check make
make --version

# Navigate to repository
cd /path/to/openperouter
```

## Quick Setup (Using Make Targets)

### Step 1: Deploy Containerlab Cluster

```bash
# Navigate to repository
cd openperouter

# Deploy cluster with hostmode (this creates the kind nodes)
make deploy-hostmode

# This creates:
# - pe-kind-control-plane (kind control plane node)
# - pe-kind-worker (kind worker node)
# - Containerlab topology with leaf switches
```

### Step 2: Deploy VPN Setup to Nodes

```bash
# Deploy VPN setup script, template, and service to all nodes
make deploy-vpn-setup KIND_CLUSTER_NAME=pe-kind

# This copies to each kind node:
# - systemdmode/setup-vpn.sh → /usr/local/bin/setup-vpn.sh
# - systemdmode/openpe_evpn.yaml.template → /etc/openperouter/templates/
# - systemdmode/quadlets/vpn-setup.service → /etc/containers/systemd/
```

**Note**: If `deploy-vpn-setup` target doesn't exist yet, use the deployment script directly:

```bash
./systemdmode/deploy-vpn-setup.sh pe-kind
```

### Step 3: Verify Setup on Worker Node

```bash
# Check VPN setup service status
docker exec pe-kind-worker systemctl status vpn-setup.service

# View logs
docker exec pe-kind-worker journalctl -u vpn-setup.service --no-pager

# Check generated static configuration
docker exec pe-kind-worker cat /var/lib/openperouter/configs/openpe_evpn.yaml
```

### Step 4: Validate BGP and EVPN

```bash
# Check BGP session (should show "Established")
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"

# Check EVPN type 2 routes (L2VPN MAC-IP routes)
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp l2vpn evpn route type 2"

# Check EVPN type 5 routes (L3VPN IP Prefix routes)
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp l2vpn evpn route type 5"

# Check VNI status (should show VNI 100 and 210)
docker exec pe-kind-worker podman exec frr vtysh -c "show evpn vni"

# Check L2VPN veth attached to br0
docker exec pe-kind-worker bridge link show | grep veth-br210-host
```

✅ **Success Indicators**:
- Service status shows "active (exited)" with RemainAfterExit
- Static config file exists at `/var/lib/openperouter/configs/openpe_evpn.yaml`
- VTEP IP is correctly set to 10.0.0.X (where X = last octet of br0's IP)
- BGP session shows "Established"
- EVPN type 2 routes present
- EVPN type 5 routes present
- VNIs 100 and 210 are listed
- veth-br210-host is enslaved to br0

## Manual Deployment (Alternative Method)

If you need to deploy manually without Make targets:

### Step 1: Get Node Names

```bash
# List kind nodes in the cluster
kind get nodes --name pe-kind

# Output:
# pe-kind-control-plane
# pe-kind-worker
```

### Step 2: Copy Files to Worker Node

```bash
# Copy setup script
docker cp systemdmode/setup-vpn.sh pe-kind-worker:/usr/local/bin/
docker exec pe-kind-worker chmod +x /usr/local/bin/setup-vpn.sh

# Create template directory and copy template
docker exec pe-kind-worker mkdir -p /etc/openperouter/templates
docker cp systemdmode/openpe_evpn.yaml.template pe-kind-worker:/etc/openperouter/templates/

# Create config output directory
docker exec pe-kind-worker mkdir -p /var/lib/openperouter/configs

# Copy systemd service unit
docker cp systemdmode/quadlets/vpn-setup.service pe-kind-worker:/etc/containers/systemd/
```

### Step 3: Start Service on Node

```bash
# Reload systemd daemon
docker exec pe-kind-worker systemctl daemon-reload

# Start VPN setup service
docker exec pe-kind-worker systemctl start vpn-setup.service

# Check status
docker exec pe-kind-worker systemctl status vpn-setup.service
```

### Step 4: Verify Configuration

```bash
# Check generated config
docker exec pe-kind-worker cat /var/lib/openperouter/configs/openpe_evpn.yaml

# Verify VTEP IP derivation
docker exec pe-kind-worker ip addr show br0
# Note the last octet of br0's IP

# Check VTEP IP in generated config matches (10.0.0.<last-octet>)
docker exec pe-kind-worker grep "vtepcidr" /var/lib/openperouter/configs/openpe_evpn.yaml

# Validate BGP and EVPN (same as Step 4 above)
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"
docker exec pe-kind-worker podman exec frr vtysh -c "show evpn vni"
```

## Configuration Customization

### Environment Variables (Inside Nodes)

To customize configuration on specific nodes, create an environment file inside the node:

```bash
# Create environment file on worker node
docker exec pe-kind-worker bash -c 'cat > /etc/openperouter/vpn-setup.env <<EOF
# VPN Setup Configuration
UNDERLAY_NIC=eth1
TOR_IP=10.1.1.254
TOR_AS=65000
LOCAL_AS=64514
FRR_READY_TIMEOUT=60

# Optional overrides
#L2_VNI=210
#L3_VNI=100
#VRF_NAME=red
#VXLAN_PORT=4789
#L2_GATEWAY_IP=192.168.110.1/24
EOF'

# Restart service to pick up new config
docker exec pe-kind-worker systemctl restart vpn-setup.service
```

### Multi-Node Deployment

Deploy to all nodes in the cluster:

```bash
# Get all nodes
NODES=$(kind get nodes --name pe-kind)

# Deploy to each node
for NODE in $NODES; do
    echo "Deploying to $NODE..."
    docker cp systemdmode/setup-vpn.sh $NODE:/usr/local/bin/
    docker exec $NODE chmod +x /usr/local/bin/setup-vpn.sh
    
    docker exec $NODE mkdir -p /etc/openperouter/templates
    docker cp systemdmode/openpe_evpn.yaml.template $NODE:/etc/openperouter/templates/
    
    docker exec $NODE mkdir -p /var/lib/openperouter/configs
    docker cp systemdmode/quadlets/vpn-setup.service $NODE:/etc/containers/systemd/
    
    docker exec $NODE systemctl daemon-reload
    docker exec $NODE systemctl start vpn-setup.service
done

# Verify all nodes
for NODE in $NODES; do
    echo "=== $NODE ==="
    docker exec $NODE systemctl status vpn-setup.service | grep Active
done
```

## Troubleshooting

All troubleshooting commands run **inside the kind node** using `docker exec`:

### Service Fails to Start

**Check FRR container status**:
```bash
docker exec pe-kind-worker podman ps | grep frr
docker exec pe-kind-worker systemctl status routerpod-pod.service
```

**Solution**: Ensure FRR container is running:
```bash
docker exec pe-kind-worker systemctl start routerpod-pod.service
```

### Timeout Waiting for FRR

**Check logs**:
```bash
docker exec pe-kind-worker journalctl -u vpn-setup.service | grep "Waiting for FRR"
```

**Solution**: Check FRR startup:
```bash
docker exec pe-kind-worker podman logs frr
```

### br0 Does Not Exist

**Error**: `ERROR: br0 bridge does not exist`

**Check bridge**:
```bash
docker exec pe-kind-worker ip link show br0
```

**Solution**: Create br0 before running service (should be done by cluster setup):
```bash
docker exec pe-kind-worker ip link add br0 type bridge
docker exec pe-kind-worker ip addr add 192.168.1.5/24 dev br0
docker exec pe-kind-worker ip link set br0 up
```

### Host NIC Not Found

**Error**: `ERROR: Host NIC eth1 not found`

**Check NIC names**:
```bash
docker exec pe-kind-worker ip link show
```

**Solution**: Update environment variable or script default with correct NIC name

### BGP Session Not Establishing

**Check BGP status**:
```bash
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"
```

**Check TOR reachability**:
```bash
docker exec pe-kind-worker podman exec frr ping -c 3 <TOR_IP>
```

**Solution**: Verify TOR IP is reachable and AS numbers are correct in generated config

### View All Logs

```bash
# View service logs
docker exec pe-kind-worker journalctl -u vpn-setup.service --no-pager

# View errors only
docker exec pe-kind-worker journalctl -u vpn-setup.service -p err

# Follow logs in real-time
docker exec pe-kind-worker journalctl -u vpn-setup.service -f
```

## Validation Script

Create a validation script to quickly check VPN setup:

```bash
# Create validation script on host
cat > systemdmode/validate-vpn-setup.sh <<'EOF'
#!/bin/bash
NODE=${1:-pe-kind-worker}

echo "=== VPN Setup Validation for $NODE ==="

echo -e "\n1. Service Status:"
docker exec $NODE systemctl is-active vpn-setup.service

echo -e "\n2. Static Config Generated:"
docker exec $NODE test -f /var/lib/openperouter/configs/openpe_evpn.yaml && echo "✓ Config file exists" || echo "✗ Config file missing"

echo -e "\n3. BGP Session:"
docker exec $NODE podman exec frr vtysh -c "show bgp summary" | grep -A1 "Neighbor" || echo "BGP not ready"

echo -e "\n4. VNI Status:"
docker exec $NODE podman exec frr vtysh -c "show evpn vni" | grep -E "VNI.*100|VNI.*210" || echo "VNIs not configured"

echo -e "\n5. L2VPN veth to br0:"
docker exec $NODE bridge link show | grep veth-br210-host && echo "✓ veth attached" || echo "✗ veth not attached"

echo -e "\n6. VTEP IP:"
docker exec $NODE grep "vtepcidr" /var/lib/openperouter/configs/openpe_evpn.yaml || echo "VTEP not configured"
EOF

chmod +x systemdmode/validate-vpn-setup.sh

# Run validation
./systemdmode/validate-vpn-setup.sh pe-kind-worker
```

## Next Steps

After successful setup:

1. **Verify Persistence**:
   ```bash
   # Restart the node
   docker restart pe-kind-worker
   
   # Wait for node to come back
   sleep 30
   
   # Verify service started automatically
   docker exec pe-kind-worker systemctl status vpn-setup.service
   ```

2. **Monitor Performance**:
   ```bash
   # Watch BGP session state
   watch -n 5 'docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"'
   ```

3. **Deploy to Multiple Nodes**:
   - Use the multi-node deployment script above
   - Each node will derive its own VTEP IP from its br0 IP
   - All nodes will peer with the same TOR switch (leafkind)

## Resources

- **Full Documentation**: [spec.md](./spec.md)
- **Implementation Plan**: [plan.md](./plan.md)
- **Research**: [research.md](./research.md)
- **Data Model**: [data-model.md](./data-model.md)
- **Contracts**: [contracts/](./contracts/)
- **FRR Documentation**: https://docs.frrouting.org/en/latest/evpn.html
- **Containerlab Guide**: https://containerlab.dev/

## What's Next?

✅ VPN setup complete! You can now:
- Deploy workloads that use L2VPN (VNI 210) for bridged connectivity
- Deploy workloads that use L3VPN (VNI 100) for routed connectivity
- Scale to multiple kind nodes in the cluster
- Each node automatically configures its unique VTEP IP based on br0
