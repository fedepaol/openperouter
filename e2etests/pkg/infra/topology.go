// SPDX-License-Identifier:Apache-2.0

package infra

import (
	"fmt"
	"os"
	"sort"

	"github.com/openperouter/openperouter/pkg/devenv"
)

// Topology holds the loaded devenv ClabConfig.
var Topology *devenv.ClabConfig

func init() {
	if state := os.Getenv("DEVENV_STATE"); state != "" {
		if err := Init(state); err != nil {
			panic(fmt.Sprintf("infra: failed to load devenv state from DEVENV_STATE=%q: %v", state, err))
		}
	}
}

// Init loads the devenv state file and populates all backward-compatible package vars.
func Init(stateFile string) error {
	cfg, err := devenv.LoadFromState(stateFile)
	if err != nil {
		return fmt.Errorf("loading devenv state: %w", err)
	}
	Topology = cfg
	populateVars()
	return nil
}

// ContainerName returns the containerlab container name for a node.
// ext-container nodes keep their original name; others get the clab-<lab>- prefix.
func ContainerName(nodeName string) string {
	if Topology == nil {
		return ""
	}
	node := Topology.Nodes[nodeName]
	if node != nil && (node.Kind == "ext-container" || node.Kind == "k8s-kind") {
		return nodeName
	}
	return fmt.Sprintf("clab-%s-%s", Topology.Name, nodeName)
}

// NodeIPv4 returns the IPv4 address (without prefix length) for a node's interface.
func NodeIPv4(nodeName, ifaceName string) string {
	if Topology == nil {
		return ""
	}
	node, ok := Topology.Nodes[nodeName]
	if !ok {
		return ""
	}
	iface, ok := node.Interfaces[ifaceName]
	if !ok {
		return ""
	}
	if !iface.IPv4.IsValid() {
		return ""
	}
	return iface.IPv4.Addr().String()
}

// NodeIPv6 returns the IPv6 address (without prefix length) for a node's interface.
func NodeIPv6(nodeName, ifaceName string) string {
	if Topology == nil {
		return ""
	}
	node, ok := Topology.Nodes[nodeName]
	if !ok {
		return ""
	}
	iface, ok := node.Interfaces[ifaceName]
	if !ok {
		return ""
	}
	if !iface.IPv6.IsValid() {
		return ""
	}
	return iface.IPv6.Addr().String()
}

func populateVars() {
	populateRouterVars()
	populateLeafVars()
	populateNodeVars()
	populateUnderlayVars()
}

// populateRouterVars builds the router name vars and neighbor link map from devenv data.
func populateRouterVars() {
	ClabPrefix = fmt.Sprintf("clab-%s-", Topology.Name)
	KindLeaf = ContainerName("leafkind")
	LeafA = ContainerName("leafA")
	LeafB = ContainerName("leafB")

	KindLeafContainer.Name = KindLeaf
	KindLeafContainer.ConfigPath = "leafkind"
	LeafAContainer.Name = LeafA
	LeafAContainer.ConfigPath = "leafA"
	LeafBContainer.Name = LeafB
	LeafBContainer.ConfigPath = "leafB"

	links = linksForRouters{nodes: map[string]node{}}

	// Build P2P link neighbor entries
	for _, link := range Topology.Links {
		if link.Type == devenv.LinkP2P {
			nodeA := link.Endpoints[0].Node
			nodeB := link.Endpoints[1].Node
			ipA := NodeIPv4(nodeA, link.Endpoints[0].Interface)
			ipB := NodeIPv4(nodeB, link.Endpoints[1].Interface)
			if ipA != "" && ipB != "" {
				links.Add(ContainerName(nodeA), ContainerName(nodeB), ipA, ipB)
			}
		}
	}

	// Build broadcast segment neighbor entries
	// Group non-bridge endpoints by their bridge node
	type member struct{ node, iface string }
	bridgeGroups := map[string][]member{}
	for _, link := range Topology.Links {
		if link.Type != devenv.LinkBroadcast {
			continue
		}
		for i, ep := range link.Endpoints {
			n := Topology.Nodes[ep.Node]
			if n != nil && n.Kind == "bridge" {
				other := link.Endpoints[1-i]
				bridgeGroups[ep.Node] = append(bridgeGroups[ep.Node], member{
					node: other.Node, iface: other.Interface,
				})
				break
			}
		}
	}

	// For each bridge group, add neighbor entries between all pairs
	for _, members := range bridgeGroups {
		sort.Slice(members, func(i, j int) bool { return members[i].node < members[j].node })
		for i, mA := range members {
			for j, mB := range members {
				if i == j {
					continue
				}
				ipA := NodeIPv4(mA.node, mA.iface)
				ipB := NodeIPv4(mB.node, mB.iface)
				if ipA != "" && ipB != "" {
					links.Add(ContainerName(mA.node), ContainerName(mB.node), ipA, ipB)
				}
			}
		}
	}
}

