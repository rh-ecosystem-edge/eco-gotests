package spoke_test

import (
	"testing"

	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestSpoke(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = ZTPConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Spoke Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Check if hub has valid apiClient")
	if HubAPIClient == nil {
		Skip("Cannot run spoke suite when hub has nil api client")
	}

	By("Check if spoke has valid apiClient")
	if SpokeAPIClient == nil {
		Skip("Cannot run spoke suite when spoke has nil api client")
	}

})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, ZTPConfig.GetReportPath(), ZTPConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})
