// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openperouter/openperouter/api/v1alpha1"
)

const testConfigFilename = "openpe_config.yaml"

const testSingleUnderlayConfig = `
underlays:
  - asn: 65001
    routeridcidr: "10.0.0.0/24"
    neighbors:
      - address: "192.168.1.1"
        asn: 65000
    nics:
      - "eth0"
    evpn:
      vtepcidr: "10.1.0.0/24"
`

func setupTestController(t *testing.T, configDir string) (*ConfigMirrorController, client.Client) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha1 to scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	controller := &ConfigMirrorController{
		Client:    fakeClient,
		Scheme:    scheme,
		Logger:    logger,
		NodeName:  "worker1",
		Namespace: "test-namespace",
		ConfigDir: configDir,
	}

	return controller, fakeClient
}

func createTestConfigFile(t *testing.T, dir string, content string) {
	path := filepath.Join(dir, testConfigFilename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}
}

// T007: Test that Reconcile creates CRs with correct names, labels, namespace, NodeSelector
func TestReconcileCreatesCRsWithCorrectMetadata(t *testing.T) {
	tempDir := t.TempDir()

	config := `
underlays:
  - asn: 65001
    routeridcidr: "10.0.0.0/24"
    neighbors:
      - address: "192.168.1.1"
        asn: 65000
    nics:
      - "eth0"
    evpn:
      vtepcidr: "10.1.0.0/24"
l3vnis:
  - vrf: "vrf1"
    vni: 1000
    vxlanport: 4789
l2vnis:
  - vni: 2000
    vxlanport: 4789
`
	createTestConfigFile(t, tempDir, config)

	controller, fakeClient := setupTestController(t, tempDir)
	ctx := context.Background()

	_, err := controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify Underlay
	var underlays v1alpha1.UnderlayList
	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays: %v", err)
	}
	if len(underlays.Items) != 1 {
		t.Fatalf("expected 1 underlay, got %d", len(underlays.Items))
	}

	underlay := underlays.Items[0]
	expectedName := "worker1-static-underlay-0"
	if underlay.Name != expectedName {
		t.Errorf("expected underlay name %s, got %s", expectedName, underlay.Name)
	}
	if underlay.Namespace != "test-namespace" {
		t.Errorf("expected namespace test-namespace, got %s", underlay.Namespace)
	}
	if underlay.Labels[LabelMirrored] != LabelValueMirrored {
		t.Errorf("expected label %s=true, got %s", LabelMirrored, underlay.Labels[LabelMirrored])
	}
	if underlay.Labels[LabelSourceNode] != "worker1" {
		t.Errorf("expected label %s=worker1, got %s", LabelSourceNode, underlay.Labels[LabelSourceNode])
	}
	if underlay.Spec.NodeSelector == nil {
		t.Fatal("expected NodeSelector to be set")
	}
	if underlay.Spec.NodeSelector.MatchLabels["kubernetes.io/hostname"] != "worker1" {
		t.Errorf("expected NodeSelector hostname=worker1, got %s", underlay.Spec.NodeSelector.MatchLabels["kubernetes.io/hostname"])
	}

	// Verify L3VNI
	var l3vnis v1alpha1.L3VNIList
	if err := fakeClient.List(ctx, &l3vnis, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list l3vnis: %v", err)
	}
	if len(l3vnis.Items) != 1 {
		t.Fatalf("expected 1 l3vni, got %d", len(l3vnis.Items))
	}

	l3vni := l3vnis.Items[0]
	expectedL3VNIName := "worker1-static-l3vni-0"
	if l3vni.Name != expectedL3VNIName {
		t.Errorf("expected l3vni name %s, got %s", expectedL3VNIName, l3vni.Name)
	}
	if l3vni.Labels[LabelMirrored] != LabelValueMirrored {
		t.Errorf("expected label %s=true, got %s", LabelMirrored, l3vni.Labels[LabelMirrored])
	}
	if l3vni.Spec.NodeSelector == nil {
		t.Fatal("expected NodeSelector to be set")
	}

	// Verify L2VNI
	var l2vnis v1alpha1.L2VNIList
	if err := fakeClient.List(ctx, &l2vnis, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list l2vnis: %v", err)
	}
	if len(l2vnis.Items) != 1 {
		t.Fatalf("expected 1 l2vni, got %d", len(l2vnis.Items))
	}

	l2vni := l2vnis.Items[0]
	expectedL2VNIName := "worker1-static-l2vni-0"
	if l2vni.Name != expectedL2VNIName {
		t.Errorf("expected l2vni name %s, got %s", expectedL2VNIName, l2vni.Name)
	}
	if l2vni.Labels[LabelMirrored] != LabelValueMirrored {
		t.Errorf("expected label %s=true, got %s", LabelMirrored, l2vni.Labels[LabelMirrored])
	}
	if l2vni.Spec.NodeSelector == nil {
		t.Fatal("expected NodeSelector to be set")
	}
}

