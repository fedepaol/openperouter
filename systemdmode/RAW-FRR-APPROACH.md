# Raw FRR Configuration Approach

This document describes the raw FRR configuration approach for OpenPERouter systemd mode, which bypasses the controller and provides direct control over FRR configuration and network infrastructure.

## Overview

**Old Approach** (controller-based):
1. Generate YAML configuration file
2. Controller reads YAML and converts to FRR config
3. Controller creates network infrastructure (VRFs, bridges, VXLAN, veths)
4. FRR applies configuration

**New Approach** (raw FRR):
1. Manually create network infrastructure with bash scripts
2. Generate raw FRR configuration directly
3. Apply configuration directly to FRR daemon

## Components

### 1. FRR Configuration Template

**File**: `systemdmode/frr-evpn.conf.template`

This is a raw FRR configuration file with placeholders that gets rendered with node-specific values:

```frr
router bgp {{LOCAL_AS}}
 bgp router-id {{ROUTER_ID}}
 neighbor {{TOR_IP}} remote-as {{TOR_AS}}
 address-family l2vpn evpn
  neighbor {{TOR_IP}} activate
  advertise-all-vni
  advertise-svi-ip
 exit-address-family
exit

vrf {{VRF_NAME}}
 vni {{L3_VNI}}
exit-vrf
```

**Placeholders**:
- `{{VTEP_IP}}` - VTEP IP derived from br0 (10.0.0.X)
- `{{ROUTER_ID}}` - BGP router ID (same as VTEP IP)
- `{{LOCAL_AS}}`, `{{TOR_AS}}`, `{{TOR_IP}}` - BGP parameters
- `{{VRF_NAME}}`, `{{L3_VNI}}`, `{{L2_VNI}}` - VNI parameters
- `{{L2_GATEWAY_IP}}` - L2VPN gateway IP
- `{{NODE_NAME}}`, `{{TIMESTAMP}}` - Metadata

### 2. Network Setup Script

**File**: `systemdmode/setup-network.sh`

This script manually creates all network infrastructure that the controller would normally create:

**What it creates in the FRR namespace**:
1. **VRF** (`vrf red`): Layer 3 routing domain
   ```bash
   ip link add red type vrf table <auto-allocated-id>
   ```

2. **L3VNI Bridge** (`br-pe-100`): Bridge for L3 VXLAN traffic
   ```bash
   ip link add br-pe-100 type bridge master red
   ```

3. **L3VNI VXLAN** (`vni100`): VXLAN interface for L3VPN
   ```bash
   ip link add vni100 type vxlan id 100 local <VTEP_IP> dstport 4789 nolearning
   ip link set vni100 master br-pe-100
   ```

4. **L2VNI Bridge** (`br-pe-210`): Bridge for L2 VXLAN traffic
   ```bash
   ip link add br-pe-210 type bridge master red
   ip addr add 192.168.110.1/24 dev br-pe-210
   ```

5. **L2VNI VXLAN** (`vni210`): VXLAN interface for L2VPN
   ```bash
   ip link add vni210 type vxlan id 210 local <VTEP_IP> dstport 4789 nolearning
   ip link set vni210 master br-pe-210
   ```

6. **Veth Pair** (`host-210` <-> `pe-210`): Connect host to L2VPN
   ```bash
   # On host
   ip link add host-210 type veth peer pe-210
   ip link set pe-210 netns <frr-pid>
   ip link set host-210 master br0
   
   # In FRR namespace
   ip link set pe-210 master br-pe-210
   ```

### 3. Main Setup Script

**File**: `systemdmode/setup-vpn-raw.sh`

This is the main orchestration script that:
1. Waits for FRR readiness
2. Derives VTEP IP from br0
3. Moves underlay NIC to FRR namespace
4. Calls `setup-network.sh` to create infrastructure
5. Renders FRR config template
6. Applies config to FRR daemon
7. Validates the setup

## Network Topology

