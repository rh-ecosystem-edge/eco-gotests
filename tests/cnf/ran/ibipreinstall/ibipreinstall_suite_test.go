package ibipreinstall_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/tests"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var _, currentFile, _, _ = runtime.Caller(0)

// TestIBIPreinstall is the entry point for the IBI preinstall test suite.
func TestIBIPreinstall(t *testing.T) {
	if RANConfig == nil {
		t.Fatal("Cannot run test suite when ran configuration failed to load")
	}

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = RANConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "IBI Preinstall Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Checking ran configuration")
	Expect(RANConfig).ToNot(BeNil(), "Cannot run test suite when ran configuration failed to load")

	By("Checking if hub cluster has valid apiClient")
	Expect(HubAPIClient).ToNot(BeNil(),
		"Cannot run test suite when hub cluster has nil api client (set ECO_CNF_RAN_KUBECONFIG_HUB)")

	By("Checking mandatory IBI preinstall configuration")
	Expect(RANConfig.IBIPreinstallConfig).ToNot(BeNil(),
		"IBI preinstall config is nil")
	Expect(RANConfig.IBIPreinstallConfig.ValidatePreinstallMandatory(
		RANConfig.BMCUsername, RANConfig.BMCPassword)).To(Succeed(),
		"IBI preinstall mandatory configuration incomplete")
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, RANConfig.GetReportPath(), RANConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	specReport := CurrentSpecReport()

	reporter.ReportIfFailedOnCluster(
		RANConfig.HubKubeconfig,
		specReport, currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)

	helpers.CollectDiagnosticsIfFailed(
		specReport, currentFile, tests.SpokeHostName, tests.WorkDir)
})
