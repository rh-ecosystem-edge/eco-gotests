package sriov

import (
	"fmt"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/tests"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
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
	// WorkerLabel is provided by GeneralConfig and is always in correct format
	if SriovOcpConfig.WorkerLabel == "" {
		Fail("Worker label not configured. Ensure ECO_WORKER_LABEL is set or default config is loaded.")
	}

	By("Cleaning up leftover resources from previous test runs")
	err := sriovenv.CleanupLeftoverResources()
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup leftover resources")

	By("Creating test namespace with privileged labels")
	testNS = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
	for key, value := range params.PrivilegedNSLabels {
		testNS.WithLabel(key, value)
	}

	_, err = testNS.Create()
	if err != nil {
		// Handle pre-existing namespace from previous partial failures
		if apierrors.IsAlreadyExists(err) {
			klog.V(90).Infof("Test namespace %q already exists, deleting and recreating", tsparams.TestNamespaceName)

			existingNS, pullErr := namespace.Pull(APIClient, tsparams.TestNamespaceName)
			Expect(pullErr).ToNot(HaveOccurred(), "Failed to pull existing test namespace")

			deleteErr := existingNS.DeleteAndWait(tsparams.DefaultTimeout)
			Expect(deleteErr).ToNot(HaveOccurred(), "Failed to delete existing test namespace")

			testNS = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
			for key, value := range params.PrivilegedNSLabels {
				testNS.WithLabel(key, value)
			}

			_, err = testNS.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to recreate test namespace")
		} else {
			Fail(fmt.Sprintf("Failed to create test namespace: %v", err))
		}
	}

	By("Verifying if sriov tests can be executed on given cluster")
	err = sriovenv.CheckSriovOperatorStatus()
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")
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
	reportPath := SriovOcpConfig.GetReportPath()
	reportxml.Create(report, reportPath, SriovOcpConfig.TCPrefix)

	// Print report file locations
	junitReportPath := SriovOcpConfig.GetJunitReportPath(currentFile)
	fmt.Printf("\n=== Test Report Files ===\n")
	fmt.Printf("JUnit Report: %s\n", junitReportPath)

	if reportPath != "" {
		fmt.Printf("Test Run Report: %s\n", reportPath)
	} else {
		fmt.Printf("Test Run Report: (disabled - set ECO_ENABLE_REPORT=true to enable)\n")
	}

	fmt.Printf("========================\n\n")
})
