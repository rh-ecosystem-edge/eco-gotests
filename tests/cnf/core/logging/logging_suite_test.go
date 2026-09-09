package logging

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/logging/internal/loginittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/logging/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/logging/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
)

func TestLogging(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = CoreConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "logging", Label(tsparams.Labels...), reporterConfig)
}

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, CoreConfig.GetReportPath(), CoreConfig.TCPrefix)
})
