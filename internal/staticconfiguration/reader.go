// SPDX-License-Identifier:Apache-2.0

package staticconfiguration

import (
	"fmt"
	"os"
	"sync"

	staticapi "github.com/openperouter/openperouter/api/static"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type StaticConfig struct {
	path          string
	notifyChannel chan event.GenericEvent
	sync.Mutex
	lastReadConfig *staticapi.PERouterConfig
}

func (s *StaticConfig) reload() error {
	if err := s.read(); err != nil {
		return err
	}

	// Notify listeners of the configuration change
	s.notifyChannel <- event.GenericEvent{}

	return nil
}

// read reads a PERouterConfig from a YAML file.
func (s *StaticConfig) read() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config staticapi.PERouterConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse YAML config: %w", err)
	}

	s.Lock()
	defer s.Unlock()
	s.lastReadConfig = &config
	return nil
}

// ReadFromFile reads a PERouterConfig from a YAML file.
func ReadFromFile(path string) (*staticapi.PERouterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config staticapi.PERouterConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &config, nil
}
