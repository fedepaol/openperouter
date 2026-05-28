// SPDX-License-Identifier:Apache-2.0

package tests

import (
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/e2etests/pkg/infra"
	corev1 "k8s.io/api/core/v1"
)

func redistributeConnectedForLeaf(leaf infra.Leaf) {
	leafConfiguration := infra.LeafConfiguration{
		Leaf: leaf,
		Red: infra.Addresses{
			RedistributeConnected: true,
		},
		Blue: infra.Addresses{
			RedistributeConnected: true,
		},
		Default: infra.Addresses{
			RedistributeConnected: true,
		},
	}
	config, err := infra.LeafConfigToFRR(leafConfiguration)
	Expect(err).NotTo(HaveOccurred())
	err = leaf.ReloadConfig(config)
	Expect(err).NotTo(HaveOccurred())
}

func redistributeConnectedForLeafKind(nodes []corev1.Node) {
	leafKinds := []struct {
		config infra.LeafKind
		name   string
	}{
		{infra.LeafKind1Config, infra.KindLeaf},
		{infra.LeafKind2Config, infra.KindLeaf2},
	}
	for _, lk := range leafKinds {
		neighbors := []infra.Neighbor{}
		for _, node := range nodes {
			neighborIP, err := infra.NeighborIP(lk.name, node.Name)
			Expect(err).NotTo(HaveOccurred())
			neighbors = append(neighbors, infra.Neighbor{ID: neighborIP})
		}
		config := infra.LeafKindConfiguration{
			ASN:                   lk.config.ASN,
			PERouterASN:           64514,
			SpinePeerAddress:      lk.config.SpinePeerAddress,
			RedistributeConnected: true,
			Neighbors:             neighbors,
		}
		configString, err := infra.LeafKindConfigToFRR(config)
		Expect(err).NotTo(HaveOccurred())
		err = lk.config.ReloadConfig(configString)
		Expect(err).NotTo(HaveOccurred())
	}
}

func ibgpForLeafKind(nodes []corev1.Node) {
	leafKinds := []struct {
		config infra.LeafKind
		name   string
	}{
		{infra.LeafKind1Config, infra.KindLeaf},
		{infra.LeafKind2Config, infra.KindLeaf2},
	}
	for _, lk := range leafKinds {
		neighbors := []infra.Neighbor{}
		for _, node := range nodes {
			neighborIP, err := infra.NeighborIP(lk.name, node.Name)
			Expect(err).NotTo(HaveOccurred())
			neighbors = append(neighbors, infra.Neighbor{ID: neighborIP})
		}
		config := infra.LeafKindConfiguration{
			ASN:              64512,
			PERouterASN:      64512,
			SpinePeerAddress: lk.config.SpinePeerAddress,
			NextHopSelf:      true,
			Neighbors:        neighbors,
		}
		configString, err := infra.LeafKindConfigToFRR(config)
		Expect(err).NotTo(HaveOccurred())
		err = lk.config.ReloadConfig(configString)
		Expect(err).NotTo(HaveOccurred())
	}
}

func resetLeafKindConfig(nodes []corev1.Node) {
	err := infra.LeafKind1Config.UpdateConfig(nodes, infra.LeafKindConfiguration{})
	Expect(err).NotTo(HaveOccurred())
	err = infra.LeafKind2Config.UpdateConfig(nodes, infra.LeafKindConfiguration{})
	Expect(err).NotTo(HaveOccurred())
}
