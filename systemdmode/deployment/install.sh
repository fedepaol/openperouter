#!/bin/bash
set -euo pipefail

# install.sh - Install OpenPERouter VPN setup on a real system
#
# This script installs the two-unit VPN setup:
# 1. setup-underlay.service - Underlay infrastructure setup
# 2. generate-config.service - Configuration generation
#
# Usage: sudo ./install.sh

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo "ERROR: This script must be run as root"
   echo "Usage: sudo ./install.sh"
   exit 1
fi

INSTALL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=========================================="
echo "OpenPERouter VPN Setup Installation"
echo "=========================================="
echo ""

# Install scripts
echo "Installing scripts to /usr/local/bin..."
install -m 755 "$INSTALL_DIR/usr/local/bin/setup-underlay.sh" /usr/local/bin/
install -m 755 "$INSTALL_DIR/usr/local/bin/generate-config.sh" /usr/local/bin/
install -m 755 "$INSTALL_DIR/usr/local/bin/common.sh" /usr/local/bin/
echo "  ✓ Installed setup-underlay.sh"
echo "  ✓ Installed generate-config.sh"
echo "  ✓ Installed common.sh"

# Install systemd units
echo ""
echo "Installing systemd units to /etc/systemd/system..."
install -m 644 "$INSTALL_DIR/etc/systemd/system/setup-underlay.service" /etc/systemd/system/
install -m 644 "$INSTALL_DIR/etc/systemd/system/generate-config.service" /etc/systemd/system/
echo "  ✓ Installed setup-underlay.service"
echo "  ✓ Installed generate-config.service"

# Install template
echo ""
echo "Installing configuration template..."
mkdir -p /etc/openperouter/templates
install -m 644 "$INSTALL_DIR/etc/openperouter/templates/openpe_evpn.yaml.template" /etc/openperouter/templates/
echo "  ✓ Installed openpe_evpn.yaml.template"

# Install example environment file
echo ""
echo "Installing example environment file..."
if [[ -f /etc/openperouter/vpn-setup.env ]]; then
    echo "  ! /etc/openperouter/vpn-setup.env already exists, not overwriting"
    echo "    See /etc/openperouter/vpn-setup.env.example for reference"
    install -m 644 "$INSTALL_DIR/etc/openperouter/vpn-setup.env.example" /etc/openperouter/
else
    install -m 644 "$INSTALL_DIR/etc/openperouter/vpn-setup.env.example" /etc/openperouter/vpn-setup.env
    echo "  ✓ Installed vpn-setup.env (customize before enabling services)"
fi

# Create runtime directories
echo ""
echo "Creating runtime directories..."
mkdir -p /var/lib/openperouter/configs
echo "  ✓ Created /var/lib/openperouter/configs"

# Reload systemd
echo ""
echo "Reloading systemd daemon..."
systemctl daemon-reload
echo "  ✓ Systemd reloaded"

echo ""
echo "=========================================="
echo "Installation Complete"
echo "=========================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Customize configuration (if needed):"
echo "   sudo vi /etc/openperouter/vpn-setup.env"
echo ""
echo "2. Enable services to start on boot:"
echo "   sudo systemctl enable setup-underlay.service"
echo "   sudo systemctl enable generate-config.service"
echo ""
echo "3. Start services now:"
echo "   sudo systemctl start setup-underlay.service"
echo "   sudo systemctl start generate-config.service"
echo ""
echo "4. Check status:"
echo "   sudo systemctl status setup-underlay.service generate-config.service"
echo ""
echo "5. View logs:"
echo "   sudo journalctl -u setup-underlay.service -u generate-config.service"
echo ""
echo "6. Verify configuration:"
echo "   cat /var/lib/openperouter/vpn-setup.vars"
echo "   cat /var/lib/openperouter/configs/openpe_evpn.yaml"
echo ""
echo "Important: Make sure br0 bridge exists with an IP address before starting services!"
echo ""
