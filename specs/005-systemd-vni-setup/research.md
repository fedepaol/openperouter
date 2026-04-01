# Research: Systemd-Based VNI Setup Script

**Feature**: 005-systemd-vni-setup  
**Date**: 2026-04-01  
**Status**: Complete

## Overview

This document consolidates research findings for implementing the VPN setup script, covering FRR command sequences, network namespace operations, VXLAN configuration patterns, and systemd service integration.

## 1. Static Configuration File Format

### Decision
Use OpenPERouter's static configuration format: YAML files in `/var/lib/openperouter/configs/` following the `PERouterConfig` structure. Files must match pattern `openpe_*.yaml`.

The script will generate configuration from a template, substituting node-specific values like:
- VTEP IP (derived from br0's last octet)
- Router ID (based on VTEP IP)
- Node-specific underlay NIC

### Rationale
OpenPERouter's controller already has a static configuration reader (`internal/staticconfiguration/reader.go`) that:
- Reads all `openpe_*.yaml` files from a configured directory
- Merges them with CRD-based configurations
- Supports all resource types: Underlay, L2VNI, L3VNI, RawFRRConfig

Using this existing infrastructure:
- Avoids direct FRR vtysh configuration (which can conflict with controller)
- Leverages existing validation and rendering logic
- Maintains consistency between systemd and Kubernetes modes
- Supports both declarative specs (Underlay, L2VNI, L3VNI) and raw FRR snippets (RawFRRConfig)

### Configuration Structure
```yaml
# /var/lib/openperouter/configs/openpe_evpn.yaml
underlays:
  - asn: 64514
    evpn:
      vtepcidr: 10.0.0.5/32  # Node-specific VTEP IP
    nics:
      - eth1  # From UNDERLAY_NIC env var
    neighbors:
      - asn: 65000  # From TOR_AS env var
        address: 10.1.1.254  # From TOR_IP env var

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
      ! Additional EVPN configuration
      router bgp 64514
        address-family l2vpn evpn
          advertise-svi-ip
        exit-address-family
```

### Template Approach
Use a template file (`openpe_evpn.yaml.template`) with placeholders:
```yaml
underlays:
  - asn: {{LOCAL_AS}}
    evpn:
      vtepcidr: {{VTEP_IP}}/32
    nics:
      - {{UNDERLAY_NIC}}
    neighbors:
      - asn: {{TOR_AS}}
        address: {{TOR_IP}}
# ... rest of config
```

Script fills in values using simple string substitution or `envsubst`.

### Alternatives Considered
- **Direct vtysh commands**: Rejected as it bypasses controller's configuration management
- **CRD-only approach**: Rejected as systemd mode doesn't have Kubernetes API server
- **Hardcoded configuration**: Rejected due to node-specific parameters (VTEP IP, NIC name)

### Reference
- Static configuration reader: `internal/staticconfiguration/reader.go`
- Configuration structure: `api/static/config.go`
- Example config: https://github.com/fedepaol/dev-scripts/.../openpe_config.yaml
- Config directory: `/var/lib/openperouter/configs/` (mounted to container as `/etc/openperouter`)

## 2. Network Namespace and NIC Movement

### Decision
Use `ip link set <NIC> netns <PID>` to move host NIC into FRR container's network namespace. The NIC must:
- Be configured with IP address on host before movement
- Preserve IP configuration when moved to namespace
- Be brought up in target namespace after movement

### Rationale
Moving the physical NIC into the FRR namespace allows FRR to:
- Directly manage the underlay interface for BGP peering
- Send/receive VXLAN-encapsulated packets
- Maintain full control over underlay routing

IP configuration preservation (clarification answer) means the host pre-configures the NIC, avoiding the need for the script to handle IP assignment.

### Key Commands
```bash
# Get FRR container PID (use existing common.sh utility)
FRR_PID=$(frr_netns_pid)

# Move NIC to FRR namespace
ip link set <NIC> netns $FRR_PID

# Bring up NIC in FRR namespace (using inns utility)
inns ip link set <NIC> up
```

### Alternatives Considered
- **veth pair bridge**: Rejected as it introduces unnecessary forwarding overhead and doesn't match TOR peering requirements
- **macvlan**: Rejected as physical NIC movement provides cleaner namespace isolation
- **IP address reconfiguration in namespace**: Rejected per clarification (use pre-configured IP)

### Reference
- systemdmode/common.sh: frr_netns_pid() and inns() utilities
- iproute2 documentation: `ip netns` and `ip link` commands

## 3. VTEP IP Address Derivation

### Decision
Extract the last octet from br0's IP address and construct VTEP IP as 10.0.0.X/24.

### Rationale
Per specification requirement FR-010 and clarifications, VTEP IP must be deterministic and unique per node. Using br0's last octet:
- Provides node-unique identifier (br0 has unique IP per node)
- Avoids configuration overlap in multi-node deployments
- Matches expected pattern from dev-scripts reference

### Implementation Pattern
```bash
# Extract br0 IP address
BR0_IP=$(ip -4 addr show br0 | grep -oP '(?<=inet\s)\d+(\.\d+){3}')

# Extract last octet
LAST_OCTET=$(echo "$BR0_IP" | cut -d. -f4)

# Construct VTEP IP
VTEP_IP="10.0.0.${LAST_OCTET}/24"
```

### Edge Case Handling
- No br0 or no IP on br0: Exit with error (per FR-014 and clarifications)
- Multiple IPs on br0: Use first/primary IP (per edge case documentation in spec)

### Reference
- Specification FR-009, FR-010
- Clarification session 2026-04-01

## 4. Environment Variable Configuration

### Decision
Use environment variables with hardcoded defaults for:
- `UNDERLAY_NIC`: Host NIC name (default: `eth1`)
- `TOR_IP`: TOR switch IP address (default: to be determined in implementation)
- `TOR_AS`: TOR switch AS number (default: to be determined in implementation)
- `LOCAL_AS`: Local BGP AS number (default: 64514, from l3vni.yaml reference)
- `FRR_READY_TIMEOUT`: Timeout in seconds (default: 60)

### Rationale
Per clarifications, environment variables provide:
- Deployment-specific configuration without script modification
- Systemd EnvironmentFile integration
- Fallback defaults for development/testing

### systemd Integration Pattern
```ini
[Service]
EnvironmentFile=-/etc/openperouter/vpn-setup.env
ExecStart=/usr/local/bin/setup-vpn.sh
```

### Alternatives Considered
- **Configuration file with custom parser**: Rejected as environment variables are simpler and systemd-native
- **Command-line arguments**: Rejected as systemd services typically use environment files

### Reference
- Clarification answers 1 and 2 from session 2026-04-01
- systemd.service documentation: EnvironmentFile directive

## 5. FRR Readiness Detection

### Decision
Enhance existing `isfrr_ready()` from systemdmode/common.sh, or use directly with polling loop and 60-second timeout.

### Rationale
The existing `isfrr_ready()` function checks:
1. FRR container is running (via podman inspect)
2. vtysh is responsive
3. bgpd daemon is active

This matches requirement FR-004 (verify FRR operational, not just container running).

### Implementation Pattern
```bash
source /path/to/systemdmode/common.sh

TIMEOUT=${FRR_READY_TIMEOUT:-60}
ELAPSED=0
INTERVAL=2

while ! isfrr_ready; do
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "Error: FRR not ready after ${TIMEOUT}s" >&2
        exit 1
    fi
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done

echo "FRR is ready, proceeding with VPN setup"
```

### Alternatives Considered
- **systemd service dependencies only**: Rejected as container running doesn't guarantee FRR daemons are ready
- **Fixed sleep duration**: Rejected as it either wastes time (too long) or fails intermittently (too short)

### Reference
- systemdmode/common.sh: isfrr_ready() function (lines 42-64)
- Specification FR-003, FR-004, FR-016

## 6. L2VPN Bridge Attachment

### Decision
Create veth pair, attach one end to VXLAN bridge in FRR namespace, attach other end to br0 on host.

### Rationale
L2VPN configuration requires:
- VXLAN interface for EVPN route advertisement
- Bridge attachment in FRR namespace for L2 switching
- Connection to host's br0 for local VM/container traffic

Per specification FR-008, the L2VPN veth must be enslaved to br0.

### Implementation Pattern
```bash
# In FRR namespace (using inns)
inns ip link add vxlan${VNI} type vxlan id ${VNI} local ${VTEP_IP} dstport 4789 nolearning
inns ip link set vxlan${VNI} up
inns brctl addbr br${VNI}
inns brctl addif br${VNI} vxlan${VNI}
inns ip link set br${VNI} up

# Create veth pair
ip link add veth-br${VNI}-ns type veth peer name veth-br${VNI}-host

# Attach to FRR namespace bridge
inns ip link set veth-br${VNI}-ns master br${VNI}
inns ip link set veth-br${VNI}-ns up

# Attach to host br0
ip link set veth-br${VNI}-host master br0
ip link set veth-br${VNI}-host up
```

### Alternatives Considered
- **Direct VXLAN to br0**: Rejected as FRR must control VXLAN for EVPN route management
- **OVS bridge**: Rejected as spec specifies Linux bridge (see l2vni.yaml hostmaster.type)

### Reference
- config/samples/l2vni.yaml: hostmaster.type=linux-bridge
- Specification FR-006, FR-007, FR-008

## 7. systemd Service Configuration and Deployment

### Decision
Create oneshot systemd service as a quadlet file (`.service`) that:
- Gets deployed to `/etc/containers/systemd/` on each kind node
- Runs after FRR container service (After=routerpod-pod.service)
- Sources environment file (EnvironmentFile)
- Executes setup script once (Type=oneshot)
- Logs to systemd journal (StandardOutput=journal)

### Deployment Pattern (Containerlab/Kind)
Following existing patterns from `systemdmode/deploy.sh`:
1. **Template + Script in repo**: `systemdmode/openpe_evpn.yaml.template` and `systemdmode/setup-vpn.sh`
2. **Deploy script copies to nodes**: `deploy.sh` copies script and template to each kind node
3. **Quadlet service unit**: `systemdmode/quadlets/vpn-setup.service` deployed like other quadlets
4. **Script runs INSIDE node**: Executed within kind node's systemd environment, not on host
5. **Per-node configuration**: Each node generates unique config based on its br0 IP

### Service Unit Pattern
```ini
[Unit]
Description=OpenPERouter VPN Setup
After=routerpod-pod.service frr.service
Requires=routerpod-pod.service

[Service]
Type=oneshot
RemainAfterExit=yes
EnvironmentFile=-/etc/openperouter/vpn-setup.env
ExecStart=/usr/local/bin/setup-vpn.sh
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### Deployment Integration
Extend `systemdmode/deploy.sh` to also copy:
- `setup-vpn.sh` → `/usr/local/bin/setup-vpn.sh` on each node
- `openpe_evpn.yaml.template` → `/etc/openperouter/templates/` on each node
- `vpn-setup.service` → `/etc/containers/systemd/` (already handled via quadlets pattern)

Or create a separate Make target:
```makefile
setup-vpn: ## Setup VPN configuration script on all nodes
    ./systemdmode/deploy-vpn-setup.sh $(KIND_CLUSTER_NAME)
```

### Alternatives Considered
- **Run on host, configure remotely**: Rejected as it breaks the containerlab isolation model
- **Manual execution only**: Rejected as spec requires systemd unit
- **Single static config for all nodes**: Rejected as VTEP IP is node-specific

### Reference
- Existing deployment: `systemdmode/deploy.sh` (copies quadlets to nodes)
- Node configuration: `systemdmode/setup_node_config.sh` (creates per-node config)
- systemd.service documentation

## 8. Error Handling and Logging

### Decision
- Log all major steps to stdout (captured by systemd journal)
- Log errors to stderr with descriptive messages
- Exit with non-zero status on any failure (set -e)
- No rollback or cleanup on failure

### Rationale
Per clarification answer 3, this is a development/testing environment:
- Fail-fast approach reveals problems immediately
- Partial state aids debugging (can inspect what succeeded)
- systemd can retry service if configured
- Simplifies script implementation

### Implementation Pattern
```bash
#!/bin/bash
set -e  # Exit on any error
set -u  # Exit on undefined variable
set -o pipefail  # Exit on pipe failure

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
    exit 1
}

