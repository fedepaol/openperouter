#!/bin/bash
set -euo pipefail

# generate-config.sh - Generate OpenPERouter static configuration
#
# This script:
# 1. Loads variables from setup-underlay.sh
# 2. Renders configuration template with node-specific values
# 3. Writes YAML config (including rawfrrconfigs) for controller
#
# Usage: Executed by systemd service generate-config.service
#
# Exit codes:
#   0   - Success
#   1   - General error

# Load environment variables with defaults
TOR_IP="${TOR_IP:-10.1.1.254}"
TOR_AS="${TOR_AS:-65000}"
LOCAL_AS="${LOCAL_AS:-64514}"
L2_VNI="${L2_VNI:-210}"
L3_VNI="${L3_VNI:-100}"
VRF_NAME="${VRF_NAME:-red}"
VXLAN_PORT="${VXLAN_PORT:-4789}"
L2_GATEWAY_IP="${L2_GATEWAY_IP:-192.168.110.1/24}"

# Paths
VARS_FILE="${VARS_FILE:-/var/lib/openperouter/vpn-setup.vars}"
CONFIG_TEMPLATE="${CONFIG_TEMPLATE:-/etc/openperouter/templates/openpe_evpn.yaml.template}"
CONFIG_OUTPUT="${CONFIG_OUTPUT:-/var/lib/openperouter/configs/openpe_evpn.yaml}"

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
    log "Configuration generation completed successfully"
    exit 0
}

exit_error() {
    local msg="$1"
    error "$msg"
    exit 1
}

# Start main execution
log "Starting configuration generation"
log "Configuration: TOR_IP=$TOR_IP, TOR_AS=$TOR_AS, LOCAL_AS=$LOCAL_AS"

#
# STEP 1: Load variables from setup-underlay.sh
#
log_step "Loading variables from underlay setup"

if [[ ! -f "$VARS_FILE" ]]; then
    error "Variables file not found: $VARS_FILE"
    error "setup-underlay.service must run first"
    exit_error "Missing variables file"
fi

source "$VARS_FILE"

log "Loaded variables from $VARS_FILE"
log "  VTEP_IP=$VTEP_IP"
log "  NODE_NAME=$NODE_NAME"
log "  UNDERLAY_NIC=$UNDERLAY_NIC"

#
# STEP 2: Verify template exists
#
log_step "Checking configuration template"

if [[ ! -f "$CONFIG_TEMPLATE" ]]; then
    error "Configuration template not found: $CONFIG_TEMPLATE"
    error "Template should be at: /etc/openperouter/templates/openpe_evpn.yaml.template"
    exit_error "Missing configuration template"
fi

log "Using template: $CONFIG_TEMPLATE"

#
# STEP 3: Render configuration from template
#
log_step "Rendering configuration from template"

# Create output directory if needed
mkdir -p "$(dirname "$CONFIG_OUTPUT")"

# Render template using sed substitution
sed -e "s|{{NODE_NAME}}|${NODE_NAME}|g" \
    -e "s|{{VTEP_IP}}|${VTEP_IP}|g" \
    -e "s|{{UNDERLAY_NIC}}|${UNDERLAY_NIC}|g" \
    -e "s|{{TOR_IP}}|${TOR_IP}|g" \
    -e "s|{{TOR_AS}}|${TOR_AS}|g" \
    -e "s|{{LOCAL_AS}}|${LOCAL_AS}|g" \
    -e "s|{{VRF_NAME}}|${VRF_NAME}|g" \
    -e "s|{{L3_VNI}}|${L3_VNI}|g" \
    -e "s|{{L2_VNI}}|${L2_VNI}|g" \
    -e "s|{{VXLAN_PORT}}|${VXLAN_PORT}|g" \
    -e "s|{{L2_GATEWAY_IP}}|${L2_GATEWAY_IP}|g" \
    "$CONFIG_TEMPLATE" > "$CONFIG_OUTPUT" || {
    error "Failed to render configuration template"
    exit_error "Template rendering failed"
}

log "Configuration written to: $CONFIG_OUTPUT"

#
# STEP 4: Validate generated configuration
#
log_step "Validating generated configuration"

# Basic validation - check for key sections
for section in "underlays:" "l3vnis:" "l2vnis:" "rawfrrconfigs:"; do
    if ! grep -q "$section" "$CONFIG_OUTPUT"; then
        error "Generated config is missing required section: $section"
        exit_error "Invalid generated configuration"
    fi
done

# Check VTEP IP is present
if ! grep -q "vtepcidr: ${VTEP_IP}" "$CONFIG_OUTPUT"; then
    error "Generated config is missing VTEP IP configuration"
    exit_error "Invalid VTEP IP in configuration"
fi

log "Configuration validated successfully"

# Show preview
log "Configuration preview (first 20 lines):"
head -20 "$CONFIG_OUTPUT" | while read line; do log "  $line"; done
log "  ..."

log "Controller will read this configuration and:"
log "  1. Create network infrastructure (VRFs, bridges, VXLAN, veths)"
log "  2. Apply rawfrrconfigs to FRR daemon"
log "  3. Establish BGP session with TOR switch"

exit_success
