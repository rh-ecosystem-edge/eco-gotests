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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	sriovfec "github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov-fec"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/accelerator/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
)

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
		err = netenv.DeployPerformanceProfile(
			APIClient,
			NetConfig,
			"performance-profile-dpdk",
			"1,3,5,7,9,11,13,15,17,19,21,23,25",
			"0,2,4,6,8,10,12,14,16,18,20",
			24)
		Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")
	})

	Context("ACC100", func() {
		var (
			sfnc        *sriovfec.NodeConfigBuilder
			accelerator *fectypes.SriovAccelerator
			err         error
		)

		BeforeAll(func() {
			By("Checking if the cluster has ACC100 cards")
			sfnc, accelerator, err = getFecNodeConfigWithAccCard(tsparams.Acc100DeviceID)
			if err != nil {
				Skip(fmt.Sprintf("Cluster does not have ACC100 cards: %s", err.Error()))
			}

			By("Deleting SriovFecClusterConfig if any present in the cluster")
			sfccList, err := sriovfec.ListClusterConfig(APIClient, tsparams.OperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list SriovFecClusterConfig")
			if len(sfccList) != 0 {
				for _, sfcc := range sfccList {
					_, err := sfcc.Delete()
					Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovFecClusterConfig")
				}
			}

			By("Creating SriovFecClusterConfig")
			_, err = defineFecClusterConfig(accelerator.PCIAddress, sfnc.Object.Name, secureBoot).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovFecClusterConfig")

			err = waitForNodeConfigToSucceed()
			Expect(err).ToNot(HaveOccurred(), "SriovFecNodeConfig never succeeded")
		})

		AfterAll(func() {
			By("Deleting SriovFecClusterConfig in the cluster")
			sfccList, err := sriovfec.ListClusterConfig(APIClient, tsparams.OperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list SriovFecClusterConfig")
			if len(sfccList) != 0 {
				for _, sfcc := range sfccList {
					_, err := sfcc.Delete()
					Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovFecClusterConfig")
				}
			}
		})

		It("node should show acc100 resource", reportxml.ID("41073"), func() {
			Eventually(getNodeResource, 10*time.Minute, time.Second).
				WithArguments(sfnc.Object.Name, tsparams.Acc100ResourceName).To(BeNumerically(">", 0))
		})

		It("validation of acc100 resource", reportxml.ID("41216"), func() {
			Eventually(getNodeResource, 10*time.Minute, time.Second).
				WithArguments(sfnc.Object.Name, tsparams.Acc100ResourceName).To(BeNumerically(">", 0))

			By("Creating bbdev test pod")
			bbdevContainer, err := pod.NewContainerBuilder(
				"bbdev", NetConfig.CnfNetTestContainer, []string{"bash", "-c", "sleep infinity"}).
				WithSecurityContext(&corev1.SecurityContext{
					Privileged:   ptr.To(false),
					RunAsUser:    ptr.To(int64(0)),
					Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE"}},
				}).
				WithCustomResourcesLimits(corev1.ResourceList{
					"hugepages-1Gi":             resource.MustParse("1Gi"),
					"memory":                    resource.MustParse("1Gi"),
					"cpu":                       *resource.NewQuantity(4, resource.DecimalSI),
					tsparams.Acc100ResourceName: resource.MustParse("1"),
				}).
				WithCustomResourcesRequests(corev1.ResourceList{
					"hugepages-1Gi":             resource.MustParse("1Gi"),
					"memory":                    resource.MustParse("1Gi"),
					"cpu":                       *resource.NewQuantity(4, resource.DecimalSI),
					tsparams.Acc100ResourceName: resource.MustParse("1"),
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
			bbdevTestsOutput := runBbdevTests(bbdevPod, secureBoot, tsparams.Acc100EnvVar)
			fmt.Println(bbdevTestsOutput)
			Expect(bbdevTestsOutput).ToNot(BeEmpty(), "Failed to run bbdev tests")

			By("Checking if bbdev tests executed")

			totalSuites, totalPassed, totalFailed := countBbdevTestResults(bbdevTestsOutput)

			By(fmt.Sprintf("BBdev test results: %d suites ran, %d tests passed, %d tests failed",
				totalSuites, totalPassed, totalFailed))

			Expect(totalSuites).To(Equal(tsparams.TotalNumberBbdevTests), "Not all test suites were executed")
			Expect(totalPassed).To(Equal(tsparams.ExpectedNumberBbdevTestsPassedForAcc100), "Not all expected tests passed")
			Expect(totalFailed).To(Equal(0), "Some tests failed")
		})
	})
})

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

func defineFecClusterConfig(pciAddress, nodeName string, secureBoot bool) *sriovfec.ClusterConfigBuilder {
	queueGroupConfig := fectypes.QueueGroupConfig{
		AqDepthLog2:     4,
		NumAqsPerGroups: 16,
		NumQueueGroups:  2,
	}

	pfDriverType := "pci-pf-stub"

	if secureBoot {
		pfDriverType = "vfio-pci"
	}

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
			PFDriver: pfDriverType,
			VFAmount: 2,
			VFDriver: "vfio-pci",
			BBDevConfig: fectypes.BBDevConfig{
				ACC100: &fectypes.ACC100BBDevConfig{
					Downlink4G:   queueGroupConfig,
					Downlink5G:   queueGroupConfig,
					Uplink4G:     queueGroupConfig,
					Uplink5G:     queueGroupConfig,
					PFMode:       false,
					MaxQueueSize: 1024,
					NumVfBundles: 2,
				},
			},
		},
	}

	return sfccBuilder
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
func runBbdevTests(bbdevPod *pod.Builder, isSecureEnabled bool, resName string) string {
	printEnvCommand := []string{"bash", "-c", "printenv | grep INTEL"}

	if isSecureEnabled {
		printEnvCommand = []string{"bash", "-c", "printenv | grep INTEL | grep -v INFO"}
	}

	pcideviceIntelComIntelFec5GBuff, err := bbdevPod.ExecCommand(printEnvCommand)
	Expect(err).NotTo(HaveOccurred(), "Failed to execute printenv command")

	pcideviceIntelComIntelFec5GString := strings.TrimSpace(
		strings.Split(pcideviceIntelComIntelFec5GBuff.String(), "=")[1])

	testCommand := fmt.Sprintf("/usr/bbdev/test-bbdev.py"+
		" -e \"-a $%v -d /usr/bbdev/\"  -c validation"+
		" -p /usr/bbdev/dpdk-test-bbdev"+
		" -n 64 -b 8"+
		" -v /usr/bbdev/test_vectors/*", resName)

	if isSecureEnabled {
		token, err := getVFIOToken(bbdevPod, pcideviceIntelComIntelFec5GString)
		Expect(err).NotTo(HaveOccurred(), "Failed to get vfio-token")

		testCommand = fmt.Sprintf("/usr/bbdev/test-bbdev.py"+
			" -e \"-w %v --vfio-vf-token %s -d /usr/bbdev/\"  -c validation"+
			" -p /usr/bbdev/dpdk-test-bbdev"+
			" -n 64 -b 8 "+
			" -v /usr/bbdev/test_vectors/*", resName, token)
	}

	bbdevTestsOutput, err := bbdevPod.ExecCommand([]string{"bash", "-c", testCommand})
	Expect(err).NotTo(HaveOccurred(), "Failed to execute test command")

	return bbdevTestsOutput.String()
}

// getVFIOToken returns the vfio-token.
func getVFIOToken(bbdevPod *pod.Builder, pci string) (string, error) {
	vfioTokenJSONBuff, err := bbdevPod.ExecCommand([]string{"bash", "-c", "printenv INFO"})

	if err != nil {
		return "", err
	}

	vfioTokenJSONString := strings.TrimSpace(strings.Split(vfioTokenJSONBuff.String(), "=")[1])
	result := map[string]tsparams.VFIOToken{}

	err = json.Unmarshal([]byte(vfioTokenJSONString), &result)

	if err != nil {
		return "", err
	}

	tokenData, ok := result[pci]
	if !ok || tokenData.Extra.VfioToken == "" {
		return "", fmt.Errorf("VFIO token not found for PCI %s", pci)
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
