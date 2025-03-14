package hostconfiguration

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/openperouter/openperouter/e2etests/pkg/ipfamily"
	"github.com/openperouter/openperouter/e2etests/pkg/k8s"
	"sigs.k8s.io/kube-storage-version-migrator/pkg/clients/clientset"
)

var _ = ginkgo.Describe("BGP", func() {
	var cs clientset.Interface
	testNamespace := ""

	ginkgo.AfterEach(func() {
	})

	ginkgo.BeforeEach(func() {
		ginkgo.By("Clearing any previous configuration")

		err := ConfigUpdater.Clean()
		Expect(err).NotTo(HaveOccurred())

		for _, c := range FRRContainers {
			err := c.UpdateBGPConfigFile(frrconfig.Empty)
			Expect(err).NotTo(HaveOccurred())
		}

		cs = k8sclient.New()
		testNamespace, err = k8s.CreateTestNamespace(cs, "bgp")
		Expect(err).NotTo(HaveOccurred())
	})

	ginkgo.DescribeTable("A service of protocol load balancer should work with ETP=cluster", func(pairingIPFamily ipfamily.Family, poolAddresses []string, tweak testservice.Tweak) {

		_, svc := setupBGPService(cs, testNamespace, pairingIPFamily, poolAddresses, FRRContainers, func(svc *corev1.Service) {
			testservice.TrafficPolicyCluster(svc)
			tweak(svc)
		})
		defer testservice.Delete(cs, svc)

		allNodes, err := cs.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		testservice.ValidateDesiredLB(svc)

		for _, c := range FRRContainers {
			validateService(svc, allNodes.Items, c)
		}
	},
		ginkgo.Entry("IPV4", ipfamily.IPv4, []string{v4PoolAddresses}, func(_ *corev1.Service) {}),
		ginkgo.Entry("IPV6", ipfamily.IPv6, []string{v6PoolAddresses}, func(_ *corev1.Service) {}),
		ginkgo.Entry("DUALSTACK", ipfamily.DualStack, []string{v4PoolAddresses, v6PoolAddresses},
			func(svc *corev1.Service) {
				testservice.DualStack(svc)
			}),
		ginkgo.Entry("IPV4 - request IPv4 via custom annotation", ipfamily.IPv4, []string{v4PoolAddresses},
			func(svc *corev1.Service) {
				testservice.WithSpecificIPs(svc, "192.168.10.100")
			}),
		ginkgo.Entry("DUALSTACK - request Dual Stack via custom annotation", ipfamily.DualStack, []string{v4PoolAddresses, v6PoolAddresses},
			func(svc *corev1.Service) {
				testservice.DualStack(svc)
				testservice.WithSpecificIPs(svc, "192.168.10.100", "fc00:f853:ccd:e799::")
			}),
	)
})
