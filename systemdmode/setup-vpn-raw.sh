#!/bin/bash
set -euo pipefail

# setup-vpn-raw.sh - VPN setup using raw FRR configuration
#
# This script sets up L2 and L3 VPNs by:
# 1. Creating network infrastructure manually (VRFs, bridges, VXLAN, veths)
# 2. Applying raw FRR configuration directly to FRR daemon
#
# This bypasses the OpenPERouter controller and provides full control.
#
# Usage: Executed by systemd service vpn-setup.service
#
# Exit codes:
#   0   - Success
#   1   - General error
#   124 - Timeout waiting for FRR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common utilities
if [[ ! -f "$SCRIPT_DIR/common.sh" ]]; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: common.sh not found at $SCRIPT_DIR/common.sh" >&2
    exit 1
fi

source "$SCRIPT_DIR/common.sh"

# Verify required functions
for func in frr_netns_pid inns isfrr_ready; do
    if ! declare -f "$func" >/dev/null 2>&1; then
        echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: Required function $func not found in common.sh" >&2
        exit 1
    fi
done

# Load environment variables with defaults
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
NODE_NAME="${NODE_NAME:-$(hostname)}"

# Paths
FRR_CONFIG_TEMPLATE="${FRR_CONFIG_TEMPLATE:-/etc/openperouter/templates/frr-evpn.conf.template}"
FRR_CONFIG_OUTPUT="${FRR_CONFIG_OUTPUT:-/etc/frr/frr.conf}"
NETWORK_SETUP_SCRIPT="${NETWORK_SETUP_SCRIPT:-$SCRIPT_DIR/setup-network.sh}"

# Logging functions
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

log_step() {
    local step="$1"
    log "=== Step: $step ==="
}

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
log "Starting VPN setup (raw FRR configuration mode)"
log "Configuration: UNDERLAY_NIC=$UNDERLAY_NIC, TOR_IP=$TOR_IP, TOR_AS=$TOR_AS, LOCAL_AS=$LOCAL_AS"

#
# STEP 1: Wait for FRR container to be ready
#
log_step "Waiting for FRR container"
log "Timeout configured: ${FRR_READY_TIMEOUT}s"
ELAPSED=0
INTERVAL=2

while ! isfrr_ready 2>/dev/null; do
    if [ $ELAPSED -ge $FRR_READY_TIMEOUT ]; then
        error "FRR not ready after ${FRR_READY_TIMEOUT}s timeout"
        error "Check FRR container: podman ps | grep frr"
        error "Check FRR logs: podman logs frr"
        exit_timeout
    fi
    if [ $((ELAPSED % 10)) -eq 0 ] && [ $ELAPSED -gt 0 ]; then
        log "Still waiting for FRR... (${ELAPSED}s elapsed)"
    fi
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done

log "FRR container is ready (bgpd operational)"

#
# STEP 2: Derive VTEP IP from br0
#
log_step "Deriving VTEP IP from br0"

if ! ip link show br0 >/dev/null 2>&1; then
    error "br0 bridge does not exist"
    error "Create br0 with: ip link add br0 type bridge && ip addr add <ip>/<cidr> dev br0 && ip link set br0 up"
    exit_error "Missing prerequisite: br0 bridge"
fi

BR0_IP=$(ip -4 addr show br0 | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || true)

if [[ -z "$BR0_IP" ]]; then
    error "br0 bridge does not have an IP address assigned"
    error "Assign an IP to br0 with: ip addr add <ip>/<cidr> dev br0"
    exit_error "br0 must have an IP address configured"
fi

# Extract last octet for VTEP IP
LAST_OCTET=$(echo "$BR0_IP" | cut -d. -f4)
VTEP_IP="10.0.0.${LAST_OCTET}"
ROUTER_ID="$VTEP_IP"  # Use VTEP IP as router ID

log "br0 IP address: $BR0_IP"
log "Derived VTEP IP: $VTEP_IP (last octet: $LAST_OCTET)"
log "Router ID: $ROUTER_ID"

#
# STEP 3: Move host NIC to FRR namespace
#
log_step "Moving host NIC to FRR namespace"

