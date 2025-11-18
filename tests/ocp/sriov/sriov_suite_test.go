package sriov

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	sriovenv "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

var (
	testNS *namespace.Builder
)

func TestSriov(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = sriovenv.NetConfig.GetJunitReportPath()

	RegisterFailHandler(Fail)
	RunSpecs(t, "OCP SR-IOV Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Cleaning up leftover resources from previous test runs")
	err := sriovenv.CleanupLeftoverResources(sriovenv.APIClient, sriovenv.NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup leftover resources")

	By("Creating test namespace with privileged labels")
	testNS = namespace.NewBuilder(sriovenv.APIClient, tsparams.TestNamespaceName)
	for key, value := range params.PrivilegedNSLabels {
		testNS.WithLabel(key, value)
	}
	_, err = testNS.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace %q", testNS.Definition.Name)

	By("Verifying if sriov tests can be executed on given cluster")
	err = sriovenv.IsSriovDeployed(sriovenv.APIClient, sriovenv.NetConfig)
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")

	By("Pulling test images on cluster before running test cases")
	err = sriovenv.PullTestImageOnNodes(sriovenv.APIClient, sriovenv.NetConfig.WorkerLabel, sriovenv.NetConfig.CnfNetTestContainer, 300)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull test image on nodes")
})

var _ = AfterSuite(func() {
	By("Deleting test namespace")
	if testNS != nil {
		err := testNS.DeleteAndWait(tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace")
	}
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, sriovenv.NetConfig.GetReportPath(), sriovenv.NetConfig.TCPrefix())
})

