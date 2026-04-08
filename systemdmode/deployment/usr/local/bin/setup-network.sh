#!/bin/bash
set -euo pipefail

# setup-network.sh - Network infrastructure setup for L2/L3 VNIs
#
# This script creates the network infrastructure (VRFs, bridges, VXLAN interfaces, veths)
# that would normally be created by the OpenPERouter controller.
# It uses the FRR namespace to set up the network configuration.
#
# Usage: Executed by setup-vpn.sh before applying FRR configuration
#
# Exit codes:
#   0   - Success
#   1   - Error

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source common utilities
if [[ ! -f "$SCRIPT_DIR/common.sh" ]]; then
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: common.sh not found" >&2
    exit 1
fi

source "$SCRIPT_DIR/common.sh"

# Logging functions
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# Load variables from setup-underlay.sh if available
VARS_FILE="${VARS_FILE:-/var/lib/openperouter/vpn-setup.vars}"
if [[ -f "$VARS_FILE" ]]; then
    log "Loading variables from $VARS_FILE"
    source "$VARS_FILE"
else
    log "Variables file not found, using environment variables"
fi

# Parameters (from environment or defaults)
VRF_NAME="${VRF_NAME:-red}"
L2_VNI="${L2_VNI:-210}"
L3_VNI="${L3_VNI:-100}"
VXLAN_PORT="${VXLAN_PORT:-4789}"
L2_GATEWAY_IP="${L2_GATEWAY_IP:-192.168.110.1/24}"
VTEP_IP="${VTEP_IP}"  # Must be provided (from vars file or environment)
VTEP_INTERFACE="${VTEP_INTERFACE:-lo}"  # Loopback for VTEP source

if [[ -z "$VTEP_IP" ]]; then
    error "VTEP_IP not set - must be provided via $VARS_FILE or environment"
    error "Run setup-underlay.service first to generate variables"
    exit 1
fi

# Get FRR namespace PID
FRR_PID=$(frr_netns_pid)
if [[ -z "$FRR_PID" || "$FRR_PID" == "0" ]]; then
    error "Failed to get FRR container PID"
    exit 1
fi

log "Setting up network infrastructure in FRR namespace (PID: $FRR_PID)"
log "Configuration: VRF=$VRF_NAME, L2_VNI=$L2_VNI, L3_VNI=$L3_VNI, VTEP_IP=$VTEP_IP"

# Helper function: run command in FRR namespace
infrr() {
    inns "$@"
}

# Helper function: find free routing table ID
find_free_routing_table() {
    local used_tables=$(infrr ip -j link show type vrf 2>/dev/null | grep -oP '"table":\s*\K\d+' || echo "")
    local table_id=1
    while echo "$used_tables" | grep -q "^${table_id}$"; do
        table_id=$((table_id + 1))
    done
    echo "$table_id"
}

#
# STEP 1: Create VRF in FRR namespace
#
log "Step 1: Creating VRF '$VRF_NAME'"

if infrr ip link show "$VRF_NAME" >/dev/null 2>&1; then
    log "  VRF '$VRF_NAME' already exists"
else
    TABLE_ID=200
    log "  Creating VRF with table ID: $TABLE_ID"
    infrr ip link add "$VRF_NAME" type vrf table "$TABLE_ID" || {
        error "Failed to create VRF $VRF_NAME"
        exit 1
    }
    infrr ip link set "$VRF_NAME" up || {
        error "Failed to bring up VRF $VRF_NAME"
        exit 1
    }
    log "  VRF '$VRF_NAME' created successfully"
fi

#
# STEP 2: Create L3VNI bridge in FRR namespace
#
L3_BRIDGE="br-pe-${L3_VNI}"
log "Step 2: Creating L3VNI bridge '$L3_BRIDGE'"

if infrr ip link show "$L3_BRIDGE" >/dev/null 2>&1; then
    log "  Bridge '$L3_BRIDGE' already exists"
