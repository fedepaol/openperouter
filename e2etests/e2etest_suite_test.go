package e2e

import (
	"flag"
	"os"
	"testing"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openperouter/openperouter/e2etests/hostconfiguration"
	"github.com/openperouter/openperouter/e2etests/pkg/config"
	"github.com/openperouter/openperouter/e2etests/pkg/executor"
	"github.com/openperouter/openperouter/e2etests/pkg/k8sclient"
	"github.com/openperouter/openperouter/e2etests/pkg/openperouter"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	skipDockerCmd bool
	updater       config.Updater
)

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	// 	flag.BoolVar(&skipDockerCmd, "skip-docker", false, "set this to true if the BGP daemon is running on the host instead of in a container")
	//	flag.StringVar(&reportPath, "report-path", "/tmp/report", "the path to be used to dump test failure information")
	flag.StringVar(&executor.Kubectl, "kubectl", "kubectl", "the path for the kubectl binary")
	flag.StringVar(&hostconfiguration.ValidatorPath, "hostvalidator", "hostvalidator", "the path for the hostvalidator binary")
	flag.Parse()

	// dump.ReportPath = reportPath
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

	RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))
	clientconfig := k8sclient.RestConfig()
	var err error
	updater, err = config.UpdaterForCRs(clientconfig, openperouter.Namespace)
	Expect(err).NotTo(HaveOccurred())
	hostconfiguration.Updater = updater

})

var _ = ginkgo.AfterSuite(func() {
	updater.Clean()
	// cs := k8sclient.New()
})
