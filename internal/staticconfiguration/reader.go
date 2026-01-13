// SPDX-License-Identifier:Apache-2.0

package staticconfiguration

import (
	"fmt"
	"os"

	"github.com/openperouter/openperouter/api/static"
	"sigs.k8s.io/yaml"
)

// FileExists checks if a file exists at the given path.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadFromFile reads a PERouterConfig from a YAML file.
// If the file does not exist, returns an empty config.
func ReadFromFile(path string) (*static.PERouterConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &static.PERouterConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config static.PERouterConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &config, nil
}
