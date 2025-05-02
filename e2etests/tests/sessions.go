// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"context"
	"fmt"
	"time"

	frrk8sv1beta1 "github.com/metallb/frr-k8s/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"github.com/openperouter/openperouter/e2etests/pkg/frr"
	"github.com/openperouter/openperouter/e2etests/pkg/frrk8s"
	"github.com/openperouter/openperouter/e2etests/pkg/infra"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

var _ = Describe("Router Host configuration", Ordered, func() {
	var cs clientset.Interface
	routerPods := []*corev1.Pod{}
	frrk8sPods := []*corev1.Pod{}
	nodes := []corev1.Node{}

	BeforeAll(func() {
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())

		cs = k8sclient.New()
		routerPods, err = openperouter.RouterPods(cs)
		Expect(err).NotTo(HaveOccurred())
		frrk8sPods, err = frrk8s.Pods(cs)
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
			return openperouter.DaemonsetRolled(cs, routerPods)
		}, 2*time.Minute, time.Second).ShouldNot(HaveOccurred())
	})

	BeforeEach(func() {
		err := Updater.CleanButUnderlay()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := Updater.CleanButUnderlay()
		Expect(err).NotTo(HaveOccurred())
	})

	It("peers with the tor", func() {
		exec := executor.ForContainer(infra.KindLeaf)
		Eventually(func() error {
			for _, node := range nodes {
				neighborIP, err := infra.NeighborIP(infra.KindLeaf, node.Name)
				Expect(err).NotTo(HaveOccurred())
				validateSessionWithNeighbor(exec, neighborIP, Established)
			}
			return nil
		}, time.Minute, time.Second).ShouldNot(HaveOccurred())
	})

	Context("with a vni", func() {
		vni := v1alpha1.VNI{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "red",
				Namespace: openperouter.Namespace,
			},
			Spec: v1alpha1.VNISpec{
				ASN:       64514,
				VNI:       100,
				LocalCIDR: "192.169.10.0/24",
				HostASN:   ptr.To(uint32(64515)),
			},
		}
		BeforeEach(func() {
			err := Updater.Update(config.Resources{
				VNIs: []v1alpha1.VNI{
					vni,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("receives type 5 routes from the fabric", func() {
			leafAConfig := infra.LeafConfiguration{
				Leaf: infra.LeafAConfig,
				Red: infra.Addresses{
					IPV4: []string{"192.168.20.0/24"},
				},
			}

			config, err := infra.LeafConfigToFRR(leafAConfig)
			Expect(err).NotTo(HaveOccurred())

			By("announcing type 5 routes from leafA")
			err = infra.LeafAContainer.ReloadConfig(config)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				for _, p := range routerPods {
					exec := executor.ForPod(p.Namespace, p.Name, "frr")
					evpn, err := frr.EVPNInfo(exec)
					Expect(err).NotTo(HaveOccurred())
					if !evpn.ContainsType5Route("192.168.20.0", leafAConfig.VTEPIP) {
						return fmt.Errorf("type5 route for 192.168.20.0 - %s not found in %v in router %s", leafAConfig.VTEPIP, evpn, p.Name)
					}
				}
				return nil
			}, 2*time.Minute, time.Second).ShouldNot(HaveOccurred())

		})

		It("establishes a session with the host and then removes it when deleting the vni", func() {
			frrConfig, err := frrk8s.ConfigFromVNI(vni)
			Expect(err).ToNot(HaveOccurred())
			err = Updater.Update(config.Resources{
				FRRConfigurations: []frrk8sv1beta1.FRRConfiguration{frrConfig},
			})
			Expect(err).NotTo(HaveOccurred())

			validateFRRK8sSessionForVNI(vni, frrk8sPods, Established)

			By("deleting the vni removes the session with the host")
			err = Updater.Client().Delete(context.Background(), &vni)
			Expect(err).NotTo(HaveOccurred())

			validateFRRK8sSessionForVNI(vni, frrk8sPods, !Established)
		})

		// This test must be the last of the ordered describe as it will remove the underlay
		It("deleting the underlay removes the sessoin with the tor", func() {
			By("deleting the vni removes the session with the host")
			err := Updater.Client().Delete(context.Background(), &infra.Underlay)
			Expect(err).NotTo(HaveOccurred())

			exec := executor.ForContainer(infra.KindLeaf)
			for _, node := range nodes {
				neighborIP, err := infra.NeighborIP(infra.KindLeaf, node.Name)
				Expect(err).NotTo(HaveOccurred())
				validateNoSuchNeigh(exec, neighborIP)
			}
		})
	})
})
