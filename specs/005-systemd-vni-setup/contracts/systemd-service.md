# systemd Service Unit Contract

**Feature**: 005-systemd-vni-setup  
**Service**: vpn-setup.service  
**Version**: 1.0  
**Date**: 2026-04-01

## Overview

This contract defines the systemd service unit interface for the VPN setup script, including service dependencies, execution model, and integration points.

## Service Unit Specification

### File Location
- **Path**: `/etc/systemd/system/vpn-setup.service` (system-level)
- **Alternative**: `~/.config/systemd/user/vpn-setup.service` (user-level, if applicable)
- **Development**: `systemdmode/quadlets/vpn-setup.service` (in repository)

### Unit File Format

```ini
[Unit]
Description=OpenPERouter VPN Setup (L2+L3 EVPN)
Documentation=https://github.com/openperouter/openperouter/blob/main/systemdmode/README.md
After=network-online.target routerpod.service frr.service
Requires=routerpod.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
User=root
Group=root

# Environment configuration
EnvironmentFile=-/etc/openperouter/vpn-setup.env

# Script execution
ExecStart=/usr/local/bin/setup-vpn.sh

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=vpn-setup

# Timeouts
TimeoutStartSec=180
TimeoutStopSec=10

# Restart policy (optional, for development)
# Restart=on-failure
# RestartSec=10

[Install]
WantedBy=multi-user.target
```

## Service Properties

### Service Type: oneshot

**Characteristics**:
- Runs exactly once during activation
- systemd waits for script completion before proceeding
- `RemainAfterExit=yes` keeps service "active" after script exits
- No process remains running after execution

**Rationale**: VPN setup is a one-time configuration task, not a persistent daemon.

### Dependencies

#### After Dependencies
Service starts **after** these units have started:

| Unit | Reason |
|------|--------|
| `network-online.target` | Ensures network stack is ready |
| `routerpod.service` | Ensures FRR container pod is started |
| `frr.service` | Ensures FRR container is running |

#### Requires Dependencies
Service **cannot start** without:

| Unit | Reason |
|------|--------|
| `routerpod.service` | FRR container must exist |

#### Wants Dependencies
Service **prefers** but doesn't require:

| Unit | Reason |
|------|--------|
| `network-online.target` | Best effort network availability |

### Execution Environment

**User/Group**: `root`
- Required for: network namespace operations, NIC movement, bridge attachment

**Working Directory**: Not specified (inherits from systemd default)

**Environment File**: `/etc/openperouter/vpn-setup.env`
- Optional (`-` prefix means non-fatal if missing)
- Provides environment variable overrides
- Format: `KEY=VALUE` (one per line)

## Timeouts

### Start Timeout
**Value**: `180` seconds (3 minutes)

**Rationale**: 
- FRR readiness wait: up to 60s
- Underlay setup: up to 30s
- VPN configuration: up to 60s
- Buffer: 30s

**Behavior**: If script doesn't exit within 180s, systemd sends SIGTERM (then SIGKILL after grace period).

### Stop Timeout
**Value**: `10` seconds

**Behavior**: Service should stop immediately (no cleanup needed). Timeout is for abnormal conditions only.

## Logging

### Output Streams

| Stream | Destination | systemd Property |
|--------|-------------|------------------|
| stdout | systemd journal | `StandardOutput=journal` |
| stderr | systemd journal | `StandardError=journal` |

### Syslog Identifier
**Value**: `vpn-setup`

**Usage**: Filter logs with `journalctl -t vpn-setup`

### Log Levels
- Informational messages → INFO level (from stdout)
- Error messages → ERR level (from stderr)

## Service Lifecycle

### Activation

```bash
# Enable at boot
systemctl enable vpn-setup.service

# Start immediately (manual)
systemctl start vpn-setup.service

# Both
systemctl enable --now vpn-setup.service
```

### Status Checking

```bash
# Service status
systemctl status vpn-setup.service

# Recent logs
journalctl -u vpn-setup.service --since "10 minutes ago"

# Live log follow
journalctl -u vpn-setup.service -f
```

### Deactivation

```bash
# Disable at boot
systemctl disable vpn-setup.service

# Stop service (doesn't undo network configuration)
systemctl stop vpn-setup.service
```

## Service States

### Active States

| State | Condition | Description |
|-------|-----------|-------------|
| `active (exited)` | Success | Script completed successfully (exit code 0), configuration applied |
| `failed` | Failure | Script exited with non-zero code |
| `activating` | In Progress | Script is currently executing |
| `inactive` | Before Run | Service not yet started |

### Failure Detection

Service enters `failed` state when:
- Script exits with non-zero code
- Timeout (TimeoutStartSec) exceeded
- ExecStart command not found
- Environment file parsing error (if present and invalid)

## Integration Points

