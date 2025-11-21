package sriov

import (
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	sriovenv "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

var (
	testNS *namespace.Builder
)

func TestSriov(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = SriovOcpConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "OCP SR-IOV Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Validating test configuration")
	// Validate and normalize worker label format - must be a valid Kubernetes label selector
	if SriovOcpConfig.OcpWorkerLabel != "" && !strings.Contains(SriovOcpConfig.OcpWorkerLabel, "=") {
		Fail("Invalid worker label configuration: OcpWorkerLabel must be in format 'key=value' or 'key=' " +
			"(e.g., 'node-role.kubernetes.io/worker='). " +
			"Current value: '" + SriovOcpConfig.OcpWorkerLabel + "'. " +
			"Please set ECO_OCP_SRIOV_WORKER_LABEL environment variable correctly.")
	}

	By("Cleaning up leftover resources from previous test runs")
	err := sriovenv.CleanupLeftoverResources(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup leftover resources")

	By("Creating test namespace with privileged labels")
	testNS = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
	for key, value := range params.PrivilegedNSLabels {
		testNS.WithLabel(key, value)
	}
	_, err = testNS.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace %q", testNS.Definition.Name)

	By("Verifying if sriov tests can be executed on given cluster")
	err = sriovoperator.IsSriovDeployed(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")

	By("Pulling test images on cluster before running test cases")
	// Use local PullTestImageOnNodes which defers image pulling to first pod creation
	// This avoids the bug in cluster.PullTestImageOnNodes and reduces test startup time
	err = sriovenv.PullTestImageOnNodes(APIClient, SriovOcpConfig.OcpWorkerLabel, SriovOcpConfig.OcpSriovTestContainer, 300)
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
	reportxml.Create(report, SriovOcpConfig.GetReportPath(), SriovOcpConfig.TCPrefix)
})