# Usage
log "Starting VPN setup"
command || error "Command failed with status $?"
```

### Alternatives Considered
- **Rollback on failure**: Rejected per clarification (not production-ready)
- **Continue on error**: Rejected as partial configurations may cause confusing failures

### Reference
- Specification FR-012, FR-013, FR-014, FR-015
- Clarification answer 3: exit with error, no rollback

## 9. Testing Strategy

### Decision
- Manual/bash-based testing only
- Containerlab deployment with kind nodes + kindswitch TOR for integration validation
- Validation via FRR vtysh commands and route inspection
- systemd journal review for debugging

### Rationale
Per user requirements, automated e2e tests are not needed. Manual testing provides:
- Quick validation during development
- Flexibility for exploratory testing
- Clear validation commands for documentation
- Sufficient coverage for development/testing environment

### Test Coverage (Manual Validation)
1. **Script execution**: `systemctl start vpn-setup.service && echo $?`
2. **Static config generated**: `cat /var/lib/openperouter/configs/openpe_evpn.yaml`
3. **VTEP IP derived correctly**: Check `vtepcidr` field matches br0's last octet
4. **NIC movement**: `ip netns exec <pid> ip link show <NIC>`
5. **BGP session**: `podman exec frr vtysh -c "show bgp summary"`
6. **EVPN type 2 routes**: `podman exec frr vtysh -c "show bgp l2vpn evpn route type 2"`
7. **EVPN type 5 routes**: `podman exec frr vtysh -c "show bgp l2vpn evpn route type 5"`
8. **L2VPN veth to br0**: `bridge link show | grep veth-br210-host`
9. **VNI status**: `podman exec frr vtysh -c "show evpn vni"`

### Validation Script (Optional)
Can create a simple bash validation script:
```bash
#!/bin/bash
# systemdmode/validate-vpn-setup.sh
echo "Checking VPN setup..."
podman exec frr vtysh -c "show bgp summary" | grep Established
podman exec frr vtysh -c "show evpn vni" | grep -E "VNI 100|VNI 210"
```

### Reference
- Specification: Validation Method section
- Success Criteria SC-001 through SC-008
- Quickstart guide: Manual validation commands

## Summary

All technical unknowns resolved. The implementation approach:
1. Uses OpenPERouter's static configuration format (`/var/lib/openperouter/configs/openpe_*.yaml`)
2. Template-based configuration with node-specific value substitution (VTEP IP, NIC names)
3. Leverages existing systemdmode/common.sh utilities for FRR container interaction
4. Uses environment variables for configuration flexibility with hardcoded defaults
5. Implements fail-fast error handling appropriate for dev/test environment
6. Integrates cleanly with systemd service management
7. Manual/bash-based validation (no automated e2e tests)

**Next Phase**: Phase 1 - Design & Contracts (data model, interface contracts, quickstart guide)
