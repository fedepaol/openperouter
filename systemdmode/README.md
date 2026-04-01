# OpenPERouter Systemd Mode

This directory contains scripts and configuration for running OpenPERouter in systemd mode (without Kubernetes).

## Contents

- `common.sh` - Common utilities for FRR container interaction
- `setup-vpn.sh` - **NEW**: Automated VPN setup script for L2 and L3 EVPN
- `openpe_evpn.yaml.template` - **NEW**: Static configuration template for VPN setup
- `deploy-vpn-setup.sh` - **NEW**: Deployment script to install VPN setup on kind nodes
- `validate-vpn-setup.sh` - **NEW**: Validation script to check VPN setup status
- `deploy.sh` - Deploy scripts to kind nodes
- `setup_node_config.sh` - Configure node-specific settings
- `verify-quadlets.sh` - Verify quadlet configuration
- `quadlets/` - Systemd service units (quadlet format)
  - `vpn-setup.service` - **NEW**: VPN setup systemd service

## VPN Setup (Feature 005)

### Overview

The VPN setup feature automatically configures L2 and L3 VPNs on system boot using static YAML configuration. The script:

1. Waits for FRR container to be ready
2. Derives VTEP IP from br0's last octet (10.0.0.X format)
3. Generates static configuration from template
4. Moves underlay NIC to FRR namespace
5. Waits for controller to apply configuration

### Quick Start

#### 1. Deploy to Kind Cluster

```bash
# Deploy cluster with hostmode
make deploy-hostmode

# Deploy VPN setup to all nodes
make deploy-vpn-setup KIND_CLUSTER_NAME=pe-kind
```

#### 2. Verify Setup

```bash
# Run validation script
./systemdmode/validate-vpn-setup.sh pe-kind-worker

# Or manually check
docker exec pe-kind-worker systemctl status vpn-setup.service
docker exec pe-kind-worker cat /var/lib/openperouter/configs/openpe_evpn.yaml
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"
```

### Configuration

Environment variables can be set in `/etc/openperouter/vpn-setup.env` on the node:

```bash
# Underlay Configuration
UNDERLAY_NIC=eth1          # Physical NIC for underlay (default: eth1)
TOR_IP=10.1.1.254          # TOR switch IP address
TOR_AS=65000               # TOR switch AS number
LOCAL_AS=64514             # Local BGP AS number (default: 64514)

# VPN Configuration
L2_VNI=210                 # L2VPN VNI (default: 210)
L3_VNI=100                 # L3VPN VNI (default: 100)
VRF_NAME=red               # VRF name (default: red)
VXLAN_PORT=4789            # VXLAN port (default: 4789)
L2_GATEWAY_IP=192.168.110.1/24  # L2VPN gateway IP

# Timeouts
FRR_READY_TIMEOUT=60       # FRR readiness timeout in seconds (default: 60)
```

### Architecture

```
Host Node (kind)
├── br0 (bridge with IP)               # Used to derive VTEP IP
├── eth1 (underlay NIC)                # Moved to FRR namespace
│
├── FRR Container (podman)
│   ├── Network Namespace
│   │   ├── eth1 (moved from host)     # Underlay connectivity
│   │   ├── vxlan-100 (L3VPN)          # VXLAN interface for L3
│   │   ├── vxlan-210 (L2VPN)          # VXLAN interface for L2
│   │   └── veth-br210-ns              # Connected to br0 on host
│   │
│   └── BGP/EVPN Configuration
│       ├── BGP session with TOR
│       ├── L3VPN (VNI 100, VRF red)
│       └── L2VPN (VNI 210, VRF red)
│
└── Static Config: /var/lib/openperouter/configs/openpe_evpn.yaml
```

### Files Created

On each kind node:
- `/usr/local/bin/setup-vpn.sh` - Main setup script
- `/etc/openperouter/templates/openpe_evpn.yaml.template` - Configuration template
- `/etc/containers/systemd/vpn-setup.service` - Systemd service unit
- `/var/lib/openperouter/configs/openpe_evpn.yaml` - Generated configuration (runtime)

### Troubleshooting

#### Service fails to start

```bash
# Check service status
docker exec pe-kind-worker systemctl status vpn-setup.service

# Check logs
docker exec pe-kind-worker journalctl -u vpn-setup.service --no-pager
```

#### FRR timeout

```bash
# Check FRR container
docker exec pe-kind-worker podman ps | grep frr

# Check FRR logs
docker exec pe-kind-worker podman logs frr
```

#### BGP not establishing

```bash
# Check BGP status
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"

# Check TOR reachability
docker exec pe-kind-worker podman exec frr ping <TOR_IP>
```

### Manual Deployment

If not using Make targets:

```bash
# Deploy to specific cluster
./systemdmode/deploy-vpn-setup.sh pe-kind

# Validate specific node
./systemdmode/validate-vpn-setup.sh pe-kind-worker
```

### Documentation

Full documentation available at:
- **Specification**: `specs/005-systemd-vni-setup/spec.md`
- **Implementation Plan**: `specs/005-systemd-vni-setup/plan.md`
- **Quick Start Guide**: `specs/005-systemd-vni-setup/quickstart.md`
- **Tasks**: `specs/005-systemd-vni-setup/tasks.md`

## Other Systemd Mode Scripts

### deploy.sh

Deploys systemd services and containers to kind nodes:

```bash
./systemdmode/deploy.sh pe-kind
```

### setup_node_config.sh

Configures node-specific settings:

```bash
./systemdmode/setup_node_config.sh pe-kind
```

### verify-quadlets.sh

Verifies quadlet configuration:

```bash
./systemdmode/verify-quadlets.sh
```

## Common Utilities (common.sh)

Provides utility functions for FRR container interaction:

- `frr_netns_pid()` - Get FRR container namespace PID
- `inns <command>` - Execute command inside FRR namespace
- `isfrr_ready()` - Check if FRR container is ready

Usage:

```bash
source systemdmode/common.sh

# Get FRR PID
PID=$(frr_netns_pid)

# Run command in FRR namespace
inns ip addr show

# Check FRR readiness
if isfrr_ready; then
    echo "FRR is ready"
fi
```
