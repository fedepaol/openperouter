// SPDX-License-Identifier:Apache-2.0

package staticconfiguration

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/openperouter/openperouter/api/static"
	"github.com/openperouter/openperouter/internal/ipfamily"
	"sigs.k8s.io/yaml"
)

// NoConfigAvailable is returned when no configuration files are available.
type NoConfigAvailable struct {
	message string
}

func (e *NoConfigAvailable) Error() string {
	return e.message
}

// FileExists checks if a file exists at the given path.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadNodeConfig reads a NodeConfig from a YAML file.
// If the file does not exist, returns an empty config.
func ReadNodeConfig(path string) (*static.NodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read node config file: %w", err)
	}

	var config static.NodeConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML node config: %w", err)
	}

	return &config, nil
}

// ReadRouterConfigs reads all openpe_*.yaml files from a directory.
// Returns NoConfigAvailable error if the directory doesn't exist or contains no matching files.
func ReadRouterConfigs(configDir string) ([]*static.PERouterConfig, error) {
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return nil, &NoConfigAvailable{
			message: fmt.Sprintf("configuration directory does not exist: %s", configDir),
		}
	}

	pattern := filepath.Join(configDir, "openpe_*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob pattern %s: %w", pattern, err)
	}

	if len(matches) == 0 {
		return nil, &NoConfigAvailable{
			message: fmt.Sprintf("no openpe_*.yaml configuration files found in directory: %s", configDir),
		}
	}

	configs := make([]*static.PERouterConfig, 0, len(matches))
	for _, path := range matches {
		config, err := readRouterConfig(path)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}

	return configs, nil
}

// readRouterConfig reads a PERouterConfig from a single YAML file.
func readRouterConfig(path string) (*static.PERouterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read router config file %s: %w", path, err)
	}

	var config static.PERouterConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML router config %s: %w", path, err)
	}

	return &config, nil
}

const (
	familyIPv4 = "ipv4"
	familyIPv6 = "ipv6"
)

// ResolveNodeIndex determines the node index from the configuration.
// If NodeIndex is explicitly set, it takes precedence. Otherwise, if
// NodeIndexFromInterface is set, the index is derived from the interface IP.
func ResolveNodeIndex(config *static.NodeConfig) (int, error) {
	if config.NodeIndex != nil {
		return *config.NodeIndex, nil
	}
	if config.NodeIndexFromInterface != nil {
		return deriveNodeIndexFromInterface(config.NodeIndexFromInterface)
	}
	return 0, fmt.Errorf("node index not configured: set either nodeIndex or nodeIndexFromInterface in node config")
}

func deriveNodeIndexFromInterface(cfg *static.NodeIndexFromInterface) (int, error) {
	if err := validateNodeIndexFromInterface(cfg); err != nil {
		return 0, fmt.Errorf("invalid nodeIndexFromInterface config: %w", err)
	}

	iface, err := net.InterfaceByName(cfg.Interface)
	if err != nil {
		return 0, fmt.Errorf("interface %q not found: %w", cfg.Interface, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return 0, fmt.Errorf("failed to get addresses for interface %q: %w", cfg.Interface, err)
	}

	targetFamily := ipfamily.IPv4
	if cfg.Family == familyIPv6 {
		targetFamily = ipfamily.IPv6
	}

	var networkFilter *net.IPNet
	if cfg.Network != "" {
		_, networkFilter, err = net.ParseCIDR(cfg.Network)
		if err != nil {
			return 0, fmt.Errorf("invalid network filter %q: %w", cfg.Network, err)
		}
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipfamily.ForAddress(ipNet.IP) != targetFamily {
			continue
		}
		if networkFilter != nil && !networkFilter.Contains(ipNet.IP) {
			continue
		}
		return hostIndex(ipNet.IP, cfg.Len, targetFamily), nil
	}

	if networkFilter != nil {
		return 0, fmt.Errorf("no %s address on interface %q matches network %q", cfg.Family, cfg.Interface, cfg.Network)
	}
	return 0, fmt.Errorf("no %s address found on interface %q", cfg.Family, cfg.Interface)
}

func validateNodeIndexFromInterface(cfg *static.NodeIndexFromInterface) error {
	if cfg.Interface == "" {
		return fmt.Errorf("interface must not be empty")
	}
	if cfg.Family != familyIPv4 && cfg.Family != familyIPv6 {
		return fmt.Errorf("family must be \"ipv4\" or \"ipv6\", got %q", cfg.Family)
	}
	if cfg.Family == familyIPv4 {
		if cfg.Len < 1 || cfg.Len > 31 {
			return fmt.Errorf("len must be between 1 and 31 for ipv4, got %d", cfg.Len)
		}
	}
	if cfg.Family == familyIPv6 {
		if cfg.Len < 1 || cfg.Len > 127 {
			return fmt.Errorf("len must be between 1 and 127 for ipv6, got %d", cfg.Len)
		}
	}
	if cfg.Network != "" {
		if _, _, err := net.ParseCIDR(cfg.Network); err != nil {
			return fmt.Errorf("invalid network CIDR %q: %w", cfg.Network, err)
		}
	}
	return nil
}

func hostIndex(ip net.IP, prefixLen int, family ipfamily.Family) int {
	totalBits := 32
	if family == ipfamily.IPv6 {
		totalBits = 128
		ip = ip.To16()
	} else {
		ip = ip.To4()
	}

	mask := net.CIDRMask(prefixLen, totalBits)
	hostBits := totalBits - prefixLen

	// Extract host portion by ANDing with inverted mask
	hostBytes := make([]byte, len(ip))
	for i := range ip {
		hostBytes[i] = ip[i] &^ mask[i]
	}

	if family == ipfamily.IPv4 {
		return int(binary.BigEndian.Uint32(hostBytes))
	}

	// For IPv6, use the last 8 bytes (64 bits max from host portion)
	if hostBits <= 32 {
		return int(binary.BigEndian.Uint32(hostBytes[12:16]))
	}
	return int(binary.BigEndian.Uint64(hostBytes[8:16]))
}
