// SPDX-License-Identifier:Apache-2.0

package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/infra"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = Describe("Static configuration", Label("systemd"), Label("beforek8s"), Ordered, func() {
	var cs clientset.Interface
	var routers openperouter.Routers

	vniRed := v1alpha1.L3VNI{
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

	vniBlue := v1alpha1.L3VNI{
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
					IPv4: "192.169.11.0/24",
					IPv6: "2001:db8:2::/64",
				},
			},
			VNI: 200,
		},
	}

	BeforeAll(func() {
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())

		cs = k8sclient.New()
		routers, err = openperouter.Get(cs, HostMode)
		Expect(err).NotTo(HaveOccurred())

		routers.Dump(GinkgoWriter)
	})

	AfterAll(func() {
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("with vnis", func() {
		AfterEach(func() {
			dumpIfFails(cs)
			err := Updater.CleanButUnderlay()
			Expect(err).NotTo(HaveOccurred())
			removeLeafPrefixes(infra.LeafAConfig)
			removeLeafPrefixes(infra.LeafBConfig)
		})

		It("receives type 5 routes from the fabric", func() {
			Contains := true

			By("announcing type 5 routes on VNI 100 from leafA")
			changeLeafPrefixes(infra.LeafAConfig, emptyPrefixes, leafAVRFRedPrefixes, leafAVRFBluePrefixes)
			checkRouteFromLeaf(infra.LeafAConfig, routers, vniRed, Contains, leafAVRFRedPrefixes)
			checkRouteFromLeaf(infra.LeafAConfig, routers, vniBlue, !Contains, leafAVRFBluePrefixes)
			checkRouteFromLeaf(infra.LeafBConfig, routers, vniRed, !Contains, leafBVRFRedPrefixes)
			checkRouteFromLeaf(infra.LeafBConfig, routers, vniBlue, !Contains, leafBVRFBluePrefixes)

			By("removing a route from leafA on vni 100")
			changeLeafPrefixes(infra.LeafAConfig, emptyPrefixes, emptyPrefixes, leafAVRFBluePrefixes)
			checkRouteFromLeaf(infra.LeafAConfig, routers, vniRed, !Contains, leafAVRFRedPrefixes)
			checkRouteFromLeaf(infra.LeafAConfig, routers, vniBlue, !Contains, leafAVRFBluePrefixes)
		})
	})
	// TODO Create vni blue with the api server
})
