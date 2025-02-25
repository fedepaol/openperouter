// SPDX-License-Identifier:Apache-2.0

package controller

import (
	"context"
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO make this stateful.

// nodeIndex returns the index of the given node.
func nodeIndex(ctx context.Context, cli client.Client, node string) (int, error) {
	var nodes v1.NodeList
	if err := cli.List(ctx, &nodes); err != nil {
		return 0, fmt.Errorf("failed to list nodes: %w", err)
	}

	sort.Slice(nodes.Items, func(i, j int) bool {
		creationTimeI := nodes.Items[i].CreationTimestamp
		creationTimeJ := nodes.Items[j].CreationTimestamp
		return creationTimeI.Compare(creationTimeJ.Time) < 0
	})
	for i := range nodes.Items {
		if nodes.Items[i].Name == node {
			return i, nil
		}
	}
	return 0, nil
}
