# Implementation Tasks: Systemd-Based VNI Setup Script

**Feature**: 005-systemd-vni-setup  
**Branch**: `005-systemd-vni-setup`  
**Date**: 2026-04-01

## Overview

This document breaks down the implementation of the Systemd-Based VNI Setup Script into discrete, executable tasks organized by user story. The feature creates a bash script that runs as a systemd service inside containerlab kind nodes to automatically configure L2 and L3 VPNs using OpenPERouter's static configuration infrastructure.

## Implementation Strategy

**Approach**: Incremental delivery by user story priority
- **MVP (Minimum Viable Product)**: User Story 1 (P1) - Core VPN setup functionality
- **Enhancement**: User Story 2 (P2) - Diagnostic logging improvements

**Key Decisions** (from research.md):
- Uses static YAML configuration (`/var/lib/openperouter/configs/openpe_*.yaml`) instead of direct FRR vtysh commands
- Template-based configuration with node-specific value substitution (VTEP IP derived from br0)
- Deploys to kind nodes via containerlab, follows existing `systemdmode/deploy.sh` pattern
- Manual/bash-based validation only (no automated e2e tests)

## Dependencies & Execution Order

### User Story Dependencies

```
Phase 1 (Setup)
    ↓
Phase 2 (Foundational: Template & Deployment)
    ↓
Phase 3 (US1: Core VPN Setup) ← MVP scope
    ↓
Phase 4 (US2: Enhanced Logging) ← Independent enhancement
    ↓
Phase 5 (Polish & Integration)
```

**Critical Path**:
1. Setup → Foundational (template) → US1 Core Script
2. All other tasks can be done in parallel within their phase

### Task Legend

- `- [ ]` = Not started
- `T###` = Task ID (sequential)
- `[P]` = Parallelizable (can run concurrently with other [P] tasks in same phase)
- `[US#]` = User Story number from spec.md

---

## Phase 1: Setup & Project Structure

**Goal**: Initialize project files and directory structure

**Tasks**:

- [X] T001 Create systemdmode/setup-vpn.sh with shebang and basic structure
- [X] T002 [P] Create systemdmode/openpe_evpn.yaml.template as empty file
- [X] T003 [P] Create systemdmode/quadlets/vpn-setup.service as empty file
- [X] T004 [P] Create systemdmode/deploy-vpn-setup.sh as empty file (optional: could extend existing deploy.sh)

**Acceptance**: All new files exist in correct locations per plan.md structure

---

## Phase 2: Foundational Components

**Goal**: Create reusable template and deployment infrastructure (blocks both user stories)

**Why foundational**: The YAML template is required by US1 for configuration generation, and the deployment script is needed to test the setup on kind nodes.

**Tasks**:

- [X] T005 Implement YAML template in systemdmode/openpe_evpn.yaml.template with placeholders per contracts/frr-config.md
- [X] T006 Implement deployment script in systemdmode/deploy-vpn-setup.sh following systemdmode/deploy.sh pattern
- [X] T007 Create systemd service unit in systemdmode/quadlets/vpn-setup.service per contracts/systemd-service.md

**Acceptance**:
- YAML template contains all required placeholders: `{{VTEP_IP}}`, `{{UNDERLAY_NIC}}`, `{{TOR_IP}}`, `{{TOR_AS}}`, `{{LOCAL_AS}}`, `{{VRF_NAME}}`, `{{L2_VNI}}`, `{{L3_VNI}}`, `{{VXLAN_PORT}}`, `{{L2_GATEWAY_IP}}`
- Template structure matches `PERouterConfig` from api/static/config.go (underlays, l3vnis, l2vnis, rawfrrconfigs sections)
- Deployment script copies script, template, and service to kind nodes
- Service unit has correct After/Requires dependencies on routerpod-pod.service

---

## Phase 3: User Story 1 - Automated VPN Setup (P1)

**User Story**: A system administrator deploying OpenPERouter in systemd mode needs the complete network stack (underlay and overlay) to be automatically configured when the system boots, without manual intervention.

**Independent Test Criteria**:
1. Enable systemd service, reboot node, verify BGP session established without manual commands
2. EVPN type 2 routes (L2VPN) present in routing table
3. EVPN type 5 routes (L3VPN) present in routing table
4. VTEP IP correctly derived from br0's last octet (10.0.0.X format)
5. L2VPN veth enslaved to br0

