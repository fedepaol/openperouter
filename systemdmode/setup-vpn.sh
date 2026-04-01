#!/bin/bash
set -euo pipefail

# setup-vpn.sh - Automated VPN setup script for OpenPERouter systemd mode
#
# This script configures L2 and L3 VPNs automatically on system boot.
# It generates static configuration from templates and waits for FRR readiness.
#
# Usage: Executed by systemd service vpn-setup.service
#
# Exit codes:
#   0   - Success
#   1   - General error
#   124 - Timeout waiting for FRR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# T008: Source common utilities
# T020: Enhanced error messages with specific guidance
if [[ ! -f "$SCRIPT_DIR/common.sh" ]]; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: common.sh not found at $SCRIPT_DIR/common.sh" >&2
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: Please ensure systemdmode/common.sh exists in the repository" >&2
    exit 1
fi

source "$SCRIPT_DIR/common.sh"

# Verify required functions are available
if ! declare -f frr_netns_pid >/dev/null 2>&1; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: Required function frr_netns_pid not found in common.sh" >&2
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: common.sh may be outdated or corrupted" >&2
    exit 1
fi

if ! declare -f inns >/dev/null 2>&1; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: Required function inns not found in common.sh" >&2
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: common.sh may be outdated or corrupted" >&2
    exit 1
fi

if ! declare -f isfrr_ready >/dev/null 2>&1; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: Required function isfrr_ready not found in common.sh" >&2
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: common.sh may be outdated or corrupted" >&2
    exit 1
fi

# T009: Load environment variables with defaults
UNDERLAY_NIC="${UNDERLAY_NIC:-eth1}"
TOR_IP="${TOR_IP:-10.1.1.254}"
TOR_AS="${TOR_AS:-65000}"
LOCAL_AS="${LOCAL_AS:-64514}"
FRR_READY_TIMEOUT="${FRR_READY_TIMEOUT:-60}"
L2_VNI="${L2_VNI:-210}"
L3_VNI="${L3_VNI:-100}"
VRF_NAME="${VRF_NAME:-red}"
VXLAN_PORT="${VXLAN_PORT:-4789}"
L2_GATEWAY_IP="${L2_GATEWAY_IP:-192.168.110.1/24}"

# Paths
CONFIG_TEMPLATE="${CONFIG_TEMPLATE:-/etc/openperouter/templates/openpe_evpn.yaml.template}"
CONFIG_OUTPUT="${CONFIG_OUTPUT:-/var/lib/openperouter/configs/openpe_evpn.yaml}"

# T017 & T018: Enhanced logging functions with timestamps
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# T019: Log progress for major steps
log_step() {
    local step="$1"
    log "Step: $step"
}

# T016: Exit code handling function
exit_success() {
    log "VPN setup completed successfully"
    exit 0
}

exit_error() {
    local msg="$1"
    error "$msg"
    exit 1
}

exit_timeout() {
    error "Operation timed out"
    exit 124
}

# Start main execution
log "Starting VPN setup"
log "Configuration: UNDERLAY_NIC=$UNDERLAY_NIC, TOR_IP=$TOR_IP, TOR_AS=$TOR_AS, LOCAL_AS=$LOCAL_AS"

# T010: Wait for FRR container to be ready
# T019: Enhanced progress logging
log_step "Waiting for FRR container to be ready"
log "Timeout configured: ${FRR_READY_TIMEOUT}s"
ELAPSED=0
INTERVAL=2

while ! isfrr_ready 2>/dev/null; do
    if [ $ELAPSED -ge $FRR_READY_TIMEOUT ]; then
        # T020 & T021: Specific error message for timeout
        error "FRR not ready after ${FRR_READY_TIMEOUT}s timeout"
        error "Check FRR container status: podman ps | grep frr"
        error "Check FRR logs: podman logs frr"
        error "Check systemd service: systemctl status routerpod-pod.service"
        exit_timeout
    fi
    if [ $((ELAPSED % 10)) -eq 0 ] && [ $ELAPSED -gt 0 ]; then
        log "Still waiting for FRR... (${ELAPSED}s elapsed)"
    fi
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done

log "FRR container is ready (bgpd operational)"

# T011: Derive VTEP IP from br0's last octet
# T019: Enhanced progress logging
log_step "Deriving VTEP IP from br0"

