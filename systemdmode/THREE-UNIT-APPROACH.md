# Three-Unit Systemd Approach - Fully Standalone

This document describes the three-systemd-unit approach for OpenPERouter VPN setup with **manual infrastructure creation**.

## Overview

The VPN setup is split into three independent systemd units that run sequentially:

1. **setup-underlay.service** - Underlay infrastructure setup
2. **setup-network.service** - Network infrastructure creation (VRFs, bridges, VXLAN, veths)
3. **generate-config.service** - Configuration generation

This approach is **fully standalone** - it creates all network infrastructure manually without relying on the OpenPERouter controller. The controller (if running) only needs to apply the FRR configuration from rawfrrconfigs.

## Key Difference from Two-Unit Approach

**Two-Unit (Controller-Dependent)**:
- Scripts generate YAML config
- **Controller creates infrastructure** (VRFs, bridges, VXLAN, veths)
- Controller applies rawfrrconfigs to FRR

**Three-Unit (Standalone)**:
- **Scripts create infrastructure** directly (VRFs, bridges, VXLAN, veths)
- Scripts generate YAML config
- Controller only applies rawfrrconfigs to FRR (optional)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ System Boot                                                 │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ routerpod-pod.service (FRR Container)                       │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ Unit 1: setup-underlay.service                              │
│   Script: /usr/local/bin/setup-underlay.sh                 │
│                                                             │
│   1. Wait for FRR container readiness                      │
│   2. Derive VTEP IP from br0 (10.0.0.X)                    │
│   3. Move underlay NIC (eth1) to FRR namespace             │
│   4. Save variables to vpn-setup.vars                      │
│                                                             │
│   Output: /var/lib/openperouter/vpn-setup.vars             │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ Unit 2: setup-network.service                               │
│   Script: /usr/local/bin/setup-network.sh                  │
│                                                             │
│   Creates in FRR namespace:                                │
│   1. VRF "red" (routing table 200)                         │
│   2. L3VNI bridge "br-pe-100" → enslaved to VRF            │
│   3. L3VNI VXLAN "vni100" → enslaved to br-pe-100          │
│   4. L2VNI bridge "br-pe-210" → enslaved to VRF            │
│   5. L2VNI VXLAN "vni210" → enslaved to br-pe-210          │
│   6. Assign gateway IP to br-pe-210                        │
│   7. Veth pair: host-210 <-> pe-210                        │
│   8. Attach host-210 to br0                                │
│                                                             │
│   Output: All network infrastructure ready                 │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ Unit 3: generate-config.service                             │
│   Script: /usr/local/bin/generate-config.sh                │
│                                                             │
│   1. Load variables from vpn-setup.vars                    │
│   2. Render YAML template with node values                 │
│   3. Write config with rawfrrconfigs section               │
│                                                             │
│   Output: /var/lib/openperouter/configs/openpe_evpn.yaml  │
└────────────────┬────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────┐
│ OpenPERouter Controller (Optional)                         │
│   - Reads openpe_evpn.yaml                                 │
│   - Infrastructure already exists (created by scripts)     │
│   - Only applies rawfrrconfigs to FRR daemon               │
└─────────────────────────────────────────────────────────────┘
```

## Network Infrastructure Created

After `setup-network.service` completes, the following exists in the FRR namespace:

### VRF
```bash
$ podman exec frr ip link show type vrf
200: red@NONE: <NOARP,MASTER,UP,LOWER_UP> mtu 65536 qdisc noqueue state UP
    link/ether xx:xx:xx:xx:xx:xx brd ff:ff:ff:ff:ff:ff
```

### Bridges
```bash
$ podman exec frr ip link show type bridge
201: br-pe-100: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 master red
202: br-pe-210: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 master red
```

### VXLAN Interfaces
```bash
$ podman exec frr ip link show type vxlan
203: vni100: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1450 master br-pe-100
204: vni210: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1450 master br-pe-210
```

### Veth Pair
```bash
# Host side (attached to br0)
$ ip link show host-210
205: host-210@if206: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 master br0

