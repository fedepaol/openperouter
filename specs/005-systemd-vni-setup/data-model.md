# Data Model: Systemd-Based VNI Setup Script

**Feature**: 005-systemd-vni-setup  
**Date**: 2026-04-01  
**Status**: Phase 1 Complete

## Overview

This document defines the data entities, configuration parameters, and state representations for the VPN setup script. Since this is a bash script that configures network infrastructure, the "data model" represents runtime configuration inputs, network entity states, and validation rules.

## Configuration Entities

### 1. Environment Configuration

**Purpose**: External configuration inputs provided via environment variables, used for both template rendering and configuration validation

**Fields**:

| Field | Type | Required | Default | Description | Validation |
|-------|------|----------|---------|-------------|------------|
| `UNDERLAY_NIC` | string | No | `eth1` | Physical NIC name to move to FRR namespace | Must exist in host namespace |
| `TOR_IP` | IPv4 address | No | TBD in implementation | TOR switch IP address for BGP peering | Valid IPv4 format |
| `TOR_AS` | integer | No | TBD in implementation | TOR switch BGP AS number | Valid AS number (1-4294967295) |
| `LOCAL_AS` | integer | No | `64514` | Local BGP AS number | Valid AS number (1-4294967295) |
| `FRR_READY_TIMEOUT` | integer | No | `60` | Timeout in seconds waiting for FRR | Positive integer, max 600 |
| `L2_VNI` | integer | No | `210` | L2VPN VXLAN Network Identifier | Valid VNI (1-16777215) |
| `L3_VNI` | integer | No | `100` | L3VPN VXLAN Network Identifier | Valid VNI (1-16777215) |
| `VRF_NAME` | string | No | `red` | VRF name for both L2 and L3 VPNs | Non-empty string |
| `VXLAN_PORT` | integer | No | `4789` | VXLAN UDP destination port | Valid port (1-65535) |
| `L2_GATEWAY_IP` | IPv4 CIDR | No | `192.168.110.1/24` | L2VPN gateway IP address | Valid IPv4 CIDR |
| `CONFIG_TEMPLATE` | path | No | `systemdmode/openpe_evpn.yaml.template` | Template file path | Must be readable file |
| `CONFIG_OUTPUT` | path | No | `/var/lib/openperouter/configs/openpe_evpn.yaml` | Generated config output path | Parent dir must exist |

**Lifecycle**: Loaded once at script start from environment or EnvironmentFile

**Relationships**: 
- Consumed by template rendering phase
- Used for configuration validation
- Substituted into YAML template placeholders

### 2. Host NIC Entity

**Purpose**: Physical network interface to be moved into FRR namespace for underlay connectivity

**State Fields**:

| Field | Type | Description | State Transition |
|-------|------|-------------|------------------|
| `name` | string | NIC device name (e.g., "eth1") | Constant |
| `namespace` | enum | Current network namespace: `host` \| `frr` | `host` → `frr` (irreversible in script) |
| `ip_address` | IPv4 CIDR | Pre-configured IP address | Preserved during move |
| `link_state` | enum | Link status: `up` \| `down` | `down` → `up` in FRR namespace |

**Validation Rules**:
- Must exist in host namespace at script start
- Must have IP address configured before movement
- After movement, must be accessible via FRR namespace PID

**Lifecycle**: 
1. Discovery (read from host namespace)
2. Validation (check existence and IP configuration)
3. Movement (transfer to FRR namespace)
4. Activation (bring link up in FRR namespace)

**Relationships**:
- Source: `UNDERLAY_NIC` environment variable
- Used by: BGP session configuration
- Required for: VXLAN tunnel endpoint

### 3. BGP Session Entity

**Purpose**: Underlay BGP peering session with TOR switch

**State Fields**:

| Field | Type | Description | State |
|-------|------|-------------|-------|
| `local_as` | integer | Local AS number | From `LOCAL_AS` config |
| `peer_ip` | IPv4 address | TOR switch IP | From `TOR_IP` config |
| `peer_as` | integer | TOR switch AS number | From `TOR_AS` config |
| `state` | enum | Session state: `idle` \| `connect` \| `established` \| `failed` | Target: `established` |
| `address_family` | list | Enabled address families | `["ipv4-unicast", "l2vpn-evpn"]` |

**Validation Rules**:
- Peer IP must be reachable from underlay NIC
- Local AS and Peer AS should differ (EBGP preferred)
- Session must reach `established` state for success

**Lifecycle**:
1. Configuration (via vtysh commands)
2. Activation (enable address families)
3. Validation (check session state)

**Relationships**:
- Requires: Host NIC in FRR namespace
- Enables: EVPN route advertisement for VPNs

### 4. VTEP Configuration Entity

