// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"fmt"
	"strconv"

	"github.com/openperouter/openperouter/internal/controller/nodeindex"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// nodeIndex returns the index of the given node.
func nodeIndex(ctx context.Context, cli client.Client, nodeName string) (int, error) {
	var node v1.Node
	if err := cli.Get(ctx, client.ObjectKey{Name: nodeName}, &node); err != nil {
		return 0, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}
	if node.Annotations == nil {
		return 0, fmt.Errorf("node %s has no annotations", nodeName)
	}
	index, ok := node.Annotations[nodeindex.OpenpeNodeIndex]
	if !ok {
		return 0, fmt.Errorf("node %s has no index annotation", nodeName)
	}
	i, err := strconv.Atoi(index)
	if err != nil {
		return 0, fmt.Errorf("failed to parse index %s: %w", index, err)
	}
	return i, nil
}