# FRR namespace side (attached to br-pe-210)
$ podman exec frr ip link show pe-210
206: pe-210@if205: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 master br-pe-210
```

### L2 Gateway IP
```bash
$ podman exec frr ip addr show br-pe-210
202: br-pe-210: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 master red
    inet 192.168.110.1/24 scope global br-pe-210
```

## Network Topology

```
┌─────────────────────────────────────────────────────────────┐
│ Host Namespace                                              │
│                                                             │
│  ┌────────┐                  ┌──────────┐                  │
│  │  br0   │◄─────────────────┤ host-210 │ (veth L2VPN)     │
│  └────────┘                  └──────────┘                  │
│      │                              ▲                       │
│      │ (VMs/containers)             │ veth pair            │
│      ▼                              │                       │
└──────────────────────────────────────│───────────────────────┘
                                       │
        ┌──────────────────────────────┼────────────────────────┐
        │ FRR Namespace                │                        │
        │                              ▼                        │
        │                      ┌───────────┐                    │
        │                      │  pe-210   │ (veth)             │
        │                      └─────┬─────┘                    │
        │                            │                          │
        │  ┌─────────────────────────┴──────────────┐           │
        │  │ VRF "red" (table 200)                  │           │
        │  │                                        │           │
        │  │  ┌────────────┐      ┌────────────┐   │           │
        │  │  │ br-pe-100  │      │ br-pe-210  │   │  bridges  │
        │  │  │ (L3VNI)    │      │ (L2VNI)    │   │           │
        │  │  │            │      │ GW:        │   │           │
        │  │  │            │      │ 192.168.   │   │           │
        │  │  │            │      │ 110.1/24   │   │           │
        │  │  └──────┬─────┘      └──────┬─────┘   │           │
        │  │         │                   │         │           │
        │  │    ┌────┴────┐         ┌────┴────┐    │           │
        │  │    │ vni100  │         │ vni210  │    │  VXLAN    │
        │  │    │ VNI:100 │         │ VNI:210 │    │           │
        │  │    └─────────┘         └─────────┘    │           │
        │  │         │                   │         │           │
        │  └─────────┴───────────────────┴─────────┘           │
        │            │                   │                     │
        │            └────────┬──────────┘                     │
        │                     │                                │
        │                VTEP IP: 10.0.0.X                     │
        │                (source for VXLAN)                    │
        │                     │                                │
        │              ┌──────┴──────┐                         │
        │              │  eth1       │ (underlay NIC)          │
        │              └──────┬──────┘                         │
        └─────────────────────┼──────────────────────────────  │
                              │
                              ▼
                      ┌───────────────┐
                      │  TOR Switch   │
                      │  (kindswitch) │
                      └───────────────┘
                        BGP Peer