if ! ip link show "$UNDERLAY_NIC" >/dev/null 2>&1; then
    error "Host NIC $UNDERLAY_NIC not found"
    error "Available NICs:"
    ip -br link show | head -10 | while read line; do
        error "  $line"
    done
    exit_error "Host NIC $UNDERLAY_NIC not found"
fi

log "Found host NIC: $UNDERLAY_NIC"

NIC_IP=$(ip -4 addr show "$UNDERLAY_NIC" | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || true)
if [[ -z "$NIC_IP" ]]; then
    log "WARNING: Host NIC $UNDERLAY_NIC does not have an IP address configured"
else
    log "Host NIC IP address: $NIC_IP (will be preserved when moved)"
fi

FRR_PID=$(frr_netns_pid)
if [[ -z "$FRR_PID" || "$FRR_PID" == "0" ]]; then
    error "Failed to get FRR container PID"
    exit_error "Cannot determine FRR namespace"
fi

log "FRR container PID: $FRR_PID"

# Move NIC to FRR namespace
log "Moving $UNDERLAY_NIC to FRR namespace (PID: $FRR_PID)..."
ip link set "$UNDERLAY_NIC" netns "$FRR_PID" 2>/dev/null || {
    log "WARNING: Failed to move $UNDERLAY_NIC to namespace (may already be there)"
}

# Bring up NIC in FRR namespace
log "Bringing up $UNDERLAY_NIC in FRR namespace..."
inns ip link set "$UNDERLAY_NIC" up 2>/dev/null || {
    log "WARNING: Failed to bring up $UNDERLAY_NIC in FRR namespace"
}

log "Host NIC $UNDERLAY_NIC configured in FRR namespace"

#
# STEP 4: Set up network infrastructure (VRFs, bridges, VXLAN, veths)
#
log_step "Setting up network infrastructure"

if [[ ! -x "$NETWORK_SETUP_SCRIPT" ]]; then
    error "Network setup script not found or not executable: $NETWORK_SETUP_SCRIPT"
    exit_error "Missing network setup script"
fi

log "Running network setup script: $NETWORK_SETUP_SCRIPT"

# Export variables for setup-network.sh
export VRF_NAME
export L2_VNI
export L3_VNI
export VXLAN_PORT
export L2_GATEWAY_IP
export VTEP_IP

if ! "$NETWORK_SETUP_SCRIPT"; then
    error "Network setup script failed"
    exit_error "Network infrastructure setup failed"
fi

log "Network infrastructure setup completed"

#
# STEP 5: Generate FRR configuration from template
#
log_step "Generating FRR configuration"

if [[ ! -f "$FRR_CONFIG_TEMPLATE" ]]; then
    error "FRR config template not found: $FRR_CONFIG_TEMPLATE"
    error "Template should be at: /etc/openperouter/templates/frr-evpn.conf.template"
    exit_error "Missing FRR config template"
fi

log "Using template: $FRR_CONFIG_TEMPLATE"
log "Output: $FRR_CONFIG_OUTPUT"

TIMESTAMP=$(date +'%Y-%m-%d %H:%M:%S')

# Create temp file in /tmp
TMP_CONFIG="/tmp/frr-evpn-$$.conf"

# Render template using sed substitution
sed -e "s|{{TIMESTAMP}}|${TIMESTAMP}|g" \
    -e "s|{{NODE_NAME}}|${NODE_NAME}|g" \
    -e "s|{{VTEP_IP}}|${VTEP_IP}|g" \
    -e "s|{{ROUTER_ID}}|${ROUTER_ID}|g" \
    -e "s|{{LOCAL_AS}}|${LOCAL_AS}|g" \
    -e "s|{{TOR_IP}}|${TOR_IP}|g" \
    -e "s|{{TOR_AS}}|${TOR_AS}|g" \
    -e "s|{{VRF_NAME}}|${VRF_NAME}|g" \
    -e "s|{{L3_VNI}}|${L3_VNI}|g" \
    -e "s|{{L2_VNI}}|${L2_VNI}|g" \
    -e "s|{{L2_GATEWAY_IP}}|${L2_GATEWAY_IP}|g" \
    "$FRR_CONFIG_TEMPLATE" > "$TMP_CONFIG" || {
    error "Failed to render FRR config template"
    exit_error "Template rendering failed"
}

