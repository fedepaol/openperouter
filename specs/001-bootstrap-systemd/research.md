# Research: Image-Based Systemd Bootstrap

Date: 2026-11-04. Scope: FR-001 through FR-018. Findings describe existing code; decisions describe proposed work, not verified runtime behavior.

## 1. Entry point and host control

**Decision:** Add Go `/bootstrap` to both images; retain `/controller` as their default entry point. A root one-shot service invokes `/bootstrap apply` in a short-lived privileged container with host PID/network namespaces and a host-root bind at `/host`. Use explicit host-root file operations and the existing `nsenter` pattern for host systemctl, Podman, and generator commands. Ensure both images contain `nsenter`. Never relabel the host root with `:Z` or interpolate YAML values into shell commands.

**Rationale:** One process owns the complete operation, lock, deadline, and result. Existing `internal/systemdctl/systemdctl.go` already controls host services this way. The image is trusted administrator code.

**Alternatives considered:** Renderer plus shell orchestrator duplicates lifecycle logic; extracting a host executable adds an executable upgrade lifecycle; a real systemd generator is unsuitable for this activation workflow.

## 2. Boot and dependency ordering

**Decision:** Enable only `openperouter-bootstrap.service`. Runtime Quadlets omit `[Install]`, dependencies on bootstrap, and explicit network-online requirements. Set `[Quadlet] DefaultDependencies=false`; retain ordinary systemd `[Unit]` defaults. Bootstrap synchronously starts runtime services after publication and reload.

**Rationale:** Existing pods explicitly wait for network-online, while the overlay may supply that network. Runtime `After=bootstrap` would deadlock a bootstrap waiting for runtime startup. Sole bootstrap enablement prevents stale definitions starting automatically.

**Alternatives considered:** Enabling runtime units adds unnecessary boot gates; disabling all systemd dependencies changes shutdown semantics.

**Primary source:** [Quadlet documentation](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html). The boot arrangement is a design inference from documented generation and dependency semantics.

## 3. Input and validation reuse

**Decision:** Strict version-one YAML has `version`, `deployment`, `node`, and `routing`. Use existing static domain types and conversion validators; reject duplicate/unknown keys and any filtered-out resource. Use presence-aware input for index zero before converting to `api/static.NodeIndex`. Resolve interface-derived indices in host networking. Routing selects exactly one inline object or existing administrator directory.

**Rationale:** Existing readers decode permissively and node-index fields lose presence. `cmd/hostcontroller/main.go` overrides CLI logging with node configuration; render the same effective logging value in both. Reject conflicting sources rather than silently override.

**Alternatives considered:** Full operator objects include irrelevant Kubernetes fields. Existing readers alone do not satisfy strict validation. Copying referenced files would break live updates.

**Evidence:** `api/static/config.go`, `internal/staticconfiguration/{reader,nodeindex}.go`, `internal/conversion/`, `cmd/hostcontroller/main.go`.

## 4. CRD parity exposes a logging gap

**Decision:** Preserve all seven supported CRD field names and constraints. Extend shared runtime logging with CRD values `all` and `none`: all enables every supported verbosity; none disables process logging. Preserve existing debug/info/warn/error behavior. Bootstrap result diagnostics remain available independently.

**Rationale:** The checked-in CRDs accept six logging values; `internal/logging/logging.go` currently accepts four. Accepting six in YAML without runtime support would fail startup. Explicit backoff zero is invalid; omission uses the default limiter.

**Alternatives considered:** Restricting values contradicts CRD parity; silently mapping none to error contradicts intent.

## 5. Complete generations and live routing

**Decision:** Stage immutable generations under `/var/lib/openperouter/bootstrap/generations/<digest>/`; publish with atomic replacement of `current`. A bootstrap-owned symlink `/etc/containers/systemd/openperouter-bootstrap` points to `current/quadlets`. Validate discovery with the host generator before publication. Reject conflicting unit names, source definitions, drop-ins, foreign enablement links, and ownership before adoption.

Generate node configuration and inline routing in the generation. Mount referenced routing directories directly into the controller. Changes to generated configuration or routing directory require restarting the controller; ordinary edits inside an external directory retain live reload. Keep FRR-written configuration, CNI cache, kubeconfig, startup gate, and restoration state outside generations.

**Rationale:** Independent file replacements expose mixed generations. Existing bind mounts and inotify watches do not retarget when a directory symlink changes.

**Alternatives considered:** Snapshotting external routing breaks reload; pointer changes without a controller restart leave old bind mounts and watches.

## 6. Idempotence and recovery

**Decision:** Require images locally present, resolve local IDs, and render `Pull=never`. A changed local tag resolution is an image change. Lock the complete operation nonblockingly. Store per-component fingerprints plus separate published and successfully activated generation identifiers. Skip matching healthy services; start inactive/failed services even with identical content. Atomic `result.json` records stage, affected services, and sanitized errors. Input failures change diagnostics only.

