# Implementation Plan: Systemd-Based VNI Setup Script

**Branch**: `005-systemd-vni-setup` | **Date**: 2026-04-01 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/005-systemd-vni-setup/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Create a bash script that runs as a systemd oneshot service **inside containerlab kind nodes** to automatically configure L2VPN (VNI 210) and L3VPN (VNI 100) networks in systemd mode. The script waits for FRR container readiness, derives node-specific VTEP IP from br0's last octet, generates static configuration YAML from a template, moves the host NIC into the FRR namespace for underlay connectivity. The controller reads the static configuration and renders FRR EVPN configuration. Deployment follows existing patterns from `systemdmode/deploy.sh` - copying script, template, and service unit to each kind node. Configuration uses environment variables with hardcoded defaults; fails fast on error without rollback (development/testing environment).

## Technical Context

**Language/Version**: Bash 4.0+  
**Primary Dependencies**: 
- FRR container (via podman/systemd quadlet)
- systemdmode/common.sh utilities (frr_netns_pid, inns, isfrr_ready)
- iproute2 (ip, bridge commands)
- nsenter (namespace operations)
- vtysh (FRR CLI)

**Storage**: N/A (runtime network configuration only)  
**Testing**: 
- Containerlab (clab) for integration testing in kind nodes
- Manual verification via systemd journal and FRR vtysh commands
- Bash-based validation scripts

**Target Platform**: Linux (containerlab kind nodes with systemd support)  
**Project Type**: Bash script + systemd unit file (infrastructure automation)  
**Performance Goals**: 
- FRR readiness wait: 60s timeout
- Underlay setup: complete within 30s
- VPN configuration: complete within 60s from FRR ready

**Constraints**: 
- No rollback on failure (development/testing environment)
- Fixed VNI values (L3=100, L2=210)
- Fixed VRF name ("red")
- Runs once as systemd oneshot service
- Must use existing systemdmode/common.sh utilities

**Scale/Scope**: 
- Multi-node containerlab deployments
- Single script execution per node boot
- 2 VPNs per node (1 L3, 1 L2)
- Single TOR switch peering per node

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Status**: No constitution defined (template only) - proceeding without constitutional constraints.

## Project Structure

### Documentation (this feature)

```text
specs/005-systemd-vni-setup/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
systemdmode/
├── common.sh                         # Existing utilities (frr_netns_pid, inns, isfrr_ready)
├── setup-vpn.sh                      # NEW: Main VPN setup script
├── openpe_evpn.yaml.template         # NEW: Static config template with placeholders
├── frrconfig/
│   ├── daemons                       # Existing FRR daemons config
│   └── frr.conf                      # Existing FRR base config
└── quadlets/
    ├── routerpod.pod                 # Existing router pod definition
    ├── frr.container                 # Existing FRR container
    └── vpn-setup.service             # NEW: Systemd oneshot service unit

/var/lib/openperouter/configs/        # Runtime configuration directory (host)
└── openpe_evpn.yaml                  # Generated from template by setup-vpn.sh

config/samples/
├── l2vni.yaml                        # Existing L2VNI CRD reference
└── l3vni.yaml                        # Existing L3VNI CRD reference
```

**Structure Decision**: 
- Extend systemdmode/ with setup script, YAML template, and systemd service unit
- Use OpenPERouter's static configuration infrastructure (`/var/lib/openperouter/configs/openpe_*.yaml`)
- Script generates configuration from template, filling in node-specific values (VTEP IP, NIC name)
- Controller reads static config and renders to FRR configuration
- Manual/bash-based testing only (no automated e2e tests)

## Deployment Model

### Containerlab/Kind Integration

This feature runs **inside kind nodes**, not on the host. Deployment follows the existing pattern from `systemdmode/deploy.sh`:

**Files to Deploy to Each Node**:
1. **Script**: `setup-vpn.sh` → `/usr/local/bin/setup-vpn.sh`
2. **Template**: `openpe_evpn.yaml.template` → `/etc/openperouter/templates/`
3. **Service Unit**: `quadlets/vpn-setup.service` → `/etc/containers/systemd/` (via quadlet pattern)

**Deployment Script** (extend existing `deploy.sh` or create `deploy-vpn-setup.sh`):
```bash
#!/bin/bash
# Deploy VPN setup to all nodes in kind cluster

CLUSTER_NAME="$1"
NODES=$(kind get nodes --name "$CLUSTER_NAME")

for NODE in $NODES; do
    # Copy script
    docker cp systemdmode/setup-vpn.sh "$NODE:/usr/local/bin/"
    docker exec "$NODE" chmod +x /usr/local/bin/setup-vpn.sh
    
    # Copy template
    docker exec "$NODE" mkdir -p /etc/openperouter/templates
    docker cp systemdmode/openpe_evpn.yaml.template "$NODE:/etc/openperouter/templates/"
    
    # Quadlet service copied by existing deploy.sh pattern
    
    # Reload systemd and start service
    docker exec "$NODE" systemctl daemon-reload
    docker exec "$NODE" systemctl start vpn-setup.service
done
```

**Makefile Integration**:
```makefile
.PHONY: deploy-hostmode-vpn
deploy-hostmode-vpn: export KUSTOMIZE_LAYER=hostmode
deploy-hostmode-vpn: kind deploy-cluster setup-hostmode deploy-controller deploy-vpn-setup
    # Deploy cluster with VPN auto-setup

.PHONY: deploy-vpn-setup
deploy-vpn-setup: ## Deploy VPN setup script to all nodes
    ./systemdmode/deploy-vpn-setup.sh $(KIND_CLUSTER_NAME)
```

**Per-Node Behavior**:
- Each node derives its own VTEP IP from its br0 IP
- Example: Node with br0=192.168.1.2 → VTEP IP=10.0.0.2
- Generated config is unique per node: `/var/lib/openperouter/configs/openpe_evpn.yaml`

### Existing Pattern Reference

**Current `deploy.sh` pattern**:
1. Gets all nodes: `kind get nodes --name "$CLUSTER_NAME"`
2. For each node: `docker exec "$NODE" <command>`
3. Copies quadlet files: `docker cp "$quadlet_file" "$NODE:$QUADLET_DIR/"`
4. Reloads systemd: `docker exec "$NODE" systemctl daemon-reload`
5. Starts services: `docker exec "$NODE" systemctl restart routerpod-pod.service`

**Current `setup_node_config.sh` pattern**:
1. Creates `/var/lib/openperouter/configs/` on each node
2. Writes `node-config.yaml` with `nodeIndex`
3. Optionally copies `openpe_*.yaml` from `NODE_CONFIG_DIR`

### Testing in Containerlab

```bash
# Deploy cluster with VPN setup (default cluster name: pe-kind)
make deploy-hostmode-vpn

# Get node names
kind get nodes --name pe-kind
# Output: pe-kind-control-plane, pe-kind-worker

# Verify inside the worker node
docker exec pe-kind-worker /usr/local/bin/setup-vpn.sh
docker exec pe-kind-worker cat /var/lib/openperouter/configs/openpe_evpn.yaml
docker exec pe-kind-worker systemctl status vpn-setup.service

# Or use containerlab to list nodes
docker ps --filter "label=clab-node-name" --format "table {{.Names}}\t{{.Status}}"
```

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No complexity tracking required - no constitution defined.
