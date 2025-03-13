// SPDX-License-Identifier:Apache-2.0

package e2e

import (
	"flag"
	"os"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/e2etests/pkg/dump"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	skipDockerCmd bool
)

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	flag.BoolVar(&skipDockerCmd, "skip-docker", false, "set this to true if the BGP daemon is running on the host instead of in a container")
	flag.StringVar(&reportPath, "report-path", "/tmp/report", "the path to be used to dump test failure information")
	flag.StringVar(&executor.Kubectl, "kubectl", "kubectl", "the path for the kubectl binary")
	flag.StringVar(&frrImage, "frr-image", "quay.io/frrouting/frr:9.1.0", "the image to use for the external frr containers")
	flag.Parse()

	dump.ReportPath = reportPath
	tests.FRRImage = frrImage
}

func TestMain(m *testing.M) {
	// Register test flags, then parse flags.
	handleFlags()
	if testing.Short() {
		return
	}

	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {
	if testing.Short() {
		return
	}

	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))
	cs := k8sclient.New()
})

var _ = ginkgo.AfterSuite(func() {
	cs := k8sclient.New()

})