**Purpose**: VXLAN Tunnel Endpoint IP address derived from br0

**State Fields**:

| Field | Type | Description | Derivation |
|-------|------|-------------|------------|
| `source_bridge` | string | Bridge name | Always `br0` |
| `source_ip` | IPv4 address | br0 IP address | Extracted via `ip addr show br0` |
| `last_octet` | integer | Last octet from source_ip | `source_ip.split('.')[-1]` |
| `vtep_ip` | IPv4 CIDR | VTEP IP address | `10.0.0.{last_octet}/24` |

**Validation Rules**:
- br0 must exist
- br0 must have IPv4 address
- If multiple IPs on br0, use first (primary)
- Last octet must be 1-254 (valid host address)

**Lifecycle**:
1. Discovery (find br0)
2. Extraction (get IP address)
3. Calculation (derive VTEP IP)
4. Assignment (configure on VXLAN interfaces)

**Relationships**:
- Source: br0 host bridge
- Used by: L2VPN and L3VPN VXLAN configurations

### 5. L3VPN Entity

**Purpose**: Layer 3 VXLAN VPN for routed traffic

**State Fields**:

| Field | Type | Description | Value |
|-------|------|-------------|-------|
| `vni` | integer | VXLAN Network Identifier | From `L3_VNI` (default: 100) |
| `vrf` | string | VRF name | From `VRF_NAME` (default: "red") |
| `vxlan_port` | integer | UDP destination port | From `VXLAN_PORT` (default: 4789) |
| `vtep_ip` | IPv4 CIDR | Tunnel endpoint IP | From VTEP Configuration |
| `route_target` | string | BGP route target | Auto-derived by FRR from VNI |

**Network Components**:
- VRF device: `vrf-{VRF_NAME}`
- VXLAN interface: `vxlan-{VNI}`
- Bridge interface: `br-{VNI}` (in FRR namespace)

**Validation Rules**:
- VNI must be unique (not overlap with L2VPN)
- VRF must be created before VXLAN assignment
- EVPN type 5 routes must be advertised after configuration

**Lifecycle**:
1. VRF creation
2. VXLAN interface creation
3. VNI-VRF association
4. BGP EVPN advertisement activation

**Relationships**:
- Requires: BGP session established
- Requires: VTEP IP configured
- Advertises: IP prefix routes (EVPN type 5)

### 6. L2VPN Entity

**Purpose**: Layer 2 VXLAN VPN for bridged traffic

**State Fields**:

| Field | Type | Description | Value |
|-------|------|-------------|-------|
| `vni` | integer | VXLAN Network Identifier | From `L2_VNI` (default: 210) |
| `vrf` | string | VRF name | From `VRF_NAME` (default: "red") |
| `vxlan_port` | integer | UDP destination port | From `VXLAN_PORT` (default: 4789) |
| `vtep_ip` | IPv4 CIDR | Tunnel endpoint IP | From VTEP Configuration |
| `gateway_ip` | IPv4 CIDR | Gateway IP on bridge | From `L2_GATEWAY_IP` |
| `host_bridge` | string | Host bridge for attachment | Always `br0` |

**Network Components**:
- VXLAN interface: `vxlan-{VNI}` (in FRR namespace)
- Bridge interface: `br-{VNI}` (in FRR namespace)
- Veth pair: `veth-br{VNI}-ns` ↔ `veth-br{VNI}-host`
- Host attachment: `veth-br{VNI}-host` enslaved to `br0`

**Validation Rules**:
- VNI must be unique (not overlap with L3VPN)
- br0 must exist before veth attachment
- EVPN type 2 routes must be advertised after configuration
- Gateway IP must be in different subnet than br0

**Lifecycle**:
1. VXLAN interface creation (in FRR namespace)
2. Bridge creation (in FRR namespace)
3. VXLAN-to-bridge attachment
4. Veth pair creation
5. Veth FRR-side attachment to bridge
6. Veth host-side attachment to br0
7. Gateway IP assignment
8. BGP EVPN advertisement activation

**Relationships**:
- Requires: BGP session established
- Requires: VTEP IP configured
- Requires: br0 exists
- Advertises: MAC-IP routes (EVPN type 2)

### 7. Static Configuration File Entity

**Purpose**: YAML configuration file that OpenPERouter controller reads to generate FRR configuration

**State Fields**:

| Field | Type | Description | Source |
|-------|------|-------------|--------|
| `template_path` | string | Path to template file | `CONFIG_TEMPLATE` env var |
| `output_path` | string | Path to generated config | `CONFIG_OUTPUT` env var |
| `rendered` | boolean | Whether file has been generated | Script execution state |
| `content` | YAML object | Parsed YAML structure | Template + substitutions |