**Manual Validation Commands** (from spec.md):
```bash
# Run on kind node (docker exec pe-kind-worker <command>)
systemctl status vpn-setup.service
cat /var/lib/openperouter/configs/openpe_evpn.yaml
podman exec frr vtysh -c "show bgp summary"
podman exec frr vtysh -c "show bgp l2vpn evpn route type 2"
podman exec frr vtysh -c "show bgp l2vpn evpn route type 5"
bridge link show | grep veth-br210-host
```

### Implementation Tasks

- [X] T008 [US1] Source systemdmode/common.sh and verify required functions (frr_netns_pid, inns, isfrr_ready) available in systemdmode/setup-vpn.sh
- [X] T009 [US1] Implement environment variable loading with defaults in systemdmode/setup-vpn.sh per contracts/script-interface.md
- [X] T010 [US1] Implement FRR readiness wait loop using isfrr_ready() with 60s timeout (FR-003, FR-004, FR-016) in systemdmode/setup-vpn.sh
- [X] T011 [US1] Implement br0 IP extraction and VTEP IP derivation (10.0.0.X/24) logic (FR-009, FR-010) in systemdmode/setup-vpn.sh
- [X] T012 [US1] Implement YAML template rendering using envsubst or sed substitution in systemdmode/setup-vpn.sh
- [X] T013 [US1] Implement static config file write to /var/lib/openperouter/configs/openpe_evpn.yaml in systemdmode/setup-vpn.sh
- [X] T014 [US1] Implement host NIC namespace movement using frr_netns_pid() and ip link set netns (FR-001) in systemdmode/setup-vpn.sh
- [X] T015 [US1] Implement validation logic: check BGP session established and EVPN routes present in systemdmode/setup-vpn.sh
- [X] T016 [US1] Implement exit code handling: 0 for success, 1 for failure (FR-014, FR-015) in systemdmode/setup-vpn.sh

**Acceptance Scenarios** (from spec.md US1):
1. ✅ Host NIC moved from host namespace to FRR namespace
2. ✅ BGP session shows "Established" state
3. ✅ L3VPN (VNI 100, VRF "red") created and operational
4. ✅ L2VPN (VNI 210, VRF "red") created and operational
5. ✅ Script waits for FRR ready before proceeding
6. ✅ L2VPN veth enslaved to br0
7. ✅ VTEP IP configured as 10.0.0.X (from br0)
8. ✅ EVPN type 2 routes present and exchanged
9. ✅ EVPN type 5 routes present and exchanged
10. ✅ Configuration matches CRD-based output

---

## Phase 4: User Story 2 - Diagnostic Logging (P2)

**User Story**: A system administrator troubleshooting deployment issues needs clear, actionable log messages that indicate what the script is doing at each step and provide specific error details when configuration fails.

**Independent Test Criteria**:
1. Intentionally cause FRR timeout failure, verify journalctl shows clear error
2. Remove br0, verify journalctl shows missing bridge error with guidance
3. Run successful setup, verify journalctl shows all steps completed

**Manual Validation Commands**:
```bash
# View logs
docker exec pe-kind-worker journalctl -u vpn-setup.service --no-pager
docker exec pe-kind-worker journalctl -u vpn-setup.service -p err
```

### Implementation Tasks

- [X] T017 [P] [US2] Add timestamped log() function for informational messages (stdout) in systemdmode/setup-vpn.sh
- [X] T018 [P] [US2] Add timestamped error() function for error messages (stderr) in systemdmode/setup-vpn.sh
- [X] T019 [US2] Add informational log messages for each major step (FR-012) in systemdmode/setup-vpn.sh
- [X] T020 [US2] Add specific error messages for all failure scenarios per contracts/script-interface.md in systemdmode/setup-vpn.sh
- [X] T021 [US2] Add edge case error handling per spec.md Edge Cases section in systemdmode/setup-vpn.sh

**Acceptance Scenarios** (from spec.md US2):
1. ✅ FRR timeout logged with actionable error after 60s
2. ✅ Missing br0 logged with guidance
3. ✅ Successful setup shows all steps (config generated, NIC moved, BGP established, VPNs created)

**Edge Case Error Messages** (from spec.md):
- Missing host NIC: "ERROR: Host NIC {name} not found"
- Missing NIC IP: "ERROR: Host NIC {name} does not have an IP address configured"
- Missing br0: "ERROR: br0 bridge does not exist"
- Missing br0 IP: "ERROR: br0 bridge does not have an IP address"
- FRR timeout: "ERROR: FRR not ready after 60s timeout"
- BGP not established: "ERROR: BGP session failed to establish"

