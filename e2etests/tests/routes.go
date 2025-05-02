// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"fmt"
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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

var _ = Describe("Routes between bgp and the fabric", Ordered, func() {
	var cs clientset.Interface
	routerPods := []*corev1.Pod{}
	//frrk8sPods := []*corev1.Pod{}
	//nodes := []corev1.Node{}

	BeforeAll(func() {
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())

		cs = k8sclient.New()
		routerPods, err = openperouter.RouterPods(cs)
		Expect(err).NotTo(HaveOccurred())
		/*
			frrk8sPods, err = frrk8s.Pods(cs)
			Expect(err).NotTo(HaveOccurred())
			nodesItems, err := cs.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			nodes = nodesItems.Items
		*/

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

	Context("with a vni", func() {
		vnired := v1alpha1.VNI{
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

		vniblue := v1alpha1.VNI{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "blue",
				Namespace: openperouter.Namespace,
			},
			Spec: v1alpha1.VNISpec{
				ASN:       64514,
				VNI:       200,
				LocalCIDR: "192.169.11.0/24",
				HostASN:   ptr.To(uint32(64515)),
			},
		}
		BeforeEach(func() {
			err := Updater.Update(config.Resources{
				VNIs: []v1alpha1.VNI{
					vnired,
					vniblue,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		FIt("receives type 5 routes from the fabric", func() {
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

			fmt.Println("ZZZ")
			time.Sleep(2 * time.Minute)
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
	})
})
