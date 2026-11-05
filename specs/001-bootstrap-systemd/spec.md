# Feature Specification: Image-Based Systemd Bootstrap

**Feature Branch**: `main` (existing branch; no branch hook configured)

**Created**: 2026-11-04

**Status**: Draft

**Input**: User description: "I'd like to have an entry point in the image that generates the systemd units and the configuration. Something where I can have a systemd unit that run once which in turns generates the real systemd units and starts them. The benefit is that I can then provide configuration knobs similar to what I do with the operator. Tell me also if it's doable."

## Clarifications

### Session 2026-11-04

- Q: How should administrators supply bootstrap settings? → A: One YAML file containing deployment settings and node configuration, with embedded or referenced routing definitions.
- Q: Should initial bootstrap support only kernel datapath or both kernel and Grout? → A: Support both kernel and Grout datapaths, selected through YAML configuration.
- Q: Which Grout underlay modes must initial release support? → A: Both TAP and PCI modes, selected through YAML.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Bootstrap a Host from Configuration (Priority: P1)

A host administrator supplies configuration and enables one bootstrap systemd unit. That unit invokes an entry point shipped in the OpenPERouter image, generates the runtime service definitions and configuration, and starts the services. The administrator does not maintain separate service definitions for each component.

**Why this priority**: This is the requested installation flow and removes manual coordination of deployment files.

**Independent Test**: On a prepared host without runtime units, supply valid configuration and start only the bootstrap unit. Verify that the controller, router, and reloader start and establish static routing without Kubernetes access.

**Acceptance Scenarios**:

1. **Given** a prepared host and valid configuration, **When** the administrator starts the bootstrap unit, **Then** the runtime definitions and configuration are generated and all required services start without manual file edits or separate start commands.
2. **Given** successful bootstrap, **When** the image entry point finishes, **Then** no bootstrap process remains running and systemd independently supervises the runtime services.
3. **Given** an enabled bootstrap unit and locally available images, **When** the host reboots without Kubernetes or registry access, **Then** static routing becomes operational automatically using the current bootstrap configuration.

---

### User Story 2 - Configure Host Runtime Knobs (Priority: P1)

An administrator configures host-relevant options familiar from the operator in one bootstrap input rather than editing generated units. Existing node identity and static routing definitions remain usable.

**Why this priority**: Configuration flexibility is the main benefit of generating service definitions from user intent.

**Independent Test**: Bootstrap with non-default supported settings and verify their effective values in running services and health checks. Repeat with omitted optional settings and verify documented defaults.

**Acceptance Scenarios**:

1. **Given** explicit logging verbosity, controller health port, BGP listen limit, maximum reconciliation backoff, OVS socket path, and OVS run directory, **When** bootstrap succeeds, **Then** the runtime components and their health checks consistently use those settings.
2. **Given** only required settings, **When** bootstrap runs, **Then** optional settings use documented defaults consistent with existing host behavior. Overlapping operator options retain their meaning and validation rules.
3. **Given** a static or interface-derived node index and supported static routing definitions, **When** bootstrap runs, **Then** it generates configuration compatible with the existing host configuration model and applies those definitions.
4. **Given** an unknown field, invalid value, conflicting input sources, mutually exclusive node settings, or unsupported deployment option, **When** bootstrap runs, **Then** it identifies the offending setting and fails before changing managed artifacts or running services.
5. **Given** either supported datapath is selected in YAML, **When** bootstrap succeeds on a prepared host, **Then** the corresponding runtime services start, pass their health checks, and establish the configured static routing.
6. **Given** Grout is selected with either TAP or PCI underlay mode and the host meets that mode's prerequisites, **When** bootstrap succeeds, **Then** Grout and the controller use the selected mode and establish the configured static routing.
7. **Given** the host lacks resources or device access required by the selected Grout mode, **When** bootstrap runs, **Then** it reports the missing prerequisite before changing managed artifacts or running services.

---

### User Story 3 - Reapply Configuration Safely (Priority: P2)

An administrator reruns bootstrap after changing configuration or selecting a new runtime image. Reapplying unchanged input leaves healthy services undisturbed. Changed settings take effect without editing generated files.

**Why this priority**: Hosts must remain maintainable after installation and across subsequent boots.

