// SPDX-License-Identifier:Apache-2.0

package static

import (
	"github.com/openperouter/openperouter/api/v1alpha1"
)

type PERouterConfig struct {
	NodeIndex      int                        `json:"node_index" yaml:"node_index" toml:"node_index"`
	Underlays      []v1alpha1.UnderlaySpec    `json:"underlays" yaml:"underlays" toml:"underlays"`
	L2VNIs         []v1alpha1.L2VNISpec       `json:"l2vnis" yaml:"l2vnis" toml:"l2vnis"`
	L3VNIs         []v1alpha1.L3VNISpec       `json:"l3vnis" yaml:"l3vnis" toml:"l3vnis"`
	BGPPassthrough v1alpha1.L3PassthroughSpec `json:"bgppassthrough" yaml:"bgppassthrough" toml:"bgppassthrough"`
}