// populateLeafVars populates leaf config vars from devenv topology data.
func populateLeafVars() {
	leafA := Topology.Nodes["leafA"]
	leafB := Topology.Nodes["leafB"]
	leafkind := Topology.Nodes["leafkind"]
	spine := Topology.Nodes["spine"]

	// Populate host IPs from link allocations
	HostARedIPv4 = NodeIPv4("hostA_red", "eth1")
	HostABlueIPv4 = NodeIPv4("hostA_blue", "eth1")
	HostADefaultIPv4 = NodeIPv4("hostA_default", "eth1")
	HostBRedIPv4 = NodeIPv4("hostB_red", "eth1")
	HostBBlueIPv4 = NodeIPv4("hostB_blue", "eth1")

	HostARedIPv6 = NodeIPv6("hostA_red", "eth1")
	HostABlueIPv6 = NodeIPv6("hostA_blue", "eth1")
	HostBRedIPv6 = NodeIPv6("hostB_red", "eth1")
	HostBBlueIPv6 = NodeIPv6("hostB_blue", "eth1")

	// Find spine's IP on the link to each leaf
	spineAddrForLeafA := ""
	spineAddrForLeafB := ""
	spineAddrForLeafKind := ""
	if spine != nil {
		for _, iface := range spine.Interfaces {
			if iface.PeerNode == "leafA" {
				spineAddrForLeafA = iface.IPv4.Addr().String()
			}
			if iface.PeerNode == "leafB" {
				spineAddrForLeafB = iface.IPv4.Addr().String()
			}
			if iface.PeerNode == "leafkind" {
				spineAddrForLeafKind = iface.IPv4.Addr().String()
			}
		}
	}

	if leafA != nil {
		LeafAConfig = Leaf{
			VTEPIP:       leafA.VTEPIP.String(),
			ASN:          leafA.ASN,
			SpineASN:     spine.ASN,
			SpineAddress: spineAddrForLeafA,
			Container:    LeafAContainer,
		}
	}
	if leafB != nil {
		LeafBConfig = Leaf{
			VTEPIP:       leafB.VTEPIP.String(),
			ASN:          leafB.ASN,
			SpineASN:     spine.ASN,
			SpineAddress: spineAddrForLeafB,
			Container:    LeafBContainer,
		}
	}
	if leafkind != nil {
		LeafKindConfig = LeafKind{
			ASN:          leafkind.ASN,
			SpineASN:     spine.ASN,
			SpineAddress: spineAddrForLeafKind,
			Container:    KindLeafContainer,
		}
	}
}

// populateNodeVars sets K8s node name vars from devenv data.
func populateNodeVars() {
	KindControlPlane = "pe-kind-control-plane"
	KindWorker = "pe-kind-worker"
}

// LinkPrefixList returns the IPv4 and IPv6 link subnet prefixes between two nodes.
// Returns a slice like ["10.0.0.0/31", "fd00::/127"] for use in ChangePrefixes.
func LinkPrefixList(nodeA, nodeB string) []string {
	if Topology == nil {
		return nil
	}
	var result []string
	for _, link := range Topology.Links {
		if (link.Endpoints[0].Node == nodeA && link.Endpoints[1].Node == nodeB) ||
			(link.Endpoints[0].Node == nodeB && link.Endpoints[1].Node == nodeA) {
			if link.Subnet4.IsValid() {
				result = append(result, link.Subnet4.String())
			}
			if link.Subnet6.IsValid() {
				result = append(result, link.Subnet6.String())
			}
			return result
		}
	}
	return result
}

// populateUnderlayVars populates the Underlay resource with devenv topology data.
func populateUnderlayVars() {
	leafkind := Topology.Nodes["leafkind"]
	if leafkind == nil {
		return
	}

	// Neighbor ASN comes from leafkind
	Underlay.Spec.Neighbors[0].ASN = leafkind.ASN

	// Neighbor address is leafkind's IP on the toswitch (broadcast) interface
	toswitchIP := NodeIPv4("leafkind", "toswitch")
	if toswitchIP != "" {
		Underlay.Spec.Neighbors[0].Address = toswitchIP
	}
}
