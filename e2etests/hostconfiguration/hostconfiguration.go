package hostconfiguration

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

var ValidatorPath string

var _ = ginkgo.Describe("Router Host configuration", func() {
	var cs clientset.Interface

	ginkgo.AfterEach(func() {
	})

	ginkgo.BeforeEach(func() {
		cs = k8sclient.New()
		ginkgo.By("ensuring the validator is in all the pods")
		routerPods, err := openperouter.RouterPods(cs)
		Expect(err).NotTo(HaveOccurred())
		for _, pod := range routerPods {
			ensureValidator(cs, pod)
		}

		/*
			err := ConfigUpdater.Clean()
			Expect(err).NotTo(HaveOccurred())
		*/

		cs = k8sclient.New()
	})

	ginkgo.It("is applied correctly", func() {
		resources := config.Resources{
			Underlays: []v1alpha1.Underlay{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "underlay",
						Namespace: openperouter.Namespace,
					},
				},
			},
			VNIs: []v1alpha1.VNI{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "vni",
						Namespace: openperouter.Namespace,
					},
				},
			},
		}
	})

})

func ensureValidator(cs clientset.Interface, pod *corev1.Pod) {
	if pod.Annotations != nil && pod.Annotations["validator"] == "true" {
		return
	}
	dst := fmt.Sprintf("%s/%s:/", pod.Namespace, pod.Name)
	fullargs := []string{"cp", ValidatorPath, dst}
	_, err := exec.Command(executor.Kubectl, fullargs...).CombinedOutput()
	Expect(err).NotTo(HaveOccurred())

	pod.Annotations["validator"] = "true"
	_, err = cs.CoreV1().Pods(pod.Namespace).Update(context.Background(), pod, metav1.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())
}