// T008: Test that Reconcile deletes mirrored CRs that no longer correspond to config entries
func TestReconcileDeletesStaleCRs(t *testing.T) {
	tempDir := t.TempDir()

	initialConfig := `
underlays:
  - asn: 65001
    routeridcidr: "10.0.0.0/24"
    neighbors:
      - address: "192.168.1.1"
        asn: 65000
    nics:
      - "eth0"
    evpn:
      vtepcidr: "10.1.0.0/24"
  - asn: 65002
    routeridcidr: "10.0.1.0/24"
    neighbors:
      - address: "192.168.1.2"
        asn: 65000
    nics:
      - "eth1"
    evpn:
      vtepcidr: "10.1.1.0/24"
`
	createTestConfigFile(t, tempDir, initialConfig)

	controller, fakeClient := setupTestController(t, tempDir)
	ctx := context.Background()

	// First reconcile to create CRs
	_, err := controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("First Reconcile failed: %v", err)
	}

	var underlays v1alpha1.UnderlayList
	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays: %v", err)
	}
	if len(underlays.Items) != 2 {
		t.Fatalf("expected 2 underlays after first reconcile, got %d", len(underlays.Items))
	}

	// Update config to remove one underlay
	createTestConfigFile(t, tempDir, testSingleUnderlayConfig)

	// Second reconcile should delete the stale CR
	_, err = controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("Second Reconcile failed: %v", err)
	}

	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays after deletion: %v", err)
	}
	if len(underlays.Items) != 1 {
		t.Fatalf("expected 1 underlay after second reconcile, got %d", len(underlays.Items))
	}
	if underlays.Items[0].Name != "worker1-static-underlay-0" {
		t.Errorf("wrong underlay remained, got %s", underlays.Items[0].Name)
	}
}

// T014: Test that Reconcile updates CRs when config content changes
func TestReconcileUpdatesCRsOnContentChange(t *testing.T) {
	tempDir := t.TempDir()

	createTestConfigFile(t, tempDir, testSingleUnderlayConfig)

	controller, fakeClient := setupTestController(t, tempDir)
	ctx := context.Background()

	// First reconcile
	_, err := controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("First Reconcile failed: %v", err)
	}

	var underlays v1alpha1.UnderlayList
	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays: %v", err)
	}
	if len(underlays.Items) != 1 {
		t.Fatalf("expected 1 underlay, got %d", len(underlays.Items))
	}
	if underlays.Items[0].Spec.ASN != 65001 {
		t.Fatalf("expected ASN 65001, got %d", underlays.Items[0].Spec.ASN)
	}

	// Update config with new ASN
	updatedConfig := `
underlays:
  - asn: 65999
    routeridcidr: "10.0.0.0/24"
    neighbors:
      - address: "192.168.1.1"
        asn: 65000
    nics:
      - "eth0"
    evpn:
      vtepcidr: "10.1.0.0/24"
`
	createTestConfigFile(t, tempDir, updatedConfig)

	// Second reconcile should update the CR
	_, err = controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("Second Reconcile failed: %v", err)
	}

	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays after update: %v", err)
	}
	if len(underlays.Items) != 1 {
		t.Fatalf("expected 1 underlay after update, got %d", len(underlays.Items))
	}
	if underlays.Items[0].Spec.ASN != 65999 {
		t.Errorf("expected ASN to be updated to 65999, got %d", underlays.Items[0].Spec.ASN)
	}
}

// T015: Test that Reconcile cleans up all CRs when all config files are removed (NoConfigAvailable)
func TestReconcileCleansUpAllCRsWhenNoConfig(t *testing.T) {
	tempDir := t.TempDir()

	initialConfig := `
underlays:
  - asn: 65001
    routeridcidr: "10.0.0.0/24"
    neighbors:
      - address: "192.168.1.1"
        asn: 65000
    nics:
      - "eth0"
    evpn:
      vtepcidr: "10.1.0.0/24"
l3vnis:
  - vrf: "vrf1"
    vni: 1000
    vxlanport: 4789
`
	createTestConfigFile(t, tempDir, initialConfig)

	controller, fakeClient := setupTestController(t, tempDir)
	ctx := context.Background()

	// First reconcile to create CRs
	_, err := controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("First Reconcile failed: %v", err)
	}

	// Verify CRs were created
	var underlays v1alpha1.UnderlayList
	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays: %v", err)
	}
	if len(underlays.Items) != 1 {
		t.Fatalf("expected 1 underlay, got %d", len(underlays.Items))
	}

	var l3vnis v1alpha1.L3VNIList
	if err := fakeClient.List(ctx, &l3vnis, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list l3vnis: %v", err)
	}
	if len(l3vnis.Items) != 1 {
		t.Fatalf("expected 1 l3vni, got %d", len(l3vnis.Items))
	}

	// Remove all config files
	if err := os.Remove(filepath.Join(tempDir, testConfigFilename)); err != nil {
		t.Fatalf("failed to remove config file: %v", err)
	}

	// Second reconcile should clean up all CRs
	_, err = controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("Second Reconcile failed: %v", err)
	}

	// Verify all CRs were deleted
	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays after cleanup: %v", err)
	}
	if len(underlays.Items) != 0 {
		t.Errorf("expected 0 underlays after cleanup, got %d", len(underlays.Items))
	}

	if err := fakeClient.List(ctx, &l3vnis, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list l3vnis after cleanup: %v", err)
	}
	if len(l3vnis.Items) != 0 {
		t.Errorf("expected 0 l3vnis after cleanup, got %d", len(l3vnis.Items))
	}
}

