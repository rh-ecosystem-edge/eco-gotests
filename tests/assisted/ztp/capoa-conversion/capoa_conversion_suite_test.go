package capoa_conversion_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-conversion/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-conversion/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-conversion/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestCAPOAConversion(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = GeneralConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "CAPOA Conversion Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Check if hub has valid apiClient")

	if HubAPIClient == nil {
		Skip("Cannot run CAPOA Conversion suite when hub has nil api client")
	}
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, GeneralConfig.GetReportPath(), GeneralConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})
