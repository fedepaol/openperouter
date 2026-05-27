// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"log/slog"
	"testing"

	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/internal/conversion"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func TestNotMirroredPredicate(t *testing.T) {
	notMirrored := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetLabels()[LabelMirrored] != LabelValueMirrored
	})

	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "object without mirrored label",
			labels:   map[string]string{},
			expected: true,
		},
		{
			name:     "object with other labels",
			labels:   map[string]string{"app": "test"},
			expected: true,
		},
		{
			name:     "object with mirrored=true",
			labels:   map[string]string{LabelMirrored: LabelValueMirrored},
			expected: false,
		},
		{
			name:     "object with mirrored=false",
			labels:   map[string]string{LabelMirrored: "false"},
			expected: true,
		},
		{
			name:     "object with mirrored=invalid",
			labels:   map[string]string{LabelMirrored: "invalid"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &v1alpha1.Underlay{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test",
					Labels: tt.labels,
				},
			}

			createEvent := event.CreateEvent{Object: obj}
			if result := notMirrored.Create(createEvent); result != tt.expected {
				t.Errorf("Create: expected %v, got %v", tt.expected, result)
			}

			updateEvent := event.UpdateEvent{ObjectNew: obj, ObjectOld: obj}
			if result := notMirrored.Update(updateEvent); result != tt.expected {
				t.Errorf("Update: expected %v, got %v", tt.expected, result)
			}

			deleteEvent := event.DeleteEvent{Object: obj}
			if result := notMirrored.Delete(deleteEvent); result != tt.expected {
				t.Errorf("Delete: expected %v, got %v", tt.expected, result)
			}

			genericEvent := event.GenericEvent{Object: obj}
			if result := notMirrored.Generic(genericEvent); result != tt.expected {
				t.Errorf("Generic: expected %v, got %v", tt.expected, result)
			}
		})
	}
}

type mirroredTestCase struct {
	name                       string
	underlays                  []v1alpha1.Underlay
	l3vnis                     []v1alpha1.L3VNI
	l2vnis                     []v1alpha1.L2VNI
	l3passthroughs             []v1alpha1.L3Passthrough
	rawfrrconfigs              []v1alpha1.RawFRRConfig
	expectedUnderlayCount      int
	expectedL3VNICount         int
	expectedL2VNICount         int
	expectedL3PassthroughCount int
	expectedRawFRRConfigCount  int
}

func (tc *mirroredTestCase) buildObjects() []client.Object {
	objects := make([]client.Object, 0, 1+len(tc.underlays)+len(tc.l3vnis)+len(tc.l2vnis)+len(tc.l3passthroughs)+len(tc.rawfrrconfigs))
	objects = append(objects, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}})
	for i := range tc.underlays {
		objects = append(objects, &tc.underlays[i])
	}
	for i := range tc.l3vnis {
		objects = append(objects, &tc.l3vnis[i])
	}
	for i := range tc.l2vnis {
		objects = append(objects, &tc.l2vnis[i])
	}
	for i := range tc.l3passthroughs {
		objects = append(objects, &tc.l3passthroughs[i])
	}
	for i := range tc.rawfrrconfigs {
		objects = append(objects, &tc.rawfrrconfigs[i])
	}
	return objects
}

func (tc *mirroredTestCase) assertCounts(t *testing.T, config conversion.APIConfigData) {
	t.Helper()
	if len(config.Underlays) != tc.expectedUnderlayCount {
		t.Errorf("expected %d underlays, got %d", tc.expectedUnderlayCount, len(config.Underlays))
	}
	if len(config.L3VNIs) != tc.expectedL3VNICount {
		t.Errorf("expected %d l3vnis, got %d", tc.expectedL3VNICount, len(config.L3VNIs))
	}
	if len(config.L2VNIs) != tc.expectedL2VNICount {
		t.Errorf("expected %d l2vnis, got %d", tc.expectedL2VNICount, len(config.L2VNIs))
	}
	if len(config.L3Passthrough) != tc.expectedL3PassthroughCount {
		t.Errorf("expected %d l3passthroughs, got %d", tc.expectedL3PassthroughCount, len(config.L3Passthrough))
	}
	if len(config.RawFRRConfigs) != tc.expectedRawFRRConfigCount {
		t.Errorf("expected %d rawfrrconfigs, got %d", tc.expectedRawFRRConfigCount, len(config.RawFRRConfigs))
	}
}

func TestGetConfigFromAPIExcludesMirroredCRs(t *testing.T) {
	runtimeScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(runtimeScheme); err != nil {
		t.Fatalf("failed to add k8s scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(runtimeScheme); err != nil {
		t.Fatalf("failed to add v1alpha1 scheme: %v", err)
	}

	mirroredLabels := map[string]string{LabelMirrored: LabelValueMirrored}

	tests := []mirroredTestCase{
		{
			name: "all non-mirrored CRs",
			underlays: []v1alpha1.Underlay{
				{ObjectMeta: metav1.ObjectMeta{Name: "underlay1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "underlay2"}},
			},
			l3vnis: []v1alpha1.L3VNI{
				{ObjectMeta: metav1.ObjectMeta{Name: "l3vni1"}},
			},
			expectedUnderlayCount: 2,
			expectedL3VNICount:    1,
		},
		{
			name: "mix of mirrored and non-mirrored",
			underlays: []v1alpha1.Underlay{
				{ObjectMeta: metav1.ObjectMeta{Name: "underlay1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "underlay2", Labels: mirroredLabels}},
			},
			l3vnis: []v1alpha1.L3VNI{
				{ObjectMeta: metav1.ObjectMeta{Name: "l3vni1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "l3vni2", Labels: mirroredLabels}},
			},
			l2vnis: []v1alpha1.L2VNI{
				{ObjectMeta: metav1.ObjectMeta{Name: "l2vni1", Labels: mirroredLabels}},
			},
			expectedUnderlayCount: 1,
			expectedL3VNICount:    1,
		},
		{
			name: "all mirrored CRs",
			underlays: []v1alpha1.Underlay{
				{ObjectMeta: metav1.ObjectMeta{Name: "underlay1", Labels: mirroredLabels}},
			},
			l3vnis: []v1alpha1.L3VNI{
				{ObjectMeta: metav1.ObjectMeta{Name: "l3vni1", Labels: mirroredLabels}},
			},
		},
		{
			name: "mirrored=false should not be filtered",
			underlays: []v1alpha1.Underlay{
				{ObjectMeta: metav1.ObjectMeta{
					Name:   "underlay1",
					Labels: map[string]string{LabelMirrored: "false"},
				}},
			},
			expectedUnderlayCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(runtimeScheme).
				WithObjects(tt.buildObjects()...).
				Build()

			reconciler := &PERouterReconciler{
				Client: fakeClient,
				Logger: slog.Default(),
				MyNode: "test-node",
			}

			config, err := reconciler.getConfigFromAPI(context.Background(), slog.Default())
			if err != nil {
				t.Fatalf("getConfigFromAPI failed: %v", err)
			}

			tt.assertCounts(t, config)
		})
	}
}
