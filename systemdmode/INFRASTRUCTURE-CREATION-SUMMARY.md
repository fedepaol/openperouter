# Infrastructure Creation Summary

## What Changed

The VPN setup now uses **THREE systemd units** instead of two, with the addition of `setup-network.service` that **manually creates all network infrastructure**.

## Three-Unit Flow

```
1. setup-underlay.service
   ├─ Wait for FRR container
   ├─ Derive VTEP IP from br0 (10.0.0.X)
   ├─ Move underlay NIC to FRR namespace
   └─ Save variables → /var/lib/openperouter/vpn-setup.vars

2. setup-network.service ← NEW!
   ├─ Create VRF "red" in FRR namespace
   ├─ Create L3VNI: br-pe-100 + vni100 (VNI 100)
   ├─ Create L2VNI: br-pe-210 + vni210 (VNI 210)
   ├─ Assign gateway IP to br-pe-210
   ├─ Create veth pair: host-210 ↔ pe-210
   └─ Attach host-210 to br0

3. generate-config.service
   ├─ Load variables from vpn-setup.vars
   ├─ Render YAML template
   └─ Write config with rawfrrconfigs → /var/lib/openperouter/configs/openpe_evpn.yaml
```

## Infrastructure Created by Scripts

After all three units complete, the following infrastructure exists:

### In FRR Namespace

| Component | Type | Name | Details |
|-----------|------|------|---------|
| VRF | Virtual Routing Domain | `red` | Routing table 200 |
| Bridge | L3VNI Bridge | `br-pe-100` | Enslaved to VRF "red" |
| VXLAN | L3VNI VXLAN | `vni100` | VNI 100, enslaved to br-pe-100 |
| Bridge | L2VNI Bridge | `br-pe-210` | Enslaved to VRF "red", has gateway IP |
| VXLAN | L2VNI VXLAN | `vni210` | VNI 210, enslaved to br-pe-210 |
| IP Address | L2 Gateway | `192.168.110.1/24` | On br-pe-210 |
| Veth | FRR side | `pe-210` | Enslaved to br-pe-210 |
| NIC | Underlay | `eth1` | Moved from host, used for BGP |

### In Host Namespace

| Component | Type | Name | Details |
|-----------|------|------|---------|
| Veth | Host side | `host-210` | Enslaved to br0, connects to FRR namespace |

## What Controller Does Now

**Before (Two-Unit Approach)**:
- Controller creates **everything**: VRFs, bridges, VXLAN, veths
- Controller applies rawfrrconfigs to FRR

**After (Three-Unit Approach)**:
- **Scripts create infrastructure**: VRFs, bridges, VXLAN, veths ✅
- Controller only applies rawfrrconfigs to FRR

**Result**: System is **fully standalone**. Infrastructure exists even if controller isn't running.

## Files Added/Modified

### New Files

```
systemdmode/
├── quadlets/
│   └── setup-network.service          ← NEW systemd unit
└── THREE-UNIT-APPROACH.md             ← NEW documentation

deployment/
├── usr/local/bin/
│   └── setup-network.sh               ← Added to package
└── etc/systemd/system/
    └── setup-network.service          ← Added to package
```

### Modified Files

```
systemdmode/
├── setup-network.sh                   ← Updated to load vpn-setup.vars
├── deploy-two-unit-vpn.sh             ← Updated for three units
├── quadlets/
│   └── generate-config.service        ← Depends on setup-network now
└── deployment/
    ├── install.sh                     ← Installs setup-network.sh
    ├── uninstall.sh                   ← Removes setup-network.service
    ├── Makefile                       ← Checks for setup-network files
    └── README.md                      ← Documents three-unit approach
```

## Deployment Package

**Location**: `systemdmode/deployment/openperouter-vpn-setup.tar.gz`

**Size**: 12K

**Contents**:
- 3 bash scripts (setup-underlay, setup-network, generate-config)
- 3 systemd units
- 1 YAML template
- 1 environment file example
- install/uninstall scripts
- Makefile
- README

## Installation

### Real System

```bash
cd systemdmode/deployment
sudo make install
sudo systemctl enable --now setup-underlay.service setup-network.service generate-config.service
```

### Containerlab

```bash
cd systemdmode
./deploy-two-unit-vpn.sh pe-kind

for NODE in $(kind get nodes --name pe-kind); do
    docker exec $NODE systemctl start setup-underlay.service
    docker exec $NODE systemctl start setup-network.service
    docker exec $NODE systemctl start generate-config.service
done
```

## Verification

After deployment, verify all infrastructure:

```bash
# Check all three services
systemctl status setup-underlay.service setup-network.service generate-config.service

# Check network infrastructure created
podman exec frr ip link show type vrf
podman exec frr ip link show type bridge
podman exec frr ip link show type vxlan
ip link show host-210
podman exec frr ip link show pe-210

# Check generated config
cat /var/lib/openperouter/configs/openpe_evpn.yaml
```

## Benefits

1. ✅ **Fully Standalone**: No controller dependency for infrastructure
2. ✅ **Explicit Control**: Scripts show exactly what's created
3. ✅ **Better Debugging**: Can verify infrastructure at each stage
4. ✅ **Production Ready**: Works on real systems without Kubernetes
5. ✅ **rawfrrconfigs**: No `podman cp` needed for FRR config

## Documentation

- **THREE-UNIT-APPROACH.md** - Complete architecture documentation
- **deployment/README.md** - Installation and usage guide
- **DEPLOYMENT-QUICKSTART.md** - Quick reference for both deployment methods
