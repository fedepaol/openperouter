# Two-Unit Systemd Approach

This document describes the two-systemd-unit approach for OpenPERouter VPN setup.

## Overview

The VPN setup is split into two independent systemd units that run sequentially:

1. **setup-underlay.service** - Sets up underlay infrastructure
2. **generate-config.service** - Generates configuration for the controller

This separation allows for:
- Better failure isolation (underlay vs config generation)
- Clearer logging and debugging
- Ability to regenerate config without touching underlay
- Modular design that matches systemd best practices

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ System Boot                                                 │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ routerpod-pod.service (FRR Container)                       │
│   - Starts FRR container via podman                         │
│   - BGP daemon becomes operational                          │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ setup-underlay.service                                      │
│   Script: /usr/local/bin/setup-underlay.sh                 │
│                                                             │
│   1. Wait for FRR container readiness (isfrr_ready)        │
│   2. Derive VTEP IP from br0 (10.0.0.X)                    │
│   3. Move underlay NIC (eth1) to FRR namespace             │
│   4. Save variables to /var/lib/openperouter/vpn-setup.vars│
│                                                             │
│   Output: /var/lib/openperouter/vpn-setup.vars             │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ generate-config.service                                     │
│   Script: /usr/local/bin/generate-config.sh                │
│                                                             │
│   1. Load variables from vpn-setup.vars                    │
│   2. Render template with node-specific values             │
│   3. Write YAML config with rawfrrconfigs section          │
│                                                             │
│   Output: /var/lib/openperouter/configs/openpe_evpn.yaml  │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ OpenPERouter Controller (perouter)                         │
│   - Reads /var/lib/openperouter/configs/openpe_*.yaml     │
│   - Creates network infrastructure:                        │
│     * VRFs (red)                                           │
│     * Bridges (br-pe-100, br-pe-210)                       │
│     * VXLAN interfaces (vni100, vni210)                    │
│     * Veth pairs (host-210 <-> pe-210)                     │
│   - Applies rawfrrconfigs to FRR daemon                    │
│   - Establishes BGP session with TOR                       │
└─────────────────────────────────────────────────────────────┘
```

## Unit 1: setup-underlay.service

**Purpose**: Prepare underlay infrastructure and derive node-specific values

**Dependencies**:
- After: `routerpod-pod.service`, `frr.service`, `network-online.target`
- Requires: `routerpod-pod.service`

**Script**: `/usr/local/bin/setup-underlay.sh`

**What it does**:
1. Waits for FRR container to be ready (up to 60 seconds)
2. Verifies br0 bridge exists and has an IP address
3. Derives VTEP IP from br0's last octet (192.168.1.5 → 10.0.0.5)
4. Moves underlay NIC to FRR namespace for BGP connectivity
5. Saves variables to `/var/lib/openperouter/vpn-setup.vars`

**Output Variables** (saved to vpn-setup.vars):
```bash
VTEP_IP="10.0.0.5"              # Derived from br0
BR0_IP="192.168.1.5"            # br0's IP address
LAST_OCTET="5"                  # Last octet from br0
NODE_NAME="pe-kind-worker"      # Hostname
UNDERLAY_NIC="eth1"             # Underlay NIC name
FRR_PID="12345"                 # FRR container PID
```

**Exit Codes**:
- 0: Success
- 1: General error (br0 missing, NIC missing, etc.)
- 124: Timeout waiting for FRR

**Logging**: `journalctl -u setup-underlay.service`

## Unit 2: generate-config.service

**Purpose**: Generate static YAML configuration with rawfrrconfigs

**Dependencies**:
- After: `setup-underlay.service`
- Requires: `setup-underlay.service`

**Script**: `/usr/local/bin/generate-config.sh`

**What it does**:
1. Loads variables from `/var/lib/openperouter/vpn-setup.vars`
2. Reads template from `/etc/openperouter/templates/openpe_evpn.yaml.template`
3. Substitutes placeholders with actual values (VTEP_IP, NODE_NAME, etc.)
4. Writes rendered config to `/var/lib/openperouter/configs/openpe_evpn.yaml`
5. Validates generated config has required sections

**Input**:
- Variables: `/var/lib/openperouter/vpn-setup.vars`
- Template: `/etc/openperouter/templates/openpe_evpn.yaml.template`
- Environment: `/etc/openperouter/vpn-setup.env` (optional)

**Output**: `/var/lib/openperouter/configs/openpe_evpn.yaml`

**Generated Config Sections**:
```yaml
underlays:         # BGP underlay session with TOR
l3vnis:            # L3VPN configuration (VNI 100)
l2vnis:            # L2VPN configuration (VNI 210)
rawfrrconfigs:     # Complete FRR EVPN configuration
```

**Exit Codes**:
- 0: Success
- 1: Error (template missing, rendering failed, validation failed)

**Logging**: `journalctl -u generate-config.service`

## Environment Configuration

Both units can read environment variables from `/etc/openperouter/vpn-setup.env`:

```bash
# /etc/openperouter/vpn-setup.env

