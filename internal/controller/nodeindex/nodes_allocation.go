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
			taken = insertSorted(taken, index)
			continue
		}
		res = append(res, n)
	}

	current := 0
	lastIndex := -1
	for i := range res {
		if res[i].Annotations == nil {
			res[i].Annotations = make(map[string]string)
		}
		lastIndex = getNextFree(lastIndex, &current, taken)
		res[i].Annotations[OpenpeNodeIndex] = strconv.Itoa(current)
		current++
	}
	return res
}

// getNextFree returns the next valid item in a sorted "taken" slice
// with all the already taken indexes.
func getNextFree(lastIndex int, current *int, taken []int) int {
	candidate := lastIndex + 1
	for *current < len(taken) {
		upper := taken[*current]

		if upper > candidate { // there's a hole
			return candidate
		}
		candidate = upper + 1
		*current++ // let's try with the next one
	}
	return candidate // we are past the rightmost
}

func insertSorted(slice []int, toAdd int) []int {
	index := sort.Search(len(slice), func(i int) bool { return slice[i] >= toAdd })
	slice = append(slice, 0)
	copy(slice[index+1:], slice[index:])
	slice[index] = toAdd
	return slice
}