else
    infrr ip link add "$L3_BRIDGE" type bridge || {
        error "Failed to create bridge $L3_BRIDGE"
        exit 1
    }
    # Enslave bridge to VRF
    infrr ip link set "$L3_BRIDGE" master "$VRF_NAME" || {
        error "Failed to enslave bridge to VRF"
        exit 1
    }
    # Disable IPv6 address auto-generation
    infrr sysctl -w "net.ipv6.conf.${L3_BRIDGE}.addr_gen_mode=1" 2>/dev/null || true
    infrr ip link set "$L3_BRIDGE" up || {
        error "Failed to bring up bridge $L3_BRIDGE"
        exit 1
    }
    log "  Bridge '$L3_BRIDGE' created and enslaved to VRF"
fi

#
# STEP 3: Create L3VNI VXLAN interface in FRR namespace
#
L3_VXLAN="vni${L3_VNI}"
log "Step 3: Creating L3VNI VXLAN interface '$L3_VXLAN'"

if infrr ip link show "$L3_VXLAN" >/dev/null 2>&1; then
    log "  VXLAN '$L3_VXLAN' already exists"
else
    infrr ip link add "$L3_VXLAN" type vxlan \
        id "$L3_VNI" \
        local "$VTEP_IP" \
        dstport "$VXLAN_PORT" \
        nolearning || {
        error "Failed to create VXLAN $L3_VXLAN"
        exit 1
    }
    # Enslave VXLAN to bridge
    infrr ip link set "$L3_VXLAN" master "$L3_BRIDGE" || {
        error "Failed to enslave VXLAN to bridge"
        exit 1
    }
    # Disable IPv6 address auto-generation and enable neighbor suppression
    infrr sysctl -w "net.ipv6.conf.${L3_VXLAN}.addr_gen_mode=1" 2>/dev/null || true
    infrr ip link set "$L3_VXLAN" type bridge_slave neigh_suppress on 2>/dev/null || true
    infrr ip link set "$L3_VXLAN" up || {
        error "Failed to bring up VXLAN $L3_VXLAN"
        exit 1
    }
    log "  VXLAN '$L3_VXLAN' created and enslaved to bridge"
fi

#
# STEP 4: Create L2VNI bridge in FRR namespace
#
L2_BRIDGE="br-pe-${L2_VNI}"
log "Step 4: Creating L2VNI bridge '$L2_BRIDGE'"

if infrr ip link show "$L2_BRIDGE" >/dev/null 2>&1; then
    log "  Bridge '$L2_BRIDGE' already exists"
else
    infrr ip link add "$L2_BRIDGE" type bridge || {
        error "Failed to create bridge $L2_BRIDGE"
        exit 1
    }
    # Enslave bridge to VRF
    infrr ip link set "$L2_BRIDGE" master "$VRF_NAME" || {
        error "Failed to enslave bridge to VRF"
        exit 1
    }
    # Disable IPv6 address auto-generation
    infrr sysctl -w "net.ipv6.conf.${L2_BRIDGE}.addr_gen_mode=1" 2>/dev/null || true
    infrr ip link set "$L2_BRIDGE" up || {
        error "Failed to bring up bridge $L2_BRIDGE"
        exit 1
    }
    log "  Bridge '$L2_BRIDGE' created and enslaved to VRF"
fi

#
# STEP 5: Assign L2 gateway IP to bridge
#
log "Step 5: Assigning gateway IP to L2 bridge"

if infrr ip addr show "$L2_BRIDGE" | grep -q "$L2_GATEWAY_IP"; then
    log "  Gateway IP already assigned"
else
    infrr ip addr add "$L2_GATEWAY_IP" dev "$L2_BRIDGE" || {
        error "Failed to assign gateway IP to bridge"
        exit 1
    }
    # Set deterministic MAC address based on VNI (00:F3:00:00:00:VNI+1)
    MAC_SUFFIX=$(printf "%02x" $((L2_VNI + 1)))
    MAC_ADDR="00:f3:00:00:00:${MAC_SUFFIX}"
    infrr ip link set "$L2_BRIDGE" address "$MAC_ADDR" 2>/dev/null || true
    log "  Gateway IP $L2_GATEWAY_IP assigned to bridge"
fi

