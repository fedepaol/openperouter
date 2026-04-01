# Static Configuration Contract

**Feature**: 005-systemd-vni-setup  
**Configuration**: OpenPERouter Static Configuration  
**Version**: 1.0  
**Date**: 2026-04-01

## Overview

This contract defines the static configuration file format for OpenPERouter in systemd mode, providing declarative VPN configuration (L2 and L3) via YAML files that the controller reads and converts to FRR configuration.

## Configuration File Location

### Directory
- **Host Path**: `/var/lib/openperouter/configs/`
- **Container Mount**: `/etc/openperouter/` (mounted by perouter container)
- **File Pattern**: `openpe_*.yaml`

All files matching `openpe_*.yaml` in the directory are read and merged.

### File Naming
- **Generated File**: `openpe_evpn.yaml`
- **Template File**: `openpe_evpn.yaml.template` (in repository: `systemdmode/`)

## File Format

### Structure

**Type**: YAML  
**Schema**: `api/static/config.go` → `PERouterConfig`

```yaml
# /var/lib/openperouter/configs/openpe_evpn.yaml

underlays:
  - asn: <integer>
    evpn:
      vtepcidr: <ipv4-cidr>
    nics:
      - <interface-name>
    neighbors:
      - asn: <integer>
        address: <ipv4-address>

l3vnis:
  - vrf: <string>
    vni: <integer>
    vxlanport: <integer>

l2vnis:
  - vni: <integer>
    vrf: <string>
    vxlanport: <integer>
    l2gatewayips:
      - <ipv4-cidr>
    hostmaster:
      type: linux-bridge
      linuxBridge:
        name: <bridge-name>

rawfrrconfigs:
  - priority: <integer>
    rawConfig: |
      <frr-config-text>
```

## Configuration Sections

### 1. Underlays

**Purpose**: Configure underlay BGP session with TOR switch and VTEP IP

**Schema**:
```yaml
underlays:
  - asn: 64514                    # Local BGP AS number
    evpn:
      vtepcidr: 10.0.0.5/32       # VTEP IP (node-specific)
    nics:
      - eth1                       # Underlay NIC name
    neighbors:
      - asn: 65000                 # TOR AS number
        address: 10.1.1.254        # TOR IP address
```

**Field Descriptions**:

| Field | Type | Required | Description | Example |
|-------|------|----------|-------------|---------|
| `asn` | integer | Yes | Local BGP AS number | `64514` |
| `evpn.vtepcidr` | IPv4 CIDR | Yes | VTEP IP with /32 mask (node-specific) | `10.0.0.5/32` |
| `nics` | string array | Yes | Physical NIC names for underlay | `["eth1"]` |
| `neighbors[].asn` | integer | Yes | TOR switch AS number | `65000` |
| `neighbors[].address` | IPv4 | Yes | TOR switch IP address | `10.1.1.254` |

**Validation**:
- `asn`: Valid AS number (1-4294967295)
- `vtepcidr`: Valid IPv4 CIDR, must be /32 for VTEP
- `nics`: Must exist on host
- `neighbors[].address`: Must be reachable from underlay NIC

### 2. L3VNIs

**Purpose**: Configure Layer 3 VPN for routed traffic

**Schema**:
```yaml
l3vnis:
  - vrf: red               # VRF name
    vni: 100               # VXLAN Network Identifier
    vxlanport: 4789        # VXLAN UDP port
```

**Field Descriptions**:

| Field | Type | Required | Description | Example |
|-------|------|----------|-------------|---------|
| `vrf` | string | Yes | VRF name | `red` |
| `vni` | integer | Yes | VXLAN VNI (1-16777215) | `100` |
| `vxlanport` | integer | No | VXLAN UDP port (default: 4789) | `4789` |

**Validation**:
- `vni`: Must be unique across all VNIs (L2 and L3)
- `vrf`: Non-empty string
- `vxlanport`: Valid UDP port (1-65535)

**Generated FRR Configuration**:
```frr
vrf red
  vni 100
exit-vrf

router bgp 64514 vrf red
  address-family l2vpn evpn
    advertise ipv4 unicast
  exit-address-family
```

### 3. L2VNIs

**Purpose**: Configure Layer 2 VPN for bridged traffic

**Schema**:
```yaml
l2vnis:
  - vni: 210
    vrf: red
    vxlanport: 4789
    l2gatewayips:
      - "192.168.110.1/24"
    hostmaster:
      type: linux-bridge
      linuxBridge:
        name: br0
```

**Field Descriptions**:

