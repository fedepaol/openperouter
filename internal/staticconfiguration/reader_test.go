// SPDX-License-Identifier:Apache-2.0

package staticconfiguration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openperouter/openperouter/api/static"
)

func TestReadNodeConfig(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expected    *static.NodeConfig
		expectError bool
	}{
		{
			name:     "valid yaml config",
			content:  "nodeIndex: 42\nlogLevel: debug\n",
			expected: &static.NodeConfig{NodeIndex: 42, LogLevel: "debug"},
		},
		{
			name:     "valid yaml with zero value",
			content:  "nodeIndex: 0\nlogLevel: info\n",
			expected: &static.NodeConfig{NodeIndex: 0, LogLevel: "info"},
		},
		{
			name:     "valid yaml with only nodeIndex",
			content:  "nodeIndex: 1\n",
			expected: &static.NodeConfig{NodeIndex: 1, LogLevel: ""},
		},
		{
			name:        "invalid yaml",
			content:     "invalid: [unclosed\n",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "node-config.yaml")

			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test config file: %v", err)
			}

			config, err := ReadNodeConfig(configPath)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if config.NodeIndex != tt.expected.NodeIndex {
				t.Errorf("expected NodeIndex %d, got %d", tt.expected.NodeIndex, config.NodeIndex)
			}

			if config.LogLevel != tt.expected.LogLevel {
				t.Errorf("expected LogLevel %s, got %s", tt.expected.LogLevel, config.LogLevel)
			}
		})
	}
}

func TestReadNodeConfig_NonExistentFile(t *testing.T) {
	_, err := ReadNodeConfig("/nonexistent/path/node-config.yaml")
	if err == nil {
		t.Errorf("expected error for non-existent file, got: %v", err)
	}
}

func TestReadRouterConfig_NonExistentFile(t *testing.T) {
	_, err := ReadRouterConfig("/nonexistent/path/openpe_test.yaml")
	if err == nil {
		t.Error("expected error when reading non-existent file")
	}
}