// Test with L3Passthrough and RawFRRConfig
func TestReconcileWithL3PassthroughAndRawFRR(t *testing.T) {
	tempDir := t.TempDir()

	config := `
bgppassthrough:
  hostsession:
    asn: 65100
    localaddress: "10.0.0.1"
    neighbors:
      - address: "10.0.0.2"
        asn: 65101
rawfrrconfigs:
  - priority: 10
    rawConfig: |
      ip prefix-list TEST seq 10 permit 192.168.0.0/16
`
	createTestConfigFile(t, tempDir, config)

	controller, fakeClient := setupTestController(t, tempDir)
	ctx := context.Background()

	_, err := controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify L3Passthrough
	var l3passthroughs v1alpha1.L3PassthroughList
	if err := fakeClient.List(ctx, &l3passthroughs, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list l3passthroughs: %v", err)
	}
	if len(l3passthroughs.Items) != 1 {
		t.Fatalf("expected 1 l3passthrough, got %d", len(l3passthroughs.Items))
	}

	l3pass := l3passthroughs.Items[0]
	expectedName := "worker1-static-l3passthrough-0"
	if l3pass.Name != expectedName {
		t.Errorf("expected l3passthrough name %s, got %s", expectedName, l3pass.Name)
	}
	if l3pass.Labels[LabelMirrored] != LabelValueMirrored {
		t.Errorf("expected label %s=true", LabelMirrored)
	}

	// Verify RawFRRConfig
	var rawfrrconfigs v1alpha1.RawFRRConfigList
	if err := fakeClient.List(ctx, &rawfrrconfigs, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list rawfrrconfigs: %v", err)
	}
	if len(rawfrrconfigs.Items) != 1 {
		t.Fatalf("expected 1 rawfrrconfig, got %d", len(rawfrrconfigs.Items))
	}

	rawfrr := rawfrrconfigs.Items[0]
	expectedRawName := "worker1-static-rawfrrconfig-0"
	if rawfrr.Name != expectedRawName {
		t.Errorf("expected rawfrrconfig name %s, got %s", expectedRawName, rawfrr.Name)
	}
	if rawfrr.Labels[LabelMirrored] != LabelValueMirrored {
		t.Errorf("expected label %s=true", LabelMirrored)
	}
	if rawfrr.Spec.Priority != 10 {
		t.Errorf("expected priority 10, got %d", rawfrr.Spec.Priority)
	}
}

// Test that only CRs from this node are deleted (doesn't touch other nodes' CRs)
func TestReconcileOnlyDeletesOwnNodeCRs(t *testing.T) {
	tempDir := t.TempDir()

	createTestConfigFile(t, tempDir, testSingleUnderlayConfig)

	controller, fakeClient := setupTestController(t, tempDir)
	ctx := context.Background()

	// Create a CR for a different node manually
	otherNodeUnderlay := &v1alpha1.Underlay{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "worker2-static-underlay-0",
			Namespace: "test-namespace",
			Labels: map[string]string{
				LabelMirrored:   LabelValueMirrored,
				LabelSourceNode: "worker2",
			},
		},
		Spec: v1alpha1.UnderlaySpec{
			ASN:          65002,
			RouterIDCIDR: "10.0.0.0/24",
			Neighbors: []v1alpha1.Neighbor{
				{Address: "192.168.1.2", ASN: 65000},
			},
			Nics: []string{"eth1"},
		},
	}
	if err := fakeClient.Create(ctx, otherNodeUnderlay); err != nil {
		t.Fatalf("failed to create other node's underlay: %v", err)
	}

	// Reconcile for worker1
	_, err := controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify both CRs exist
	var underlays v1alpha1.UnderlayList
	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays: %v", err)
	}
	if len(underlays.Items) != 2 {
		t.Fatalf("expected 2 underlays, got %d", len(underlays.Items))
	}

	// Verify worker2's CR is still there
	foundWorker2 := false
	for _, u := range underlays.Items {
		if u.Name == "worker2-static-underlay-0" {
			foundWorker2 = true
			break
		}
	}
	if !foundWorker2 {
		t.Error("worker2's CR should not have been deleted")
	}

	// Now remove config and reconcile again
	if err := os.Remove(filepath.Join(tempDir, testConfigFilename)); err != nil {
		t.Fatalf("failed to remove config file: %v", err)
	}

	_, err = controller.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		t.Fatalf("Second Reconcile failed: %v", err)
	}

	// Verify only worker2's CR remains
	if err := fakeClient.List(ctx, &underlays, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("failed to list underlays after cleanup: %v", err)
	}
	if len(underlays.Items) != 1 {
		t.Fatalf("expected 1 underlay after cleanup, got %d", len(underlays.Items))
	}
	if underlays.Items[0].Name != "worker2-static-underlay-0" {
		t.Errorf("expected worker2's CR to remain, got %s", underlays.Items[0].Name)
	}
}