#
# STEP 6: Create L2VNI VXLAN interface in FRR namespace
#
L2_VXLAN="vni${L2_VNI}"
log "Step 6: Creating L2VNI VXLAN interface '$L2_VXLAN'"

if infrr ip link show "$L2_VXLAN" >/dev/null 2>&1; then
    log "  VXLAN '$L2_VXLAN' already exists"
else
    infrr ip link add "$L2_VXLAN" type vxlan \
        id "$L2_VNI" \
        local "$VTEP_IP" \
        dstport "$VXLAN_PORT" \
        nolearning || {
        error "Failed to create VXLAN $L2_VXLAN"
        exit 1
    }
    # Enslave VXLAN to bridge
    infrr ip link set "$L2_VXLAN" master "$L2_BRIDGE" || {
        error "Failed to enslave VXLAN to bridge"
        exit 1
    }
    # Disable IPv6 address auto-generation and enable neighbor suppression
    infrr sysctl -w "net.ipv6.conf.${L2_VXLAN}.addr_gen_mode=1" 2>/dev/null || true
    infrr ip link set "$L2_VXLAN" type bridge_slave neigh_suppress on 2>/dev/null || true
    infrr ip link set "$L2_VXLAN" up || {
        error "Failed to bring up VXLAN $L2_VXLAN"
        exit 1
    }
    log "  VXLAN '$L2_VXLAN' created and enslaved to bridge"
fi

#
# STEP 7: Create veth pair for L2VNI (host <-> FRR namespace)
#
HOST_VETH="host-${L2_VNI}"
PE_VETH="pe-${L2_VNI}"
log "Step 7: Creating veth pair for L2VNI: $HOST_VETH <-> $PE_VETH"

if ip link show "$HOST_VETH" >/dev/null 2>&1; then
    log "  Veth pair already exists"
else
    # Create veth pair in host namespace
    ip link add "$HOST_VETH" type veth peer name "$PE_VETH" || {
        error "Failed to create veth pair"
        exit 1
    }
    log "  Veth pair created"

    # Move PE side to FRR namespace
    ip link set "$PE_VETH" netns "$FRR_PID" || {
        error "Failed to move $PE_VETH to FRR namespace"
        exit 1
    }
    log "  Moved $PE_VETH to FRR namespace"

    # Bring up host side
    ip link set "$HOST_VETH" up || {
        error "Failed to bring up $HOST_VETH"
        exit 1
    }

    # Enslave PE side to L2 bridge in FRR namespace
    infrr ip link set "$PE_VETH" master "$L2_BRIDGE" || {
        error "Failed to enslave $PE_VETH to bridge"
        exit 1
    }
    infrr ip link set "$PE_VETH" up || {
        error "Failed to bring up $PE_VETH in namespace"
        exit 1
    }
    log "  Veth $PE_VETH enslaved to $L2_BRIDGE and brought up"
fi

#
# STEP 8: Attach host-side veth to br0
#
log "Step 8: Attaching $HOST_VETH to br0"

if ! ip link show br0 >/dev/null 2>&1; then
    error "br0 bridge does not exist - cannot attach veth"
    exit 1
fi

# Check if already attached
if ip link show "$HOST_VETH" | grep -q "master br0"; then
    log "  $HOST_VETH already attached to br0"
else
    ip link set "$HOST_VETH" master br0 || {
        error "Failed to attach $HOST_VETH to br0"
        exit 1
    }
    log "  $HOST_VETH attached to br0"
fi

log ""
log "Network infrastructure setup completed successfully!"
log ""
log "Summary:"
log "  VRF: $VRF_NAME"
log "  L3VNI: Bridge=$L3_BRIDGE, VXLAN=$L3_VXLAN, VNI=$L3_VNI"
log "  L2VNI: Bridge=$L2_BRIDGE, VXLAN=$L2_VXLAN, VNI=$L2_VNI"
log "  L2 Gateway IP: $L2_GATEWAY_IP"
log "  Veth pair: $HOST_VETH (on br0) <-> $PE_VETH (on $L2_BRIDGE)"
log "  VTEP IP: $VTEP_IP"
log ""

exit 0
