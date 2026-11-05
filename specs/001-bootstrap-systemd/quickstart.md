# Quickstart and Validation Guide

Use a disposable prepared host or the project's development environment. The implementation validation record is [validation.md](validation.md); unexecuted hardware checks do not count as forwarding proof.

## Prerequisites

- Rootful systemd/Podman host with cgroup v2 and the required Quadlet capabilities.
- Locally loaded images built from this feature, including `/bootstrap`; registry images predating implementation do not provide that executable.
- Valid static routing definitions and matching underlay connectivity. Kernel mode needs its ordinary network prerequisites; production Grout additionally needs prepared host resources and TAP or PCI access.
- No conflicting manually installed OpenPERouter units or foreign objects in the reserved namespace. Existing installations require an explicit administrator migration, not silent overwrite.
- Run actual deployment through the deploy skill. The combined hostmode+Grout development lane is `make deploy-hostmode-grout-smoke`; standalone `grout-deploy` uses Kubernetes.

## Minimal kernel configuration

Prepare `/etc/openperouter/routing/` with valid existing `openpe_*.yaml` files for this host. The static suite's topology-specific samples live in `e2etests/systemd_static_suite/testdata/`; use matching node links and addresses rather than copying them to an unrelated host. Retain the directory itself when replacing individual files so directory watching remains active.

Create `/etc/openperouter/bootstrap.yaml` with mode 0600:

```yaml
version: 1
deployment:
  image: quay.io/openperouter/router:bootstrap-dev
node:
  nodeIndex:
    index: 0
routing:
  directory: /etc/openperouter/routing
```

Use a unique node index on each host. An inline alternative replaces routing.directory with routing.inline containing the same existing static configuration object; the two sources cannot be combined. See [configuration contract](contracts/configuration.md) for the customized Grout example, all defaults, and validation rules.

Load the locally built image into this host's Podman store. Set bootstrap image selection separately in `/etc/openperouter/bootstrap.env` (mode 0600):

```text
BOOTSTRAP_IMAGE=quay.io/openperouter/router:bootstrap-dev
```

The bootstrap image and deployment.image may be the same. For Grout, use a compatible locally built Grout runtime image for all runtime components. Local images must remain available for offline reboot and transition recovery.

## Validate before activation

On the prepared host:

```shell
sudo podman run --rm --pull=never --privileged --pid=host --network=host \
  --security-opt=label=disable \
  --mount=type=bind,src=/,dst=/host,bind-propagation=rslave \
  --entrypoint=/bootstrap quay.io/openperouter/router:bootstrap-dev \
  validate --config=/etc/openperouter/bootstrap.yaml --host-root=/host
```

Expected: exit 0 after strict input, host/image, and generated-unit validation; no managed runtime files or services change. Missing images, malformed configuration, legacy-unit conflicts, or unsupported device resources produce a nonzero exit and an actionable stage/cause.

## Install and start the one-shot unit

From the repository root, after the required host inputs exist:

```shell
sudo install -m 0644 systemdmode/openperouter-bootstrap.service \
  /etc/systemd/system/openperouter-bootstrap.service
sudo systemctl daemon-reload
sudo systemctl enable --now openperouter-bootstrap.service
sudo systemctl status openperouter-bootstrap.service --no-pager
sudo systemctl is-active controller.service frr.service reloader.service
sudo cat /var/lib/openperouter/bootstrap/result.json
```

Expected: bootstrap is active/exited, runtime components are active, result reports complete/success, and the short-lived bootstrap container is gone. Runtime processes remain supervised by their own units. In Grout mode, also require grout.service and Grout control readiness. Do not enable generated runtime units.

Check host controller health at its configured port and existing FRR/reloader health inside the router namespace. Confirm actual route installation and packet forwarding with the static-suite topology checks; HTTP health alone does not prove routing convergence.


## Required systemd deployment and one-test gate

Before reporting runtime validation complete, perform an actual hostmode deployment through the new bootstrap entry point and run at least one passing e2e test. This is the minimum executable gate in addition to the broader acceptance matrix. It is not satisfied by generation dry runs or legacy-unit deployments.

Use the deploy skill in hostmode. These commands build the image and exercise the bootstrap flow:

```shell
make docker-build
make deploy-hostmode
```

`systemdmode/deploy.sh` preloads images, installs/enables the one-shot bootstrap unit, and invokes the image entry point on the kind nodes. It reports the saved result and journal on failure. Use fresh disposable nodes to avoid accidentally passing against an older installation. Hostmode here means systemd inside the development nodes, with a hostbridge connection for the API-managed recovery test.

