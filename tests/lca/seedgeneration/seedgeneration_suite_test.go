package seedgeneration_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/seedgeneration/internal/seedgenerationinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/seedgeneration/internal/tsparams"

	// Blank import to trigger test registration - this is a standard Ginkgo pattern.
	// The linter may show a false positive, but the build succeeds and the package is valid.
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/seedgeneration/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestSeedGeneration(t *testing.T) {
	if SeedGenerationConfig == nil {
		t.Skip("SeedGenerationConfig is nil; check envconfig inputs")
	}

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = SeedGenerationConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Seed Generation Suite", Label(tsparams.LabelSuite), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Checking if target sno cluster has valid apiClient")
	if TargetSNOAPIClient == nil {
		Skip("Cannot run test suite when target sno cluster has nil api client")
	}
})

var _ = ReportAfterSuite("", func(report Report) {
	if SeedGenerationConfig == nil {
		return
	}
	reportxml.Create(
		report, SeedGenerationConfig.GetReportPath(), SeedGenerationConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailedOnCluster(
		SeedGenerationConfig.TargetSNOKubeConfig,
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})
