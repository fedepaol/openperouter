# OpenPerOuter E2E on OpenShift

Run the OpenPerOuter E2E test suite against an OCP cluster deployed via dev-scripts.

## Prerequisites

- RHEL server with OCP installed via dev-scripts
- dev-scripts config must include:
  ```bash
  export IP_STACK=v4v6
  export EXTRA_NETWORK_NAMES="toswitch1 toswitch2"
  export TOSWITCH1_NETWORK_SUBNET_V4='192.168.11.0/24'
  export TOSWITCH1_NETWORK_SUBNET_V6='2001:db8:11::/64'
  export TOSWITCH2_NETWORK_SUBNET_V4='192.168.12.0/24'
  export TOSWITCH2_NETWORK_SUBNET_V6='2001:db8:12::/64'
  ```
- OpenPerOuter operator + FRR-K8s deployed (see `OCP_CI_STATUS.md` Phase 2)
- `routingViaHost: true` applied via CNO patch
- containerlab installed
- Go installed (for building config generators and test binaries)

## Usage

```bash
export KUBECONFIG=/root/dev-scripts/ocp/ostest/auth/kubeconfig

# 1. Set up the clab fabric
bash openshift/e2e/setup-clab.sh

# 2. Build hostvalidator (must be static)
CGO_ENABLED=0 go test -c -tags=externaltests -o bin/validatehost ./internal/hostnetwork

# 3. Run the E2E tests
cd e2etests
CONTAINER_RUNTIME=podman go test -count 1 -v -timeout 180m ./suite/ \
  --nodelink-config=/root/openperouter/openshift/e2e/nodelink.json \
  --frrk8s-namespace=openshift-frr-k8s \
  --hostvalidator=/root/openperouter/bin/validatehost \
  -ginkgo.v \
  -ginkgo.label-filter='!systemdmode' \
  -ginkgo.skip='editing the underlay parameters|auto-recover when the named netns is deleted|Webhook|Unnumbered'

# 4. Tear down (optional)
containerlab destroy --runtime podman --topo openshift/e2e/ocp.clab.yml
```

## Documentation

- `OCP_CLAB_SETUP.md` — step-by-step justification of setup-clab.sh, mapped against upstream kind equivalents
- `OCP_CI_STATUS.md` (repo root) — full status, test results, architecture differences, root cause analysis
