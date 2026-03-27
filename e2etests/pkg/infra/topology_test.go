// SPDX-License-Identifier:Apache-2.0

package infra

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openperouter/openperouter/pkg/devenv"
)

func generateStateFile(t *testing.T) string {
	t.Helper()
	// Use the testdata from the devenv package
	repoRoot := findRepoRoot(t)
	clabFile := filepath.Join(repoRoot, "pkg", "devenv", "testdata", "kind.clab.yml")
	envFile := filepath.Join(repoRoot, "pkg", "devenv", "testdata", "env.yaml")

	cfg, err := devenv.LoadFromEnvConfig(clabFile, envFile)
	if err != nil {
		t.Fatalf("LoadFromEnvConfig failed: %v", err)
	}

	stateFile := filepath.Join(t.TempDir(), "state.yaml")
	if err := cfg.Save(stateFile); err != nil {
		t.Fatalf("Save state failed: %v", err)
	}
	return stateFile
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the current working directory looking for go.mod in root
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "pkg", "devenv", "testdata")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func TestInitPopulatesVars(t *testing.T) {
	stateFile := generateStateFile(t)

	if err := Init(stateFile); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Topology should be loaded
	if Topology == nil {
		t.Fatal("Topology is nil after Init")
	}

	// ClabPrefix should be set
	if ClabPrefix == "" {
		t.Error("ClabPrefix is empty")
	}

	// Leaf container names should be populated
	if KindLeaf == "" {
		t.Error("KindLeaf is empty")
	}
	if LeafA == "" {
		t.Error("LeafA is empty")
	}
	if LeafB == "" {
		t.Error("LeafB is empty")
	}

	// Leaf configs should have ASN set
	if LeafAConfig.ASN == 0 {
		t.Error("LeafAConfig.ASN is 0")
	}
	if LeafBConfig.ASN == 0 {
		t.Error("LeafBConfig.ASN is 0")
	}
	if LeafKindConfig.ASN == 0 {
		t.Error("LeafKindConfig.ASN is 0")
	}

	// Host IPs should be populated
	if HostARedIPv4 == "" {
		t.Error("HostARedIPv4 is empty")
	}
	if HostBRedIPv4 == "" {
		t.Error("HostBRedIPv4 is empty")
	}
	if HostARedIPv6 == "" {
		t.Error("HostARedIPv6 is empty")
	}

	// Underlay neighbor should be populated
	if Underlay.Spec.Neighbors[0].ASN == 0 {
		t.Error("Underlay neighbor ASN is 0")
	}
	if Underlay.Spec.Neighbors[0].Address == "" {
		t.Error("Underlay neighbor address is empty")
	}
}

func TestContainerName(t *testing.T) {
	stateFile := generateStateFile(t)
	if err := Init(stateFile); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Regular nodes should get clab prefix
	name := ContainerName("spine")
	expected := "clab-" + Topology.Name + "-spine"
	if name != expected {
		t.Errorf("expected %q, got %q", expected, name)
	}

	// leafA should also get clab prefix
	name = ContainerName("leafA")
	expected = "clab-" + Topology.Name + "-leafA"
	if name != expected {
		t.Errorf("expected %q, got %q", expected, name)
	}
}

func TestLinkPrefixList(t *testing.T) {
	stateFile := generateStateFile(t)
	if err := Init(stateFile); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	prefixes := LinkPrefixList("leafA", "hostA_red")
	if len(prefixes) == 0 {
		t.Error("expected non-empty prefix list for leafA-hostA_red link")
	}

	// Non-existent link should return empty
	prefixes = LinkPrefixList("nonexistent", "alsonothere")
	if len(prefixes) != 0 {
		t.Errorf("expected empty prefix list for non-existent link, got %v", prefixes)
	}
}

func TestNeighborIPFromDevenv(t *testing.T) {
	stateFile := generateStateFile(t)
	if err := Init(stateFile); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// KindLeaf should have neighbors for k8s nodes
	ip, err := NeighborIP(KindLeaf, KindControlPlane)
	if err != nil {
		t.Fatalf("NeighborIP failed: %v", err)
	}
	if ip == "" {
		t.Error("NeighborIP returned empty string")
	}
}
