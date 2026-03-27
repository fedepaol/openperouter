// SPDX-License-Identifier:Apache-2.0

package infra

import (
	"fmt"

	"github.com/openperouter/openperouter/e2etests/pkg/frr"
)

var (
	ClabPrefix string
	KindLeaf   string
	LeafA      string
	LeafB      string
)

var (
	KindLeafContainer frr.Container
	LeafAContainer    frr.Container
	LeafBContainer    frr.Container
)

var links linksForRouters

func NeighborIP(from, to string) (string, error) {
	fromNeighbors, ok := links.nodes[from]
	if !ok {
		return "", fmt.Errorf("node %s not found", from)
	}
	if fromNeighbors.neighs == nil {
		return "", fmt.Errorf("node %s has no neighbors", from)
	}
	toIP, ok := fromNeighbors.neighs[to]
	if !ok {
		return "", fmt.Errorf("node %s has no neighbor %s", from, to)
	}
	return toIP, nil
}

type linksForRouters struct {
	nodes map[string]node
}

func (l *linksForRouters) Add(first, second, addressFirst, addressSecond string) {
	addLink := func(from, to, addressTo string) {
		n, ok := l.nodes[from]
		if !ok {
			n = node{
				neighs: map[string]string{},
			}
			l.nodes[from] = n
		}
		if n.neighs == nil {
			n.neighs = map[string]string{}
		}
		n.neighs[to] = addressTo
	}
	addLink(first, second, addressSecond)
	addLink(second, first, addressFirst)
}

type node struct {
	neighs map[string]string
}
