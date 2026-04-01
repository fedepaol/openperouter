# Script Interface Contract

**Feature**: 005-systemd-vni-setup  
**Interface**: setup-vpn.sh  
**Version**: 1.0  
**Date**: 2026-04-01

## Overview

This contract defines the external interface for the `setup-vpn.sh` bash script, including execution requirements, exit codes, logging format, and environment variable interface.

## Execution

### Location
- **Installation Path**: `/usr/local/bin/setup-vpn.sh` (or systemdmode/setup-vpn.sh in development)
- **Permissions**: Executable by root (`chmod +x`)
- **Interpreter**: `#!/bin/bash`

### Invocation
```bash
# Direct execution (requires root/CAP_NET_ADMIN)
/usr/local/bin/setup-vpn.sh

# Via systemd service
systemctl start vpn-setup.service
```

### Prerequisites
- FRR container running (podman container named `frr`)
- br0 bridge exists with IPv4 address
- Host NIC (default: eth1) exists with IPv4 address
- Root privileges or CAP_NET_ADMIN capability
- iproute2 tools installed (ip, bridge)
- nsenter available
- systemdmode/common.sh sourced or in PATH

## Environment Variables

### Input Configuration

All environment variables are **optional** with hardcoded defaults.

| Variable | Type | Default | Description | Example |
|----------|------|---------|-------------|---------|
| `UNDERLAY_NIC` | string | `eth1` | Physical NIC name for underlay | `UNDERLAY_NIC=ens3` |
| `TOR_IP` | IPv4 | TBD | TOR switch BGP peer IP | `TOR_IP=10.1.1.254` |
| `TOR_AS` | integer | TBD | TOR switch AS number | `TOR_AS=65000` |
| `LOCAL_AS` | integer | `64514` | Local BGP AS number | `LOCAL_AS=64514` |
| `FRR_READY_TIMEOUT` | integer | `60` | FRR readiness timeout (seconds) | `FRR_READY_TIMEOUT=120` |
| `L2_VNI` | integer | `210` | L2VPN VXLAN VNI | `L2_VNI=210` |
| `L3_VNI` | integer | `100` | L3VPN VXLAN VNI | `L3_VNI=100` |
| `VRF_NAME` | string | `red` | VRF name for VPNs | `VRF_NAME=red` |
| `VXLAN_PORT` | integer | `4789` | VXLAN UDP port | `VXLAN_PORT=4789` |
| `L2_GATEWAY_IP` | IPv4 CIDR | `192.168.110.1/24` | L2VPN gateway IP | `L2_GATEWAY_IP=192.168.110.1/24` |

### systemd EnvironmentFile Format

```bash
# /etc/openperouter/vpn-setup.env
UNDERLAY_NIC=eth1
TOR_IP=10.1.1.254
TOR_AS=65000
LOCAL_AS=64514
FRR_READY_TIMEOUT=60
```

## Exit Codes

The script uses standard Unix exit codes:

| Exit Code | Meaning | Example Scenario |
|-----------|---------|------------------|
| `0` | Success | All VPN configuration completed successfully |
| `1` | General error | Any configuration step failed |
| `2` | Configuration error | Environment variable validation failed |
| `124` | Timeout | FRR not ready within timeout period |

**Error Handling**: Script exits on first error (`set -e`). No rollback is performed.

## Output Format

### Standard Output (stdout)

Informational messages in timestamped format:

```
[YYYY-MM-DD HH:MM:SS] <message>
```

**Examples**:
```
[2026-04-01 10:15:32] Starting VPN setup
[2026-04-01 10:15:33] Waiting for FRR container to be ready...
[2026-04-01 10:15:35] FRR is ready (bgpd running)
[2026-04-01 10:15:36] VTEP IP derived: 10.0.0.5/24 (from br0: 192.168.1.5)
[2026-04-01 10:15:37] Moving NIC eth1 to FRR namespace (PID: 12345)
[2026-04-01 10:15:38] Configuring BGP session with TOR 10.1.1.254 (AS 65000)
[2026-04-01 10:15:40] Configuring L3VPN (VNI: 100, VRF: red)
[2026-04-01 10:15:42] Configuring L2VPN (VNI: 210, VRF: red)
[2026-04-01 10:15:44] Enslaving veth-br210-host to br0
[2026-04-01 10:15:45] Validating BGP session: Established
[2026-04-01 10:15:46] Validating EVPN routes: Type 2 (3 routes), Type 5 (2 routes)
[2026-04-01 10:15:47] VPN setup completed successfully
```

### Standard Error (stderr)

Error messages in timestamped format with ERROR prefix:

