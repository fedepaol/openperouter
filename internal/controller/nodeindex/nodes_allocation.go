// SPDX-License-Identifier:Apache-2.0

package nodeindex

import (
	"sort"
	"strconv"

	v1 "k8s.io/api/core/v1"
)

const OpenpeNodeIndex = "openpe.io/nodeindex"

func nodesToAnnotate(allNodes []v1.Node) []v1.Node {
	taken := make([]int, 0)

	res := make([]v1.Node, 0)
	for _, n := range allNodes {
		if n.Annotations[OpenpeNodeIndex] != "" {
			index, err := strconv.Atoi(n.Annotations[OpenpeNodeIndex])
			if err != nil { // not an int, let's ignore it so we'll add again
				res = append(res, n)
				continue
			}
			if alreadyExistsInSorted(taken, index) {
				res = append(res, n)
				continue
			}
			taken = insertSorted(taken, index)
			continue
		}
		res = append(res, n)
	}

	allocator := newAllocator(taken)
	for i := range res {
		if res[i].Annotations == nil {
			res[i].Annotations = make(map[string]string)
		}
		index := allocator.getNextFree()
		res[i].Annotations[OpenpeNodeIndex] = strconv.Itoa(index)
	}
	return res
}

type allocator struct {
	lastAssigned int
	upperIndex   int
	taken        []int
}

func newAllocator(taken []int) *allocator {
	return &allocator{
		lastAssigned: -1,
		upperIndex:   0,
		taken:        taken,
	}
}

func (a *allocator) getNextFree() int {
	nextFree := a.findNextFree()
	a.lastAssigned = nextFree
	return nextFree
}

// findNextFree returns the next valid item starting from the lastAssigned
// index in a sorted "taken" slice containing all the already taken indexes.
// An upperIndex pointer is maintained to track the upper limit item
// in the taken index.
func (a *allocator) findNextFree() int {
	if len(a.taken) == 0 {
		return a.lastAssigned + 1 // no taken seats, we don't need much logic
	}
	candidate := a.lastAssigned + 1
	for a.upperIndex < len(a.taken) {
		upper := a.taken[a.upperIndex]

		if upper > candidate { // there's a hole
			return candidate
		}
		// no holes, let's try with right one more than upper and
		// check against the next upper
		candidate = upper + 1
		a.upperIndex++
	}
	return candidate // we are past the rightmost
}

func alreadyExistsInSorted(slice []int, toCheck int) bool {
	if len(slice) == 0 {
		return false
	}
	index := sort.Search(len(slice), func(i int) bool { return slice[i] >= toCheck })
	if index >= len(slice) {
		return false
	}
	return slice[index] == toCheck
}

func insertSorted(slice []int, toAdd int) []int {
	index := sort.Search(len(slice), func(i int) bool { return slice[i] >= toAdd })
	slice = append(slice, 0)
	copy(slice[index+1:], slice[index:])
	slice[index] = toAdd
	return slice
}
