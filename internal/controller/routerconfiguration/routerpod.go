// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const nodeNameIndex = "spec.NodeName"

// routerPodForNode returns the router pod for the given node
func routerPodForNode(ctx context.Context, cli client.Client, node string) (*v1.Pod, error) {
	var pods v1.PodList
	if err := cli.List(ctx, &pods, client.MatchingLabels{"app": "router"},
		client.MatchingFields{
			nodeNameIndex: node,
		}); err != nil {
		return nil, fmt.Errorf("failed to get router pod for node %s: %v", node, err)
	}
	if len(pods.Items) > 1 {
		return nil, fmt.Errorf("more than one router pod found for node %s", node)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no router pods found for node %s", node)
	}
	return &pods.Items[0], nil
}
