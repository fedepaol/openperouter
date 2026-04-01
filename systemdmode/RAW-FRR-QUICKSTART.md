# Raw FRR VPN Setup - Quick Start Guide

This guide shows you how to set up L2/L3 VPNs using **raw FRR configuration** and **manual network setup scripts** instead of the OpenPERouter controller.

## What's Different?

**Old Way** (controller-based):
- Generate YAML config → Controller reads it → Controller creates network infrastructure → Controller updates FRR

**New Way** (raw FRR):
- Bash scripts create network infrastructure directly → Apply raw FRR config directly to FRR daemon

## Files Created

```
systemdmode/
├── frr-evpn.conf.template      # Raw FRR configuration template
├── setup-network.sh            # Creates VRFs, bridges, VXLAN, veths
├── setup-vpn-raw.sh            # Main orchestration script
├── deploy-vpn-raw.sh           # Deployment script for kind nodes
└── RAW-FRR-APPROACH.md         # Detailed documentation
```

## Quick Deploy

### 1. Deploy to Kind Cluster

```bash
cd /home/fpaoline/openperouter1

# Deploy to all nodes in cluster
./systemdmode/deploy-vpn-raw.sh pe-kind
```

This copies all files to each node and sets up the systemd service.

### 2. Start VPN Setup on a Node

```bash
# Start on worker node
docker exec pe-kind-worker systemctl start vpn-setup.service

# Check status
docker exec pe-kind-worker systemctl status vpn-setup.service

# View logs
docker exec pe-kind-worker journalctl -u vpn-setup.service --no-pager -f
```

### 3. Verify Setup

```bash
NODE="pe-kind-worker"

# Check BGP session
docker exec $NODE podman exec frr vtysh -c 'show bgp summary'

# Check EVPN VNIs
docker exec $NODE podman exec frr vtysh -c 'show evpn vni'

# Check EVPN routes (type 2 = MAC-IP, type 5 = IP Prefix)
docker exec $NODE podman exec frr vtysh -c 'show bgp l2vpn evpn route'

# Check network interfaces (host)
docker exec $NODE ip link show | grep -E 'br0|host-210'

# Check network interfaces (FRR namespace)
docker exec $NODE podman exec frr ip link show | grep -E 'vni|br-pe|pe-|red'

# Check FRR configuration
docker exec $NODE podman exec frr cat /etc/frr/frr.conf
```

## What Gets Created

### In FRR Namespace

1. **VRF** (`red`): Layer 3 routing domain
2. **L3VNI Bridge** (`br-pe-100`): Bridge for L3 VXLAN
3. **L3VNI VXLAN** (`vni100`): VXLAN interface (VNI 100)
4. **L2VNI Bridge** (`br-pe-210`): Bridge for L2 VXLAN with gateway IP
5. **L2VNI VXLAN** (`vni210`): VXLAN interface (VNI 210)
6. **PE Veth** (`pe-210`): Namespace side of veth pair

### On Host

1. **Host Veth** (`host-210`): Host side of veth pair, attached to `br0`

### In FRR Config

1. **BGP Session**: With TOR switch (kindswitch)
2. **EVPN Address Family**: For advertising VNI routes
3. **VRF-VNI Mapping**: VRF `red` ↔ VNI `100`
4. **L2 Gateway IP**: `192.168.110.1/24` on `br-pe-210`

## Configuration

### Environment Variables

Create `/etc/openperouter/vpn-setup.env` on each node:

```bash
# Underlay
UNDERLAY_NIC=eth1              # Physical NIC for underlay
TOR_IP=10.1.1.254              # TOR switch IP
TOR_AS=65000                   # TOR switch AS
LOCAL_AS=64514                 # Local BGP AS

# VPN
VRF_NAME=red                   # VRF name
L3_VNI=100                     # L3VPN VNI
L2_VNI=210                     # L2VPN VNI
VXLAN_PORT=4789                # VXLAN port
L2_GATEWAY_IP=192.168.110.1/24 # L2 gateway IP

# Timeout
FRR_READY_TIMEOUT=60           # Seconds to wait for FRR

# Optional
NODE_NAME=$(hostname)          # Node name in FRR config
```