**Rationale:** Hash equality does not prove running health. Publication and activation are distinct recovery states. Offline boot must not pull through its own overlay.

**Alternatives considered:** Automatic image distribution and activation rollback are unnecessary scope. Trusting only the last success record misses interrupted activation.

## 7. Grout wiring

**Decision:** One runtime `deployment.image` supplies every component. Grout requires the compatible `Dockerfile.grout` variant for controller, FRR, reloader, and Grout. Add Grout to the router pod, share its socket, render FRR `dplane_grout`, retain host-network controller, persistent `perouter` namespace, `routerpod-pod.service`, and `/etc/perouter/frr/frr.pid`.

Mount host `/var/lib/openperouter/grout/device-state` at controller `/var/run/grout/device-state`. Runtime sockets are separate. Carry chart sizing defaults into Grout service configuration. Support optional `grout.maxMTU` (zero/omitted delegates to Grout) and `grout.testMode` (false); TAP does not imply test mode. Use `/bootstrap probe` for HTTP health checks in both images.

**Rationale:** Controller invokes `grcli`; standard image is insufficient. Restoration records must survive controller recreation and reboot. Grout image installs curl rather than explicitly installing wget, so inherited wget health checks are unreliable.

**Evidence:** `Dockerfile.grout`, `charts/openperouter/templates/router.yaml`, `internal/controller/routerconfiguration/grout_config.go`, `internal/grout/grout_client.go`.

## 8. TAP, PCI, and transitions

**Decision:** Validate mode-specific TUN, device, driver, hugepage, resource, and permission prerequisites without changing drivers during preflight. PCI checks distinguish bifurcated drivers from VFIO devices. Add bounded `/bootstrap teardown` using the previous runtime image for datapath or TAP/PCI changes. Stop controller and FRR/reloader, retain old Grout and namespace during device restoration, then replace services. Persist transition progress before stopping anything.

Reuse existing passthrough/VNI/VRF/L3VPN removal and underlay restoration primitives. Propagate every error. Require ownership inventory before cleanup; reject foreign objects and use selection-aware cleanup where existing helpers assume ownership by name. Never delete the whole namespace. Resume interrupted restoration before starting the target controller.

**Rationale:** TAP state lacks the PCI metadata required by accelerated setup. Blind restart cannot switch modes safely. Existing private cleanup methods log some errors rather than return them. Existing broad removal helpers require a dedicated namespace/daemon and explicit inventory checks.

**Alternatives considered:** Rejecting all mode changes narrows FR-011; deleting state or namespace loses restoration information; generic migration infrastructure is unnecessary.

**Evidence:** `internal/grout/{underlay,underlay_accelerated}.go`, `internal/controller/routerconfiguration/{grout_config,host_config}.go`, `internal/hostnetwork/vni.go`.

## 9. Deadline and validation matrix

**Decision:** Startup timeout defaults to 300 seconds, range 30–1800; one-shot `TimeoutStartSec=1830` bounds launch overhead. Check each runtime service, controller health, router/reloader health inside `perouter`, and Grout control readiness. Bootstrap health is not a routing-convergence guarantee; validate actual packets separately.

Add combined hostmode+Grout deployment/test selection. Run kernel, production Grout TAP, and hardware Grout PCI acceptance, including three offline reboots and unchanged reapplications. Test-mode smoke checks supplement production validation.

**Rationale:** Current deployment and e2e tools select hostmode and Grout separately. A veth-only kind run cannot prove PCI support. Controller ping health may pass before routing convergence.

**Primary source:** [systemd one-shot semantics](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html).

## 10. Gates

The constitution is an unfilled template, not adopted governance. Apply repository coding and testing instructions. Add no Kubernetes API, resident daemon, host package installer, or generic deployment framework. All design unknowns above are resolved; runtime and hardware validation remains implementation work, not an unresolved design question.

## Planning validation performed

On the current environment's Podman 5.8.1 generator, an isolated temporary directory test confirmed that a discovery symlink to a generation directory produces both pod and container services. The generated output had no network-online/bootstrap dependency or install links. The installed generator uses `QUADLET_UNIT_DIRS` for an isolated search path; the singular name was ignored and was corrected before recording the passing result. No real unit was installed and no container was started.

`systemd-analyze verify` accepted the proposed bootstrap service with exit status 0. This validates unit syntax/dependency availability in this environment, not successful execution or the future bootstrap binary. The repository and plan checks do not establish forwarding, cold-boot behavior, Grout hardware access, or device restoration; those remain required implementation validation.

## Required runtime validation refinement — 2026-11-05

The user requires a real systemd deployment and at least one executed test. The existing `make deploy-hostmode` target and `Systemd Router Restart Resiliency / recovers BGP sessions after FRR restart` test provide a concrete minimum lane after deployment is changed to use bootstrap. The current script's legacy Quadlet-copying flow cannot demonstrate the image bootstrap feature. Require nonempty passing test results and preserve deployment and test evidence; the planning-only checks above do not satisfy this runtime gate.
