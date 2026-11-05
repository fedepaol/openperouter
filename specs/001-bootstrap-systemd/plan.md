# Implementation Plan: Image-Based Systemd Bootstrap

**Branch**: `main` (actual checkout; Spec Kit feature identifier `001-bootstrap-systemd`) | **Date**: 2026-11-04 | **Spec**: [spec.md](spec.md)

**Input**: `specs/001-bootstrap-systemd/spec.md`

## Summary

Ship `/bootstrap` in both OpenPERouter images. A root one-shot service reads one YAML file, validates the host and images, renders a complete generation of Quadlets and static configuration, publishes that generation, reloads host systemd, and starts and verifies the runtime services. Preserve operator CRD field names for the seven host-relevant options. Support kernel, Grout TAP, and Grout PCI, including coordinated restoration during mode changes.

Only bootstrap is enabled at boot. Runtime Quadlets have neither install targets nor a dependency on the starting bootstrap unit. Remove explicit and implicit network-online dependencies so the overlay can supply host connectivity. Preserve existing service identities, namespace, controller-managed FRR files, CNI cache, and later hostbridge integration.

## Technical Context

**Language/Version**: Go 1.26.4, as declared in `go.mod`; shell only for installation examples and existing development scripts.

**Primary Dependencies**: Existing `sigs.k8s.io/yaml`, static API/conversion/validation packages, `golang.org/x/sys/unix` for locking, Podman Quadlet, systemd, existing FRR and Grout images. No new third-party runtime dependency is planned.

**Storage**: Root-owned YAML, immutable artifact generations, atomic publication symlink, applied inventory, and sanitized result/transition JSON on the host. Runtime sockets and durable device restoration records remain separate.

**Testing**: Standard Go/testify unit tests, real Quadlet generator and isolated host lifecycle tests, existing Ginkgo/Gomega static boot and resiliency suites, and dedicated PCI-capable host acceptance. Use `make test` and `make lint`; use deploy/e2e skills for actual environment operations.

**Target Platform**: Rootful Linux hosts with systemd and cgroup v2, Podman supporting `.pod` and `[Quadlet] DefaultDependencies=false`, preloaded compatible images, and mode-specific networking/device resources. Detect required capabilities by running the installed generator rather than trusting a version string alone.

**Project Type**: Image-packaged administrative CLI and generated host deployment resources.

**Performance Goals**: Default apply deadline 300 seconds, configurable 30–1800 seconds. Unchanged reapplication restarts zero healthy services. No throughput benchmark is introduced.

**Constraints**: Offline boot; no Kubernetes prerequisite; no host package installation; one managed deployment and dedicated router namespace per host; strict input validation; no partial artifact activation; no automatic rollback of partially changed services. Configuration and runtime images are trusted root-administrator inputs.

**Scale/Scope**: Seven operator deployment fields plus host-only input and Grout controls. Three production validation lanes: kernel, Grout TAP, and Grout PCI. No Kubernetes scheduling/resource translation or public network API.

## Constitution Check

*Pre-research gate: PASS.* `.specify/memory/constitution.md` contains template placeholders, not adopted principles. Its commented examples do not impose approval or development requirements.

Applicable repository gates: read `CODING_STYLE.md`; prefer modern Go and existing validators; keep environmental reads in main; use standard/testify unit tests and Ginkgo/Gomega e2e tests; use deploy/e2e skills when executing environment work. No commit is required by this planning command.

*Post-design gate: PASS.* The design reuses domain models, host service control, and device restoration. The generation and transition state are necessary for FR-011 through FR-014. No new CRD, chart interface, resident daemon, generalized framework, or host package installer is introduced. Supporting existing CRD logging values requires a bounded shared-logging change. Actual runtime/hardware proof is an implementation acceptance gate, not claimed by this plan.

## Project Structure

### Documentation (this feature)

```text
specs/001-bootstrap-systemd/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── configuration.md
│   ├── lifecycle.md
│   └── openperouter-bootstrap.service
└── checklists/requirements.md
```

`tasks.md` is generated later by `speckit-tasks`.

### Source Code (repository root)

```text
cmd/bootstrap/main.go                         # CLI wiring, host-root and command selection
internal/bootstrap/                          # strict input, render, preflight, publication, apply, teardown
internal/bootstrap/templates/                # embedded Quadlet and FRR daemon templates
internal/bootstrap/*_test.go                  # table-driven contracts and lifecycle failure cases
internal/systemdctl/systemdctl.go             # bounded reload/start/stop/inspection through host namespaces
internal/staticconfiguration/                # reuse readers and node validation; strict decode adapter as needed
internal/controller/routerconfiguration/     # focused ownership inventory and teardown integration
internal/grout/                              # reuse restoration; selection-aware cleanup where needed
internal/hostnetwork/                        # reuse kernel cleanup and ownership conventions
internal/logging/                            # complete all/none CRD semantics with regression tests
Dockerfile                                   # build/copy bootstrap, ensure host-control utility exists
Dockerfile.grout                             # same entry point; retain compatible Grout/FRR packages
systemdmode/                                 # bootstrap unit/examples; deploy.sh must invoke image bootstrap
clab/                                        # combined hostmode+Grout deployment inputs where required
.agents/skills/deploy/SKILL.md                 # document combined deployment lane after tools support it
.agents/skills/e2e-tests/SKILL.md              # detect combined hostmode+Grout, preserve both flags
e2etests/systemd_static_suite/                # bootstrap and static-before-Kubernetes acceptance
e2etests/tests/systemd_resiliency.go           # extend existing restart proof
website/content/docs/installation/systemd-mode.md
website/content/docs/configuration/systemd-mode.md
```

