# Bootstrap CLI and Lifecycle Contract

All commands below are proposed interfaces. `/bootstrap` is installed in both images while the existing default image entry point remains `/controller`.

## Commands

```text
/bootstrap validate --config /etc/openperouter/bootstrap.yaml --host-root /host
/bootstrap apply --config /etc/openperouter/bootstrap.yaml --host-root /host
/bootstrap teardown --previous-state /var/lib/openperouter/bootstrap/transition.json --transition-id <id> --host-root /host
/bootstrap probe --url http://127.0.0.1:9081/healthz --timeout 5s
```

Host-root and config paths are command-line arguments, not environment reads inside packages. All config, state, and artifact paths identify host paths; file access prefixes host-root after safe resolution. Host commands enter PID 1's mount/UTS/IPC/network namespaces. Run validate/apply with host network and PID access as shown in the unit. Probe runs in the runtime container's own network namespace without host-root access.

- `validate`: strict parse, domain and host/image/ownership preflight, deterministic staged rendering, real host generator validation; no runtime changes. Clean temporary staging on normal exit. May write sanitized diagnostics.
- `apply`: validate, nonblocking lock, complete generation publication, necessary reload/restarts/old-mode restoration, and health verification. Does not pull images. Deadline defaults to 300 seconds and follows YAML limits.
- `teardown`: internal transition command, invoked by apply in the **previous runtime image** with prior applied inventory. It requires the active transition identity, dedicated namespace/daemon ownership, and existing restoration state. The parent apply retains the host lock while invoking teardown; the child validates that active parent and transition identity and does not acquire a second lock. It cannot accept arbitrary namespace names or unrelated manifests. Reuse targeted cleanup and restoration, propagate failures, and do not delete the namespace.
- `probe`: bounded HTTP GET, success only on HTTP 2xx; no external network dependency. Default timeout 5 seconds. Used instead of image-specific wget/curl in generated health checks.

Success exits 0. Validation, conflict, unsupported capability, lock, publication, activation, timeout, and recovery failures exit nonzero with a stage and cause on stderr. Do not make consumers depend on exact numeric error subclasses. Apply also writes protected atomic result JSON; stdout/stderr never print full configuration or passwords.

## Host paths

| Path | Ownership/purpose |
|------|-------------------|
| `/etc/openperouter/bootstrap.yaml` | Administrator input, 0600. |
| `/etc/openperouter/bootstrap.env` | Administrator bootstrap image selection, 0600; `BOOTSTRAP_IMAGE` only. |
| `/etc/systemd/system/openperouter-bootstrap.service` | Initial administrator-installed root unit. |
| `/etc/containers/systemd/openperouter-bootstrap` | Bootstrap-owned discovery symlink to current generation's Quadlets. |
| `/var/lib/openperouter/bootstrap/generations/` | Root-private immutable complete generations. |
| `/var/lib/openperouter/bootstrap/current` | Atomic pointer to published generation. |
| `/var/lib/openperouter/bootstrap/state.json` | Published and successfully activated state/fingerprints. |
| `/var/lib/openperouter/bootstrap/transition.json` | Incomplete old-mode restoration/activation progress where needed. |
| `/var/lib/openperouter/bootstrap/runtime-inventory.json` | Controller-maintained ownership inventory for safe cleanup. |
| `/var/lib/openperouter/bootstrap/result.json` | Sanitized most recent outcome. |
| `/run/lock/openperouter-bootstrap.lock` | Nonblocking host process lock. |
| `/var/lib/openperouter/grout/device-state` | Durable Grout driver/address restoration records. |
| `/run/openperouter/grout` | Shared Grout runtime socket. |
| Existing FRR, CNI cache, hostbridge, startup gate paths | Preserve existing ownership and behavior; outside generated configuration. |

Never adopt legacy or foreign unit files by overwriting them. Preflight reports exact conflicts. Installation documentation supplies an explicit migration procedure for administrator review rather than having apply delete those files. Only bootstrap is enabled; generated service names are unavailable as independently enabled boot services.

