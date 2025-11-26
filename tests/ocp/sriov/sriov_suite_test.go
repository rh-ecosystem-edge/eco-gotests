package sriov

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	sriovconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	sriovenv "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
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
	// Generate timestamp once at suite start for consistent report naming
	timestamp := sriovconfig.GetTimestamp()
	SriovOcpConfig.SetReportTimestamp(timestamp)

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = SriovOcpConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "OCP SR-IOV Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Validating test configuration")
	// Ensure worker label uses "key=value" or "key=" format to avoid invalid selectors
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
	if err != nil {
		// Handle pre-existing namespace from previous partial failures
		if apierrors.IsAlreadyExists(err) {
			klog.V(90).Infof("Test namespace %q already exists, deleting and recreating", tsparams.TestNamespaceName)

			// Pull the existing namespace so we can delete it
			existingNS, pullErr := namespace.Pull(APIClient, tsparams.TestNamespaceName)
			Expect(pullErr).ToNot(HaveOccurred(), "Failed to pull existing test namespace %q", tsparams.TestNamespaceName)

			// Delete the existing namespace
			deleteErr := existingNS.DeleteAndWait(tsparams.DefaultTimeout)
			Expect(deleteErr).ToNot(HaveOccurred(), "Failed to delete existing test namespace %q", tsparams.TestNamespaceName)

			// Recreate the namespace with fresh state
			testNS = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
			for key, value := range params.PrivilegedNSLabels {
				testNS.WithLabel(key, value)
			}

			_, err = testNS.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to recreate test namespace %q", tsparams.TestNamespaceName)
		} else {
			Fail(fmt.Sprintf("Failed to create test namespace %q: %v", tsparams.TestNamespaceName, err))
		}
	}

	By("Verifying if sriov tests can be executed on given cluster")
	err = sriovoperator.IsSriovDeployed(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")

	By("Pulling test images on cluster before running test cases")
	// Use local PullTestImageOnNodes which defers image pulling to first pod creation
	// This avoids the bug in cluster.PullTestImageOnNodes and reduces test startup time
	err = sriovenv.PullTestImageOnNodes(
		APIClient,
		SriovOcpConfig.OcpWorkerLabel,
		SriovOcpConfig.OcpSriovTestContainer,
		int(tsparams.DefaultTimeout.Seconds()))
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
	// Get cluster and operator versions for report metadata
	var ocpVersion, sriovVersion string
	var versionErr error

	ocpVersion, versionErr = sriovenv.GetOCPVersion(APIClient)
	if versionErr != nil {
		klog.V(90).Infof("Failed to get OCP version: %v", versionErr)
		ocpVersion = "unknown"
	}

	sriovVersion, versionErr = sriovenv.GetSriovOperatorVersion(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	if versionErr != nil {
		klog.V(90).Infof("Failed to get SR-IOV operator version: %v", versionErr)
		sriovVersion = "unknown"
	}

	// Get SR-IOV operator pod container information
	var containerInfo []sriovenv.PodContainerInfo
	containerInfo, versionErr = sriovenv.GetSriovOperatorPodContainers(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	if versionErr != nil {
		klog.V(90).Infof("Failed to get SR-IOV operator pod containers: %v", versionErr)
		containerInfo = []sriovenv.PodContainerInfo{}
	}

	// Log version information
	klog.V(90).Infof("Test Report Metadata - OCP Version: %s, SR-IOV Operator Version: %s", ocpVersion, sriovVersion)

	// Create the report
	reportPath := SriovOcpConfig.GetReportPath()
	reportxml.Create(report, reportPath, SriovOcpConfig.TCPrefix)

	// Write version metadata to a separate file alongside the report
	// Metadata file format:
	// - Plain text format for human readability
	// - Contains: OCP version, SR-IOV operator version, report timestamp, and container information
	// - Container information is grouped by pod name, showing both regular and init containers
	// - Format: "Container: <name>" for regular containers, "Container: <name> (init)" for init containers
	// - Each container entry includes its image
	// Note: For machine parsing, consider future enhancement to YAML/JSON format
	// Only create metadata file if report path is available (EnableReport must be true)
	if reportPath != "" {
		metadataPath := strings.TrimSuffix(reportPath, ".xml") + "_metadata.txt"

		var metadataBuilder strings.Builder
		metadataBuilder.WriteString(fmt.Sprintf("OpenShift Cluster Version: %s\n", ocpVersion))
		metadataBuilder.WriteString(fmt.Sprintf("SR-IOV Operator Version: %s\n", sriovVersion))
		metadataBuilder.WriteString(fmt.Sprintf("Report Generated: %s\n", report.EndTime.Format("2006-01-02 15:04:05")))
		metadataBuilder.WriteString("\nSR-IOV Operator Pod Containers:\n")

		if len(containerInfo) > 0 {
			// Group by pod name for better readability
			podMap := make(map[string][]sriovenv.PodContainerInfo)
			for _, info := range containerInfo {
				podMap[info.PodName] = append(podMap[info.PodName], info)
			}

			// Sort pod names for consistent output order
			podNames := make([]string, 0, len(podMap))
			for podName := range podMap {
				podNames = append(podNames, podName)
			}
			sort.Strings(podNames)

			for _, podName := range podNames {
				containers := podMap[podName]
				metadataBuilder.WriteString(fmt.Sprintf("\n  Pod: %s\n", podName))
				for _, container := range containers {
					metadataBuilder.WriteString(fmt.Sprintf("    Container: %s\n", container.ContainerName))
					metadataBuilder.WriteString(fmt.Sprintf("    Image: %s\n", container.Image))
				}
			}
		} else {
			metadataBuilder.WriteString("  (No container information available)\n")
		}

		if err := os.WriteFile(metadataPath, []byte(metadataBuilder.String()), 0600); err != nil {
			klog.Errorf("Failed to write metadata file %s: %v", metadataPath, err)
		} else {
			klog.V(90).Infof("Version metadata written to: %s", metadataPath)
		}
	}

	// Print report file locations for visibility in test logs
	junitReportPath := SriovOcpConfig.GetJunitReportPath(currentFile)
	fmt.Printf("\n=== Test Report Files ===\n")
	fmt.Printf("JUnit Report: %s\n", junitReportPath)

	if reportPath != "" {
		fmt.Printf("Test Run Report: %s\n", reportPath)
		// Reuse metadataPath from above (already computed when writing the file)
		fmt.Printf("Metadata File: %s\n", strings.TrimSuffix(reportPath, ".xml")+"_metadata.txt")
	} else {
		fmt.Printf("Test Run Report: (disabled - set ECO_ENABLE_REPORT=true to enable)\n")
		fmt.Printf("Metadata File: (disabled - set ECO_ENABLE_REPORT=true to enable)\n")
	}
	fmt.Printf("========================\n\n")
})
