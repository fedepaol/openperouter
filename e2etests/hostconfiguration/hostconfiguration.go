package hostconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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
	"k8s.io/utils/ptr"
)

var (
	ValidatorPath string
	Updater       config.Updater
)

var _ = ginkgo.Describe("Router Host configuration", func() {
	var cs clientset.Interface
	routerPods := []*corev1.Pod{}

	ginkgo.AfterEach(func() {
		err := Updater.Clean()
		Expect(err).NotTo(HaveOccurred())
	})

	ginkgo.BeforeEach(func() {
		cs = k8sclient.New()
		ginkgo.By("ensuring the validator is in all the pods")
		var err error
		routerPods, err = openperouter.RouterPods(cs)
		Expect(err).NotTo(HaveOccurred())
		for _, pod := range routerPods {
			ensureValidator(cs, pod)
		}

		err = Updater.Clean()
		Expect(err).NotTo(HaveOccurred())

		cs = k8sclient.New()
	})

	ginkgo.It("is applied correctly", func() {
		underlay := v1alpha1.Underlay{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "underlay",
				Namespace: openperouter.Namespace,
			},
			Spec: v1alpha1.UnderlaySpec{
				ASN:      64514,
				VTEPCIDR: "100.65.0.0/24",
				Nics:     []string{"toswitch"},
				Neighbors: []v1alpha1.Neighbor{
					{
						ASN:     64514,
						Address: "192.168.11.2",
					},
				},
			},
		}
		vni := v1alpha1.VNI{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vni",
				Namespace: openperouter.Namespace,
			},
			Spec: v1alpha1.VNISpec{
				ASN:       64514,
				VNI:       100,
				LocalCIDR: "192.169.10.0/24",
				HostASN:   ptr.To(uint32(64515)),
			},
		}
		err := Updater.Update(config.Resources{
			Underlays: []v1alpha1.Underlay{
				underlay,
			},
			VNIs: []v1alpha1.VNI{
				vni,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		ginkgo.By("validating VNI")

		validateVNI(vniParams{
			VRF:        vni.Name,
			VethHostIP: "192.169.10.0",
			VNI:        100,
			VXLanPort:  4789,
		}, routerPods)
	})

})

type vniParams struct {
	VRF        string `json:"vrf"`
	VTEPIP     string `json:"vtepip"`
	VethHostIP string `json:"vethhostip"`
	VNI        int    `json:"vni"`
	VXLanPort  int    `json:"vxlanport"`
}

func validateVNI(vni vniParams, pod *corev1.Pod) {
	fileToValidate := sendConfigToValidate(pod, vni)
	Eventually(func() error {
		exec := executor.ForPod(pod.Namespace, pod.Name, "frr")
		res, err := exec.Exec("/validatehost", "--ginkgo.focus", "EXTERNAL.*vni", "--paramsfile", fileToValidate)
		if err != nil {
			return fmt.Errorf("failed to validate vni: %s %w", res, err)
		}
		return nil
	}, 2*time.Minute, time.Second).ShouldNot(HaveOccurred(), "a downtime should be observed")
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

func sendConfigToValidate[T any](pod *corev1.Pod, toValidate T) string {
	jsonData, err := json.MarshalIndent(toValidate, "", "  ")
	if err != nil {
		panic(err)
	}

	toValidateFile, err := os.CreateTemp(os.TempDir(), "validate-*.json")
	Expect(err).NotTo(HaveOccurred())

	_, err = toValidateFile.Write(jsonData)
	Expect(err).NotTo(HaveOccurred())

	err = k8s.SendFileToPod(toValidateFile.Name(), pod)
	Expect(err).NotTo(HaveOccurred())
	return filepath.Base(toValidateFile.Name())
}
