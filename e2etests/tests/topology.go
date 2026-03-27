// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"github.com/openperouter/openperouter/e2etests/pkg/frr"
	"github.com/openperouter/openperouter/e2etests/pkg/infra"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	"github.com/openperouter/openperouter/pkg/devenv"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = Describe("Configurable dev environment topology", Label("topology"), Ordered, func() {
	var cs clientset.Interface
	var routers openperouter.Routers
	var nodes []corev1.Node

	BeforeAll(func() {
		Expect(infra.Topology).NotTo(BeNil(), "devenv topology must be loaded (set DEVENV_STATE or --devenv-state)")
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())

		cs = k8sclient.New()
		routers, err = openperouter.Get(cs, HostMode)
		Expect(err).NotTo(HaveOccurred())

		nodesItems, err := cs.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		nodes = nodesItems.Items

		err = Updater.Update(config.Resources{
			Underlays: []v1alpha1.Underlay{
				infra.Underlay,
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())
		By("waiting for the router pod to rollout after removing the underlay")
		Eventually(func() error {
			newRouters, err := openperouter.Get(cs, HostMode)
			if err != nil {
				return err
			}
			return openperouter.DaemonsetRolled(routers, newRouters)
		}, 2*time.Minute, time.Second).ShouldNot(HaveOccurred())
	})

	It("validates the default topology", func() {
		By("checking that topology data is populated from devenv")
		Expect(infra.KindLeaf).NotTo(BeEmpty(), "KindLeaf should be populated")
		Expect(infra.LeafA).NotTo(BeEmpty(), "LeafA should be populated")
		Expect(infra.LeafB).NotTo(BeEmpty(), "LeafB should be populated")
		Expect(infra.LeafAConfig.ASN).NotTo(BeZero(), "LeafA ASN should be set")
		Expect(infra.LeafBConfig.ASN).NotTo(BeZero(), "LeafB ASN should be set")
		Expect(infra.LeafKindConfig.ASN).NotTo(BeZero(), "LeafKind ASN should be set")
		Expect(infra.HostARedIPv4).NotTo(BeEmpty(), "HostARedIPv4 should be populated")
		Expect(infra.HostBRedIPv4).NotTo(BeEmpty(), "HostBRedIPv4 should be populated")

		By("verifying the spine-leafkind link IPs match devenv data")
		spineLeafkindIPv4Spine, spineLeafkindIPv4Leafkind, _, _, err := infra.Topology.LinkIPsBetween("spine", "leafkind")
		Expect(err).NotTo(HaveOccurred())
		Expect(spineLeafkindIPv4Spine).NotTo(BeEmpty())
		Expect(spineLeafkindIPv4Leafkind).NotTo(BeEmpty())

		By("verifying BGP sessions between openperouter and leafkind are established")
		leafkindExec := executor.ForContainer(infra.KindLeaf)
		for _, node := range nodes {
			neighborIP, err := infra.NeighborIP(infra.KindLeaf, node.Name)
			Expect(err).NotTo(HaveOccurred())
			validateSessionWithNeighbor(infra.KindLeaf, node.Name, leafkindExec, neighborIP, Established)
		}

		By("verifying the spine-leafkind BGP session is established")
		spineExec := executor.ForContainer(infra.ContainerName("spine"))
		validateSessionWithNeighbor("spine", "leafkind", spineExec, spineLeafkindIPv4Leafkind, Established)
	})

	It("re-establishes BGP sessions after changing the spine-leafkind link IP", func() {
		By("reading original spine-leafkind link IPs")
		origSpineIP, origLeafkindIP, _, _, err := infra.Topology.LinkIPsBetween("spine", "leafkind")
		Expect(err).NotTo(HaveOccurred())

		By("generating a new topology with different link subnet allocation")
		newTopo := regenerateTopologyWithDifferentLinkSubnet(GinkgoT())

		newSpineIP, newLeafkindIP, _, _, err := newTopo.LinkIPsBetween("spine", "leafkind")
		Expect(err).NotTo(HaveOccurred())
		Expect(newSpineIP).NotTo(Equal(origSpineIP), "new spine IP should differ from original")

		By(fmt.Sprintf("swapping spine:eth3 IP from %s to %s", origSpineIP, newSpineIP))
		spineExec := executor.ForContainer(infra.ContainerName("spine"))
		replaceInterfaceIP(spineExec, "eth3", origSpineIP+"/31", newSpineIP+"/31")

		By(fmt.Sprintf("swapping leafkind:eth1 IP from %s to %s", origLeafkindIP, newLeafkindIP))
		leafkindExec := executor.ForContainer(infra.KindLeaf)
		replaceInterfaceIP(leafkindExec, "eth1", origLeafkindIP+"/31", newLeafkindIP+"/31")

		DeferCleanup(func() {
			By("restoring original spine-leafkind link IPs")
			replaceInterfaceIP(spineExec, "eth3", newSpineIP+"/31", origSpineIP+"/31")
			replaceInterfaceIP(leafkindExec, "eth1", newLeafkindIP+"/31", origLeafkindIP+"/31")

			By("reloading original FRR configs")
			reloadSpineConfig(infra.Topology)
			reloadLeafkindConfig(infra.Topology, nodes)

			By("verifying original BGP sessions are restored")
			Eventually(func() error {
				_, err := frr.NeighborInfo(origLeafkindIP, spineExec)
				return err
			}, 3*time.Minute, time.Second).ShouldNot(HaveOccurred())
		})

		By("reloading FRR config on spine with new peer IP for leafkind")
		reloadSpineConfig(newTopo)

		By("reloading FRR config on leafkind with new peer IP for spine")
		reloadLeafkindConfig(newTopo, nodes)

		By("verifying spine-leafkind BGP session re-establishes with new IPs")
		Eventually(func() error {
			neigh, err := frr.NeighborInfo(newLeafkindIP, spineExec)
			if err != nil {
				return fmt.Errorf("querying spine for neighbor %s: %w", newLeafkindIP, err)
			}
			if neigh.BgpState != "Established" {
				return fmt.Errorf("spine->leafkind session with %s is %s, want Established", newLeafkindIP, neigh.BgpState)
			}
			return nil
		}, 3*time.Minute, time.Second).ShouldNot(HaveOccurred())

		By("verifying openperouter-leafkind sessions are still established")
		for _, node := range nodes {
			neighborIP, err := infra.NeighborIP(infra.KindLeaf, node.Name)
			Expect(err).NotTo(HaveOccurred())
			validateSessionWithNeighbor(infra.KindLeaf, node.Name, leafkindExec, neighborIP, Established)
		}
	})
})

// regenerateTopologyWithDifferentLinkSubnet creates a new ClabConfig using
// a modified env config with a different link_subnet_base_v4.
func regenerateTopologyWithDifferentLinkSubnet(t GinkgoTInterface) *devenv.ClabConfig {
	t.Helper()

	origEnvData, err := os.ReadFile(infra.Topology.EnvConfigFile)
	Expect(err).NotTo(HaveOccurred())

	// Create a temporary env config with different link subnet base.
	// The original uses 192.168.1.0/24; we change to 192.168.100.0/24.
	modifiedEnv := replaceInString(string(origEnvData),
		infra.Topology.Allocations.LinkSubnetBase4.String(), "192.168.100.0/24")

	tmpDir := GinkgoT().TempDir()
	tmpEnvFile := filepath.Join(tmpDir, "env.yaml")
	Expect(os.WriteFile(tmpEnvFile, []byte(modifiedEnv), 0644)).To(Succeed())

	newTopo, err := devenv.LoadFromEnvConfig(infra.Topology.TopoFile, tmpEnvFile)
	Expect(err).NotTo(HaveOccurred())

	return newTopo
}

// replaceInterfaceIP removes the old IP and adds the new IP on the given interface.
func replaceInterfaceIP(exec executor.Executor, iface, oldCIDR, newCIDR string) {
	out, err := exec.Exec("ip", "addr", "del", oldCIDR, "dev", iface)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "failed to delete IP %s from %s: %s", oldCIDR, iface, out)
	out, err = exec.Exec("ip", "addr", "add", newCIDR, "dev", iface)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "failed to add IP %s to %s: %s", newCIDR, iface, out)
}

