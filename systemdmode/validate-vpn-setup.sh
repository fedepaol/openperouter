#!/bin/bash
# validate-vpn-setup.sh - Validate VPN setup on a kind node
#
# Usage: ./validate-vpn-setup.sh [node-name]
# Example: ./validate-vpn-setup.sh pe-kind-worker
#
# If no node name is provided, defaults to pe-kind-worker

set -euo pipefail

NODE=${1:-pe-kind-worker}

echo "=== VPN Setup Validation for $NODE ==="
echo ""

# Check if node exists
if ! docker ps --format '{{.Names}}' | grep -q "^${NODE}$"; then
    echo "❌ ERROR: Node $NODE not found"
    echo "Available nodes:"
    docker ps --format '{{.Names}}' | grep -E 'kind-(control-plane|worker)'
    exit 1
fi

# 1. Service Status
echo "1. Service Status:"
SERVICE_STATUS=$(docker exec "$NODE" systemctl is-active vpn-setup.service 2>/dev/null || echo "inactive")
if [[ "$SERVICE_STATUS" == "active" ]]; then
    echo "   ✓ Service is active"
else
    echo "   ✗ Service is $SERVICE_STATUS"
    echo "   Check: docker exec $NODE systemctl status vpn-setup.service"
fi
echo ""

# 2. Static Config File
echo "2. Static Configuration:"
if docker exec "$NODE" test -f /var/lib/openperouter/configs/openpe_evpn.yaml 2>/dev/null; then
    echo "   ✓ Config file exists"

    # Check VTEP IP
    VTEP=$(docker exec "$NODE" grep "vtepcidr:" /var/lib/openperouter/configs/openpe_evpn.yaml 2>/dev/null | awk '{print $2}' || echo "not found")
    echo "   VTEP IP: $VTEP"
else
    echo "   ✗ Config file missing: /var/lib/openperouter/configs/openpe_evpn.yaml"
fi
echo ""

# 3. BGP Session
echo "3. BGP Session:"
BGP_OUTPUT=$(docker exec "$NODE" podman exec frr vtysh -c "show bgp summary" 2>/dev/null || echo "FRR not accessible")
if echo "$BGP_OUTPUT" | grep -q "Established"; then
    echo "   ✓ BGP session established"
    echo "$BGP_OUTPUT" | grep -A1 "Neighbor" | tail -2
else
    echo "   ✗ BGP session not established"
    echo "   Check: docker exec $NODE podman exec frr vtysh -c 'show bgp summary'"
fi
echo ""

# 4. VNI Status
echo "4. VNI Status:"
VNI_OUTPUT=$(docker exec "$NODE" podman exec frr vtysh -c "show evpn vni" 2>/dev/null || echo "EVPN not accessible")
if echo "$VNI_OUTPUT" | grep -qE "VNI.*100|VNI.*210"; then
    echo "   ✓ VNIs configured:"
    echo "$VNI_OUTPUT" | grep -E "VNI|^[0-9]+" | head -5
else
    echo "   ✗ VNIs not configured"
    echo "   Check: docker exec $NODE podman exec frr vtysh -c 'show evpn vni'"
fi
echo ""

# 5. L2VPN veth to br0
echo "5. L2VPN veth attachment:"
if docker exec "$NODE" bridge link show 2>/dev/null | grep -q "veth-br210-host"; then
    echo "   ✓ veth-br210-host attached to br0"
else
    echo "   ✗ veth-br210-host not attached to br0"
    echo "   Check: docker exec $NODE bridge link show"
fi
echo ""

# 6. Service Logs
echo "6. Recent Logs (last 10 lines):"
docker exec "$NODE" journalctl -u vpn-setup.service --no-pager -n 10 2>/dev/null || echo "   Cannot access journal"
echo ""

# Summary
echo "=== Validation Summary ==="
if [[ "$SERVICE_STATUS" == "active" ]] && \
   docker exec "$NODE" test -f /var/lib/openperouter/configs/openpe_evpn.yaml 2>/dev/null && \
   echo "$BGP_OUTPUT" | grep -q "Established" 2>/dev/null; then
    echo "✅ VPN Setup appears to be working correctly"
    exit 0
else
    echo "⚠️  Some checks failed - review output above"
    exit 1
fi