| Field | Type | Required | Description | Example |
|-------|------|----------|-------------|---------|
| `vni` | integer | Yes | VXLAN VNI (1-16777215) | `210` |
| `vrf` | string | Yes | VRF name | `red` |
| `vxlanport` | integer | No | VXLAN UDP port (default: 4789) | `4789` |
| `l2gatewayips` | string array | Yes | Gateway IPs for L2VPN | `["192.168.110.1/24"]` |
| `hostmaster.type` | enum | Yes | Bridge type: `linux-bridge` \| `ovs-bridge` | `linux-bridge` |
| `hostmaster.linuxBridge.name` | string | Yes | Host bridge name to attach veth | `br0` |

**Validation**:
- `vni`: Must be unique across all VNIs
- `vrf`: Should match L3VNI VRF if routing between L2/L3 desired
- `l2gatewayips`: Valid IPv4 CIDR array
- `hostmaster.linuxBridge.name`: Bridge must exist on host

**Generated Configuration**:
- VXLAN interface: `vxlan-210`
- Bridge in FRR namespace: `br-210`
- Veth pair: `veth-br210-ns` ↔ `veth-br210-host`
- Host attachment: `veth-br210-host` enslaved to `br0`

### 4. RawFRRConfigs

**Purpose**: Provide additional FRR configuration snippets not covered by declarative specs

**Schema**:
```yaml
rawfrrconfigs:
  - priority: 100
    rawConfig: |
      ! Additional EVPN tuning
      router bgp 64514
        address-family l2vpn evpn
          advertise-svi-ip
        exit-address-family
```

**Field Descriptions**:

| Field | Type | Required | Description | Example |
|-------|------|----------|-------------|---------|
| `priority` | integer | No | Rendering order (lower first, default: 0) | `100` |
| `rawConfig` | string | Yes | Raw FRR configuration text | See example |

**Validation**:
- `rawConfig`: Must be valid FRR configuration syntax (no validation at admission)
- `priority`: Controls ordering when multiple snippets exist

**Use Cases**:
- EVPN route policies
- BFD configuration
- Advanced BGP tuning
- Route maps

## Template File

### Location
- **Repository**: `systemdmode/openpe_evpn.yaml.template`
- **Format**: YAML with placeholder variables

### Template Example

```yaml
# OpenPERouter Static Configuration Template
# Generated by setup-vpn.sh

underlays:
  - asn: {{LOCAL_AS}}
    evpn:
      vtepcidr: {{VTEP_IP}}/32
    nics:
      - {{UNDERLAY_NIC}}
    neighbors:
      - asn: {{TOR_AS}}
        address: {{TOR_IP}}

l3vnis:
  - vrf: {{VRF_NAME}}
    vni: {{L3_VNI}}
    vxlanport: {{VXLAN_PORT}}

l2vnis:
  - vni: {{L2_VNI}}
    vrf: {{VRF_NAME}}
    vxlanport: {{VXLAN_PORT}}
    l2gatewayips:
      - "{{L2_GATEWAY_IP}}"
    hostmaster:
      type: linux-bridge
      linuxBridge:
        name: br0

rawfrrconfigs:
  - priority: 100
    rawConfig: |
      ! EVPN SVI IP advertisement
      router bgp {{LOCAL_AS}}
        address-family l2vpn evpn
          advertise-svi-ip
        exit-address-family
```

### Placeholder Variables

| Placeholder | Source | Example Value |
|------------|--------|---------------|
| `{{LOCAL_AS}}` | Environment: `LOCAL_AS` | `64514` |
| `{{TOR_AS}}` | Environment: `TOR_AS` | `65000` |
| `{{TOR_IP}}` | Environment: `TOR_IP` | `10.1.1.254` |
| `{{UNDERLAY_NIC}}` | Environment: `UNDERLAY_NIC` | `eth1` |
| `{{VTEP_IP}}` | Derived from br0 IP | `10.0.0.5` |
| `{{VRF_NAME}}` | Environment: `VRF_NAME` | `red` |
| `{{L3_VNI}}` | Environment: `L3_VNI` | `100` |
| `{{L2_VNI}}` | Environment: `L2_VNI` | `210` |
| `{{VXLAN_PORT}}` | Environment: `VXLAN_PORT` | `4789` |
| `{{L2_GATEWAY_IP}}` | Environment: `L2_GATEWAY_IP` | `192.168.110.1/24` |

### Template Rendering

The setup script renders the template using one of:

**Option 1: envsubst** (simple, recommended):
```bash
export LOCAL_AS=64514
export TOR_AS=65000
export TOR_IP=10.1.1.254
export VTEP_IP=10.0.0.5
# ... other variables

envsubst < openpe_evpn.yaml.template > /var/lib/openperouter/configs/openpe_evpn.yaml
```

**Option 2: sed** (bash-only):
```bash
sed -e "s|{{LOCAL_AS}}|${LOCAL_AS}|g" \
    -e "s|{{TOR_AS}}|${TOR_AS}|g" \
    -e "s|{{TOR_IP}}|${TOR_IP}|g" \
    -e "s|{{VTEP_IP}}|${VTEP_IP}|g" \
    openpe_evpn.yaml.template > /var/lib/openperouter/configs/openpe_evpn.yaml
```

