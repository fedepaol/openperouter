#!/bin/bash
set -euo pipefail

# OpenPerouter Systemd Quadlet Deployment Script
# Deploys Quadlet files to /etc/containers/systemd/openperouter/

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QUADLET_SOURCE_DIR="${SCRIPT_DIR}/quadlets"
SYSTEMD_QUADLET_DIR="/etc/containers/systemd/openperouter"
ENV_FILE="${SYSTEMD_QUADLET_DIR}/router.env"

log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*"
}

log_error() {
    echo "[ERROR] $*"
}

# Check if quadlet source directory exists
if [[ ! -d "$QUADLET_SOURCE_DIR" ]]; then
    log_error "Quadlet source directory not found: $QUADLET_SOURCE_DIR"
    exit 1
fi

log_info "Deploying OpenPerouter Quadlet files..."

# Create systemd quadlet directory for openperouter
log_info "Creating directory: $SYSTEMD_QUADLET_DIR"
mkdir -p "$SYSTEMD_QUADLET_DIR"

# Copy all quadlet files except the .env.example
log_info "Copying Quadlet files..."
for file in "$QUADLET_SOURCE_DIR"/*.{container,pod,volume}; do
    if [[ -f "$file" ]]; then
        cp -v "$file" "$SYSTEMD_QUADLET_DIR/"
    fi
done

# Handle environment file
if [[ ! -f "$ENV_FILE" ]]; then
    log_warn "Environment file not found, creating from example..."
    cp "$QUADLET_SOURCE_DIR/router.env.example" "$ENV_FILE"
    log_info "Created $ENV_FILE - please review and customize if needed"
else
    log_info "Environment file already exists: $ENV_FILE (not overwriting)"
fi

# Create required host directories
log_info "Creating required host directories..."
mkdir -p /etc/perouter/frr
mkdir -p /var/lib/hostambassador

# Set appropriate permissions
log_info "Setting permissions..."
chmod 644 "$SYSTEMD_QUADLET_DIR"/*.{container,pod,volume} 2>/dev/null || true
chmod 600 "$ENV_FILE"

# Reload systemd to recognize new quadlet files
log_info "Reloading systemd daemon..."
systemctl daemon-reload

# Check if services are already running
ROUTERPOD_RUNNING=false
CONTROLLERPOD_RUNNING=false

if systemctl is-active --quiet routerpod.service 2>/dev/null; then
    ROUTERPOD_RUNNING=true
fi

if systemctl is-active --quiet controllerpod.service 2>/dev/null; then
    CONTROLLERPOD_RUNNING=true
fi

# Start or restart services
log_info "Managing services..."

if $ROUTERPOD_RUNNING; then
    log_info "Restarting routerpod.service..."
    systemctl restart routerpod.service
else
    log_info "Starting routerpod.service..."
    systemctl start routerpod.service
fi

if $CONTROLLERPOD_RUNNING; then
    log_info "Restarting controllerpod.service..."
    systemctl restart controllerpod.service
else
    log_info "Starting controllerpod.service..."
    systemctl start controllerpod.service
fi

# Enable services for auto-start
log_info "Enabling services for auto-start..."
systemctl enable routerpod.service controllerpod.service

# Show status
echo ""
log_info "Deployment complete! Service status:"
echo ""
systemctl status routerpod.service --no-pager -l || true
echo ""
systemctl status controllerpod.service --no-pager -l || true

echo ""
log_info "Useful commands:"
echo "  View logs:           journalctl -u routerpod.service -f"
echo "  View logs:           journalctl -u controllerpod.service -f"
echo "  Restart services:    systemctl restart routerpod.service controllerpod.service"
echo "  Stop services:       systemctl stop routerpod.service controllerpod.service"
echo "  Disable auto-start:  systemctl disable routerpod.service controllerpod.service"
echo "  Edit environment:    vim $ENV_FILE"
echo ""
log_info "Quadlet files location: $SYSTEMD_QUADLET_DIR"