## Runtime graph

Maintain `routerpod-pod.service`, `controllerpod-pod.service`, `controller.service`, `frr.service`, `reloader.service`, and existing volume identity. Grout adds `grout.service`.

The router pod creates/reuses `perouter` and brings up loopback. FRR and reloader share router namespace/PID/socket behavior. Grout joins that pod with the common socket; FRR loads its Grout plugin. Start Grout control readiness before starting the target controller; verify both router health and controller health. Preserve FRR PID-file watch behavior. The existing controller startup gate runs as host ExecStartPre and is covered by the apply deadline.

No generated unit has `[Install]` targets or `After/Requires=openperouter-bootstrap.service`. All generated Quadlets suppress implicit network dependencies with `[Quadlet] DefaultDependencies=false`. Do not disable ordinary systemd shutdown dependencies. Do not start only pod units and assume their member containers started: explicitly activate the full required service set.

## Option propagation

| Input | Required effective output |
|-------|---------------------------|
| deployment.logLevel | Generated node-config logLevel, controller/reloader verbosity, and existing FRR/Grout verbosity behavior; no conflicting node-level value. |
| deployment.healthProbePort | Controller `--health-probe-bind-address`, container health command, and bootstrap controller probe use the same port. |
| deployment.bgpListenLimit | Controller `--bgplistenlimit`. |
| deployment.controllerMaxReconcileBackoffSeconds | Controller `--max-reconcile-backoff` with an explicit seconds duration; omission does not emit a zero override. |
| deployment.ovsSocketPath | Controller `--ovssocket`; preserve Unix-address semantics. |
| deployment.ovsRunDir | Host directory bind exposes the configured socket at the same effective container path. |
| deployment.datapath | Controller `--datapath`, runtime service set, compatible FRR daemon configuration, and health dependencies. |
| deployment.grout.tapUnderlay | Controller `--grout-tap-underlay` selection and corresponding Grout underlay setup; never substitute testMode. |
| deployment.grout.maxMTU | Existing Grout maximum-MTU environment setting when positive; zero leaves daemon default. |
| deployment.grout.testMode | Existing Grout test-mode arguments/algorithms only when explicitly enabled. |

## Generation publication and retry

1. Obtain the lock, validate input/resources/local images, render into a root-private stage, and dry-run the installed generator with `QUADLET_UNIT_DIRS` pointing to staging. Expected services, syntax, dependencies, and absence of install links must pass before runtime mutation.
2. Finalize the immutable generation and ownership manifest. Publish through one atomic current-pointer rename. Initial discovery registration points to that already complete generation; interrupted first registration remains retryable. Do not daemon-reload a partially registered or unvalidated generation.
3. Reload systemd, apply the previously calculated changed/removed service set, and observe actual health. Inline routing/node changes require a controller restart even if unit bytes are identical. External directory edits remain hot reload.
4. On success, record effective running fingerprints and activated generation. On activation failure, retain complete artifacts and record partial progress. A matching hash never suppresses repair of failed/inactive units.

Only metadata and staged files may change on failed validation. Previous-image availability and restoration state are prerequisites for mode transitions. Retain generations referenced by published/applied/transition state; unreferenced staging can be cleaned. Do not implement a broad automatic cleanup service.

## Datapath and TAP/PCI transitions

A mode change can interrupt routing. Apply preflights the target and requires all cleanup candidates to be owned. Save source image/mode/inventory and target intent before stopping anything. Stop old controller, FRR, and reloader; retain the dedicated namespace and old Grout daemon while removing owned overlays/passthrough and restoring old underlay state. Run old-mode teardown with the previous image, including CNI setup for kernel underlays. Propagate every cleanup failure, retain records, and resume before changing modes. Stop old Grout only after restoration, publish the target generation, and start the new graph.

No blanket cleanup may touch unowned interfaces, VRFs, or administrator OVS bridges. Adapt selection-aware cleanup where existing name-based helpers cannot establish ownership. A missing inventory or unavailable previous image fails before starting a transition. Automatic rollback is not promised.