**Independent Test**: Apply valid configuration twice, then change one setting and reapply. Compare generated content, service identities, restart counts, and effective settings.

**Acceptance Scenarios**:

1. **Given** healthy services matching the input, **When** bootstrap runs again with unchanged input and images, **Then** generated content remains identical, no duplicate resources appear, and healthy services do not restart.
2. **Given** valid changed settings, **When** bootstrap runs again, **Then** affected artifacts are updated and affected services adopt those settings while unrelated administrator files remain unchanged.
3. **Given** an explicit new runtime image reference, **When** bootstrap succeeds, **Then** the intended runtime components use that image without manual edits to service definitions.

---

### User Story 4 - Diagnose and Recover from Failure (Priority: P2)

An administrator can distinguish input, generation, and service startup failures, correct their causes, and retry the same operation.

**Why this priority**: Failed installation must be actionable and must not appear successful when services are incomplete.

**Independent Test**: Exercise invalid input, missing prerequisites, an unwritable destination, a conflicting administrator file, and a runtime startup failure. Verify reporting and retry after correction.

**Acceptance Scenarios**:

1. **Given** invalid input or missing prerequisites, **When** bootstrap runs, **Then** it reports failure and its cause while leaving existing managed artifacts and services unchanged.
2. **Given** failure while generating or publishing artifacts, **When** bootstrap stops, **Then** it does not activate a partial artifact set and identifies the failed stage.
3. **Given** complete artifact generation but a runtime startup failure, **When** bootstrap finishes, **Then** it reports failure, identifies the affected service and any services already changed, and preserves diagnostic information.
4. **Given** a corrected generation or startup failure, **When** bootstrap runs again, **Then** it reaches the desired state without manual cleanup or duplicate resources.

### Edge Cases

- Missing, unreadable, malformed, or unsupported configuration.
- A generated destination conflicts with an administrator-owned file.
- Bootstrap is interrupted or two invocations overlap.
- Services are healthy, inactive, or failed when bootstrap is reapplied.
- A selected image is unavailable locally and cannot be fetched.
- Grout is selected but compatible images or mode-specific host resources or device access are unavailable; prerequisite validation must fail before managed state changes.
- TAP and PCI mode selections must remain consistent between the controller and Grout service.
- Kubernetes credentials are absent initially and appear later.
- An optional controller startup gate delays startup beyond the documented deadline.
- A changed health port or OVS path requires matching service arguments, health checks, and host access.
- Existing generated definitions are present at boot while the bootstrap input has changed; services must not race ahead with stale settings.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The image MUST expose an explicitly invocable bootstrap entry point while preserving existing image invocation behavior.
- **FR-002**: The feature MUST provide a documented one-shot bootstrap systemd unit that invokes the image entry point and causes generated runtime services to start. Successful bootstrap MUST leave no persistent bootstrap process.
- **FR-003**: Administrators MUST be able to supply one YAML bootstrap file containing deployment settings and node configuration, with embedded or referenced static routing definitions without editing generated units. Static routing definitions MAY remain in existing administrator-maintained files referenced by that input.
- **FR-004**: Bootstrap MUST generate the definitions and configuration required by the existing host deployment, including the controller, router, reloader, and supporting resources. Selecting Grout MUST additionally generate and start the Grout service and configure its integration with the controller and router; selecting kernel MUST NOT start a Grout service. It MUST preserve existing namespace, shared state, health checking, restart, and optional startup gate behavior.
- **FR-005**: Supported deployment knobs MUST include datapath selection (`kernel` or `grout`, defaulting to `kernel`), image reference, logging verbosity, controller health port, BGP listen limit, maximum reconciliation backoff, OVS socket path, and OVS run directory. Overlapping operator options MUST retain the same documented field names, meanings, and permitted values. Host-only settings MUST be documented separately.
- **FR-006**: Optional settings MUST have documented defaults preserving existing host behavior. Explicit bootstrap values MUST apply consistently to configuration, service arguments, mounts, and health checks. Conflicting input sources MUST fail validation rather than silently override each other.
- **FR-007**: Generated node and routing configuration MUST preserve existing host configuration semantics, including both node index methods and their mutual exclusion. Existing administrator-maintained static routing files MUST remain usable without rewriting their routing definitions.
- **FR-008**: Before changing managed artifacts or running services, bootstrap MUST validate all input, required host capabilities, destination access, ownership conflicts, and availability of selected images. Unknown fields and unsupported options MUST fail explicitly.
- **FR-009**: Bootstrap MUST make generated definitions available to systemd and start required services in dependency order. Success MUST mean all required services have started and their existing health checks pass within a documented finite startup deadline.
- **FR-010**: With bootstrap enabled and images available locally, host reboots MUST restore the desired deployment without registry or Kubernetes access. Startup MUST apply current bootstrap input before runtime services use generated definitions and MUST avoid cyclic dependencies.
- **FR-011**: Applying unchanged input and images MUST produce identical generated content, no duplicate resources, and no restarts of healthy services. Applying changed input MUST update affected artifacts and activate changed settings through necessary reloads or restarts.
- **FR-012**: Bootstrap MUST distinguish generated artifacts from administrator-owned files, refuse to overwrite conflicting administrator-owned files, and preserve unrelated files and services. Removed or renamed managed artifacts MUST NOT leave obsolete managed services active after successful reapplication.
- **FR-013**: Input and prerequisite failures MUST leave existing managed artifacts and services unchanged. Generation or publication failure MUST NOT activate a partial artifact set. Concurrent invocations MUST NOT interleave updates or activation; a conflicting invocation MUST fail clearly.
- **FR-014**: Every failure MUST produce an unsuccessful exit status and identify the failed stage and relevant setting, path, or service. Activation failures MUST identify services already changed and preserve diagnostics for retry. Automatic rollback of partially activated services is not required.
- **FR-015**: Static startup MUST NOT require Kubernetes access. Existing later integration with Kubernetes and runtime reload of static routing files MUST continue to work.
- **FR-016**: Documentation MUST cover prerequisites, bootstrap unit setup, knobs and defaults, ownership, startup deadline, reapplication, reboots, and recovery. It MUST include one minimal and one customized input example, document the operator field mapping below, and distinguish CRD fields from additional operator deployment settings.
- **FR-017**: Grout deployments MUST support both TAP and PCI underlay modes selected through YAML, using the existing `grout.tapUnderlay` setting and its default (`false`, selecting PCI). Bootstrap MUST apply that selection consistently to Grout service configuration and controller settings, validate mode-specific host resource and device access prerequisites before managed state changes, and document those prerequisites.