# Underlay Setup (setup-underlay.service)
UNDERLAY_NIC=eth1              # Physical NIC for underlay
FRR_READY_TIMEOUT=60           # FRR readiness timeout (seconds)
NODE_NAME=$(hostname)          # Node name for FRR config

# Config Generation (generate-config.service)
TOR_IP=10.1.1.254              # TOR switch IP address
TOR_AS=65000                   # TOR switch AS number
LOCAL_AS=64514                 # Local BGP AS number
VRF_NAME=red                   # VRF name
L3_VNI=100                     # L3VPN VXLAN VNI
L2_VNI=210                     # L2VPN VXLAN VNI
VXLAN_PORT=4789                # VXLAN UDP port
L2_GATEWAY_IP=192.168.110.1/24 # L2VPN gateway IP

# File paths (optional overrides)
VARS_FILE=/var/lib/openperouter/vpn-setup.vars
CONFIG_TEMPLATE=/etc/openperouter/templates/openpe_evpn.yaml.template
CONFIG_OUTPUT=/var/lib/openperouter/configs/openpe_evpn.yaml
```

## Deployment

### Files to Deploy to Kind Nodes

1. **Scripts**:
   - `systemdmode/setup-underlay.sh` → `/usr/local/bin/setup-underlay.sh`
   - `systemdmode/generate-config.sh` → `/usr/local/bin/generate-config.sh`
   - `systemdmode/common.sh` → `/usr/local/bin/common.sh`

2. **Systemd Units**:
   - `systemdmode/quadlets/setup-underlay.service` → `/etc/systemd/system/`
   - `systemdmode/quadlets/generate-config.service` → `/etc/systemd/system/`

3. **Templates**:
   - `systemdmode/openpe_evpn.yaml.template` → `/etc/openperouter/templates/`

4. **Environment** (optional):
   - `/etc/openperouter/vpn-setup.env` (created manually or via deployment script)

### Deployment Script Example

```bash
#!/bin/bash
CLUSTER_NAME="${1:-pe-kind}"
NODES=$(kind get nodes --name "$CLUSTER_NAME")

for NODE in $NODES; do
    echo "Deploying to $NODE..."
    
    # Create directories
    docker exec "$NODE" mkdir -p /usr/local/bin \
                                 /etc/openperouter/templates \
                                 /etc/systemd/system \
                                 /var/lib/openperouter/configs
    
    # Copy scripts
    docker cp systemdmode/setup-underlay.sh "$NODE:/usr/local/bin/"
    docker cp systemdmode/generate-config.sh "$NODE:/usr/local/bin/"
    docker cp systemdmode/common.sh "$NODE:/usr/local/bin/"
    docker exec "$NODE" chmod +x /usr/local/bin/setup-underlay.sh \
                                  /usr/local/bin/generate-config.sh \
                                  /usr/local/bin/common.sh
    
    # Copy template
    docker cp systemdmode/openpe_evpn.yaml.template "$NODE:/etc/openperouter/templates/"
    
    # Copy systemd units
    docker cp systemdmode/quadlets/setup-underlay.service "$NODE:/etc/systemd/system/"
    docker cp systemdmode/quadlets/generate-config.service "$NODE:/etc/systemd/system/"
    
    # Reload systemd and enable services
    docker exec "$NODE" systemctl daemon-reload
    docker exec "$NODE" systemctl enable setup-underlay.service generate-config.service
    
    # Start services
    docker exec "$NODE" systemctl start setup-underlay.service
    docker exec "$NODE" systemctl start generate-config.service
    
    echo "Deployed to $NODE successfully"