```

## Service Details

### Unit 1: setup-underlay.service

**File**: `/usr/local/bin/setup-underlay.sh`

**Purpose**: Prepare underlay and derive node-specific values

**Creates**:
- Moves `eth1` (or configured NIC) from host to FRR namespace
- Derives `VTEP_IP` from br0's last octet
- Saves variables to `/var/lib/openperouter/vpn-setup.vars`

**Exit Codes**:
- 0: Success
- 1: Error (br0 missing, NIC not found, etc.)
- 124: Timeout waiting for FRR

### Unit 2: setup-network.service

**File**: `/usr/local/bin/setup-network.sh`

**Purpose**: Create all network infrastructure manually

**Creates (all in FRR namespace)**:
1. **VRF**: `ip link add red type vrf table 200`
2. **L3 Bridge**: `ip link add br-pe-100 type bridge master red`
3. **L3 VXLAN**: `ip link add vni100 type vxlan id 100 local $VTEP_IP dstport 4789 nolearning`
4. **L2 Bridge**: `ip link add br-pe-210 type bridge master red`
5. **L2 VXLAN**: `ip link add vni210 type vxlan id 210 local $VTEP_IP dstport 4789 nolearning`
6. **L2 Gateway IP**: `ip addr add 192.168.110.1/24 dev br-pe-210`
7. **Veth Pair**: `ip link add host-210 type veth peer pe-210` (host-side → br0, FRR-side → br-pe-210)

**Exit Codes**:
- 0: Success
- 1: Error (infrastructure creation failed)

**Idempotency**: Script checks if infrastructure already exists before creating

### Unit 3: generate-config.service

**File**: `/usr/local/bin/generate-config.sh`

**Purpose**: Generate YAML configuration with rawfrrconfigs

**Creates**:
- `/var/lib/openperouter/configs/openpe_evpn.yaml` with:
  - underlays section (BGP with TOR)
  - l3vnis section (VNI 100)
  - l2vnis section (VNI 210)
  - rawfrrconfigs section (complete FRR EVPN config)

**Exit Codes**:
- 0: Success
- 1: Error (template missing, rendering failed)

## Verification Commands

### After setup-underlay.service

```bash
# Check variables saved
cat /var/lib/openperouter/vpn-setup.vars

# Verify VTEP IP derived correctly
grep VTEP_IP /var/lib/openperouter/vpn-setup.vars

# Check underlay NIC in FRR namespace
podman exec frr ip link show eth1
```

### After setup-network.service

```bash
# Check VRF
podman exec frr ip link show type vrf

# Check all bridges
podman exec frr ip link show type bridge

# Check VXLAN interfaces
podman exec frr ip link show type vxlan

# Check L2 gateway IP
podman exec frr ip addr show br-pe-210

# Check veth pair exists
ip link show host-210
podman exec frr ip link show pe-210

# Verify host-210 attached to br0
bridge link show | grep host-210

# Full infrastructure summary
podman exec frr ip -br link show
```

### After generate-config.service

```bash
# Check config generated
cat /var/lib/openperouter/configs/openpe_evpn.yaml

# Preview rawfrrconfigs section
grep -A 50 "rawfrrconfigs:" /var/lib/openperouter/configs/openpe_evpn.yaml
```

## Advantages of Three-Unit Approach

1. **Fully Standalone**: No dependency on controller for infrastructure creation
2. **Explicit Infrastructure**: Clear what network objects exist and how they're created
3. **Easier Debugging**: Can verify infrastructure at each stage
4. **Controller Optional**: Infrastructure works even if controller isn't running
5. **Better Separation**: Each unit has a single, clear responsibility

## When Controller Is Not Running

Even without the controller, you have:
- ✅ VRF created
- ✅ Bridges created
- ✅ VXLAN interfaces created
- ✅ Veth pairs connected to br0
- ✅ L2 gateway IP configured

What's missing:
- ❌ FRR configuration (no BGP, no EVPN)

You can manually apply FRR config:
```bash
# If controller isn't running, manually copy FRR config
# (Extract from rawfrrconfigs section of YAML)
```

## Troubleshooting

### setup-network.service Failed

```bash
# View detailed logs
journalctl -u setup-network.service --no-pager

# Check what infrastructure exists
podman exec frr ip link show

# Manually retry infrastructure creation
sudo systemctl restart setup-network.service
```

### Infrastructure Already Exists

The script is idempotent - it checks if infrastructure exists before creating:

```bash
# Example log output
[2026-04-08 10:30:00] VRF 'red' already exists
[2026-04-08 10:30:00] Bridge 'br-pe-100' already exists
```

To force recreation, delete infrastructure first:

```bash
# WARNING: This will disrupt networking
podman exec frr ip link del red  # Deletes VRF and all enslaved interfaces
```

## Deployment

See `deployment/README.md` for full installation instructions.

Quick deployment to real system:

```bash
cd systemdmode/deployment
sudo make install
sudo systemctl enable --now setup-underlay.service setup-network.service generate-config.service
```