- **FR-018**: Implementation validation MUST include an actual systemd-based deployment created through the image bootstrap entry point and at least one executed, passing routing or recovery acceptance test against that deployment. Static artifact checks, skipped tests, and a deployment created only from legacy hand-installed units MUST NOT satisfy this requirement.

**Operator deployment option mapping**:

The operator CRD defines 12 deployment fields. The bootstrap configuration preserves the following seven host-relevant fields and their validation semantics:

| Operator CRD field | Bootstrap behavior and default |
|--------------------|--------------------------------|
| `logLevel` | Accept `all`, `debug`, `info`, `warn`, `error`, or `none`; default `info`. |
| `ovsSocketPath` | Configure the OVS database socket; omitted values retain the existing host default, `unix:/var/run/openvswitch/db.sock`. |
| `ovsRunDir` | Configure the mounted OVS run directory; default `/var/run/openvswitch`. |
| `healthProbePort` | Configure controller health and readiness checks; integer 1–65535, default 9081. |
| `bgpListenLimit` | Configure the maximum accepted dynamic BGP sessions; integer 1–65535, default 65535. |
| `controllerMaxReconcileBackoffSeconds` | Configure maximum API reconciliation retry delay in seconds; integer at least 1 when supplied. Omission retains the controller default rate limiter; explicit zero is invalid. |
| `datapath` | Accept `kernel` or `grout`; default `kernel`. |

The remaining five fields are excluded from the initial bootstrap contract:

| Operator CRD field | Reason |
|--------------------|--------|
| `nodeSelector` | Kubernetes node scheduling; a bootstrap unit configures its own host. |
| `tolerations` | Kubernetes scheduling against node taints. |
| `affinity` | Kubernetes pod scheduling. |
| `priorityClassName` | Kubernetes pod priority; no automatic mapping to systemd priority is required. |
| `resources` | Kubernetes per-container requests and limits for controller, nodemarker, FRR, and reloader. Host resource controls require a separate contract and remain outside the initial scope. |