Apply configuration:
```bash
docker exec <node> systemctl restart vpn-setup.service
```

## Manual Execution (for Testing)

Run scripts manually on a node:

```bash
# Enter node
docker exec -it pe-kind-worker bash

# Set environment
export VTEP_IP=10.0.0.5
export VRF_NAME=red
export L3_VNI=100
export L2_VNI=210
export VXLAN_PORT=4789
export L2_GATEWAY_IP=192.168.110.1/24

# Run network setup
/usr/local/bin/setup-network.sh

# Run full VPN setup
/usr/local/bin/setup-vpn-raw.sh
```

## Troubleshooting

### Service Fails to Start

```bash
# Check logs
docker exec <node> journalctl -u vpn-setup.service -p err

# Check FRR is running
docker exec <node> podman ps | grep frr

# Check br0 exists
docker exec <node> ip link show br0
```

### BGP Not Establishing

```bash
# Check underlay NIC is in namespace
docker exec <node> podman exec frr ip link show eth1

# Check TOR reachability
docker exec <node> podman exec frr ping -c 3 10.1.1.254

# Check BGP configuration
docker exec <node> podman exec frr vtysh -c 'show running-config'
```

### Network Infrastructure Missing

```bash
# Re-run network setup
docker exec <node> bash -c 'VTEP_IP=10.0.0.5 /usr/local/bin/setup-network.sh'

# Check what was created
docker exec <node> podman exec frr ip link show
```

### Veth Not on br0

```bash
# Check veth exists
docker exec <node> ip link show host-210

# Manually attach to br0
docker exec <node> ip link set host-210 master br0
docker exec <node> ip link set host-210 up
```

## Expected Output

### BGP Summary
```
BGP router identifier 10.0.0.5, local AS number 64514
...
Neighbor        V         AS MsgRcvd MsgSent   TblVer  InQ OutQ  Up/Down State/PfxRcd
10.1.1.254      4      65000      15      18        0    0    0 00:05:32            5
```

### EVPN VNI
```
VNI        Type VxLAN IF      # MACs   # ARPs   # Remote VTEPs
100        L3   vni100        0        0        1
210        L2   vni210        5        3        1
```

### EVPN Routes
```
BGP table version is 10, local router ID is 10.0.0.5
...
*> [2]:[0]:[48]:[aa:bb:cc:dd:ee:ff] RD 10.0.0.5:2
*> [5]:[0]:[24]:[192.168.110.0] RD 10.0.0.5:2
```

## Network Topology

```
Host: br0 ──┬── VMs/Containers
            └── host-210 (veth)
                    │
         ┌──────────┴────────────────────────┐
         │ FRR Namespace                     │
         │                                   │
         │  pe-210 (veth)                    │
         │      │                            │
         │      ▼                            │
         │  VRF "red"                        │
         │    ├─ br-pe-100 ── vni100 (L3)    │
         │    └─ br-pe-210 ── vni210 (L2)    │
         │            │                      │
         │            │ (192.168.110.1/24)   │
         │            │                      │
         │        VTEP IP: 10.0.0.X          │
         │            │                      │
         │         eth1 (underlay)           │
         └────────────┼─────────────────────┘
                      │
                      ▼
                 TOR Switch
```

## Next Steps

1. **Test L2 Connectivity**: Deploy VMs/containers on br0, verify they can reach `192.168.110.1`
2. **Test EVPN**: Deploy same setup on another node, verify MAC learning
3. **Monitor BGP**: `watch -n 5 'docker exec <node> podman exec frr vtysh -c "show bgp summary"'`
4. **View Routes**: `docker exec <node> podman exec frr vtysh -c 'show bgp l2vpn evpn route'`

## Learn More

- Detailed documentation: `systemdmode/RAW-FRR-APPROACH.md`
- FRR EVPN docs: https://docs.frrouting.org/en/latest/evpn.html
- Network script: `systemdmode/setup-network.sh` (shows all `ip link` commands)
- FRR template: `systemdmode/frr-evpn.conf.template` (shows FRR config)