On every deployed node, record bootstrap success from result.json, verify bootstrap is active/exited, verify controller/FRR/reloader units are active, and record the resolved runtime image ID. Then use the e2e-tests skill, focusing on the existing restart recovery test. Its concrete command for the kernel hostmode lane is:

```shell
make e2etests TEST_ARGS="--systemdmode" \
  GINKGO_ARGS='--label-filter=systemdmode --focus="Systemd Router Restart Resiliency.*recovers BGP sessions after FRR restart" --fail-on-empty'
```

This test establishes BGP sessions, restarts the router through systemd on every node, and requires those sessions to reestablish. The e2e skill must retain applicable mode exclusions when constructing the final command. Exactly this test, or another explicitly documented routing/recovery acceptance test, must actually execute and pass; zero selected tests, skipped-only results, or a failing run do not count.

Keep the deployment command, image ID, node service evidence, final test command, executed/passed counts, and `e2e-report.json` artifact path in the implementation validation report. If deployment or the test fails, investigate using the required triage workflow and rerun after correction. Do not report this gate as complete while it remains unexecuted.

## Repeatability and changed settings

Record `readlink /var/lib/openperouter/bootstrap/current`, each runtime unit's MainPID, InvocationID, NRestarts, and the contents of the generation manifest. Run the following three times and compare:

```shell
sudo systemctl restart openperouter-bootstrap.service
```

Expected: identical generation content, no duplicate resources, unchanged healthy runtime invocations. A fresh bootstrap invocation is expected; runtime invocations must not change.

Set healthProbePort to 9181, restart bootstrap, and verify both runtime arguments and health checks use 9181. Verify only affected components restart. Change an existing external routing file by atomic file replacement and confirm reconciliation without bootstrap or controller restart. Change inline routing through YAML and confirm reapplication changes the managed configuration and controller.

For all six logging levels, bootstrap valid input and confirm process startup. Check `all` emits supported verbose logs and `none` suppresses runtime logs while bootstrap still reports failures.

## Failure and recovery scenarios

On a disposable host, exercise unknown/duplicate keys, conflicting index/interface settings (including explicit index zero), explicit backoff zero, malformed routing, unavailable image, unwritable artifact destination, foreign unit or network ownership, and a held bootstrap lock. Compare managed generation and runtime identities before/after: validation failures change neither.

Inject generation/publication failures in integration tests and verify no partial generation activation. Cause a required runtime service to fail startup, inspect result.json and journal, correct the cause, and rerun bootstrap once. Matching artifact content must not prevent recovery of an inactive or failed service. Delay the existing startup gate beyond a short configured timeout and require a bounded actionable failure.

Datapath/mode transition tests are disruptive and belong on disposable hosts. Test kernel-to-Grout, Grout-to-kernel, TAP-to-PCI, and PCI-to-TAP. Inject interruption during device restoration, retry, and prove original drivers/addresses recover before target activation. Preserve administrator bridges and unrelated interfaces. Missing prior image or ownership inventory must reject a transition before stopping services.

## Offline boot and production matrix

After preloading images and verifying connectivity, make registry and Kubernetes endpoints unavailable using the isolated test environment. Reboot the host three times per production lane; external test orchestration must observe disappearance/reappearance of host identity and reestablishment of static forwarding. No unit may wait for Kubernetes or network-online through its own overlay.

| Lane | Required proof |
|------|----------------|
| Kernel | Static routing, changed defaults, live reload, idempotence, restart recovery, three offline reboots. |
| Grout TAP, testMode false | Grout/FRR integration and packet forwarding with production resources, plus the same lifecycle proof. |
| Grout PCI, testMode false | Hardware forwarding, applicable VFIO/bifurcated driver behavior, restoration, mode transitions, restart recovery, and offline reboots. |
| Grout testMode true | Optional cheap smoke checks only; never substitutes for production TAP/PCI acceptance. |

## Repository gates

Run the repository gates:

```shell
make test
make lint
```

Use the e2e-tests skill for existing Ginkgo suites after deployment. Combined hostmode+Grout detection inspects the running controller arguments and supplies both `--systemdmode` and `--groutmode`, with a single compatible label-filter expression. Keep the static-before-Kubernetes suite and existing resiliency coverage. The current flags already exist; deployment and filtering must select the combination correctly.

Use the e2etriage skill if those tests fail. Hardware-unavailable PCI checks remain explicitly unverified and block claiming complete PCI support.
