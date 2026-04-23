package preinstall_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	raninittools "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/internal/raninittools"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPreinstall(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	if raninittools.RANConfig != nil {
		reporterConfig.JUnitReport = raninittools.RANConfig.GetJunitReportPath(currentFile)
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "IBI Preinstall Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Checking ran (IBI CNF) configuration")

	Expect(raninittools.RANConfig).ToNot(BeNil(), "Cannot run test suite when ran configuration failed to load")

	By("Checking if hub cluster has valid apiClient")

	Expect(raninittools.HubAPIClient).ToNot(BeNil(),
		"Cannot run test suite when hub cluster has nil api client (set ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG)")

	By("Checking mandatory preinstall configuration")

	Expect(raninittools.RANConfig.ValidateMandatory()).To(Succeed(),
		"IBI preinstall mandatory configuration incomplete")
})

var _ = ReportAfterSuite("", func(report Report) {
	if raninittools.RANConfig == nil {
		return
	}

	reportxml.Create(
		report, raninittools.RANConfig.GetReportPath(), raninittools.RANConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump,
	)
})