```
┌─────────────────────────────────────────────────────────────────┐
│ Host Namespace                                                  │
│                                                                 │
│  ┌────────┐                  ┌──────────┐                      │
│  │  br0   │◄─────────────────┤ host-210 │ (veth, L2VPN)        │
│  └────────┘                  └──────────┘                      │
│      │                              ▲                           │
│      │ (VMs/containers)             │ veth pair                │
│      ▼                              │                           │
└──────────────────────────────────────│───────────────────────────┘
                                       │
                ┌──────────────────────┼────────────────────────────┐
                │ FRR Namespace        │                            │
                │                      ▼                            │
                │              ┌───────────┐                        │
                │              │  pe-210   │ (veth)                 │
                │              └─────┬─────┘                        │
                │                    │                              │
                │  ┌─────────────────┴────────────────┐             │
                │  │ VRF "red"                        │             │
                │  │                                  │             │
                │  │  ┌────────────┐  ┌────────────┐ │             │
                │  │  │ br-pe-100  │  │ br-pe-210  │ │  (bridges)  │
                │  │  │ (L3VNI)    │  │ (L2VNI)    │ │             │
                │  │  └──────┬─────┘  └──────┬─────┘ │             │
                │  │         │                │       │             │
                │  │    ┌────┴────┐      ┌────┴────┐ │             │
                │  │    │ vni100  │      │ vni210  │ │  (VXLAN)    │
                │  │    └─────────┘      └─────────┘ │             │
                │  │         │                │       │             │
                │  └─────────┴────────────────┴───────┘             │
                │            │                │                     │
                │            └────────┬───────┘                     │
                │                     │                             │
                │                VTEP IP: 10.0.0.X                  │
                │                (source for VXLAN)                 │
                │                     │                             │
                │              ┌──────┴──────┐                      │
                │              │  eth1       │ (underlay NIC)       │
                │              └──────┬──────┘                      │
                └─────────────────────┼─────────────────────────────┘
                                      │
                                      ▼
                              ┌───────────────┐
                              │  TOR Switch   │
                              │  (kindswitch) │
                              └───────────────┘
                                BGP Peer
```

## Deployment

### Files to Deploy

Copy these files to each kind node:

1. **Templates**:
   - `frr-evpn.conf.template` → `/etc/openperouter/templates/`

2. **Scripts**:
   - `setup-network.sh` → `/usr/local/bin/`
   - `setup-vpn-raw.sh` → `/usr/local/bin/`
   - `common.sh` → `/usr/local/bin/` (if not already there)

3. **Systemd Service**:
   - `vpn-setup.service` → `/etc/systemd/system/`
     - Update `ExecStart=/usr/local/bin/setup-vpn-raw.sh`

### Deployment Script

```bash
#!/bin/bash
CLUSTER_NAME="$1"
NODES=$(kind get nodes --name "$CLUSTER_NAME")

for NODE in $NODES; do
    echo "Deploying to $NODE..."
    
    # Create directories
    docker exec "$NODE" mkdir -p /etc/openperouter/templates /usr/local/bin
    
    # Copy template
    docker cp systemdmode/frr-evpn.conf.template "$NODE:/etc/openperouter/templates/"
    
    # Copy scripts
    docker cp systemdmode/setup-network.sh "$NODE:/usr/local/bin/"
    docker cp systemdmode/setup-vpn-raw.sh "$NODE:/usr/local/bin/"
    docker cp systemdmode/common.sh "$NODE:/usr/local/bin/"
    
    # Make executable
    docker exec "$NODE" chmod +x /usr/local/bin/setup-network.sh
    docker exec "$NODE" chmod +x /usr/local/bin/setup-vpn-raw.sh
    docker exec "$NODE" chmod +x /usr/local/bin/common.sh
    
    # Copy systemd service
    docker cp systemdmode/quadlets/vpn-setup.service "$NODE:/etc/systemd/system/"
    
    # Update service to use new script
    docker exec "$NODE" sed -i 's|setup-vpn.sh|setup-vpn-raw.sh|' /etc/systemd/system/vpn-setup.service
    
    # Reload and start
    docker exec "$NODE" systemctl daemon-reload
    docker exec "$NODE" systemctl start vpn-setup.service
done
```

## Environment Variables

Configure via `/etc/openperouter/vpn-setup.env`:

```bash
# Underlay Configuration
UNDERLAY_NIC=eth1          # Physical NIC for underlay
TOR_IP=10.1.1.254          # TOR switch IP
TOR_AS=65000               # TOR switch AS number
LOCAL_AS=64514             # Local BGP AS number

# VPN Configuration
VRF_NAME=red               # VRF name
L3_VNI=100                 # L3VPN VXLAN VNI
L2_VNI=210                 # L2VPN VXLAN VNI
VXLAN_PORT=4789            # VXLAN UDP port
L2_GATEWAY_IP=192.168.110.1/24  # L2VPN gateway IP

# Timeouts
FRR_READY_TIMEOUT=60       # FRR readiness timeout (seconds)

# Node identification
NODE_NAME=$(hostname)      # Node name for FRR config
```

## Verification

After deployment, verify the setup:

### 1. Service Status
```bash
docker exec pe-kind-worker systemctl status vpn-setup.service
```

### 2. Network Infrastructure (Host)
```bash
docker exec pe-kind-worker ip link show | grep -E 'br0|host-210'
```

Expected:
- `br0`: Host bridge
- `host-210`: Veth attached to br0

