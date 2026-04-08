#!/bin/bash
set -euo pipefail

# uninstall.sh - Uninstall OpenPERouter VPN setup
#
# This script removes all installed files and stops services
#
# Usage: sudo ./uninstall.sh

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo "ERROR: This script must be run as root"
   echo "Usage: sudo ./uninstall.sh"
   exit 1
fi

echo "=========================================="
echo "OpenPERouter VPN Setup Uninstallation"
echo "=========================================="
echo ""

# Stop and disable services
echo "Stopping and disabling services..."
systemctl stop setup-underlay.service generate-config.service 2>/dev/null || true
systemctl disable setup-underlay.service generate-config.service 2>/dev/null || true
echo "  ✓ Services stopped and disabled"

# Remove systemd units
echo ""
echo "Removing systemd units..."
rm -f /etc/systemd/system/setup-underlay.service
rm -f /etc/systemd/system/generate-config.service
echo "  ✓ Removed systemd units"

# Remove scripts
echo ""
echo "Removing scripts..."
rm -f /usr/local/bin/setup-underlay.sh
rm -f /usr/local/bin/generate-config.sh
echo "  ✓ Removed scripts (kept common.sh as it may be used elsewhere)"

# Remove templates
echo ""
echo "Removing templates..."
rm -f /etc/openperouter/templates/openpe_evpn.yaml.template
echo "  ✓ Removed templates"

# Reload systemd
echo ""
echo "Reloading systemd daemon..."
systemctl daemon-reload
echo "  ✓ Systemd reloaded"

echo ""
echo "=========================================="
echo "Uninstallation Complete"
echo "=========================================="
echo ""
echo "The following were NOT removed (manual cleanup if needed):"
echo "  - /etc/openperouter/vpn-setup.env (configuration)"
echo "  - /var/lib/openperouter/configs/ (generated configurations)"
echo "  - /var/lib/openperouter/vpn-setup.vars (runtime variables)"
echo "  - /usr/local/bin/common.sh (may be used by other components)"
echo ""
echo "To remove all configuration data:"
echo "  sudo rm -rf /etc/openperouter /var/lib/openperouter"
echo ""
