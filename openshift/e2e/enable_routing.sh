#!/usr/bin/env bash

set -xeuo pipefail

ovn_namespace=openshift-ovn-kubernetes
ovn_daemonset=ovnkube-node

# Remember whether this invocation changes the rendered OVN node configuration.
# CNO may still report healthy for its previous revision immediately after patch.
current_gateway_config=$(oc get network.operator.openshift.io cluster \
  -o jsonpath='{.spec.defaultNetwork.ovnKubernetesConfig.gatewayConfig.routingViaHost}{" "}{.spec.defaultNetwork.ovnKubernetesConfig.gatewayConfig.ipForwarding}')
ovn_daemonset_generation=$(oc get daemonset -n "$ovn_namespace" "$ovn_daemonset" \
  -o jsonpath='{.metadata.generation}')

# Route external host-session traffic through node host networking.
oc patch network.operator.openshift.io cluster --type=merge \
  -p '{"spec":{"defaultNetwork":{"ovnKubernetesConfig":{"gatewayConfig":{"routingViaHost":true,"ipForwarding":"Global"}}}}}'

# Confirm API accepted requested values before waiting for rendered OVN rollout.
oc wait --for=jsonpath='{.spec.defaultNetwork.ovnKubernetesConfig.gatewayConfig.routingViaHost}'=true \
  --timeout=10m network.operator.openshift.io/cluster
oc wait --for=jsonpath='{.spec.defaultNetwork.ovnKubernetesConfig.gatewayConfig.ipForwarding}'=Global \
  --timeout=10m network.operator.openshift.io/cluster

# A changed gateway config must produce a new ovnkube-node revision.  Without
# this check, a healthy CNO can satisfy its conditions before it starts rollout.
if [[ "$current_gateway_config" != 'true Global' ]]; then
  deadline=$((SECONDS + 600))
  while [[ $(oc get daemonset -n "$ovn_namespace" "$ovn_daemonset" \
    -o jsonpath='{.metadata.generation}') == "$ovn_daemonset_generation" ]]; do
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for $ovn_namespace/$ovn_daemonset rollout to start" >&2
      exit 1
    fi
    sleep 5
  done
fi

oc rollout status --watch=true --timeout=10m \
  daemonset/"$ovn_daemonset" -n "$ovn_namespace"

# CNO reports healthy only after rollout has reconciled.
oc wait --for=condition=Available=True --timeout=10m co/network
oc wait --for=condition=Progressing=False --timeout=10m co/network
oc wait --for=condition=Degraded=False --timeout=10m co/network

oc get network.operator.openshift.io cluster \
  -o jsonpath='{.spec.defaultNetwork.ovnKubernetesConfig.gatewayConfig}{"\n"}'