Image selection and detailed Grout controls are not fields of the operator CRD. The operator also uses deployment environment settings: `CONTROLLER_IMAGE`, `FRR_IMAGE`, `GROUT_IMAGE`, `GROUT_TAP_UNDERLAY`, `GROUT_TEST_MODE`, and `GROUT_MAX_MTU`. Bootstrap image selection and TAP/PCI selection are already required above. Planning must account for component image compatibility and assess the additional Grout controls separately from CRD parity; Kubernetes monitoring and proxy deployment settings do not imply host deployment requirements.

For reference, the existing operator defaults Grout test mode to disabled and treats maximum MTU zero as using the Grout default. TAP mode and test mode are distinct: choosing TAP does not imply test mode or eliminate production resource prerequisites. Routing acceptance must exercise normal forwarding, rather than rely solely on test mode.

Sources checked: [operator CRD](../../operator/config/crd/bases/network.openperouter.io_openperouters.yaml), [operator API types](../../operator/api/v1alpha1/openperouter_types.go), [deployment value mapping](../../operator/internal/helm/chart.go), and [operator environment settings](../../operator/internal/envconfig/envconfig.go).

### Key Entities *(include if feature involves data)*

- **Bootstrap input**: One administrator-maintained YAML file containing desired deployment settings, node configuration, and embedded or referenced static routing definitions.
- **Bootstrap unit**: The administrator-enabled one-shot service initiating the desired host deployment through the image entry point.
- **Managed artifact set**: Generated service definitions and configuration whose ownership is distinguishable from administrator files.
- **Runtime service set**: Controller, router, reloader, optional Grout service selected by datapath, and supporting resources supervised after bootstrap exits.
- **Bootstrap result**: Success or failure, the failed stage where applicable, and information about affected artifacts and services.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a prepared host, an administrator establishes configured static routing by supplying one bootstrap input and starting one bootstrap unit, with zero manual edits to generated artifacts.
- **SC-002**: For the kernel datapath and for Grout in both TAP and PCI modes, every supported deployment knob has a passing acceptance case demonstrating its effective value; every omitted optional knob has a passing case demonstrating its documented default.
- **SC-003**: Three consecutive reapplications of unchanged input and images produce zero duplicate runtime instances, zero generated content differences, and zero restarts of healthy services.
- **SC-004**: Three consecutive reboots with images preloaded and Kubernetes and registry access unavailable restore static routing without administrator intervention within the documented startup deadline, for each supported datapath.
- **SC-005**: All invalid-input and missing-prerequisite acceptance cases report an actionable cause and produce zero changes to existing managed artifacts or running services.
- **SC-006**: After correction of each tested generation or activation failure, one bootstrap retry reaches the desired running state without manual cleanup or duplicate resources.

- **SC-007**: At least one routing or recovery acceptance test executes and passes against a running systemd deployment created by bootstrap, with deployment evidence and test results recorded. Zero selected, skipped-only, or failed test runs do not meet this outcome.

## Assumptions

- Initial support targets container-based systemd host deployment with both kernel and Grout datapaths selected through YAML configuration. Grout supports both TAP and PCI underlay modes. Native host installation is separate work.
- Hosts already provide systemd, Podman with the required Quadlet capabilities, administrative permissions, and required networking dependencies, including OVS where applicable. Bootstrap does not install host packages.
- An administrator provisions and enables the initial bootstrap unit and makes its image available. Image acquisition must not depend on the overlay being bootstrapped.
- The entry point and bootstrap unit may divide generation and host service control responsibilities; that division is a planning decision. The one-shot operation owns the complete user-visible outcome.
- Bootstrap runs once per boot or explicit administrator reapplication, not only once in a host's lifetime. It does not continuously reconcile deployment settings.
- Operator parity covers the explicitly listed host-relevant knobs. Kubernetes scheduling, pod priority, resource requests and limits, and cluster-side deployment are outside scope.
- Existing static routing semantics and file reload remain authoritative. This feature generates deployment wiring and input configuration; it does not introduce another routing model or reconciliation loop.
- Only one managed OpenPERouter deployment per host is required.
- The startup deadline and exact YAML field structure will be defined during planning and documented before implementation acceptance. No new throughput target is introduced.
- The repository constitution currently contains an unfilled template with no adopted principles. No specification extension hooks are configured.