# Validate generated config (basic syntax check)
if ! grep -q "router bgp ${LOCAL_AS}" "$TMP_CONFIG"; then
    error "Generated config is missing BGP configuration"
    rm -f "$TMP_CONFIG"
    exit_error "Invalid generated configuration"
fi

log "Generated FRR configuration (preview):"
head -20 "$TMP_CONFIG" | while read line; do log "  $line"; done
log "  ..."

# Apply configuration to FRR container
log "Copying configuration to FRR container..."
podman cp "$TMP_CONFIG" "frr:${FRR_CONFIG_OUTPUT}" || {
    error "Failed to copy config to FRR container"
    rm -f "$TMP_CONFIG"
    exit_error "Cannot apply configuration"
}

# Cleanup temp file
rm -f "$TMP_CONFIG"

log "FRR configuration applied to ${FRR_CONFIG_OUTPUT}"

#
# STEP 6: Reload FRR configuration
#
log_step "Reloading FRR configuration"

log "Reloading FRR daemon..."
podman exec frr vtysh -c "configure terminal" -c "do write memory" 2>/dev/null || true
podman exec frr pkill -HUP bgpd 2>/dev/null || {
    log "WARNING: Failed to reload BGP daemon via signal, trying vtysh reload..."
    podman exec frr vtysh -c "clear bgp *" 2>/dev/null || true
}

# Give FRR time to process the configuration
log "Waiting for FRR to process configuration (5s)..."
sleep 5

#
# STEP 7: Validation
#
log_step "Validating configuration"

# Check BGP daemon is running
if ! podman exec frr vtysh -c "show daemons" 2>/dev/null | grep -q "bgpd"; then
    error "BGP daemon is not running"
    exit_error "FRR validation failed"
fi

log "BGP daemon is running"

# Check VRF is configured
if ! podman exec frr vtysh -c "show vrf" 2>/dev/null | grep -q "$VRF_NAME"; then
    log "WARNING: VRF $VRF_NAME not visible in FRR (may take time to sync)"
else
    log "VRF $VRF_NAME is configured"
fi

# Check BGP neighbor
BGP_NEIGHBOR_STATUS=$(podman exec frr vtysh -c "show bgp summary" 2>/dev/null | grep "$TOR_IP" || echo "Not found")
log "BGP neighbor status: $BGP_NEIGHBOR_STATUS"

log ""
log "============================================"
log "VPN Setup Summary"
log "============================================"
log "Underlay:"
log "  NIC: $UNDERLAY_NIC (in FRR namespace)"
log "  TOR: $TOR_IP (AS $TOR_AS)"
log "  Local AS: $LOCAL_AS"
log "  Router ID: $ROUTER_ID"
log ""
log "Overlay:"
log "  VTEP IP: $VTEP_IP"
log "  VRF: $VRF_NAME"
log "  L3VNI: $L3_VNI (bridge: br-pe-${L3_VNI}, vxlan: vni${L3_VNI})"
log "  L2VNI: $L2_VNI (bridge: br-pe-${L2_VNI}, vxlan: vni${L2_VNI})"
log "  L2 Gateway IP: $L2_GATEWAY_IP"
log "  L2 Veth: host-${L2_VNI} (on br0) <-> pe-${L2_VNI} (on br-pe-${L2_VNI})"
log ""
log "============================================"
log "Verification Commands"
log "============================================"
log "  BGP summary:     podman exec frr vtysh -c 'show bgp summary'"
log "  BGP neighbors:   podman exec frr vtysh -c 'show bgp neighbors'"
log "  VRF status:      podman exec frr vtysh -c 'show vrf'"
log "  EVPN VNIs:       podman exec frr vtysh -c 'show evpn vni'"
log "  EVPN routes:     podman exec frr vtysh -c 'show bgp l2vpn evpn route'"
log "  Interfaces:      podman exec frr vtysh -c 'show interface brief'"
log "  Network (host):  ip link show | grep -E 'br0|host-'"
log "  Network (FRR):   podman exec frr ip link show | grep -E 'vni|br-pe|pe-'"
log "============================================"
log ""

exit_success
