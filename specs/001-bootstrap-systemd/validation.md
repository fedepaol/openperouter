# Bootstrap Implementation Validation

Status: implementation in progress. Planning checks do not count as runtime proof.

## Team and review

Three developers: gpt-6-luna, xhigh reasoning. Independent reviewer: gpt-6.1-sol, fresh context containing only feature description and review assignment. Review findings and developer fixes are recorded below as they occur.

## Required systemd deployment and test

- Image/build identity: kernel image built with `make docker-build`; first deployment image `sha256:95ac874bb5db6253292e522ad389e1005e05f72daf6f4238ca8248e43e8f19e7`. A final rebuild remains required after current changes.
- Bootstrap-based systemd deployment: attempted on the three-node `pe-kind` cluster; first node reached active controller/FRR/reloader services. Bootstrap completion and remaining nodes are pending.
- Bootstrap active/exited and runtime units: not verified.
- Focused BGP restart e2e command: not executed.
- Executed/passed test counts and report path: not available.

## Other required validation

- Unit tests and lint: pending.
- Repeat apply and failure recovery: pending.
- Offline kernel boot/forwarding: pending.
- Production Grout TAP: pending.
- Production Grout PCI and driver restoration: pending; requires prepared hardware.

## Review findings and fixes

Three Sol review passes reported 14 findings. Developers fixed the reported host-path escape, subprocess timeout/stderr/cwd issues, incomplete activation recovery, invalid passthrough omission, CID cleanup, installation path, Podman object ownership, production TAP reapply, nested Quadlet conflicts, initial PCI VFIO check, empty-routing Grout resource check, and FRR SELinux bind. Fourth-pass verification is pending. Focused bootstrap/CLI/logging/systemd packages passed after those changes. Inventory and transition work continues.

## Deployment attempts

- The kernel image builds, contains bootstrap/controller/reloader/FRR/CNI, provides util-linux nsenter, and preserves ENTRYPOINT /controller.
- The initial lab attempt hit pre-existing orphaned Docker endpoints. Only endpoints belonging to the requested lab were detached after verifying its replacement containers had not started.
- Kind then failed its console-log readiness expression although systemd reached multi-user.target. Retained-node inspection proved one console line was missing. A local derivative of the same node image adds SYSTEMD_LOG_LEVEL=info and SYSTEMD_LOG_TARGET=console. The three-node cluster then became ready. Environment image: openperouter-bootstrap-kind:validation.
- A duplicate image export exhausted /tmp quota. Docker reports sha256-prefixed IDs while Podman reports bare IDs; both comparisons and preflight now normalize these. Bootstrap exports honor TMPDIR. Verified completed task exports were removed.
- Bootstrap activation and the executed e2e gate remain pending. Cluster creation does not satisfy acceptance.
- Docker cp preserved the invoking user's UID on configuration and routing files. Bootstrap rejected these files before mutation; the installer now explicitly assigns root ownership and protected configuration permissions.
- First runtime graph started on `pe-kind-worker2`, but nested host Podman health probes failed because nsenter retained the bootstrap container's cgroup namespace. The same probe succeeds with `--cgroup`; host execution now joins PID 1's cgroup namespace. A rebuilt image must verify this fix.
- The fourth independent review verified the earlier fixes and found inherited Quadlet drop-ins and missing `kind` prerequisites. Both are fixed. A later pass found transition target changes after partial activation, target-generation verification after source teardown, and kernel-owned moved NIC rejection during Grout preflight. The first two are fixed; moved NIC inventory/preflight integration is in progress.

## Focused checks during integration

- `go test ./internal/bootstrap ./cmd/bootstrap ./internal/runtimeinventory ./internal/systemdctl ./internal/logging`: passed after the cgroup namespace, protected lock, renderer inventory bind/readiness, and CLI integration changes. Controller inventory hooks and further transition changes still require rerunning these checks.
- `go test ./pkg/systemd ./systemd_static_suite -run '^$'` from `e2etests`: compiled successfully. This compilation does not count as executed e2e acceptance.
- Focused lint found issues in the new inventory/teardown/transition code; developers are correcting them. Full repository gates remain pending.

## Transition review and regression progress

- Sol reviewed apply/store/transition, protected kernel source-inventory preflight, CID child cleanup, and new e2e helpers. It reported two P1 issues: kernel original-device state rejected by teardown, and an inventoried OVS port moved to a foreign bridge being deleted after its original bridge vanished. Developer fixes and regression tests are in progress.
- Targeted transition/cancellation/finalization tests passed. New kernel inventory preflight regressions passed for owned moved NICs, namespace replacement, missing/unowned records, MTU limits, and PCI resource constraints. These are unit checks, not hardware transition evidence.
- Parent-lock tests exposed an environment-specific filesystem device mismatch: stat reported device minor 50 while `/proc/locks` reported underlying device minor 32 for the same held lock and inode. The fix must retain exact descriptor/file identity checks rather than weaken validation to inode alone.
- The subsequent image rebuild copied a temporary intermediate teardown edit and failed compilation. No new deployment success is claimed from that build. Final builds wait for a consistent source snapshot.