func TestReadRouterConfigs(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		configs, err := ReadRouterConfigs(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(configs) != 0 {
			t.Errorf("expected 0 configs, got %d", len(configs))
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		configs, err := ReadRouterConfigs("/nonexistent/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(configs) != 0 {
			t.Errorf("expected 0 configs, got %d", len(configs))
		}
	})

	t.Run("single file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "openpe_underlay.yaml")
		content := `underlays:
  - asn: 64515
    routeridcidr: "10.0.0.0/24"
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test config file: %v", err)
		}

		configs, err := ReadRouterConfigs(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(configs) != 1 {
			t.Fatalf("expected 1 config, got %d", len(configs))
		}
		if len(configs[0].Underlays) != 1 {
			t.Errorf("expected 1 underlay, got %d", len(configs[0].Underlays))
		}
	})

	t.Run("multiple files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create first config file
		configPath1 := filepath.Join(tmpDir, "openpe_underlay.yaml")
		content1 := `underlays:
  - asn: 64515
    routeridcidr: "10.0.0.0/24"
`
		if err := os.WriteFile(configPath1, []byte(content1), 0644); err != nil {
			t.Fatalf("failed to write test config file: %v", err)
		}

		// Create second config file
		configPath2 := filepath.Join(tmpDir, "openpe_l3vni.yaml")
		content2 := `l3vnis:
  - vrf: "vrf-test"
    vni: 1000
`
		if err := os.WriteFile(configPath2, []byte(content2), 0644); err != nil {
			t.Fatalf("failed to write test config file: %v", err)
		}

		// Create a non-matching file (should be ignored)
		nonMatchingPath := filepath.Join(tmpDir, "other.yaml")
		if err := os.WriteFile(nonMatchingPath, []byte("test: value\n"), 0644); err != nil {
			t.Fatalf("failed to write non-matching file: %v", err)
		}

		configs, err := ReadRouterConfigs(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(configs) != 2 {
			t.Fatalf("expected 2 configs, got %d", len(configs))
		}

		// Verify contents
		var hasUnderlay, hasL3VNI bool
		for _, cfg := range configs {
			if len(cfg.Underlays) > 0 {
				hasUnderlay = true
			}
			if len(cfg.L3VNIs) > 0 {
				hasL3VNI = true
			}
		}
		if !hasUnderlay {
			t.Error("expected at least one config with underlays")
		}
		if !hasL3VNI {
			t.Error("expected at least one config with l3vnis")
		}
	})

	t.Run("invalid file in directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "openpe_invalid.yaml")
		content := "invalid: [unclosed\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test config file: %v", err)
		}

		_, err := ReadRouterConfigs(tmpDir)
		if err == nil {
			t.Error("expected error for invalid YAML file")
		}
	})

	t.Run("comprehensive testdata files", func(t *testing.T) {
		// Use the actual testdata directory with comprehensive resource types
		testdataDir := "./testdata"

		configs, err := ReadRouterConfigs(testdataDir)
		if err != nil {
			t.Fatalf("unexpected error reading testdata: %v", err)
		}

		if len(configs) != 4 {
			t.Fatalf("expected 4 config files, got %d", len(configs))
		}

		// Aggregate all configs to verify we have all resource types
		for _, cfg := range configs {
			if len(cfg.Underlays) > 0 {
				if len(cfg.Underlays) != 1 {
					t.Errorf("expected 1 underlay, got %d", len(cfg.Underlays))
				}
				// Verify underlay details
				underlay := cfg.Underlays[0]
				if underlay.ASN != 64514 {
					t.Errorf("expected underlay ASN 64514, got %d", underlay.ASN)
				}
				if len(underlay.Nics) < 2 {
					t.Errorf("expected at least 2 NICs in underlay, got %d", len(underlay.Nics))
				}
				if len(underlay.Neighbors) < 2 {
					t.Errorf("expected at least 2 neighbors in underlay, got %d", len(underlay.Neighbors))
				}
				if underlay.EVPN == nil {
					t.Error("expected EVPN config in underlay")
				}
			}

			if len(cfg.L3VNIs) > 0 {
				if len(cfg.L3VNIs) != 2 {
					t.Errorf("expected 2 L3VNIs, got %d", len(cfg.L3VNIs))
				}
				// Verify L3VNI details
				for _, vni := range cfg.L3VNIs {
					if vni.VRF == "" {
						t.Error("expected VRF name in L3VNI")
					}
					if vni.VNI == 0 {
						t.Error("expected non-zero VNI")
					}
					if vni.HostSession == nil {
						t.Error("expected HostSession in L3VNI")
					} else {
						if vni.HostSession.ASN == 0 {
							t.Error("expected non-zero ASN in HostSession")
						}
						if vni.HostSession.LocalCIDR.IPv4 == "" && vni.HostSession.LocalCIDR.IPv6 == "" {
							t.Error("expected at least one LocalCIDR in HostSession")
						}
					}
				}
			}

			if len(cfg.L2VNIs) > 0 {
				if len(cfg.L2VNIs) != 2 {
					t.Errorf("expected 2 L2VNIs, got %d", len(cfg.L2VNIs))
				}
				// Verify L2VNI details
				for _, vni := range cfg.L2VNIs {
					if vni.VNI == 0 {
						t.Error("expected non-zero VNI in L2VNI")
					}
					if vni.HostMaster == nil {
						t.Error("expected HostMaster in L2VNI")
					}
				}
			}

			if cfg.BGPPassthrough.HostSession.ASN != 0 {
				// Verify BGPPassthrough details
				if cfg.BGPPassthrough.HostSession.LocalCIDR.IPv4 == "" && cfg.BGPPassthrough.HostSession.LocalCIDR.IPv6 == "" {
					t.Error("expected at least one LocalCIDR in BGPPassthrough")
				}
			}
		}
	})
}
