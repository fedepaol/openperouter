# Data Model: Image-Based Systemd Bootstrap

This is an administrative file contract, not a new Kubernetes API. Exact input details are in [configuration.md](contracts/configuration.md).

## Bootstrap configuration

- `version`: required integer `1`; reject unsupported versions.
- `deployment`: required deployment settings, image, optional Grout settings, and startup timeout. The seven overlapping operator fields retain their CRD names and validation rules.
- `node`: required node configuration. `nodeIndex.index` is an optional integer in the input model so explicit zero is distinguishable from omission. Exactly one of index/interfaceName is supplied. Optional nodeName is preserved. Logging belongs only in deployment; generate the node logLevel from it.
- `routing`: required exclusive selection of embedded `inline` static configuration or administrator-owned `directory`. Reuse `api/static.PERouterConfig` and its existing underlay/VNI/VPN/passthrough/raw FRR semantics, including static authentication data.

Unknown/duplicate keys fail. A supplied null value is not an omitted field. Node index is nonnegative and must fit the supported host index type; interface CIDR requires interfaceName. Validate actual selected interface/address in host networking, retaining the source expression for future boots.

## Effective deployment

Resolved defaults, local image ID, selected datapath and underlay mode, namespace identity, node identity, config source, component arguments, mounts, probes, dependency order, and startup deadline. This object is the only input to templates. Do not read environment variables or decide runtime behavior inside templates.

One runtime image supplies controller, FRR, reloader, and optional Grout. A separate bootstrap image executes installation; changing it does not inherently restart runtime services. The selected image must contain the expected binaries, plugins, and compatible bootstrap contract.

## Artifact generation

Identity: SHA-256 of canonical effective deployment and ordered generated file bytes; exclude timestamps and result metadata. Stable logical paths reference `current`, not their own digest, preventing recursive hashing. For external routing, include directory identity in the generation and validate current file contents, but do not include mutable file contents in deployment fingerprints. Their edits belong to existing runtime reload.

Contents: `quadlets/`, `node/node-config.yaml`, optional `routing/openpe_bootstrap.yaml`, optional Grout FRR daemon configuration, and `manifest.json`. Preserve deterministic filenames and ordering. Root-owned generation directories are 0700; configuration/manifest files are 0600; unit sources may be 0644 within the protected directory. Do not store secrets in unit environment lines or diagnostic JSON.

A `current` symlink identifies the published complete generation. An atomic pointer rename is the publication boundary. Existing bind mounts continue using their original resolved sources until affected services restart.

## Ownership manifest and running inventory

Record schema version, generation ID, component fingerprints, required/removed service names, Podman container and pod identities, local runtime image ID, datapath/mode, generated discovery link, config source, and reserved namespace identity. All managed paths must stay under the designated roots; reject traversal, escaping symlinks, and foreign ownership.

Record restoration state separately from generated configuration. For transition cleanup, capture exact owned namespace, interfaces, VRFs, VNIs, OVS attachments, and devices. Controller integration updates managed network inventory after reconciliation, including later API-managed resources. Before destructive cleanup, compare observed candidates with inventory and existing ownership markers. Fail on unowned objects instead of assuming a matching name proves ownership. Never use an unrestricted cleanup helper against a shared namespace or daemon.

Durable host device records live at `/var/lib/openperouter/grout/device-state` and are mounted at the existing controller path `/var/run/grout/device-state`. Preserve original driver, addresses, port identity, and restoration progress using existing devicestate types.

## Applied state and transition state

`state.json`: published generation, last successfully activated generation, actual component fingerprints/image IDs, and previous-image reference needed for recovery.

Optional transition record: source generation/image/mode/inventory, target generation, stage, services stopped, remaining restoration actions, and failure cause. Transitions are strictly local to one managed deployment.

Stages: `preflight`, `render`, `validate-artifacts`, `teardown`, `publish`, `reload`, `activate`, `health`, `complete`, `failed`. Persist transition intent before stopping a service. Restoration state is removed only after restoration succeeds. A retry resumes unfinished old-mode restoration before publishing/activating its target; a different requested target must not discard an unfinished transition.

## Result

Atomic `result.json`: schema version, success/failure, stage, source/target generation, changed and already-activated services, failed service where relevant, sanitized cause, and start/finish times. JSON is diagnostic output; exit status is authoritative for systemd.

Results and lock files may change on failed preflight; managed runtime artifacts and services may not. Bootstrap logging remains enabled for actionable failures even if deployment.logLevel is none.

## Lifecycle invariants

1. One apply/teardown operation owns the nonblocking host lock.
2. No managed runtime mutation precedes complete input, host, image, and generated-artifact validation.
3. A published generation is always complete; publication and successful activation are distinct states.
4. Unchanged input/local image IDs plus healthy matching components produce no restarts.
5. A failed/inactive component is repaired even if its artifact fingerprint is unchanged.
6. Only bootstrap starts automatically at boot; no runtime dependency waits for bootstrap completion.
7. External routing input and unrelated host objects remain administrator-owned.
8. No automatic service rollback or deletion of restoration records occurs after a partial activation failure.
