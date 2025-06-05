// SPDX-License-Identifier:Apache-2.0

package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = Describe("Webhooks", func() {
	var cs clientset.Interface

	BeforeEach(func() {
		cs = k8sclient.New()
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		dumpIfFails(cs)
		err := Updater.CleanAll()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when webhooks are enabled", func() {
		FIt("should deny creation of two VNIs with the same VNI number", func() {
			vni1 := &v1alpha1.VNI{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vni1",
					Namespace: openperouter.Namespace,
				},
				Spec: v1alpha1.VNISpec{
					VNI:       1001,
					LocalCIDR: "192.168.1.0/24",
				},
			}

			By("creating the first VNI")
			err := Updater.Update(config.Resources{
				VNIs: []v1alpha1.VNI{*vni1},
			})
			Expect(err).NotTo(HaveOccurred())

			By("creating the second VNI")
			vni2 := &v1alpha1.VNI{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vni2",
					Namespace: openperouter.Namespace,
				},
				Spec: v1alpha1.VNISpec{
					VNI:       1001,
					LocalCIDR: "192.168.2.0/24",
				},
			}
			err = Updater.Update(config.Resources{
				VNIs: []v1alpha1.VNI{*vni2},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
