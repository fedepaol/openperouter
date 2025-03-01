package nodeindex

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodesToAnnotate(t *testing.T) {
	tests := []struct {
		name                string
		nodes               []v1.Node
		expectedAnnotations map[string]string
	}{
		{
			name: "Nodes with existing annotations",
			nodes: []v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "first", Annotations: map[string]string{openpeNodeIndex: "0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "second", Annotations: map[string]string{openpeNodeIndex: "1"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "third", Annotations: map[string]string{}}},
			},
			expectedAnnotations: map[string]string{
				"third": "2",
			},
		},
		{
			name: "Nodes without annotations",
			nodes: []v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "first", Annotations: map[string]string{}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "second", Annotations: map[string]string{}}},
			},
			expectedAnnotations: map[string]string{
				"first":  "0",
				"second": "1",
			},
		},
		{
			name: "Nodes with hole in sequence",
			nodes: []v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "first", Annotations: map[string]string{openpeNodeIndex: "2"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "second", Annotations: map[string]string{openpeNodeIndex: "0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "third", Annotations: map[string]string{}}},
			},
			expectedAnnotations: map[string]string{
				"third": "1",
			},
		},
		{
			name: "Nodes with non int index",
			nodes: []v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "first", Annotations: map[string]string{openpeNodeIndex: "2"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "second", Annotations: map[string]string{openpeNodeIndex: "5"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "third", Annotations: map[string]string{openpeNodeIndex: "foo"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "fourth", Annotations: map[string]string{openpeNodeIndex: "7"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "fifth", Annotations: map[string]string{}}},
			},
			expectedAnnotations: map[string]string{
				"third": "0",
				"fifth": "1",
			},
		},
		{
			name: "Nodes with full sequence",
			nodes: []v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "first", Annotations: map[string]string{}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "second", Annotations: map[string]string{}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "third", Annotations: map[string]string{openpeNodeIndex: "0"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "fourth", Annotations: map[string]string{openpeNodeIndex: "1"}}},
			},
			expectedAnnotations: map[string]string{
				"first":  "2",
				"second": "3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotatedNodes := nodesToAnnotate(tt.nodes)
			for _, node := range annotatedNodes {
				if node.Annotations[openpeNodeIndex] != tt.expectedAnnotations[node.Name] {
					t.Errorf("expected %s, got %s", tt.expectedAnnotations[node.Name], node.Annotations[openpeNodeIndex])
				}
			}
		})
	}
}