### 3. Network Infrastructure (FRR Namespace)
```bash
docker exec pe-kind-worker podman exec frr ip link show
```

Expected:
- `red`: VRF
- `br-pe-100`: L3VNI bridge
- `br-pe-210`: L2VNI bridge
- `vni100`: L3VNI VXLAN interface
- `vni210`: L2VNI VXLAN interface
- `pe-210`: Veth namespace side
- `eth1`: Underlay NIC (moved from host)

### 4. BGP Status
```bash
docker exec pe-kind-worker podman exec frr vtysh -c 'show bgp summary'
```

Expected: Neighbor `<TOR_IP>` in state `Established`

### 5. EVPN VNIs
```bash
docker exec pe-kind-worker podman exec frr vtysh -c 'show evpn vni'
```

Expected:
- VNI 100 (L3)
- VNI 210 (L2)

### 6. EVPN Routes
```bash
docker exec pe-kind-worker podman exec frr vtysh -c 'show bgp l2vpn evpn route'
```

Expected: Type 2 (MAC-IP) and Type 5 (IP Prefix) routes

### 7. FRR Configuration
```bash
docker exec pe-kind-worker podman exec frr cat /etc/frr/frr.conf
```

Verify rendered configuration matches expected values.

## Advantages of Raw FRR Approach

1. **No Controller Dependency**: Direct control without controller layer
2. **Full Visibility**: Can see exact FRR commands being executed
3. **Easier Debugging**: FRR config and network setup are explicit
4. **Faster**: No controller reconciliation loop
5. **Portable**: Bash scripts can be used in any systemd environment
6. **Matches Dev-Scripts**: Aligns with existing dev-scripts patterns

## Disadvantages

1. **Manual Infrastructure**: Must manage network interfaces directly
2. **No Automatic Cleanup**: Removing VNIs requires manual cleanup
3. **Less Dynamic**: Changing config requires script re-run
4. **State Management**: No declarative reconciliation

## Comparison with Controller Approach

| Aspect | Controller (YAML) | Raw FRR (Bash) |
|--------|------------------|----------------|
| Configuration | YAML → Controller → FRR | FRR config directly |
| Network Setup | Controller creates | Bash scripts create |
| Dependencies | OpenPERouter controller | FRR + bash + iproute2 |
| Validation | Controller reconciliation | Manual validation |
| Updates | Controller reconciles | Re-run scripts |
| Cleanup | Controller removes | Manual cleanup |
| Debugging | Check controller logs | Check script logs |
| Flexibility | Limited to API | Full FRR access |

## Migration from Controller Approach

To switch an existing deployment:

1. **Backup current config**:
   ```bash
   docker exec <node> podman exec frr vtysh -c 'show running-config' > backup.conf
   ```

2. **Stop controller** (if running):
   ```bash
   docker exec <node> systemctl stop perouter.service
   ```

3. **Deploy raw FRR scripts** (see Deployment section)

4. **Run setup**:
   ```bash
   docker exec <node> /usr/local/bin/setup-vpn-raw.sh
   ```

5. **Verify** (see Verification section)

## Troubleshooting

### FRR Config Not Applied

**Symptom**: `show running-config` doesn't show expected config

**Fix**:
```bash
# Copy config manually
docker exec <node> podman cp /etc/frr/frr.conf frr:/etc/frr/

# Reload FRR
docker exec <node> podman exec frr pkill -HUP bgpd
```

### Network Infrastructure Missing

**Symptom**: Interfaces like `br-pe-100` don't exist

**Fix**:
```bash
# Re-run network setup
docker exec <node> VTEP_IP=10.0.0.5 /usr/local/bin/setup-network.sh
```

### BGP Session Not Establishing

**Symptom**: Neighbor stuck in `Active` or `Connect` state

**Debug**:
```bash
# Check underlay NIC is in namespace
docker exec <node> podman exec frr ip link show eth1

# Check TOR reachability
docker exec <node> podman exec frr ping -c 3 <TOR_IP>

# Check BGP logs
docker exec <node> podman exec frr tail -f /etc/frr/frr.log
```

### Veth Not Attached to br0

**Symptom**: `host-210` not visible or not attached to br0

**Fix**:
```bash
# Check if veth exists
docker exec <node> ip link show host-210

# Manually attach to br0
docker exec <node> ip link set host-210 master br0
docker exec <node> ip link set host-210 up
```

## References

- FRR EVPN Documentation: https://docs.frrouting.org/en/latest/evpn.html
- Linux VXLAN: https://www.kernel.org/doc/Documentation/networking/vxlan.txt
- Network Namespaces: `man ip-netns`
- Original Dev-Scripts: https://github.com/fedepaol/dev-scripts/