## Configuration Loading

### Process Flow

1. **Script generates YAML** from template → `/var/lib/openperouter/configs/openpe_evpn.yaml`
2. **OpenPERouter controller** reads all `openpe_*.yaml` files
3. **Controller converts** YAML to API objects (Underlay, L2VNI, L3VNI, RawFRRConfig)
4. **Controller renders** FRR configuration from API objects
5. **FRR loads** rendered configuration

### Controller Integration

The controller's `StaticConfigurationReconciler` (or `UnderlayVNIReconciler` in systemd mode):
- Watches the config directory: `/etc/openperouter/` (container path)
- Reads files via `staticconfiguration.ReadRouterConfigs()`
- Merges with any CRD-based configs (if applicable)
- Renders to FRR configuration

### Validation

**Static validation**: Performed by controller at load time
- YAML syntax
- Field types and ranges
- Required field presence

**Runtime validation**: Performed by FRR
- BGP neighbor reachability
- VNI uniqueness
- Network interface existence

## Complete Example

### Rendered Configuration

```yaml
# /var/lib/openperouter/configs/openpe_evpn.yaml
# Generated by setup-vpn.sh on node1 (br0: 192.168.1.5)

underlays:
  - asn: 64514
    evpn:
      vtepcidr: 10.0.0.5/32
    nics:
      - eth1
    neighbors:
      - asn: 65000
        address: 10.1.1.254

l3vnis:
  - vrf: red
    vni: 100
    vxlanport: 4789

l2vnis:
  - vni: 210
    vrf: red
    vxlanport: 4789
    l2gatewayips:
      - "192.168.110.1/24"
    hostmaster:
      type: linux-bridge
      linuxBridge:
        name: br0

rawfrrconfigs:
  - priority: 100
    rawConfig: |
      ! EVPN SVI IP advertisement
      router bgp 64514
        address-family l2vpn evpn
          advertise-svi-ip
        exit-address-family
```

### Expected FRR Output

The controller renders this to FRR configuration like:

```frr
! Underlay configuration
router bgp 64514
  bgp router-id 10.0.0.5
  neighbor 10.1.1.254 remote-as 65000
  address-family ipv4 unicast
    network 10.0.0.5/32
  exit-address-family
  address-family l2vpn evpn
    neighbor 10.1.1.254 activate
    advertise-all-vni
  exit-address-family

! L3VNI configuration
vrf red
  vni 100
exit-vrf

router bgp 64514 vrf red
  address-family l2vpn evpn
    advertise ipv4 unicast
  exit-address-family

! L2VNI bridge
interface br-210
  ip address 192.168.110.1/24

! Raw config snippet
router bgp 64514
  address-family l2vpn evpn
    advertise-svi-ip
  exit-address-family
```

## CRD Equivalence

This static configuration is equivalent to creating these CRDs in Kubernetes mode:

**Underlay**:
```yaml
apiVersion: openpe.openperouter.github.io/v1alpha1
kind: Underlay
metadata:
  name: static-underlay-0
spec:
  asn: 64514
  evpn:
    vtepcidr: 10.0.0.5/32
  nics:
    - eth1
  neighbors:
    - asn: 65000
      address: 10.1.1.254
```

**L3VNI**:
```yaml
apiVersion: openpe.openperouter.github.io/v1alpha1
kind: L3VNI
metadata:
  name: red
spec:
  vrf: red
  vni: 100
```

**L2VNI**:
```yaml
apiVersion: openpe.openperouter.github.io/v1alpha1
kind: L2VNI
metadata:
  name: l2red
spec:
  vni: 210
  vrf: red
  hostmaster:
    type: linux-bridge
    linuxBridge:
      name: br0
```

**RawFRRConfig**:
```yaml
apiVersion: openpe.openperouter.github.io/v1alpha1
kind: RawFRRConfig
metadata:
  name: evpn-svi
spec:
  priority: 100
  rawConfig: |
    router bgp 64514
      address-family l2vpn evpn
        advertise-svi-ip
      exit-address-family
```

## File Permissions

```bash
# Configuration directory
/var/lib/openperouter/configs/
  Owner: root
  Permissions: 755 (drwxr-xr-x)

# Configuration file
/var/lib/openperouter/configs/openpe_evpn.yaml
  Owner: root
  Permissions: 644 (-rw-r--r--)
```

## References

- **Static Config Reader**: `internal/staticconfiguration/reader.go`
- **Config Structure**: `api/static/config.go`
- **Controller Integration**: `internal/controller/routerconfiguration/static_configuration_reader.go`
- **Example Config**: https://github.com/fedepaol/dev-scripts/.../openpe_config.yaml
- **API Types**: `api/v1alpha1/{underlay,l2vni,l3vni,rawfrrconfig}_types.go`
