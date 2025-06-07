// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"time"

	nad "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/infra"
	"github.com/openperouter/openperouter/e2etests/pkg/k8s"
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

	vniRed := v1alpha1.VNI{
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

	l2VniRed := v1alpha1.L2VNI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "red",
			Namespace: openperouter.Namespace,
		},
		Spec: v1alpha1.L2VNISpec{
			VRF:         ptr.To("red"),
			VNI:         110,
			L2GatewayIP: "192.171.24.1/24",
			HostMaster:  "l2-bridge-110",
		},
	}

	redistributeConnectedForLeaf := func(leaf infra.Leaf) {
		leafConfiguration := infra.LeafConfiguration{
			Leaf: leaf,
			Red: infra.Addresses{
				RedistributeConnected: true,
			},
			Blue: infra.Addresses{
				RedistributeConnected: true,
			},
		}
		config, err := infra.LeafConfigToFRR(leafConfiguration)
		Expect(err).NotTo(HaveOccurred())
		err = leaf.ReloadConfig(config)
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeAll(func() {
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())

		cs = k8sclient.New()
		routerPods, err = openperouter.RouterPods(cs)
		Expect(err).NotTo(HaveOccurred())

		DumpPods("Router pods", routerPods)

		err = Updater.Update(config.Resources{
			Underlays: []v1alpha1.Underlay{
				infra.Underlay,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		// TODO create a bridge on each host
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
		By("setting redistribute connected on leaves")
		redistributeConnectedForLeaf(infra.LeafAConfig)
		redistributeConnectedForLeaf(infra.LeafBConfig)

		err := Updater.CleanButUnderlay()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		dumpIfFails(cs)
		err := Updater.CleanButUnderlay()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("with vnis", func() {
		const testNamespace = "test-namespace"
		var (
			firstPod  *corev1.Pod
			secondPod *corev1.Pod
			l2bridge  = "l2-bridge-110"
			nad       nad.NetworkAttachmentDefinition
		)

		BeforeEach(func() {
			err := Updater.Update(config.Resources{
				VNIs: []v1alpha1.VNI{
					vniRed,
				},
				L2VNIs: []v1alpha1.L2VNI{
					l2VniRed,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			nad, err = k8s.CreateMacvlanNad("110", testNamespace, l2bridge, "192.171.24.1/24")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create two pods connected to the l2 overlay", func() {
			var err error
			firstPod, err = k8s.CreateAgnhostPod(cs, testNamespace, "pod1", k8s.WithNad(nad.Name, testNamespace, "192.171.24.2/24"))
			Expect(err).NotTo(HaveOccurred())
			secondPod, err = k8s.CreateAgnhostPod(cs, testNamespace, "pod2", k8s.WithNad(nad.Name, testNamespace, "192.171.24.3/24"))
			Expect(err).NotTo(HaveOccurred())
		})

	})
})
