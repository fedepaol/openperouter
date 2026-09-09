#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Run e2e tests ==="

pushd "$SCRIPT_DIR"/../..

CONTAINER_RUNTIME=podman make e2etests \
	TEST_ARGS="--nodelink-config=$SCRIPT_DIR/nodelink.json \
		--frrk8s-namespace=openshift-frr-k8s \
		--openperouter-namespace=openshift-openperouter-system" \
	KUBECONFIG_PATH=$KUBECONFIG \
	GINKGO_ARGS="--label-filter=\!'systemd-mode' \
        --focus='Single.Session.Baseline' \
		--skip='editing.the.underlay.parameters|auto-recover.when.the.named.netns.is.deleted|Webhook|Unnumbered|Router.Host.configuration|DHCP'"

popd