**YAML Structure** (api/static/config.go → PERouterConfig):
```yaml
underlays: [...]       # Underlay BGP and VTEP configuration
l3vnis: [...]          # L3VPN specifications
l2vnis: [...]          # L2VPN specifications
rawfrrconfigs: [...]   # Additional FRR config snippets
```

**Template Placeholders**:
- `{{VTEP_IP}}`: Derived from br0 IP (10.0.0.X format)
- `{{UNDERLAY_NIC}}`: From environment or default
- `{{TOR_IP}}`, `{{TOR_AS}}`: TOR switch parameters
- `{{LOCAL_AS}}`: Local BGP AS number
- `{{L2_VNI}}`, `{{L3_VNI}}`: VNI values
- `{{VRF_NAME}}`: VRF name
- `{{VXLAN_PORT}}`: VXLAN port
- `{{L2_GATEWAY_IP}}`: L2VPN gateway IP

**Validation Rules**:
- Template file must exist and be readable
- Output directory must exist
- Rendered YAML must be valid syntax
- All required fields in PERouterConfig must be present

**Lifecycle**:
1. Load template file
2. Substitute placeholders with environment/derived values
3. Validate YAML syntax
4. Write to output path
5. Controller reads and applies configuration

**Relationships**:
- Source: Template file + Environment configuration + VTEP IP entity
- Consumer: OpenPERouter static configuration reader
- Triggers: Controller FRR configuration re-render

### 8. FRR Container Entity

**Purpose**: FRR routing daemon container state tracking

**State Fields**:

| Field | Type | Description | State Check |
|-------|------|-------------|-------------|
| `container_name` | string | Podman container name | Always `frr` |
| `pid` | integer | Container process ID | Retrieved via `podman inspect` |
| `running` | boolean | Container running state | `pid > 0` |
| `bgpd_ready` | boolean | BGP daemon operational | Checked via `vtysh -c "show daemons"` |
| `vtysh_responsive` | boolean | Management interface ready | Command execution succeeds |

**Validation Rules**:
- Container must be running (`running == true`)
- PID must be valid and process must exist
- vtysh must respond to commands
- bgpd must be listed in active daemons

**Lifecycle**:
1. Wait loop (poll until ready)
2. PID retrieval (for namespace operations)
3. Readiness validation
4. Configuration application

**Relationships**:
- Required by: All namespace operations (via PID)
- Required by: All FRR configuration commands (via vtysh)
- Provided by: systemd quadlet service

## State Transitions

### Script Execution Flow

```
START
  ↓
[Load Environment Configuration] ─(validation failure)→ ERROR_EXIT
  ↓
[Wait for FRR Container Ready] ─(timeout after 60s)→ ERROR_EXIT
  ↓
[Derive VTEP IP from br0] ─(br0 missing or no IP)→ ERROR_EXIT
  ↓
[Render Static Config from Template] ─(template missing or invalid YAML)→ ERROR_EXIT
  ↓
[Write Config to /var/lib/openperouter/configs/] ─(directory missing or permission denied)→ ERROR_EXIT
  ↓
[Move Host NIC to FRR Namespace] ─(NIC missing)→ ERROR_EXIT
  ↓
[Wait for Controller to Apply Config] ─(optional: brief pause for config reload)
  ↓
[Validate BGP Session Established] ─(not established after timeout)→ ERROR_EXIT
  ↓
[Validate EVPN Routes Present] ─(routes missing)→ ERROR_EXIT
  ↓
SUCCESS_EXIT
```

**Key Changes from Original Flow**:
1. **Template rendering** replaces direct FRR vtysh commands
2. **Controller applies configuration** instead of script executing FRR commands
3. **No explicit L2VPN veth enslavement** - handled by controller based on L2VNI hostmaster spec
4. **Validation only** - script validates end state, doesn't configure VPNs directly

### Error States

All error states result in immediate script termination (exit code 1) with:
- Error message logged to stderr
- Partial configuration left in place (no rollback)
- systemd journal captures output for debugging

## Data Validation Summary

| Entity | Pre-Conditions | Post-Conditions | Rollback on Failure |
|--------|---------------|-----------------|---------------------|
| Environment Config | Valid syntax, types | All fields validated | N/A (fail before changes) |
| Host NIC | Exists, has IP | In FRR namespace, link up | No |
| BGP Session | NIC in namespace | State: established | No |
| VTEP IP | br0 exists with IP | VTEP IP calculated | No |
| L3VPN | BGP established | VRF+VNI configured, routes advertised | No |
| L2VPN | BGP established, br0 exists | Bridge+VXLAN configured, veth attached | No |
| FRR Container | Running, bgpd active | Configuration applied | No |

**Note**: No rollback is performed on any failure per clarification (development/testing environment).