**Structure Decision**: Keep the new lifecycle in one specific internal package, with embedded templates consumed by a separate binary. Reuse existing routing models rather than copy the operator reconciliation machinery. Preserve legacy manually installed Quadlets as a supported separate installation path; do not silently adopt them. Existing development scripts switch clean test hosts to bootstrap deployment explicitly.

## Phase 0: Research Outcome

[research.md](research.md) resolves input shape, host control, boot cycles, immutable publication, logging parity, Grout wiring, restoration, and deadlines. Both research agents inspected repository code. Primary systemd/Podman documentation corroborates dependency and generation behavior; host lifecycle behavior still requires real tests.

## Phase 1: Design

### Apply sequence

1. Acquire a nonblocking host lock. Strictly parse YAML and validate all embedded/referenced configuration and CRD constraints. Resolve node index in host networking. Establish the overall deadline.
2. Inspect required host capabilities, existing ownership and unit conflicts, local image ID and packaged components, configuration source permissions, namespace/device state, OVS access where needed, and Grout mode-specific prerequisites. Do not pull images or bind devices.
3. Render deterministic files and a manifest into a private staging directory. Run the real host Quadlet generator against staging; require all expected generated services and verify absence of network-online/bootstrap ordering and install links.
4. Compare desired fingerprints with published/applied state and actual service health. Prepare the complete activation action set before any runtime changes.
5. If changing datapath or TAP/PCI mode, persist transition intent and perform old-mode teardown with the previous runtime image. Stop controller and FRR/reloader first; keep old Grout and namespace alive until restoration completes. Validate deletion candidates against ownership inventory. Propagate errors and leave restoration records until successful restoration. Ordinary setting/image changes skip teardown.
6. Atomically publish the complete generation and ownership registration. Run `daemon-reload`. Stop removed components; restart only changed components in dependency order, starting missing/failed components even when fingerprints match. Controller-generated configuration changes count as controller changes. Router pod recreation restarts its member set.
7. Verify unit states and configured health endpoints until deadline. Atomically record successful activation or sanitized partial failure. Finish with no resident bootstrap process. Corrected retries resume transitions before activating a new mode.

### Grout-specific work

Use the Grout-capable image for every component; add Grout to the router pod and share its runtime socket. Render `dplane_grout` daemon arguments, compatible health checks, existing chart memory sizing, persistent restoration records, and mode-specific device mounts. Keep TAP selection distinct from test mode. New Grout maximum MTU and optional test mode fields are supplemental host deployment controls, not operator CRD fields.

### Failure boundaries

Preflight/generation failure leaves runtime files and services unchanged. Staging/diagnostic metadata is allowed. Publication is a complete-generation operation. Activation/teardown failure is reported with affected services and a durable stage for retry; full service rollback is not promised. Do not remove previous-image availability or restoration state needed for recovery. Never overwrite administrator files, alter external routing definitions, delete a shared namespace, or log routing passwords.

### Verification and acceptance

**Required first runtime gate:** Build the feature image, deploy hostmode through the new image bootstrap flow, and execute at least one passing e2e test against that systemd deployment. Use the deploy skill in hostmode and the e2e-tests skill with the focused case `Systemd Router Restart Resiliency / recovers BGP sessions after FRR restart`. The deployment script must exercise the image entry point rather than copy legacy units. A zero-test or skipped-only run fails the gate. Record the image ID, bootstrap result, active runtime units, exact test command, executed/passed counts, and report location. Static checks cannot satisfy FR-018/SC-007. See the executable sequence in [quickstart.md](quickstart.md#required-systemd-deployment-and-one-test-gate).

- Parser/model tests cover strict keys, duplicate keys, explicit index zero, conflicting input, all seven CRD fields and defaults, all six logging values, and kernel/Grout validation failures.
- Publication tests inject failure before/after staging and pointer switch; concurrent apply tests prove serialization; retry tests distinguish published artifacts from running health. Validate actual generator output rather than template text alone.
- Runtime tests prove no boot cycles, no registry/Kubernetes dependency, unchanged content and restart counts across three applies, changed health-port/OVS settings, inline update and external-directory live reload, startup gate timeout, and sanitized actionable failures.
- Transition tests cover kernel-to-Grout, Grout-to-kernel, TAP-to-PCI, PCI-to-TAP, and interruption during restoration; require preservation of foreign links/bridges and actual device recovery.
- Production host lanes each prove packet forwarding, restart recovery, and three offline reboots. PCI hardware validation is required before claiming PCI completion. Test-mode smoke checks are additional, not substitutes.
- Run repository unit/lint gates after implementation. The existing Quadlet verifier's `systemctl enable` checks must be replaced for bootstrap deployments; only bootstrap is enabled.

## Complexity Tracking

No constitution violations require exceptions. Generation state, per-component fingerprints, and bounded old-mode teardown directly satisfy explicit repeatability and recovery requirements; keep them inside the bootstrap lifecycle rather than create a generic state framework.

## Planning Validation

The installed Podman 5.8.1 generator passed an isolated symlink-discovery/dependency dry run using `QUADLET_UNIT_DIRS`. `systemd-analyze verify` accepted the proposed bootstrap service. Artifact structure, local Markdown links, example YAML, unresolved-marker checks, and repository whitespace checks are validated separately before completion. No implementation or deployment is performed in this phase.
