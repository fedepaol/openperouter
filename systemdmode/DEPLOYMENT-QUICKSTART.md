# OpenPERouter VPN Setup - Deployment Quick Start

This guide shows how to deploy the VPN setup on both containerlab (dev/test) and real systems (production).

## Overview

All systemd units and related files are collected in the `deployment/` directory, ready for installation on real systems.

## Directory Structure

```
systemdmode/
├── deployment/                          # ← Production deployment package
│   ├── usr/local/bin/                  # Scripts
│   │   ├── setup-underlay.sh
│   │   ├── generate-config.sh
│   │   └── common.sh
│   ├── etc/systemd/system/             # Systemd units
│   │   ├── setup-underlay.service
│   │   └── generate-config.service
│   ├── etc/openperouter/               # Configuration
│   │   ├── templates/
│   │   │   └── openpe_evpn.yaml.template
│   │   └── vpn-setup.env.example
│   ├── install.sh                      # Installation script
│   ├── uninstall.sh                    # Removal script
│   ├── README.md                       # Full documentation
│   ├── Makefile                        # Build & install targets
│   └── openperouter-vpn-setup.tar.gz  # Distribution tarball
│
├── deploy-two-unit-vpn.sh              # ← Containerlab deployment
├── TWO-UNIT-APPROACH.md                # Architecture docs
└── ... (other development files)
```

---

## Option 1: Real System Installation (Production)

### Method A: Using Tarball (Recommended)

1. **Extract tarball on target system:**
   ```bash
   scp systemdmode/deployment/openperouter-vpn-setup.tar.gz root@target:/tmp/
   ssh root@target
   cd /tmp
   tar xzf openperouter-vpn-setup.tar.gz
   cd openperouter-vpn-setup
   ```

2. **Install:**
   ```bash
   sudo make install
   # Or: sudo ./install.sh
   ```

3. **Configure:**
   ```bash
   sudo vi /etc/openperouter/vpn-setup.env
   ```
   
   Update at minimum:
   - `UNDERLAY_NIC` (your NIC name, e.g., ens3, eth1)
   - `TOR_IP` (your TOR switch IP)
   - `TOR_AS` and `LOCAL_AS` (BGP AS numbers)

4. **Enable and start:**
   ```bash
   sudo systemctl enable --now setup-underlay.service generate-config.service
   ```

5. **Verify:**
   ```bash
   sudo systemctl status setup-underlay.service generate-config.service
   sudo journalctl -u setup-underlay.service -u generate-config.service
   ```

### Method B: Using Deployment Directory Directly

```bash
cd systemdmode/deployment
sudo ./install.sh
# Then follow steps 3-5 from Method A
```

---

## Option 2: Containerlab/Kind Deployment (Development)

For testing in containerlab or kind clusters:

```bash
cd systemdmode
./deploy-two-unit-vpn.sh pe-kind

# Start services on all nodes
for NODE in $(kind get nodes --name pe-kind); do
    docker exec $NODE systemctl start setup-underlay.service
    docker exec $NODE systemctl start generate-config.service
done

# Check status
docker exec pe-kind-worker systemctl status setup-underlay.service generate-config.service
```

---

## Installed File Locations

After installation on a real system:

```
/usr/local/bin/
├── setup-underlay.sh          # Underlay infrastructure setup
├── generate-config.sh         # YAML config generation
└── common.sh                  # Shared utilities

/etc/systemd/system/
├── setup-underlay.service     # Systemd unit (underlay)
└── generate-config.service    # Systemd unit (config gen)

/etc/openperouter/
├── templates/
│   └── openpe_evpn.yaml.template
└── vpn-setup.env              # Your configuration

/var/lib/openperouter/
├── configs/
│   └── openpe_evpn.yaml       # Generated (controller reads this)
└── vpn-setup.vars             # Generated (passed between units)
```

---

## Common Operations

### View Status

```bash
systemctl status setup-underlay.service generate-config.service
```

### View Logs

```bash
# Recent logs
journalctl -u setup-underlay.service -u generate-config.service --no-pager

# Follow in real-time
journalctl -u setup-underlay.service -u generate-config.service -f
```

### Regenerate Configuration

To regenerate YAML config without touching underlay:

```bash
sudo systemctl restart generate-config.service
```

### Full Restart

```bash
sudo systemctl restart setup-underlay.service generate-config.service
```

### Check Generated Files

```bash
# Variables from underlay setup
cat /var/lib/openperouter/vpn-setup.vars

# Generated YAML configuration (controller reads this)
cat /var/lib/openperouter/configs/openpe_evpn.yaml
```

---

## Building Distribution Tarball

To create a fresh distribution tarball:

```bash
cd systemdmode/deployment
make tarball
```

Creates: `openperouter-vpn-setup.tar.gz` (ready for distribution)

---

## Prerequisites

Before starting services, ensure:

1. **br0 bridge exists with IP:**
   ```bash
   ip link show br0
   ip addr show br0
   ```
   
   If missing:
   ```bash
   ip link add br0 type bridge
   ip addr add 192.168.1.5/24 dev br0
   ip link set br0 up
   ```

2. **FRR container is configured** (routerpod-pod.service or equivalent)

3. **Underlay NIC exists** (check with `ip link show`)

---

## Uninstallation

### Real System

```bash
cd /tmp/openperouter-vpn-setup
sudo ./uninstall.sh
```

Or:
```bash
cd systemdmode/deployment
sudo make uninstall
```

### Containerlab

Just delete the cluster or manually remove files from nodes.

---

## Documentation

- **Full deployment guide**: `systemdmode/deployment/README.md`
- **Architecture details**: `systemdmode/TWO-UNIT-APPROACH.md`
- **Spec documentation**: `specs/005-systemd-vni-setup/`

---

## Troubleshooting

See `systemdmode/deployment/README.md` for detailed troubleshooting steps.

Quick checks:

1. **Service failed?**
   ```bash
   journalctl -u setup-underlay.service -u generate-config.service -p err
   ```

2. **FRR not ready?**
   ```bash
   systemctl status routerpod-pod.service
   podman logs frr
   ```

3. **br0 missing?**
   ```bash
   ip link add br0 type bridge
   ip addr add 192.168.1.5/24 dev br0
   ip link set br0 up
   ```

4. **Wrong NIC name?**
   ```bash
   ip -br link show  # Find correct name
   sudo vi /etc/openperouter/vpn-setup.env  # Update UNDERLAY_NIC
   ```
