package ipchange_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/ipcinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/tsparams"

	// Blank import to trigger test registration - this is a standard Ginkgo pattern.
	// The linter may show a false positive, but the build succeeds and the package is valid.
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestIPChange(t *testing.T) {
	if IPCConfig == nil {
		t.Skip("IPCConfig is nil; check envconfig inputs")
	}

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = IPCConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "IPChange Suite", Label(tsparams.LabelSuite), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Checking if API client is valid")

	if APIClient == nil {
		Skip("Cannot run test suite when API client is nil")
	}
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, IPCConfig.GetReportPath(), IPCConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailedOnCluster(
		APIClient.KubeconfigPath,
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})
