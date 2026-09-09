#!/usr/bin/env bash
# Deploy FRR-K8s through the OpenShift Cluster Network Operator.

set -euo pipefail

FRRK8S_NAMESPACE="${FRRK8S_NAMESPACE:-openshift-frr-k8s}"
FRRK8S_SELECTOR="${FRRK8S_SELECTOR:-app=frr-k8s}"
FRRK8S_TIMEOUT="${FRRK8S_TIMEOUT:-5m}"

command -v oc >/dev/null 2>&1 || {
    echo "oc is required" >&2
    exit 1
}

oc get --raw=/readyz >/dev/null

echo "Creating FRR-K8s namespace and debug configuration"
oc create namespace "${FRRK8S_NAMESPACE}" --dry-run=client -o yaml | oc apply -f -
oc apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: env-overrides
  namespace: ${FRRK8S_NAMESPACE}
data:
  frrk8s-loglevel: "--log-level=debug"
  frrk8s-poll-interval: "--poll-interval=5s"
EOF

echo "Enabling the FRR additional routing capability in CNO"
oc patch networks.operator.openshift.io cluster --type=json \
    -p='[{"op":"add","path":"/spec/additionalRoutingCapabilities","value":{"providers":["FRR"]}}]'

echo "Waiting for FRR-K8s pods to be created"
timeout "${FRRK8S_TIMEOUT}" bash -c \
    'until [[ -n "$(oc get pods -n "$1" -l "$2" -o name 2>/dev/null)" ]]; do sleep 5; done' \
    bash "${FRRK8S_NAMESPACE}" "${FRRK8S_SELECTOR}"

echo "Waiting for FRR-K8s pods to become ready"
oc wait --for=condition=Ready pod \
    -n "${FRRK8S_NAMESPACE}" -l "${FRRK8S_SELECTOR}" \
    --timeout="${FRRK8S_TIMEOUT}"

echo "FRR-K8s is ready in ${FRRK8S_NAMESPACE}"
