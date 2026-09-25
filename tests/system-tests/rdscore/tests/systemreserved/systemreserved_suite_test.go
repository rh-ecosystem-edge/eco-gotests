package systemreserved

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	performanceprofileV2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	tunedv1 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/tuned/v1"
)

const (
	// LabelSuite represents systemreserved test suite label.
	labelSuite = "systemreserved"
	// LabelSystemReservedValidation represents systemreserved validation test label.
	labelSystemReservedValidation = "systemreserved-validation"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)

	// labels represents the range of labels that can be used for test cases selection.
	labels = []string{labelSuite, labelSystemReservedValidation}

	// reporterNamespacesToDump tells reporter which namespaces to collect pod logs from.
	reporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator":       "openshift-performance-addon-operator",
		"openshift-cluster-node-tuning-operator":     "openshift-cluster-node-tuning-operator",
	}

	// reporterCRDsToDump tells reporter which CRDs to dump.
	reporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &performanceprofileV2.PerformanceProfileList{}},
		{Cr: &tunedv1.TunedList{}},
		{Cr: &mcfgv1.MachineConfigList{}},
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
)

func TestSystemReserved(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()

	reporterConfig.JUnitReport = GeneralConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "SystemReserved Suite", Label(labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Verifying cluster is ready for systemReserved testing")
	Expect(APIClient).ToNot(BeNil(), "K8s client is nil")
	// Additional pre-test validations can be added here
	// For example: verify Performance Addon Operator or Node Tuning Operator is installed
})

var _ = AfterSuite(func() {
	By("Cleaning up after systemReserved tests")
	// Cleanup logic if needed
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, reporterNamespacesToDump, reporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, GeneralConfig.GetReportPath(), GeneralConfig.TCPrefix)
})