### Configuration File

**Path**: `/etc/openperouter/vpn-setup.env`

**Format**:
```bash
# VPN Setup Configuration
# This file is sourced as environment variables

# Underlay Configuration
UNDERLAY_NIC=eth1
TOR_IP=10.1.1.254
TOR_AS=65000
LOCAL_AS=64514

# Timeouts
FRR_READY_TIMEOUT=60

# VPN Parameters (usually defaults are fine)
#L2_VNI=210
#L3_VNI=100
#VRF_NAME=red
```

**Permissions**: `644` (world-readable, systemd reads as root)

### Dependencies on Other Services

#### routerpod.service
- **Type**: podman quadlet pod
- **Location**: `systemdmode/quadlets/routerpod.pod`
- **Provides**: FRR container runtime environment

#### frr.service
- **Type**: podman quadlet container
- **Location**: `systemdmode/quadlets/frr.container`
- **Provides**: FRR routing daemon (bgpd, zebra)

## Testing Integration

### Manual Test

```bash
# Install service unit
sudo cp systemdmode/quadlets/vpn-setup.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Start service
sudo systemctl start vpn-setup.service

# Check status
systemctl status vpn-setup.service

# Check logs
journalctl -u vpn-setup.service --no-pager
```

### Validation Commands

```bash
# Verify static config was generated
cat /var/lib/openperouter/configs/openpe_evpn.yaml

# Check service is active
systemctl is-active vpn-setup.service

# Verify BGP session
podman exec frr vtysh -c "show bgp summary"

# Verify EVPN routes
podman exec frr vtysh -c "show evpn vni"
```

## Error Handling

### Common Failure Scenarios

| Scenario | systemd State | Log Message | Recovery |
|----------|---------------|-------------|----------|
| FRR not running | failed | ERROR: FRR container is not running | Start routerpod.service first |
| Timeout waiting for FRR | failed | ERROR: FRR not ready after 60s | Check FRR container logs |
| br0 missing | failed | ERROR: br0 bridge does not exist | Create br0 before running service |
| Host NIC missing | failed | ERROR: Host NIC eth1 not found | Check UNDERLAY_NIC configuration |

### Debugging

```bash
# Verbose service logs
journalctl -u vpn-setup.service -p debug --no-pager

# Check service dependencies
systemctl list-dependencies vpn-setup.service

# Manual execution (bypass systemd)
sudo /usr/local/bin/setup-vpn.sh
```

## Restart Policy (Optional)

For development/testing environments, automatic restart on failure:

```ini
[Service]
Restart=on-failure
RestartSec=10
StartLimitBurst=3
StartLimitIntervalSec=60
```

**Behavior**:
- Retry up to 3 times within 60 seconds
- Wait 10 seconds between attempts
- If all retries fail, service remains in `failed` state

**Production Recommendation**: Do not auto-restart (partial configurations may cause repeated failures).

## Version Compatibility

### systemd Versions
- **Minimum**: 240 (quadlet support)
- **Tested**: 250+
- **Features Used**: 
  - `Type=oneshot`
  - `RemainAfterExit=yes`
  - `EnvironmentFile` with `-` prefix
  - `After=` / `Requires=` / `Wants=`

### Podman Integration
- Requires podman quadlet for routerpod.service and frr.service
- Service unit depends on podman-generated service units

## Installation

### Deployment Steps

1. **Copy script**:
   ```bash
   sudo cp systemdmode/setup-vpn.sh /usr/local/bin/
   sudo chmod +x /usr/local/bin/setup-vpn.sh
   ```

2. **Copy service unit**:
   ```bash
   sudo cp systemdmode/quadlets/vpn-setup.service /etc/systemd/system/
   ```

3. **Create environment file** (optional):
   ```bash
   sudo mkdir -p /etc/openperouter
   sudo cp vpn-setup.env.example /etc/openperouter/vpn-setup.env
   sudo chmod 644 /etc/openperouter/vpn-setup.env
   ```

4. **Reload systemd**:
   ```bash
   sudo systemctl daemon-reload
   ```

5. **Enable service**:
   ```bash
   sudo systemctl enable vpn-setup.service
   ```

6. **Start service** (or reboot):
   ```bash
   sudo systemctl start vpn-setup.service
   ```

## Uninstallation

```bash
# Stop and disable service
sudo systemctl stop vpn-setup.service
sudo systemctl disable vpn-setup.service

# Remove service unit
sudo rm /etc/systemd/system/vpn-setup.service

# Remove script
sudo rm /usr/local/bin/setup-vpn.sh

# Remove environment file
sudo rm /etc/openperouter/vpn-setup.env

# Reload systemd
sudo systemctl daemon-reload
```

**Note**: This does not undo network configuration changes. FRR container restart or system reboot will clear the configuration.
