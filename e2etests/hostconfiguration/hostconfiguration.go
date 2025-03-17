package hostconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"github.com/openperouter/openperouter/e2etests/pkg/k8s"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
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
					Spec: v1alpha1.VNISpec{
						ASN:       64514,
						VNI:       100,
						LocalCIDR: "192.169.10.0/24",
						HostASN:   pointer.Uint32(64515),
					},
				},
			},
		}
	})

})

func sendConfigToValidate[T any](pods []*corev1.Pod, toValidate T) {
	jsonData, err := json.MarshalIndent(toValidate, "", "  ")
	if err != nil {
		panic(err)
	}

	toValidateFile, err := ioutil.TempFile(os.TempDir(), "validate-*.json")
	Expect(err).NotTo(HaveOccurred())

	_, err = toValidateFile.Write(jsonData)
	Expect(err).NotTo(HaveOccurred())

	for _, p := range pods {
		err := k8s.SendFileToPod(toValidateFile.Name(), p)
		Expect(err).NotTo(HaveOccurred())
	}
}

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