---

## Phase 5: Polish & Integration

**Goal**: Complete deployment integration, documentation, and validation tooling

**Tasks**:

- [ ] T022 [P] Add Makefile target deploy-vpn-setup to deploy VPN setup to nodes in Makefile
- [ ] T023 [P] Optionally extend Makefile target deploy-hostmode to include VPN setup in Makefile
- [ ] T024 [P] Create bash validation script systemdmode/validate-vpn-setup.sh per research.md testing strategy
- [ ] T025 [P] Update systemdmode/README.md (if exists) with VPN setup usage instructions
- [ ] T026 Run manual validation on containerlab deployment following quickstart.md validation steps

**Acceptance**:
- `make deploy-vpn-setup KIND_CLUSTER_NAME=pe-kind` deploys to all nodes
- `systemdmode/validate-vpn-setup.sh` checks BGP, VNI, veth attachment
- Documentation updated with setup instructions

---

## Parallel Execution Opportunities

### Within Phase 1 (Setup)
All T002, T003, T004 can be done in parallel (file creation)

### Within Phase 2 (Foundational)
T005 (template), T006 (deployment), T007 (service unit) are independent - can all be done in parallel

### Within Phase 3 (US1)
- T008 (verify utilities) must complete first
- T009 (env vars) can be done in parallel with T017-T018 (logging functions from US2)
- T010-T014 (core logic) are sequential dependencies
- T015-T016 (validation) depend on T010-T014

### Within Phase 4 (US2)
- T017-T018 (logging functions) can be done in parallel with US1 tasks
- T019-T021 (add logging) depend on T017-T018 and US1 tasks being complete

### Within Phase 5 (Polish)
All T022-T025 can be done in parallel

## MVP Scope Recommendation

**Minimum Viable Product**: Complete through Phase 3 (User Story 1)

This delivers:
- ✅ Automated VPN setup on boot
- ✅ Template-based configuration generation
- ✅ Deployment to kind nodes
- ✅ Core functionality validated

**Defer to v1.1**:
- User Story 2 (Enhanced logging) - can use basic error output for MVP
- Validation script (T024) - can use manual commands initially
- Makefile integration (T022-T023) - can use direct script execution

## Task Summary

**Total Tasks**: 26
- Phase 1 (Setup): 4 tasks
- Phase 2 (Foundational): 3 tasks
- Phase 3 (US1 - P1): 9 tasks
- Phase 4 (US2 - P2): 5 tasks
- Phase 5 (Polish): 5 tasks

**Parallel Opportunities**: 10 tasks marked [P] can run concurrently

**User Story Distribution**:
- User Story 1 (P1): 9 tasks (T008-T016)
- User Story 2 (P2): 5 tasks (T017-T021)
- Infrastructure: 12 tasks (Setup, Foundational, Polish)

## Implementation Notes

### From Clarifications
- Environment variables with hardcoded defaults for NIC name, TOR IP/AS
- No authentication for BGP session
- Exit on error, no rollback (development environment)
- 60 second FRR readiness timeout
- NIC IP preserved when moved to namespace

### From Research
- Use `envsubst` or `sed` for template variable substitution
- Config path: `/var/lib/openperouter/configs/openpe_evpn.yaml`
- Template path: `/etc/openperouter/templates/openpe_evpn.yaml.template`
- Script path: `/usr/local/bin/setup-vpn.sh`

### From Contracts
- Script exit codes: 0=success, 1=error, 124=timeout
- Log format: `[YYYY-MM-DD HH:MM:SS] <message>` for stdout
- Error format: `[YYYY-MM-DD HH:MM:SS] ERROR: <message>` for stderr
- Service type: oneshot with RemainAfterExit=yes

### Testing Approach
Manual validation on containerlab kind nodes:
```bash
# Deploy
make deploy-hostmode-vpn

# Verify
docker exec pe-kind-worker systemctl status vpn-setup.service
docker exec pe-kind-worker cat /var/lib/openperouter/configs/openpe_evpn.yaml
docker exec pe-kind-worker podman exec frr vtysh -c "show bgp summary"
docker exec pe-kind-worker podman exec frr vtysh -c "show evpn vni"
```

---

**Next Steps**: Start with Phase 1 (Setup) tasks T001-T004 to create the file structure, then proceed to Phase 2 (Foundational) to implement the template and deployment infrastructure.
