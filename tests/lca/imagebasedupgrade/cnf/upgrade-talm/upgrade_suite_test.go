package upgrade_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/upgrade-talm/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/upgrade-talm/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestUpgrade(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = CNFConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgrade Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	// should have top level check to skip all tests in case test env vars unavailable.
	By("Checking if target hub cluster has valid apiClient")
	if TargetHubAPIClient == nil {
		Skip("Cannot run test suite when target hub cluster has nil api client")
	}

	By("Checking if target sno cluster has valid apiClient")
	if TargetSNOAPIClient == nil {
		Skip("Cannot run test suite when target sno cluster has nil api client")
	}
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, CNFConfig.GetReportPath(), CNFConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})
