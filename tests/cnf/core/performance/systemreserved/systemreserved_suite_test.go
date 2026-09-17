package systemreserved

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/performance/systemreserved/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/performance/systemreserved/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
	// APIClient is the k8s client.
	APIClient *clients.Settings
	// PerfConfig contains performance test configuration.
	PerfConfig *inittools.PerfConfig
)

func TestSystemReserved(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()

	// Initialize test configuration
	PerfConfig = inittools.NewPerfConfig()
	reporterConfig.JUnitReport = PerfConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "SystemReserved Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Initializing k8s client")
	var err error
	APIClient, err = clients.New("")
	Expect(err).ToNot(HaveOccurred(), "Failed to create k8s client")
	Expect(APIClient).ToNot(BeNil(), "K8s client is nil")

	By("Verifying cluster is ready for systemReserved testing")
	// Additional pre-test validations can be added here
	// For example: verify Performance Addon Operator or Node Tuning Operator is installed
})

var _ = AfterSuite(func() {
	By("Cleaning up after systemReserved tests")
	// Cleanup logic if needed
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, PerfConfig.GetReportPath(), PerfConfig.TCPrefix)
})
