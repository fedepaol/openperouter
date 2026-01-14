// SPDX-License-Identifier:Apache-2.0

package conversion

import (
	"testing"

	"github.com/openperouter/openperouter/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeAPIConfigs_EmptyInput(t *testing.T) {
	merged, err := MergeAPIConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.NodeIndex != 0 {
		t.Errorf("expected NodeIndex 0, got %d", merged.NodeIndex)
	}
	if len(merged.Underlays) != 0 {
		t.Errorf("expected 0 underlays, got %d", len(merged.Underlays))
	}
}

func TestMergeAPIConfigs_SingleConfig(t *testing.T) {
	config := ApiConfigData{
		NodeIndex: 1,
		LogLevel:  "debug",
		Underlays: []v1alpha1.Underlay{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "underlay1"},
				Spec: v1alpha1.UnderlaySpec{
					ASN: 64515,
				},
			},
		},
	}

	merged, err := MergeAPIConfigs(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if merged.NodeIndex != 1 {
		t.Errorf("expected NodeIndex 1, got %d", merged.NodeIndex)
	}
	if merged.LogLevel != "debug" {
		t.Errorf("expected LogLevel debug, got %s", merged.LogLevel)
	}
	if len(merged.Underlays) != 1 {
		t.Fatalf("expected 1 underlay, got %d", len(merged.Underlays))
	}
	if merged.Underlays[0].Spec.ASN != 64515 {
		t.Errorf("expected ASN 64515, got %d", merged.Underlays[0].Spec.ASN)
	}
}

func TestMergeAPIConfigs_MultipleConfigs(t *testing.T) {
	config1 := ApiConfigData{
		NodeIndex: 1,
		LogLevel:  "info",
		Underlays: []v1alpha1.Underlay{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "underlay1"},
				Spec:       v1alpha1.UnderlaySpec{ASN: 64515},
			},
		},
	}

	config2 := ApiConfigData{
		NodeIndex: 1,
		LogLevel:  "info",
		L3VNIs: []v1alpha1.L3VNI{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "l3vni1"},
				Spec:       v1alpha1.L3VNISpec{VNI: 1000},
			},
		},
	}

	config3 := ApiConfigData{
		NodeIndex: 1,
		LogLevel:  "info",
		L2VNIs: []v1alpha1.L2VNI{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "l2vni1"},
				Spec:       v1alpha1.L2VNISpec{VNI: 2000},
			},
		},
	}

	merged, err := MergeAPIConfigs(config1, config2, config3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if merged.NodeIndex != 1 {
		t.Errorf("expected NodeIndex 1, got %d", merged.NodeIndex)
	}
	if len(merged.Underlays) != 1 {
		t.Errorf("expected 1 underlay, got %d", len(merged.Underlays))
	}
	if len(merged.L3VNIs) != 1 {
		t.Errorf("expected 1 l3vni, got %d", len(merged.L3VNIs))
	}
	if len(merged.L2VNIs) != 1 {
		t.Errorf("expected 1 l2vni, got %d", len(merged.L2VNIs))
	}
}

func TestMergeAPIConfigs_NodeIndexMismatch(t *testing.T) {
	config1 := ApiConfigData{
		NodeIndex: 1,
		Underlays: []v1alpha1.Underlay{
			{ObjectMeta: metav1.ObjectMeta{Name: "underlay1"}},
		},
	}

	config2 := ApiConfigData{
		NodeIndex: 2,
		L3VNIs: []v1alpha1.L3VNI{
			{ObjectMeta: metav1.ObjectMeta{Name: "l3vni1"}},
		},
	}

	_, err := MergeAPIConfigs(config1, config2)
	if err == nil {
		t.Error("expected error for NodeIndex mismatch, got none")
	}
	expectedErr := "NodeIndex mismatch: config[0] has 1, config[1] has 2"
	if err != nil && err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestMergeAPIConfigs_UnderlayFromMultusMismatch(t *testing.T) {
	config1 := ApiConfigData{
		NodeIndex:          1,
		UnderlayFromMultus: true,
	}

	config2 := ApiConfigData{
		NodeIndex:          1,
		UnderlayFromMultus: false,
	}

	_, err := MergeAPIConfigs(config1, config2)
	if err == nil {
		t.Error("expected error for UnderlayFromMultus mismatch, got none")
	}
	expectedErr := "UnderlayFromMultus mismatch: config[0] has true, config[1] has false"
	if err != nil && err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestMergeAPIConfigs_AllResourceTypes(t *testing.T) {
	config := ApiConfigData{
		NodeIndex: 1,
		Underlays: []v1alpha1.Underlay{
			{ObjectMeta: metav1.ObjectMeta{Name: "underlay1"}},
		},
		L3VNIs: []v1alpha1.L3VNI{
			{ObjectMeta: metav1.ObjectMeta{Name: "l3vni1"}},
		},
		L2VNIs: []v1alpha1.L2VNI{
			{ObjectMeta: metav1.ObjectMeta{Name: "l2vni1"}},
		},
		L3Passthrough: []v1alpha1.L3Passthrough{
			{ObjectMeta: metav1.ObjectMeta{Name: "passthrough1"}},
		},
	}

	merged, err := MergeAPIConfigs(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(merged.Underlays) != 1 {
		t.Errorf("expected 1 underlay, got %d", len(merged.Underlays))
	}
	if len(merged.L3VNIs) != 1 {
		t.Errorf("expected 1 l3vni, got %d", len(merged.L3VNIs))
	}
	if len(merged.L2VNIs) != 1 {
		t.Errorf("expected 1 l2vni, got %d", len(merged.L2VNIs))
	}
	if len(merged.L3Passthrough) != 1 {
		t.Errorf("expected 1 l3passthrough, got %d", len(merged.L3Passthrough))
	}
}

func TestMergeAPIConfigs_ResourcesConcatenated(t *testing.T) {
	config1 := ApiConfigData{
		NodeIndex: 1,
		Underlays: []v1alpha1.Underlay{
			{ObjectMeta: metav1.ObjectMeta{Name: "underlay1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "underlay2"}},
		},
	}

	config2 := ApiConfigData{
		NodeIndex: 1,
		Underlays: []v1alpha1.Underlay{
			{ObjectMeta: metav1.ObjectMeta{Name: "underlay3"}},
		},
	}

	merged, err := MergeAPIConfigs(config1, config2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(merged.Underlays) != 3 {
		t.Errorf("expected 3 underlays, got %d", len(merged.Underlays))
	}

	// Verify order is preserved
	expectedNames := []string{"underlay1", "underlay2", "underlay3"}
	for i, underlay := range merged.Underlays {
		if underlay.Name != expectedNames[i] {
			t.Errorf("expected underlay name %s at index %d, got %s", expectedNames[i], i, underlay.Name)
		}
	}
}