```
[YYYY-MM-DD HH:MM:SS] ERROR: <error description>
```

**Examples**:
```
[2026-04-01 10:15:35] ERROR: FRR container is not running
[2026-04-01 10:15:36] ERROR: br0 bridge does not exist
[2026-04-01 10:15:37] ERROR: br0 bridge does not have an IP address
[2026-04-01 10:15:38] ERROR: Host NIC eth1 not found
[2026-04-01 10:15:39] ERROR: FRR not ready after 60s timeout
[2026-04-01 10:15:40] ERROR: BGP session failed to establish
```

### Logging Destination

When run via systemd service:
- stdout → systemd journal (info level)
- stderr → systemd journal (error level)

View logs:
```bash
# All output
journalctl -u vpn-setup.service

# Errors only
journalctl -u vpn-setup.service -p err

# Follow during execution
journalctl -u vpn-setup.service -f
```

## Side Effects

### Network Configuration Changes

The script makes irreversible changes to the network configuration:

1. **Host NIC Movement**: Physical NIC moved from host namespace to FRR namespace
2. **BGP Session**: Established with TOR switch
3. **VXLAN Interfaces**: Created in FRR namespace
4. **VRF**: Created in FRR namespace
5. **Bridges**: Created in FRR namespace
6. **Veth Pair**: Created and attached to br0
7. **FRR Configuration**: BGP and EVPN configuration applied

**Persistence**: Changes persist until:
- FRR container is restarted (namespace configuration lost)
- System is rebooted (requires script re-run)

**Idempotency**: Script is **NOT idempotent**. Running multiple times may cause errors or duplicate configurations.

## Dependencies

### Required Commands

The script will fail if these commands are not available:

- `ip` (iproute2)
- `bridge` (iproute2)
- `nsenter` (util-linux)
- `podman` (container runtime)
- `vtysh` (FRR CLI, via podman exec)
- Standard bash builtins: `source`, `echo`, `sleep`, `grep`, `cut`

### Required Files

- `systemdmode/common.sh`: Must be sourceable (provides `frr_netns_pid`, `inns`, `isfrr_ready`)

### Required System State

- FRR container `frr` running under podman
- BGP daemon `bgpd` active in FRR container
- Bridge `br0` exists with IPv4 address
- Host NIC (configured via `UNDERLAY_NIC`) exists with IPv4 address

## Behavioral Guarantees

### Success Guarantees

When script exits with code 0:
- FRR container is running and bgpd is active
- Host NIC is in FRR namespace with link up
- BGP session with TOR is in "Established" state
- L3VPN (VNI 100, VRF red) is configured
- L2VPN (VNI 210, VRF red) is configured
- L2VPN veth is enslaved to br0
- VTEP IP is configured as 10.0.0.X/24 (X = br0's last octet)
- EVPN type 2 routes are being advertised
- EVPN type 5 routes are being advertised

### Failure Guarantees

When script exits with non-zero code:
- Partial configuration may exist (no rollback)
- Error message logged to stderr indicates failure point
- systemd journal contains detailed execution log
- System is in intermediate state (manual cleanup may be required)

### Timeout Behavior

FRR readiness wait respects `FRR_READY_TIMEOUT`:
- Polls every 2 seconds
- Exits with code 124 if timeout exceeded
- Logs progress messages during wait

## Version Compatibility

### FRR Version
- Minimum: FRR 8.0 (EVPN support)
- Tested: FRR 8.4+

### Kernel Version
- Minimum: Linux 4.0 (VXLAN support)
- Tested: Linux 5.10+

### Systemd Version
- Minimum: systemd 240 (podman quadlet support)
- Tested: systemd 250+

## Testing Interface

### Validation Commands

After successful execution, verify with:

```bash
# Check BGP session status
podman exec frr vtysh -c "show bgp summary"

# Check EVPN type 2 routes
podman exec frr vtysh -c "show bgp l2vpn evpn route type 2"

# Check EVPN type 5 routes
podman exec frr vtysh -c "show bgp l2vpn evpn route type 5"

# Check L2VPN veth attachment to br0
bridge link show | grep veth-br210-host

# Check VTEP IP
ip -n $(podman inspect frr --format '{{.State.Pid}}') addr show vxlan-100
```

### Example Test Execution

```bash
# Set test environment
export UNDERLAY_NIC=eth1
export TOR_IP=10.1.1.254
export TOR_AS=65000
export FRR_READY_TIMEOUT=30

# Run script
/usr/local/bin/setup-vpn.sh

# Verify exit code
echo $?  # Should be 0 on success
```
