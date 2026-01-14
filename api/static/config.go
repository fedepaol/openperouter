// SPDX-License-Identifier:Apache-2.0

package static

import (
	"github.com/openperouter/openperouter/api/v1alpha1"
)

type NodeConfig struct {
	NodeIndex int    `json:"nodeIndex"`
	NodeName  string `json:"nodeName"`
	LogLevel  string `json:"logLevel"`
}

type PERouterConfig struct {
	Underlays      []v1alpha1.UnderlaySpec    `json:"underlays"`
	L2VNIs         []v1alpha1.L2VNISpec       `json:"l2vnis"`
	L3VNIs         []v1alpha1.L3VNISpec       `json:"l3vnis"`
	BGPPassthrough v1alpha1.L3PassthroughSpec `json:"bgppassthrough"`
}
