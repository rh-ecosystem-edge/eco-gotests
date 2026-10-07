package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/fec/fectypes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/perfprofile"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	sriovfec "github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov-fec"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/accelerator/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
)

type accCardProfile struct {
	cardName                 string
	deviceID                 string
	resourceName             string
	envVar                   string
	expectedBbdevTestsPassed int
	buildClusterConfig       func(pciAddress, nodeName string, secureBoot bool) *sriovfec.ClusterConfigBuilder
}

var _ = Describe("Intel Accelerator", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	var secureBoot bool

	BeforeAll(func() {
		By("Checking if operator is installed and has required resources")

		fecDeploy, err := deployment.Pull(APIClient, "sriov-fec-controller-manager", tsparams.OperatorNamespace)
		if err != nil && err.Error() == "no matches for kind \"SriovFecNodeConfig\" in version \"sriovfec.intel.com/v1\"" {
			Skip("Cluster does not have operator installed")
		}

		Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovFecOperator")
		Expect(fecDeploy.IsReady(2*time.Minute)).To(BeTrue(), "SriovFecOperator is not ready")

		By("Checking if node config exists")

		sfncList, err := sriovfec.List(APIClient, tsparams.OperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to list SriovFecNodeConfig")

		if len(sfncList) == 0 {
			Skip("No SriovFecNodeConfig found")
		}

		By("Checking if all daemonsets are present and ready")

		for _, dsName := range tsparams.DaemonsetNames {
			ds, err := daemonset.Pull(APIClient, dsName, tsparams.OperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull %s", dsName)
			Expect(ds.IsReady(2*time.Minute)).To(BeTrue(), "%s is not ready", dsName)
		}

		secureBoot = isSecureBootEnabled()

		By("Deploying PerformanceProfile if it's not installed")

		performanceProfiles, err := nto.ListProfiles(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list PerformanceProfiles")

		if len(performanceProfiles) == 0 {
			err = perfprofile.DeployPerformanceProfile(
				APIClient,
				NetConfig.WorkerLabelMap,
				NetConfig.CnfMcpLabel,
				"performance-profile-dpdk",
				"1,3,5,7,9,11,13,15,17,19,21,23,25",
				"0,2,4,6,8,10,12,14,16,18,20",
				24,
				tsparams.MCOWaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")
		} else {
			names := make([]string, 0, len(performanceProfiles))
			for _, profile := range performanceProfiles {
				names = append(names, profile.Object.Name)
			}

			By(fmt.Sprintf("Skipping PerformanceProfile deploy; cluster already has: %s", strings.Join(names, ", ")))
		}
	})

	defineAcceleratorCardContext(&secureBoot, accCardProfile{
		cardName:                 "ACC100",
		deviceID:                 tsparams.Acc100DeviceID,
		resourceName:             tsparams.Acc100ResourceName,
		envVar:                   tsparams.Acc100EnvVar,
		expectedBbdevTestsPassed: tsparams.ExpectedNumberBbdevTestsPassedForAcc100,
		buildClusterConfig:       defineFecClusterConfigAcc100,
	}, "41073", "41216")

	defineAcceleratorCardContext(&secureBoot, accCardProfile{
		cardName:                 "ACC200",
		deviceID:                 tsparams.Acc200DeviceID,
		resourceName:             tsparams.Acc200ResourceName,
		envVar:                   tsparams.Acc200EnvVar,
		expectedBbdevTestsPassed: tsparams.ExpectedNumberBbdevTestsPassedForAcc200,
		buildClusterConfig:       defineFecClusterConfigAcc200,
	}, "", "")
})

func defineAcceleratorCardContext(
	secureBoot *bool, profile accCardProfile, nodeResourceReportID, validationReportID string,
) {
	Context(profile.cardName, func() {
		var (
			sfnc        *sriovfec.NodeConfigBuilder
			accelerator *fectypes.SriovAccelerator
			err         error
		)

		BeforeAll(func() {
			By(fmt.Sprintf("Checking if the cluster has %s cards", profile.cardName))

			sfnc, accelerator, err = getFecNodeConfigWithAccCard(profile.deviceID)
			if err != nil {
				Skip(fmt.Sprintf("Cluster does not have %s cards: %s", profile.cardName, err.Error()))
			}

			deleteSriovFecClusterConfigs()

			By("Creating SriovFecClusterConfig")

			_, err = profile.buildClusterConfig(accelerator.PCIAddress, sfnc.Object.Name, *secureBoot).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovFecClusterConfig")

			err = waitForNodeConfigToSucceed()
			Expect(err).ToNot(HaveOccurred(), "SriovFecNodeConfig never succeeded")
		})

		AfterAll(func() {
			By("Deleting SriovFecClusterConfig in the cluster")
			deleteSriovFecClusterConfigs()
		})

		nodeResourceSpec := func() {
			Eventually(getNodeResource, 10*time.Minute, time.Second).
				WithArguments(sfnc.Object.Name, profile.resourceName).To(BeNumerically(">", 0))
		}
		if nodeResourceReportID != "" {
			It(fmt.Sprintf("node should show %s resource", strings.ToLower(profile.cardName)),
				reportxml.ID(nodeResourceReportID), nodeResourceSpec)
		} else {
			It(fmt.Sprintf("node should show %s resource", strings.ToLower(profile.cardName)), nodeResourceSpec)
		}

		validationSpec := func() {
			runAcceleratorResourceValidation(sfnc, profile)
		}
		if validationReportID != "" {
			It(fmt.Sprintf("validation of %s resource", strings.ToLower(profile.cardName)),
				reportxml.ID(validationReportID), validationSpec)
		} else {
			It(fmt.Sprintf("validation of %s resource", strings.ToLower(profile.cardName)), validationSpec)
		}
	})
}

func runAcceleratorResourceValidation(sfnc *sriovfec.NodeConfigBuilder, profile accCardProfile) {
	Eventually(getNodeResource, 10*time.Minute, time.Second).
		WithArguments(sfnc.Object.Name, profile.resourceName).To(BeNumerically(">", 0))

	By("Creating bbdev test pod")

	bbdevContainer, err := pod.NewContainerBuilder(
		"bbdev", NetConfig.CnfNetTestContainer, []string{"bash", "-c", "sleep infinity"}).
		WithSecurityContext(&corev1.SecurityContext{
			Privileged:   ptr.To(false),
			RunAsUser:    ptr.To(int64(0)),
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE"}},
		}).
		WithCustomResourcesLimits(corev1.ResourceList{
			"hugepages-1Gi": resource.MustParse("1Gi"),
			"memory":        resource.MustParse("1Gi"),
			"cpu":           *resource.NewQuantity(4, resource.DecimalSI),
			corev1.ResourceName(profile.resourceName): resource.MustParse("1"),
		}).
		WithCustomResourcesRequests(corev1.ResourceList{
			"hugepages-1Gi": resource.MustParse("1Gi"),
			"memory":        resource.MustParse("1Gi"),
			"cpu":           *resource.NewQuantity(4, resource.DecimalSI),
			corev1.ResourceName(profile.resourceName): resource.MustParse("1"),
		}).
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container configuration")

	bbdevPod, err := pod.NewBuilder(APIClient, "bbdev-test", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		RedefineDefaultContainer(*bbdevContainer).
		DefineOnNode(sfnc.Object.Name).
		WithHugePages().
		CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create bbdev test pod")

	By("Running bbdev tests")

	bbdevTestsOutput := runBbdevTests(bbdevPod, profile.envVar)
	fmt.Println(bbdevTestsOutput)
	Expect(bbdevTestsOutput).ToNot(BeEmpty(), "Failed to run bbdev tests")

	By("Checking if bbdev tests executed")

	totalSuites, totalPassed, totalFailed := countBbdevTestResults(bbdevTestsOutput)

	By(fmt.Sprintf("BBdev test results: %d suites ran, %d tests passed, %d tests failed",
		totalSuites, totalPassed, totalFailed))

	Expect(totalSuites).To(Equal(tsparams.TotalNumberBbdevTests), "Not all test suites were executed")
	Expect(totalPassed).To(Equal(profile.expectedBbdevTestsPassed), "Not all expected tests passed")
	Expect(totalFailed).To(Equal(0), "Some tests failed")
}

func deleteSriovFecClusterConfigs() {
	By("Deleting SriovFecClusterConfig if any present in the cluster")

	sfccList, err := sriovfec.ListClusterConfig(APIClient, tsparams.OperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to list SriovFecClusterConfig")

	for _, sfcc := range sfccList {
		_, err := sfcc.Delete()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovFecClusterConfig")
	}
}

func getNodeResource(nodeName, resName string) (int64, error) {
	testNode, err := nodes.Pull(APIClient, nodeName)
	if err != nil {
		return 0, err
	}

	quantity, exists := testNode.Object.Status.Allocatable[corev1.ResourceName(resName)]

	if !exists {
		return 0, fmt.Errorf("resource %s is not available", resName)
	}

	quantityInt, ok := quantity.AsInt64()
	if !ok {
		return 0, fmt.Errorf("failed to convert quantity to int64")
	}

	return quantityInt, nil
}

func isSecureBootEnabled() bool {
	workernodeList, err := nodes.List(APIClient, metav1.ListOptions{LabelSelector: NetConfig.WorkerLabel})
	Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")
	Expect(len(workernodeList)).To(BeNumerically(">=", 1), "Worker node list length must be > 1")

	By("Creating test pod")

	testcontainer, err := pod.NewContainerBuilder(
		"testcontainer", NetConfig.CnfNetTestContainer, []string{"sleep", "infinity"}).
		WithVolumeMount(corev1.VolumeMount{Name: "host", MountPath: "/host", ReadOnly: false}).
		WithSecurityContext(&corev1.SecurityContext{
			Privileged:   ptr.To(true),
			RunAsUser:    ptr.To(int64(0)),
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "SYS_ADMIN"}},
		}).
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container configuration")

	testPod, err := pod.NewBuilder(
		APIClient, "testpod1", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithVolume(
			corev1.Volume{Name: "host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}}).
		DefineOnNode(workernodeList[0].Object.Name).
		RedefineDefaultContainer(*testcontainer).
		CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create test pod")

	output, err := testPod.ExecCommand([]string{"cat", "/host/sys/kernel/security/lockdown"})
	Expect(err).ToNot(HaveOccurred(), "Failed to get /host/sys/kernel/security/lockdown")

	if strings.Contains(output.String(), "No such file or directory") || err != nil {
		return false
	}

	return strings.Contains(output.String(), "[integrity]") ||
		strings.Contains(output.String(), "[confidentiality]")
}

func getFecNodeConfigWithAccCard(devID string) (*sriovfec.NodeConfigBuilder, *fectypes.SriovAccelerator, error) {
	sfncList, err := sriovfec.List(APIClient, tsparams.OperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to list SriovFecNodeConfig")

	for _, sfnc := range sfncList {
		for _, accelerator := range sfnc.Object.Status.Inventory.SriovAccelerators {
			if accelerator.DeviceID == devID {
				return sfnc, &accelerator, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("cluster doesn`t have sriovfecnodeconfig with accelerator id %s", devID)
}

func defaultQueueGroupConfig() fectypes.QueueGroupConfig {
	return fectypes.QueueGroupConfig{
		AqDepthLog2:     4,
		NumAqsPerGroups: 16,
		NumQueueGroups:  2,
	}
}

func acc100BBDevConfig(queueGroupConfig fectypes.QueueGroupConfig) *fectypes.ACC100BBDevConfig {
	return &fectypes.ACC100BBDevConfig{
		Downlink4G:   queueGroupConfig,
		Downlink5G:   queueGroupConfig,
		Uplink4G:     queueGroupConfig,
		Uplink5G:     queueGroupConfig,
		PFMode:       false,
		MaxQueueSize: 1024,
		NumVfBundles: 2,
	}
}

func newFecClusterConfigBuilder(
	pciAddress, nodeName string, _ bool, bbdevConfig fectypes.BBDevConfig,
) *sriovfec.ClusterConfigBuilder {
	sfccBuilder := sriovfec.NewClusterConfigBuilder(APIClient, "config", tsparams.OperatorNamespace)

	sfccBuilder.Definition.Spec = fectypes.SriovFecClusterConfigSpec{
		Priority: 1,
		NodeSelector: map[string]string{
			"kubernetes.io/hostname": nodeName,
		},
		AcceleratorSelector: fectypes.AcceleratorSelector{
			PCIAddress: pciAddress,
		},
		PhysicalFunction: fectypes.PhysicalFunctionConfig{
			PFDriver:    "vfio-pci",
			VFAmount:    2,
			VFDriver:    "vfio-pci",
			BBDevConfig: bbdevConfig,
		},
	}

	return sfccBuilder
}

func defineFecClusterConfigAcc100(pciAddress, nodeName string, secureBoot bool) *sriovfec.ClusterConfigBuilder {
	queueGroupConfig := defaultQueueGroupConfig()

	return newFecClusterConfigBuilder(pciAddress, nodeName, secureBoot, fectypes.BBDevConfig{
		ACC100: acc100BBDevConfig(queueGroupConfig),
	})
}

func defineFecClusterConfigAcc200(pciAddress, nodeName string, secureBoot bool) *sriovfec.ClusterConfigBuilder {
	queueGroupConfig := defaultQueueGroupConfig()

	return newFecClusterConfigBuilder(pciAddress, nodeName, secureBoot, fectypes.BBDevConfig{
		ACC200: &fectypes.ACC200BBDevConfig{
			ACC100BBDevConfig: *acc100BBDevConfig(queueGroupConfig),
			QFFT:              queueGroupConfig,
		},
	})
}

func waitForNodeConfigToSucceed() error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			sfncList, err := sriovfec.List(APIClient, tsparams.OperatorNamespace)
			if err != nil {
				return false, nil
			}

			for _, sfnc := range sfncList {
				for _, condition := range sfnc.Object.Status.Conditions {
					if condition.Reason == "Succeeded" {
						return true, nil
					}
				}
			}

			return false, nil
		})
}

// runBbdevTests executes bbdev tests in bbdev pod.
func runBbdevTests(bbdevPod *pod.Builder, deviceEnvVar string) string {
	pciAddress, vfioToken, err := getFecDevicePluginEnv(bbdevPod, deviceEnvVar)
	Expect(err).NotTo(HaveOccurred(), "Failed to read FEC device plugin environment")

	ealParams := buildBbdevEALParams(pciAddress, vfioToken)
	testCommand := fmt.Sprintf("/usr/bbdev/test-bbdev.py"+
		" -e \"%s\"  -c validation"+
		" -p /usr/bbdev/dpdk-test-bbdev"+
		" -n 64 -b 8"+
		" -v /usr/bbdev/test_vectors/*", ealParams)

	bbdevTestsOutput, err := bbdevPod.ExecCommand([]string{"bash", "-c", testCommand})
	Expect(err).NotTo(HaveOccurred(), "Failed to execute test command")

	return bbdevTestsOutput.String()
}

func getFecDevicePluginEnv(bbdevPod *pod.Builder, deviceEnvVar string) (pciAddress, vfioToken string, err error) {
	pciAddress, err = podPrintenv(bbdevPod, deviceEnvVar)
	if err != nil {
		return "", "", fmt.Errorf("reading %s: %w", deviceEnvVar, err)
	}

	if pciAddress == "" {
		return "", "", fmt.Errorf("%s is not set in the pod", deviceEnvVar)
	}

	infoEnvVar := deviceEnvVar + "_INFO"
	infoJSON, err := podPrintenv(bbdevPod, infoEnvVar)
	if err != nil || infoJSON == "" {
		return pciAddress, "", nil
	}

	vfioToken, err = vfioTokenFromDeviceInfo(infoJSON, pciAddress)
	if err != nil {
		return pciAddress, "", fmt.Errorf("reading %s: %w", infoEnvVar, err)
	}

	return pciAddress, vfioToken, nil
}

func podPrintenv(bbdevPod *pod.Builder, name string) (string, error) {
	output, err := bbdevPod.ExecCommand([]string{"printenv", name})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output.String()), nil
}

func buildBbdevEALParams(pciAddress, vfioToken string) string {
	if vfioToken != "" {
		return fmt.Sprintf("-a %s --vfio-vf-token %s -d /usr/bbdev/", pciAddress, vfioToken)
	}

	return fmt.Sprintf("-a %s -d /usr/bbdev/", pciAddress)
}

func vfioTokenFromDeviceInfo(infoJSON, pciAddress string) (string, error) {
	result := map[string]tsparams.VFIOToken{}

	err := json.Unmarshal([]byte(infoJSON), &result)
	if err != nil {
		return "", err
	}

	tokenData, ok := result[pciAddress]
	if !ok || tokenData.Extra.VfioToken == "" {
		return "", fmt.Errorf("VFIO token not found for PCI %s", pciAddress)
	}

	return tokenData.Extra.VfioToken, nil
}

// countBbdevTestResults extracts comprehensive test statistics from bbdev output.
func countBbdevTestResults(output string) (totalSuites, totalPassed, totalFailed int) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Count all test suites that started (including those that were skipped)
		if strings.Contains(line, "Starting Test Suite :") {
			totalSuites++
		}

		// Count passed tests from summary lines
		if strings.Contains(line, "+ Tests Passed :") {
			parts := strings.Fields(line)
			if len(parts) >= 5 {
				if passed := parseInt(parts[4]); passed > 0 {
					totalPassed += passed
				}
			}
		}

		// Count failed tests from summary lines
		if strings.Contains(line, "+ Tests Failed :") {
			parts := strings.Fields(line)
			if len(parts) >= 5 {
				if failed := parseInt(parts[4]); failed > 0 {
					totalFailed += failed
				}
			}
		}
	}

	return totalSuites, totalPassed, totalFailed
}

// parseInt safely converts string to int, returns 0 if conversion fails.
func parseInt(s string) int {
	var result int

	for _, r := range s {
		if r >= '0' && r <= '9' {
			result = result*10 + int(r-'0')
		} else {
			break
		}
	}

	return result
}
