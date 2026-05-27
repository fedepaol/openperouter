// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/k8s"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mirroredLabel     = "openperouter.io/mirrored"
	sourceNodeLabel   = "openperouter.io/source-node"
	staticNamePattern = "static"
)

var _ = Describe("Config Mirror Controller", Label("systemdmode"), Ordered, func() {
	var cs clientset.Interface
	var crClient crclient.Client
	var nodes []string
	var configPods []*corev1.Pod

	BeforeAll(func() {
		cs = k8sclient.New()
		cfg, err := k8sclient.RestConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme := runtime.NewScheme()
		err = clientgoscheme.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())
		err = v1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())
		crClient, err = crclient.New(cfg, crclient.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		nodeList, err := k8s.GetNodes(cs)
		Expect(err).NotTo(HaveOccurred())
		nodes = make([]string, len(nodeList))
		for i, node := range nodeList {
			nodes[i] = node.Name
		}
		Expect(nodes).NotTo(BeEmpty(), "No nodes found in cluster")

		By("Creating config helper DaemonSet for mirror tests")
		configPods, err = createConfigHelperDaemonSet(cs)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Deleting config helper DaemonSet from mirror tests")
		err := cs.AppsV1().DaemonSets(openperouter.Namespace).Delete(
			context.Background(), "config-helper", metav1.DeleteOptions{})
		if err != nil {
			GinkgoWriter.Printf("Warning: failed to delete DaemonSet: %v\n", err)
		}
	})

	Context("T016: mirrored CRs appear after host mode startup", func() {
		It("should create mirrored CRs for static config files", func() {
			By("waiting for mirrored Underlay CRs to appear")
			Eventually(func(g Gomega) {
				var underlayList v1alpha1.UnderlayList
				err := crClient.List(context.Background(), &underlayList,
					crclient.InNamespace(openperouter.Namespace),
					crclient.MatchingLabels{mirroredLabel: "true"})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(underlayList.Items).NotTo(BeEmpty(), "Expected at least one mirrored Underlay CR")

				for _, underlay := range underlayList.Items {
					g.Expect(underlay.Name).To(ContainSubstring(staticNamePattern))
					g.Expect(underlay.Labels).To(HaveKeyWithValue(mirroredLabel, "true"))
					g.Expect(underlay.Labels).To(HaveKey(sourceNodeLabel))

					nodeName := underlay.Labels[sourceNodeLabel]
					g.Expect(nodes).To(ContainElement(nodeName))
					g.Expect(underlay.Spec.NodeSelector).NotTo(BeNil())
					g.Expect(underlay.Spec.NodeSelector.MatchLabels).To(
						HaveKeyWithValue("kubernetes.io/hostname", nodeName))
				}
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			By("verifying mirrored L3VNI CRs")
			Eventually(func(g Gomega) {
				var vniList v1alpha1.L3VNIList
				err := crClient.List(context.Background(), &vniList,
					crclient.InNamespace(openperouter.Namespace),
					crclient.MatchingLabels{mirroredLabel: "true"})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vniList.Items).NotTo(BeEmpty(), "Expected at least one mirrored L3VNI CR")

				for _, vni := range vniList.Items {
					g.Expect(vni.Name).To(ContainSubstring(staticNamePattern))
					g.Expect(vni.Labels).To(HaveKeyWithValue(mirroredLabel, "true"))
					g.Expect(vni.Labels).To(HaveKey(sourceNodeLabel))

					nodeName := vni.Labels[sourceNodeLabel]
					g.Expect(nodes).To(ContainElement(nodeName))
					g.Expect(vni.Spec.NodeSelector).NotTo(BeNil())
					g.Expect(vni.Spec.NodeSelector.MatchLabels).To(
						HaveKeyWithValue("kubernetes.io/hostname", nodeName))
				}
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("T017: mirrored CRs update on file change", func() {
		var testNodeName string
		var testPod *corev1.Pod

		BeforeEach(func() {
			testNodeName = nodes[0]
			for _, pod := range configPods {
				if pod.Spec.NodeName == testNodeName {
					testPod = pod
					break
				}
			}
			Expect(testPod).NotTo(BeNil(), "No config helper pod on node %s", testNodeName)
		})

		AfterEach(func() {
			_, _ = execInConfigPod(testPod,
				fmt.Sprintf("rm -f %s/openpe_mirror_test.yaml", podConfigMount))

			Eventually(func(g Gomega) {
				var underlayList v1alpha1.UnderlayList
				err := crClient.List(context.Background(), &underlayList,
					crclient.InNamespace(openperouter.Namespace),
					crclient.MatchingLabels{
						mirroredLabel:   "true",
						sourceNodeLabel: testNodeName,
					})
				g.Expect(err).NotTo(HaveOccurred())
				for _, u := range underlayList.Items {
					g.Expect(u.Name).NotTo(Equal(
						fmt.Sprintf("%s-static-underlay-0", testNodeName)))
				}
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})

		It("should update mirrored CR when config file changes", func() {
			initialASN := uint32(64520)
			updatedASN := uint32(64521)

			staticConfigYAML := func(asn uint32) string {
				return fmt.Sprintf(`underlays:
  - asn: %d
    nics:
      - eth0
    neighbors:
      - asn: 64512
        address: "192.168.100.2"
    evpn:
      vtepCIDR: "100.70.0.0/24"
`, asn)
			}

			mirrorTestFile := fmt.Sprintf("%s/openpe_mirror_test.yaml", podConfigMount)

			By("creating initial config file with ASN " + fmt.Sprint(initialASN))
			_, err := execInConfigPod(testPod,
				fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", mirrorTestFile, staticConfigYAML(initialASN)))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for initial mirrored CR to appear")
			var mirroredCRName string
			Eventually(func(g Gomega) {
				var underlayList v1alpha1.UnderlayList
				err := crClient.List(context.Background(), &underlayList,
					crclient.InNamespace(openperouter.Namespace),
					crclient.MatchingLabels{
						mirroredLabel:   "true",
						sourceNodeLabel: testNodeName,
					})
				g.Expect(err).NotTo(HaveOccurred())

				expectedName := fmt.Sprintf("%s-static-underlay-0", testNodeName)
				var testCR *v1alpha1.Underlay
				for i := range underlayList.Items {
					if underlayList.Items[i].Name == expectedName {
						testCR = &underlayList.Items[i]
						break
					}
				}

				g.Expect(testCR).NotTo(BeNil(), "Expected mirrored CR not found")
				g.Expect(testCR.Spec.ASN).To(Equal(initialASN))
				mirroredCRName = testCR.Name
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("modifying config file with ASN " + fmt.Sprint(updatedASN))
			_, err = execInConfigPod(testPod,
				fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", mirrorTestFile, staticConfigYAML(updatedASN)))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for mirrored CR to update")
			Eventually(func(g Gomega) {
				var underlay v1alpha1.Underlay
				err := crClient.Get(context.Background(),
					crclient.ObjectKey{
						Name:      mirroredCRName,
						Namespace: openperouter.Namespace,
					}, &underlay)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(underlay.Spec.ASN).To(Equal(updatedASN))
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Context("T018: mirrored CRs deleted on file removal", func() {
		var testNodeName string
		var testPod *corev1.Pod

		BeforeEach(func() {
			testNodeName = nodes[0]
			for _, pod := range configPods {
				if pod.Spec.NodeName == testNodeName {
					testPod = pod
					break
				}
			}
			Expect(testPod).NotTo(BeNil(), "No config helper pod on node %s", testNodeName)
		})

		AfterEach(func() {
			_, _ = execInConfigPod(testPod,
				fmt.Sprintf("rm -f %s/openpe_delete_test.yaml", podConfigMount))
		})

		It("should delete mirrored CRs when config file is removed", func() {
			staticConfigYAML := `l3vnis:
  - vrf: testdelete
    hostSession:
      asn: 64530
      hostASN: 64531
      localCIDR:
        ipv4: "192.172.10.0/24"
        ipv6: "2001:db9:3::/64"
    vni: 300
`
			deleteTestFile := fmt.Sprintf("%s/openpe_delete_test.yaml", podConfigMount)

			By("creating config file")
			_, err := execInConfigPod(testPod,
				fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", deleteTestFile, staticConfigYAML))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for mirrored CR to appear")
			var mirroredCRName string
			Eventually(func(g Gomega) {
				var vniList v1alpha1.L3VNIList
				err := crClient.List(context.Background(), &vniList,
					crclient.InNamespace(openperouter.Namespace),
					crclient.MatchingLabels{
						mirroredLabel:   "true",
						sourceNodeLabel: testNodeName,
					})
				g.Expect(err).NotTo(HaveOccurred())

				expectedName := fmt.Sprintf("%s-static-l3vni-0", testNodeName)
				var testCR *v1alpha1.L3VNI
				for i := range vniList.Items {
					if vniList.Items[i].Name == expectedName {
						testCR = &vniList.Items[i]
						break
					}
				}

				g.Expect(testCR).NotTo(BeNil(), "Expected mirrored L3VNI CR not found")
				g.Expect(testCR.Spec.VRF).To(Equal("testdelete"))
				mirroredCRName = testCR.Name
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("removing config file from node")
			_, err = execInConfigPod(testPod, fmt.Sprintf("rm -f %s", deleteTestFile))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for mirrored CR to be deleted")
			Eventually(func(g Gomega) {
				var vni v1alpha1.L3VNI
				err := crClient.Get(context.Background(),
					crclient.ObjectKey{
						Name:      mirroredCRName,
						Namespace: openperouter.Namespace,
					}, &vni)
				g.Expect(err).To(HaveOccurred(), "CR should be deleted")
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Context("T019: mirrored CRs excluded from reconciliation", func() {
		var testNodeName string
		var testPod *corev1.Pod
		var routers openperouter.Routers

		BeforeEach(func() {
			testNodeName = nodes[0]
			for _, pod := range configPods {
				if pod.Spec.NodeName == testNodeName {
					testPod = pod
					break
				}
			}
			Expect(testPod).NotTo(BeNil(), "No config helper pod on node %s", testNodeName)

			var err error
			routers, err = openperouter.Get(cs, HostMode)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			_, _ = execInConfigPod(testPod,
				fmt.Sprintf("rm -f %s/openpe_reconcile_test.yaml", podConfigMount))

			userVNI := &v1alpha1.L3VNI{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-vni-reconcile-test",
					Namespace: openperouter.Namespace,
				},
			}
			_ = crClient.Delete(context.Background(), userVNI)

			Eventually(func(g Gomega) {
				var vni v1alpha1.L3VNI
				err := crClient.Get(context.Background(),
					crclient.ObjectKey{
						Name:      "user-vni-reconcile-test",
						Namespace: openperouter.Namespace,
					}, &vni)
				g.Expect(err).To(HaveOccurred(), "User CR should be deleted")
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				var vniList v1alpha1.L3VNIList
				err := crClient.List(context.Background(), &vniList,
					crclient.InNamespace(openperouter.Namespace),
					crclient.MatchingLabels{
						mirroredLabel:   "true",
						sourceNodeLabel: testNodeName,
					})
				g.Expect(err).NotTo(HaveOccurred())
				expectedName := fmt.Sprintf("%s-static-l3vni-0", testNodeName)
				for _, vni := range vniList.Items {
					g.Expect(vni.Name).NotTo(Equal(expectedName))
				}
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})

		It("should not reconcile mirrored CRs into FRR config", func() {
			mirroredVNI := uint32(400)
			userVNI := uint32(500)

			mirroredConfigYAML := fmt.Sprintf(`l3vnis:
  - vrf: mirrored-vrf
    hostSession:
      asn: 64540
      hostASN: 64541
      localCIDR:
        ipv4: "192.173.10.0/24"
    vni: %d
`, mirroredVNI)

			reconcileTestFile := fmt.Sprintf("%s/openpe_reconcile_test.yaml", podConfigMount)

			By("creating mirrored CR via static config file")
			_, err := execInConfigPod(testPod,
				fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", reconcileTestFile, mirroredConfigYAML))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for mirrored CR to appear")
			Eventually(func(g Gomega) {
				var vniList v1alpha1.L3VNIList
				err := crClient.List(context.Background(), &vniList,
					crclient.InNamespace(openperouter.Namespace),
					crclient.MatchingLabels{
						mirroredLabel:   "true",
						sourceNodeLabel: testNodeName,
					})
				g.Expect(err).NotTo(HaveOccurred())

				expectedName := fmt.Sprintf("%s-static-l3vni-0", testNodeName)
				var found bool
				for _, vni := range vniList.Items {
					if vni.Name == expectedName && vni.Spec.VNI == mirroredVNI {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Mirrored CR should exist")
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("creating a user-defined CR via API")
			userCR := &v1alpha1.L3VNI{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-vni-reconcile-test",
					Namespace: openperouter.Namespace,
				},
				Spec: v1alpha1.L3VNISpec{
					VRF: "user-vrf",
					HostSession: &v1alpha1.HostSession{
						ASN:     64550,
						HostASN: 64551,
						LocalCIDR: v1alpha1.LocalCIDRConfig{
							IPv4: "192.174.10.0/24",
						},
					},
					VNI: userVNI,
				},
			}

			err = crClient.Create(context.Background(), userCR)
			Expect(err).NotTo(HaveOccurred())

			By("verifying FRR config contains user VNI but not mirrored VNI")
			routerExec, err := routers.ExecutorForNode(testNodeName)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				config, err := routerExec.Exec("vtysh", "-c", "show running-config")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(config).To(ContainSubstring("user-vrf"))
				g.Expect(config).NotTo(ContainSubstring("mirrored-vrf"))
				g.Expect(config).To(MatchRegexp(fmt.Sprintf(`vni %d`, userVNI)))
				g.Expect(config).NotTo(MatchRegexp(fmt.Sprintf(`vni %d`, mirroredVNI)))
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})
	})
})
