// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"fmt"
	"strings"
	"time"

	nad "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
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
			Name:      "red110",
			Namespace: openperouter.Namespace,
		},
		Spec: v1alpha1.L2VNISpec{
			VRF:         ptr.To("red"),
			VNI:         110,
			L2GatewayIP: "192.171.24.1/24",
			HostMaster: &v1alpha1.HostMaster{
				AutoCreate: true,
			},
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

	const testNamespace = "test-namespace"
	var (
		firstPod  *corev1.Pod
		secondPod *corev1.Pod
		nad       nad.NetworkAttachmentDefinition
	)

	BeforeEach(func() {
		By("setting redistribute connected on leaves")
		redistributeConnectedForLeaf(infra.LeafAConfig)
		redistributeConnectedForLeaf(infra.LeafBConfig)

		err := Updater.CleanButUnderlay()
		Expect(err).NotTo(HaveOccurred())

		err = Updater.Update(config.Resources{
			VNIs: []v1alpha1.VNI{
				vniRed,
			},
			L2VNIs: []v1alpha1.L2VNI{
				l2VniRed,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = k8s.CreateNamespace(cs, testNamespace)
		Expect(err).NotTo(HaveOccurred())

		nad, err = k8s.CreateMacvlanNad("110", testNamespace, "br-hs-110", "192.171.24.1")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		dumpIfFails(cs)
		err := Updater.CleanButUnderlay()
		Expect(err).NotTo(HaveOccurred())
		err = k8s.DeleteNamespace(cs, testNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	FIt("should create two pods connected to the l2 overlay", func() {
		var err error
		const (
			firstPodIP  = "192.171.24.2"
			secondPodIP = "192.171.24.3"
		)
		nodes, err := k8s.GetNodes(cs)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(nodes)).To(BeNumerically(">=", 2), "Expected at least 2 nodes, but got fewer")

		By("creating the pods")
		firstPod, err = k8s.CreateAgnhostPod(cs, "pod1", testNamespace, k8s.WithNad(nad.Name, testNamespace, firstPodIP+"/24"), k8s.OnNode(nodes[0].Name))
		Expect(err).NotTo(HaveOccurred())
		secondPod, err = k8s.CreateAgnhostPod(cs, "pod2", testNamespace, k8s.WithNad(nad.Name, testNamespace, secondPodIP+"/24"), k8s.OnNode(nodes[1].Name))
		Expect(err).NotTo(HaveOccurred())

		podExecutor := executor.ForPod(firstPod.Namespace, firstPod.Name, "agnhost")
		secondPodExecutor := executor.ForPod(secondPod.Namespace, secondPod.Name, "agnhost")

		Eventually(func() error {
			By(fmt.Sprintf("trying to hit second pod %s on the %s network", secondPodIP, l2VniRed.Name))
			url := fmt.Sprintf("http://%s:8090/clientip", secondPodIP)
			res, err := podExecutor.Exec("curl", "-sS", url)
			if err != nil {
				return fmt.Errorf("curl %s:8090 failed: %s", secondPodIP, res)
			}
			clientIP := strings.Split(res, ":")[0]
			if clientIP != firstPodIP {
				return fmt.Errorf("curl %s:8090 returned %s, expected %s", secondPodIP, clientIP, firstPodIP)
			}

			return nil
		}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		Eventually(func() error {
			By(fmt.Sprintf("trying to hit first pod %s on the %s network", firstPodIP, l2VniRed.Name))
			url := fmt.Sprintf("http://%s:8090/clientip", firstPodIP)
			res, err := secondPodExecutor.Exec("curl", "-sS", url)
			if err != nil {
				return fmt.Errorf("curl %s:8090 failed: %s", firstPodIP, res)
			}
			clientIP := strings.Split(res, ":")[0]
			if clientIP != secondPodIP {
				return fmt.Errorf("curl %s:8090 returned %s, expected %s", firstPodIP, clientIP, secondPodIP)
			}

			return nil
		}, 5*time.Minute, 5*time.Second).ShouldNot(HaveOccurred())

		fmt.Println("ZZ")
		time.Sleep(20 * time.Second) // Wait for the pods to be ready and stable
	})
})
