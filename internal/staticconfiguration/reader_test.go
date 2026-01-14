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

func TestReadRouterConfig(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		validate    func(*testing.T, *static.PERouterConfig)
	}{
		{
			name: "valid config with underlays",
			content: `underlays:
  - asn: 64515
    routeridcidr: "10.0.0.0/24"
`,
			expectError: false,
			validate: func(t *testing.T, cfg *static.PERouterConfig) {
				if len(cfg.Underlays) != 1 {
					t.Errorf("expected 1 underlay, got %d", len(cfg.Underlays))
				}
				if cfg.Underlays[0].ASN != 64515 {
					t.Errorf("expected ASN 64515, got %d", cfg.Underlays[0].ASN)
				}
			},
		},
		{
			name: "valid config with l3vnis",
			content: `l3vnis:
  - vrf: "vrf-test"
    vni: 1000
`,
			expectError: false,
			validate: func(t *testing.T, cfg *static.PERouterConfig) {
				if len(cfg.L3VNIs) != 1 {
					t.Errorf("expected 1 l3vni, got %d", len(cfg.L3VNIs))
				}
				if cfg.L3VNIs[0].VRF != "vrf-test" {
					t.Errorf("expected VRF vrf-test, got %s", cfg.L3VNIs[0].VRF)
				}
			},
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
			configPath := filepath.Join(tmpDir, "openpe_test.yaml")

			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test config file: %v", err)
			}

			config, err := ReadRouterConfig(configPath)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
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
}
