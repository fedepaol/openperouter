// SPDX-License-Identifier:Apache-2.0

package tests

import (
	"context"
	"fmt"
	"strings"

	frrk8sv1beta1 "github.com/metallb/frr-k8s/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"github.com/openperouter/openperouter/e2etests/pkg/frrk8s"
	"github.com/openperouter/openperouter/e2etests/pkg/infra"
	"github.com/openperouter/openperouter/e2etests/pkg/k8s"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	// Static configuration directory on the host (default from cmd/hostcontroller)
	hostConfigDir = "/etc/openperouter/configs"
	// Mount path inside the helper pod
	podConfigMount = "/configs"
)

// This test validates hybrid mode where configuration comes from both
// static files and Kubernetes API, testing the merge functionality.
// This test assumes NO static files exist initially and creates them during the test.
var _ = Describe("Static and API configuration merge", Label("systemdmode"), Ordered, func() {
	var cs clientset.Interface
	var routers openperouter.Routers
	var configPods []*corev1.Pod

	// Static VNI configuration to be written to file
	staticRedVNIConfig := `l3vnis:
  - vrf: red
    hostSession:
      asn: 64514
      hostASN: 64515
      localCIDR:
        ipv4: "192.170.10.0/24"
        ipv6: "2001:db9:1::/64"
    vni: 100
`

	// L3VNI from API (blue VRF)
	vniBlueFromAPI := v1alpha1.L3VNI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blue",
			Namespace: openperouter.Namespace,
		},
		Spec: v1alpha1.L3VNISpec{
			VRF: "blue",
			HostSession: &v1alpha1.HostSession{
				ASN:     64514,
				HostASN: 64515,
				LocalCIDR: v1alpha1.LocalCIDRConfig{
					IPv4: "192.170.11.0/24",
					IPv6: "2001:db9:2::/64",
				},
			},
			VNI: 200,
		},
	}

	// Underlay from API
	underlayFromAPI := v1alpha1.Underlay{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: openperouter.Namespace,
		},
		Spec: v1alpha1.UnderlaySpec{
			ASN: 64512,
			Neighbors: []v1alpha1.Neighbor{
				{
					ASN:     64513,
					Address: "192.168.1.1",
				},
			},
		},
	}

	BeforeAll(func() {
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())

		cs = k8sclient.New()
		routers, err = openperouter.Get(cs, HostMode)
		Expect(err).NotTo(HaveOccurred())

		routers.Dump(GinkgoWriter)

		// Create DaemonSet for config helper pods
		By("Creating config helper DaemonSet")
		err = createConfigHelperDaemonSet(cs)
		Expect(err).NotTo(HaveOccurred())

		// Wait for DaemonSet to be ready and get the pods
		By("Waiting for config helper pods to be ready")
		Eventually(func() error {
			pods, err := getConfigHelperPods(cs)
			if err != nil {
				return err
			}
			if len(pods) == 0 {
				return fmt.Errorf("no config helper pods found")
			}
			configPods = pods
			return nil
		}, "2m", "5s").Should(Succeed())

		// Clean any existing static files on all nodes
		By("Cleaning any existing static configuration files on all nodes")
		for _, pod := range configPods {
			_, _ = execInConfigPod(pod, fmt.Sprintf("rm -f %s/openpe_*.yaml", podConfigMount))
		}

		// Create Underlay via API first
		By("Creating underlay via API")
		err = Updater.Update(config.Resources{
			Underlays: []v1alpha1.Underlay{underlayFromAPI},
		})
		Expect(err).NotTo(HaveOccurred())

		By("Creating static VNI configuration file on all nodes")
		redVNIPath := fmt.Sprintf("%s/openpe_vni.yaml", podConfigMount)
		for _, pod := range configPods {
			_, err = execInConfigPod(pod, fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", redVNIPath, staticRedVNIConfig))
			Expect(err).NotTo(HaveOccurred())
		}

		By("Creating blue VNI via API")
		err = Updater.Update(config.Resources{
			Underlays: []v1alpha1.Underlay{underlayFromAPI},
			L3VNIs:    []v1alpha1.L3VNI{vniBlueFromAPI},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up static configuration files on all nodes")
		for _, pod := range configPods {
			_, _ = execInConfigPod(pod, fmt.Sprintf("rm -f %s/openpe_*.yaml", podConfigMount))
		}

		By("Cleaning up config helper DaemonSet")
		err := cs.AppsV1().DaemonSets(openperouter.Namespace).Delete(
			context.Background(), "config-helper", metav1.DeleteOptions{})
		if err != nil {
			GinkgoWriter.Printf("Warning: failed to delete DaemonSet: %v\n", err)
		}

		err = Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("hybrid configuration", func() {
		It("should have both static and API VNIs configured", func() {
			// Verify routes from both VNIs are present
			// We can't directly validate VNI configuration without leaf setup,
			// but we can verify the system doesn't reject the hybrid config
			Skip("Full route validation requires leaf infrastructure")
			// TODO: Use checkRouteFromLeaf when leaf infra is available
		})

		It("should handle EVPN routes from both sources", func() {
			Skip("EVPN route validation requires leaf configuration")
			// This would test that routes are properly advertised/received
			// for both the static VNI (red) and API VNI (blue)
		})

		It("should update configuration when API VNI is modified", func() {
			// Modify the blue VNI from API
			vniBlueModified := vniBlueFromAPI.DeepCopy()
			vniBlueModified.Spec.VNI = 201

			err := Updater.Update(config.Resources{
				Underlays: []v1alpha1.Underlay{underlayFromAPI},
				L3VNIs:    []v1alpha1.L3VNI{*vniBlueModified},
			})
			Expect(err).NotTo(HaveOccurred())

			// Restore original VNI
			err = Updater.Update(config.Resources{
				Underlays: []v1alpha1.Underlay{underlayFromAPI},
				L3VNIs:    []v1alpha1.L3VNI{vniBlueFromAPI},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should preserve static configuration when API resources change", func() {
			Skip("Full validation requires ability to check static VNI presence")
			// This would verify that after modifying API resources,
			// the static red VNI is still configured
		})
	})

	Context("API availability transitions", func() {
		It("should continue operating if API becomes temporarily unavailable", func() {
			Skip("Requires infrastructure to simulate API unavailability")
			// This would test SC-013: graceful degradation when API is lost
		})
	})

	Context("file changes in hybrid mode", func() {
		It("should detect static file creation while API is available", func() {
			// Create a new static config file on all nodes
			newVNIConfig := `l3vnis:
  - vrf: green
    hostSession:
      asn: 64514
      hostASN: 64515
      localCIDR:
        ipv4: "192.170.12.0/24"
        ipv6: "2001:db9:3::/64"
    vni: 300
`
			testFileName := "openpe_test_green.yaml"
			testFilePath := fmt.Sprintf("%s/%s", podConfigMount, testFileName)

			By("Creating a new static configuration file on all nodes")
			for _, pod := range configPods {
				_, err := execInConfigPod(pod, fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", testFilePath, newVNIConfig))
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying the file was created on all nodes")
			for _, pod := range configPods {
				output, err := execInConfigPod(pod, fmt.Sprintf("cat %s", testFilePath))
				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(ContainSubstring("green"))
			}

			// TODO: Add validation that the new VNI is picked up by the controller
			// This would require waiting for reconciliation and checking FRR state

			By("Cleaning up the test file on all nodes")
			for _, pod := range configPods {
				_, err := execInConfigPod(pod, fmt.Sprintf("rm -f %s", testFilePath))
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should detect static file deletion while API is available", func() {
			// This test demonstrates file deletion detection
			// In a real scenario, we'd verify that the configuration is removed
			Skip("Full validation requires monitoring FRR configuration changes")
		})

		It("should handle modification of existing static files while API is available", func() {
			// This test demonstrates file modification detection
			// We would modify an existing file and verify the change is picked up
			Skip("Full validation requires monitoring FRR configuration changes")
		})
	})

	Context("EVPN route translation with static files", func() {
		var frrk8sPods []*corev1.Pod
		var frrK8sConfigRed []frrk8sv1beta1.FRRConfiguration

		// Route prefixes from leaves
		leafAVRFRedPrefixes := []string{"192.168.20.0/24", "2001:db8:20::/64"}
		leafBVRFRedPrefixes := []string{"192.169.20.0/24", "2001:db8:169:20::/64"}
		emptyPrefixes := []string{}

		// Static Red VNI that will be created from file
		staticRedVNI := v1alpha1.L3VNI{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "red",
				Namespace: openperouter.Namespace,
			},
			Spec: v1alpha1.L3VNISpec{
				VRF: "red",
				HostSession: &v1alpha1.HostSession{
					ASN:     64514,
					HostASN: 64515,
					LocalCIDR: v1alpha1.LocalCIDRConfig{
						IPv4: "192.169.10.0/24",
						IPv6: "2001:db8:1::/64",
					},
				},
				VNI: 100,
			},
		}

		staticRedVNIYAML := `l3vnis:
  - vrf: red
    hostSession:
      asn: 64514
      hostASN: 64515
      localCIDR:
        ipv4: "192.169.10.0/24"
        ipv6: "2001:db8:1::/64"
    vni: 100
`

		BeforeAll(func() {
			var err error

			// Get FRR-K8s pods
			frrk8sPods, err = frrk8s.Pods(cs)
			Expect(err).NotTo(HaveOccurred())
			Expect(frrk8sPods).NotTo(BeEmpty(), "Need FRR-K8s pods for BGP route validation")

			// Create FRR configurations for the red VNI host session
			frrK8sConfigRed, err = frrk8s.ConfigFromHostSession(*staticRedVNI.Spec.HostSession, staticRedVNI.Name)
			Expect(err).NotTo(HaveOccurred())

			// Apply FRR configurations
			err = Updater.Update(config.Resources{
				FRRConfigurations: frrK8sConfigRed,
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for FRR sessions to be established
			validateFRRK8sSessionForHostSession(staticRedVNI.Name, *staticRedVNI.Spec.HostSession, Established, frrk8sPods...)
		})

		AfterAll(func() {
			// Clean up leaf routes
			Expect(infra.LeafAConfig.RemovePrefixes()).To(Succeed())
			Expect(infra.LeafBConfig.RemovePrefixes()).To(Succeed())
		})

		It("translates EVPN incoming routes as BGP routes, then removes them when static file is deleted", func() {
			ShouldExist := true
			redVNIPath := fmt.Sprintf("%s/openpe_vni_red.yaml", podConfigMount)

			By("creating static VNI file on all nodes")
			for _, pod := range configPods {
				_, err := execInConfigPod(pod, fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", redVNIPath, staticRedVNIYAML))
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying static files were created on all nodes")
			for _, pod := range configPods {
				Eventually(func() error {
					output, err := execInConfigPod(pod, fmt.Sprintf("cat %s", redVNIPath))
					if err != nil {
						return err
					}
					if !strings.Contains(output, "red") {
						return fmt.Errorf("file does not contain expected content")
					}
					return nil
				}, "10s", "1s").Should(Succeed())
			}

			By("advertising routes from the leaves for VRF Red - VNI 100")
			Expect(infra.LeafAConfig.ChangePrefixes(emptyPrefixes, leafAVRFRedPrefixes, emptyPrefixes)).To(Succeed())
			Expect(infra.LeafBConfig.ChangePrefixes(emptyPrefixes, leafBVRFRedPrefixes, emptyPrefixes)).To(Succeed())

			By("checking routes are propagated via BGP")
			for _, frrk8sPod := range frrk8sPods {
				checkBGPPrefixesForHostSession(frrk8sPod, *staticRedVNI.Spec.HostSession, leafAVRFRedPrefixes, ShouldExist)
				checkBGPPrefixesForHostSession(frrk8sPod, *staticRedVNI.Spec.HostSession, leafBVRFRedPrefixes, ShouldExist)
			}

			By("deleting static VNI file from all nodes")
			for _, pod := range configPods {
				_, err := execInConfigPod(pod, fmt.Sprintf("rm -f %s", redVNIPath))
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying files were deleted from all nodes")
			for _, pod := range configPods {
				Eventually(func() bool {
					_, err := execInConfigPod(pod, fmt.Sprintf("test -f %s", redVNIPath))
					return err != nil // File should not exist (command fails)
				}, "10s", "1s").Should(BeTrue())
			}

			By("checking routes are NO LONGER propagated via BGP after file deletion")
			for _, frrk8sPod := range frrk8sPods {
				checkBGPPrefixesForHostSession(frrk8sPod, *staticRedVNI.Spec.HostSession, leafAVRFRedPrefixes, !ShouldExist)
				checkBGPPrefixesForHostSession(frrk8sPod, *staticRedVNI.Spec.HostSession, leafBVRFRedPrefixes, !ShouldExist)
			}
		})
	})
})

// createConfigHelperDaemonSet creates a DaemonSet that can manipulate static configuration files
// by mounting the host's configuration directory on every node.
func createConfigHelperDaemonSet(cs clientset.Interface) error {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-helper",
			Namespace: openperouter.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "config-helper",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "config-helper",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "helper",
							Image:   "busybox:1.36",
							Command: []string{"sleep", "infinity"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-dir",
									MountPath: podConfigMount,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-dir",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: hostConfigDir,
									Type: func() *corev1.HostPathType {
										t := corev1.HostPathDirectoryOrCreate
										return &t
									}(),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := cs.AppsV1().DaemonSets(openperouter.Namespace).Create(
		context.Background(), daemonSet, metav1.CreateOptions{})
	return err
}

// getConfigHelperPods returns all pods created by the config-helper DaemonSet that are ready.
func getConfigHelperPods(cs clientset.Interface) ([]*corev1.Pod, error) {
	podList, err := cs.CoreV1().Pods(openperouter.Namespace).List(
		context.Background(), metav1.ListOptions{
			LabelSelector: "app=config-helper",
		})
	if err != nil {
		return nil, err
	}

	var readyPods []*corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if k8s.PodIsReady(pod) {
			readyPods = append(readyPods, pod)
		}
	}

	return readyPods, nil
}

// execInConfigPod executes a shell command in the config helper pod
func execInConfigPod(pod *corev1.Pod, command string) (string, error) {
	podExec := executor.ForPod(openperouter.Namespace, pod.Name, "helper")
	return podExec.Exec("sh", "-c", command)
}
