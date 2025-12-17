// SPDX-License-Identifier:Apache-2.0

package conversion

import (
	"fmt"

	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/internal/hostnetwork"
)

type ApiConfigData struct {
	NodeIndex          int
	UnderlayFromMultus bool
	Underlays          []v1alpha1.Underlay
	L3VNIs             []v1alpha1.L3VNI
	L2VNIs             []v1alpha1.L2VNI
	L3Passthrough      []v1alpha1.L3Passthrough
	LogLevel           string
}

type HostConfigData struct {
	Underlay      hostnetwork.UnderlayParams
	L3VNIs        []hostnetwork.L3VNIParams
	L2VNIs        []hostnetwork.L2VNIParams
	L3Passthrough *hostnetwork.PassthroughParams
}

func MergeAPIConfigs(configs ...ApiConfigData) (ApiConfigData, error) {
	if len(configs) == 0 {
		return ApiConfigData{}, nil
	}

	merged := ApiConfigData{
		NodeIndex:          configs[0].NodeIndex,
		UnderlayFromMultus: configs[0].UnderlayFromMultus,
		LogLevel:           configs[0].LogLevel,
	}

	for i, config := range configs {
		if i > 0 && config.NodeIndex != merged.NodeIndex {
			return ApiConfigData{}, fmt.Errorf("NodeIndex mismatch: config[0] has %d, config[%d] has %d",
				merged.NodeIndex, i, config.NodeIndex)
		}

		if i > 0 && config.UnderlayFromMultus != merged.UnderlayFromMultus {
			return ApiConfigData{}, fmt.Errorf("UnderlayFromMultus mismatch: config[0] has %v, config[%d] has %v",
				merged.UnderlayFromMultus, i, config.UnderlayFromMultus)
		}

		merged.Underlays = append(merged.Underlays, config.Underlays...)
		merged.L3VNIs = append(merged.L3VNIs, config.L3VNIs...)
		merged.L2VNIs = append(merged.L2VNIs, config.L2VNIs...)
		merged.L3Passthrough = append(merged.L3Passthrough, config.L3Passthrough...)
	}

	return merged, nil
}