// reloadSpineConfig regenerates and reloads FRR config on the spine container
// using BGP peer addresses from the given topology.
func reloadSpineConfig(topo *devenv.ClabConfig) {
	spineNode, err := topo.NodeByName("spine")
	Expect(err).NotTo(HaveOccurred())

	// The spine FRR config is generated by devenv during LoadFromEnvConfig.
	Expect(spineNode.FRRConfig).NotTo(BeEmpty(), "spine FRR config should be generated")

	spineContainer := infra.ContainerName("spine")
	container := frr.Container{Name: spineContainer, ConfigPath: "spine"}
	Expect(container.ReloadConfig(spineNode.FRRConfig)).To(Succeed())
}

// reloadLeafkindConfig regenerates and reloads FRR config on the leafkind container,
// using the spine address from the given topology and the existing kind-node neighbors.
func reloadLeafkindConfig(topo *devenv.ClabConfig, nodes []corev1.Node) {
	spineIP := infra.NodeIPv4("spine", "eth3")

	// For the new topology, extract the spine's new IP for leafkind
	newSpineIP, _, _, _, err := topo.LinkIPsBetween("spine", "leafkind")
	Expect(err).NotTo(HaveOccurred())
	if newSpineIP != "" {
		spineIP = newSpineIP
	}

	neighbors := []string{}
	for _, node := range nodes {
		neighborIP, err := infra.NeighborIP(infra.KindLeaf, node.Name)
		Expect(err).NotTo(HaveOccurred())
		neighbors = append(neighbors, neighborIP)
	}

	leafkindCfg := infra.LeafKindConfiguration{
		ASN:          infra.LeafKindConfig.ASN,
		SpineASN:     infra.LeafKindConfig.SpineASN,
		SpineAddress: spineIP,
		PeerASN:      infra.Underlay.Spec.ASN,
		Neighbors:    neighbors,
	}

	configString, err := infra.LeafKindConfigToFRR(leafkindCfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(infra.LeafKindConfig.ReloadConfig(configString)).To(Succeed())
}

func replaceInString(s, old, replacement string) string {
	return strings.ReplaceAll(s, old, replacement)
}