done
```

## Verification

### Check Service Status

```bash
# On kind node
docker exec pe-kind-worker systemctl status setup-underlay.service
docker exec pe-kind-worker systemctl status generate-config.service

# Check both services
docker exec pe-kind-worker systemctl status setup-underlay.service generate-config.service
```

### Check Logs

```bash
# Underlay setup logs
docker exec pe-kind-worker journalctl -u setup-underlay.service --no-pager

# Config generation logs
docker exec pe-kind-worker journalctl -u generate-config.service --no-pager

# Combined logs
docker exec pe-kind-worker journalctl -u setup-underlay.service -u generate-config.service --no-pager
```

### Verify Output Files

```bash
# Check variables file
docker exec pe-kind-worker cat /var/lib/openperouter/vpn-setup.vars

# Check generated config
docker exec pe-kind-worker cat /var/lib/openperouter/configs/openpe_evpn.yaml

# Preview first 30 lines of config
docker exec pe-kind-worker head -30 /var/lib/openperouter/configs/openpe_evpn.yaml
```

### Verify Network Setup

```bash
# Check underlay NIC moved to FRR namespace
docker exec pe-kind-worker podman exec frr ip link show eth1

# Check VTEP IP is correct
docker exec pe-kind-worker cat /var/lib/openperouter/vpn-setup.vars | grep VTEP_IP

# Check BGP session (after controller applies config)
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"
```

## Troubleshooting

### Unit 1 Fails (setup-underlay.service)

**Timeout waiting for FRR**:
```bash
docker exec pe-kind-worker systemctl status routerpod-pod.service
docker exec pe-kind-worker podman ps | grep frr
docker exec pe-kind-worker podman logs frr
```

**br0 bridge missing**:
```bash
docker exec pe-kind-worker ip link show br0
# Create if missing:
docker exec pe-kind-worker ip link add br0 type bridge
docker exec pe-kind-worker ip addr add 192.168.1.5/24 dev br0
docker exec pe-kind-worker ip link set br0 up
```

**Underlay NIC not found**:
```bash
docker exec pe-kind-worker ip -br link show
# Update UNDERLAY_NIC in /etc/openperouter/vpn-setup.env
```

### Unit 2 Fails (generate-config.service)

**Variables file missing**:
```bash
# Unit 1 must succeed first
docker exec pe-kind-worker systemctl status setup-underlay.service
docker exec pe-kind-worker cat /var/lib/openperouter/vpn-setup.vars
```

**Template not found**:
```bash
docker exec pe-kind-worker ls -la /etc/openperouter/templates/
# Redeploy template if missing
```

**Invalid generated config**:
```bash
docker exec pe-kind-worker cat /var/lib/openperouter/configs/openpe_evpn.yaml
# Check for missing sections or invalid YAML syntax
```

## Manual Restart

To regenerate configuration without touching underlay:

```bash
# Just restart config generation
docker exec pe-kind-worker systemctl restart generate-config.service

# Or both units
docker exec pe-kind-worker systemctl restart setup-underlay.service generate-config.service
```

To force underlay reconfiguration:

```bash
# Stop both
docker exec pe-kind-worker systemctl stop generate-config.service setup-underlay.service

# Restart both
docker exec pe-kind-worker systemctl start setup-underlay.service
docker exec pe-kind-worker systemctl start generate-config.service
```

## Benefits of Two-Unit Approach

1. **Separation of Concerns**:
   - Underlay setup is independent of config generation
   - Can regenerate config without touching underlay

2. **Better Debugging**:
   - Clear logs for each stage
   - Easy to identify which stage failed

3. **Idempotent Config Generation**:
   - Can re-run generate-config.service without side effects
   - Useful for testing template changes

4. **Systemd Best Practices**:
   - Each unit does one thing well
   - Clear dependency chain
   - Proper use of `After` and `Requires`

5. **Variable Persistence**:
   - Derived values (VTEP_IP) saved to file
   - Can be inspected and reused
   - Config generation can be decoupled from underlay setup timing
