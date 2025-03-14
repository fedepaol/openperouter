//go:build externaltests
// +build externaltests

package hostnetwork

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	paramsFile string
)

func init() {
	flag.StringVar(&paramsFile, "paramsfile", "", "the json file containing the parameters to verify")
	flag.Parse()
}

func TestHostNetwork(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "HostNetwork Suite")
}

var _ = Describe("EXTERNAL", func() {

	Context("underlay", func() {
		var params UnderlayParams
		BeforeEach(func() {
			var err error
			params, err = readParamsFromFile[UnderlayParams](paramsFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be configured", func() {
			Eventually(func(g Gomega) {
				validateUnderlay(g, params)
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Context("vni", func() {
		var params VNIParams
		BeforeEach(func() {
			var err error
			params, err = readParamsFromFile[VNIParams](paramsFile)
			Expect(err).NotTo(HaveOccurred())

		})

		It("should be configured", func() {
			Eventually(func(g Gomega) {
				validateVNI(g, params)
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})
	})

})

func readParamsFromFile[T any](filePath string) (T, error) {
	var params T

	file, err := os.Open(filePath)
	if err != nil {
		return params, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		return params, fmt.Errorf("failed to read file: %w", err)
	}

	err = json.Unmarshal(byteValue, &params)
	if err != nil {
		return params, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return params, nil
}