# T021: Edge case - br0 does not exist
if ! ip link show br0 >/dev/null 2>&1; then
    error "br0 bridge does not exist"
    error "The br0 bridge must be created and configured before running VPN setup"
    error "Create br0 with: ip link add br0 type bridge"
    error "Assign IP with: ip addr add <ip>/<cidr> dev br0"
    error "Bring up with: ip link set br0 up"
    exit_error "Missing prerequisite: br0 bridge"
fi

# T021: Edge case - br0 has no IP address
BR0_IP=$(ip -4 addr show br0 | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || true)

if [[ -z "$BR0_IP" ]]; then
    error "br0 bridge does not have an IP address assigned"
    error "Assign an IP to br0 with: ip addr add <ip>/<cidr> dev br0"
    error "Example: ip addr add 192.168.1.5/24 dev br0"
    exit_error "br0 must have an IP address configured"
fi

# Extract last octet
LAST_OCTET=$(echo "$BR0_IP" | cut -d. -f4)
VTEP_IP="10.0.0.${LAST_OCTET}"

log "br0 IP address: $BR0_IP"
log "Derived VTEP IP: $VTEP_IP (last octet: $LAST_OCTET)"

# T012 & T013: Render YAML template and write static config
# T019: Enhanced progress logging
log_step "Generating static configuration from template"

# T021: Edge case - template file missing
if [[ ! -f "$CONFIG_TEMPLATE" ]]; then
    error "Template file not found: $CONFIG_TEMPLATE"
    error "Template should be deployed to: /etc/openperouter/templates/openpe_evpn.yaml.template"
    error "Run deployment script: systemdmode/deploy-vpn-setup.sh <cluster-name>"
    exit_error "Missing template file"
fi

log "Using template: $CONFIG_TEMPLATE"
log "Substituting values:"
log "  VTEP_IP=$VTEP_IP"
log "  UNDERLAY_NIC=$UNDERLAY_NIC"
log "  TOR_IP=$TOR_IP, TOR_AS=$TOR_AS"
log "  LOCAL_AS=$LOCAL_AS"
log "  VRF_NAME=$VRF_NAME"
log "  L2_VNI=$L2_VNI, L3_VNI=$L3_VNI"

# Create output directory if needed
OUTPUT_DIR=$(dirname "$CONFIG_OUTPUT")
mkdir -p "$OUTPUT_DIR" 2>/dev/null || {
    error "Failed to create output directory: $OUTPUT_DIR"
    error "Check permissions or ensure parent directory exists"
    exit_error "Cannot create config directory"
}

# Render template using sed substitution
sed -e "s|{{VTEP_IP}}|${VTEP_IP}|g" \
    -e "s|{{UNDERLAY_NIC}}|${UNDERLAY_NIC}|g" \
    -e "s|{{TOR_IP}}|${TOR_IP}|g" \
    -e "s|{{TOR_AS}}|${TOR_AS}|g" \
    -e "s|{{LOCAL_AS}}|${LOCAL_AS}|g" \
    -e "s|{{VRF_NAME}}|${VRF_NAME}|g" \
    -e "s|{{L2_VNI}}|${L2_VNI}|g" \
    -e "s|{{L3_VNI}}|${L3_VNI}|g" \
    -e "s|{{VXLAN_PORT}}|${VXLAN_PORT}|g" \
    -e "s|{{L2_GATEWAY_IP}}|${L2_GATEWAY_IP}|g" \
    "$CONFIG_TEMPLATE" > "$CONFIG_OUTPUT" 2>/dev/null || {
    error "Failed to render template to: $CONFIG_OUTPUT"
    error "Check template syntax or write permissions"
    exit_error "Template rendering failed"
}

log "Static configuration written to: $CONFIG_OUTPUT"
log "Configuration will be picked up by the controller"

# T014: Move host NIC to FRR namespace
# T019: Enhanced progress logging
log_step "Moving host NIC to FRR namespace"

# T021: Edge case - NIC does not exist
if ! ip link show "$UNDERLAY_NIC" >/dev/null 2>&1; then
    error "Host NIC $UNDERLAY_NIC not found"
    error "Available NICs:"
    ip -br link show | head -10 | while read line; do
        error "  $line"
    done
    error "Set UNDERLAY_NIC environment variable to the correct NIC name"
    exit_error "Host NIC $UNDERLAY_NIC not found"
fi

log "Found host NIC: $UNDERLAY_NIC"

# T021: Edge case - NIC has no IP address (warning only, as per clarifications)
NIC_IP=$(ip -4 addr show "$UNDERLAY_NIC" | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || true)
if [[ -z "$NIC_IP" ]]; then
    error "WARNING: Host NIC $UNDERLAY_NIC does not have an IP address configured"
    error "WARNING: The NIC should have an IP address for underlay connectivity"
    error "WARNING: Continuing anyway as IP may be configured later..."
else
    log "Host NIC IP address: $NIC_IP (will be preserved when moved)"
fi

# Get FRR namespace PID
FRR_PID=$(frr_netns_pid)
if [[ -z "$FRR_PID" || "$FRR_PID" == "0" ]]; then
    error "Failed to get FRR container PID"
    error "FRR container may not be running properly"
    error "Check: podman inspect frr --format '{{.State.Pid}}'"
    exit_error "Cannot determine FRR namespace"
fi

log "FRR container PID: $FRR_PID"

# Move NIC to FRR namespace
log "Moving $UNDERLAY_NIC to FRR namespace (PID: $FRR_PID)..."
ip link set "$UNDERLAY_NIC" netns "$FRR_PID" 2>/dev/null || {
    error "WARNING: Failed to move $UNDERLAY_NIC to namespace (may already be there or in use)"
    error "WARNING: Continuing anyway..."
}

# Bring up NIC in FRR namespace
log "Bringing up $UNDERLAY_NIC in FRR namespace..."
inns ip link set "$UNDERLAY_NIC" up 2>/dev/null || {
    error "WARNING: Failed to bring up $UNDERLAY_NIC in FRR namespace"
    error "WARNING: Continuing anyway..."
}

log "Host NIC $UNDERLAY_NIC configured in FRR namespace"

# T015: Validation logic (basic check - controller will apply config)
# T019: Enhanced progress logging
log_step "Validating configuration"

# Give controller a few seconds to pick up the static config
log "Waiting for controller to detect static configuration (5s)..."
sleep 5

# Check if config file exists and is readable
if [[ ! -f "$CONFIG_OUTPUT" ]]; then
    error "Configuration file was not created: $CONFIG_OUTPUT"
    exit_error "Configuration validation failed"
fi

# Check if config file is not empty
if [[ ! -s "$CONFIG_OUTPUT" ]]; then
    error "Configuration file is empty: $CONFIG_OUTPUT"
    exit_error "Configuration validation failed"
fi

# Verify config has expected content
if ! grep -q "underlays:" "$CONFIG_OUTPUT" || \
   ! grep -q "l3vnis:" "$CONFIG_OUTPUT" || \
   ! grep -q "l2vnis:" "$CONFIG_OUTPUT"; then
    error "Configuration file missing expected sections"
    error "Check template file: $CONFIG_TEMPLATE"
    exit_error "Configuration validation failed"
fi

log "Configuration file validated: $CONFIG_OUTPUT"
log ""
log "Summary of configuration:"
log "  - Underlay: NIC=$UNDERLAY_NIC, TOR=$TOR_IP:$TOR_AS"
log "  - L3VPN: VNI=$L3_VNI, VRF=$VRF_NAME"
log "  - L2VPN: VNI=$L2_VNI, VRF=$VRF_NAME, Gateway=$L2_GATEWAY_IP"
log "  - VTEP IP: $VTEP_IP"
log ""
log "Next steps:"
log "  1. Controller will read config from: $CONFIG_OUTPUT"
log "  2. FRR will be configured with BGP and EVPN settings"
log "  3. BGP session will establish with TOR switch"
log "  4. EVPN routes (type 2 and 5) will be advertised"
log ""
log "To verify BGP session:"
log "  podman exec frr vtysh -c 'show bgp summary'"
log "To verify EVPN routes:"
log "  podman exec frr vtysh -c 'show bgp l2vpn evpn route'"
log "To verify VNI status:"
log "  podman exec frr vtysh -c 'show evpn vni'"

# Note: Full BGP and EVPN validation would require waiting for controller
# to apply the config and FRR to establish sessions. For now, we just
# verify the config was generated successfully.

# T016: Exit with success
exit_success
