package rdscorecommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	// Names of deployments.
	jumboFrameOneDeployName = "jumbo-frame-one"
	jumboFrameTwoDeployName = "jumbo-frame-two"
	// ConfigMap names.
	jumboFrameCMName = "jumbo-frame-config"
	// ServiceAccount names.
	jumboFrameSAName = "jumbo-frame-sa"
	// Container names within deployments.
	jumboFrameContainerName = "jumbo-frame-test"
	// Labels for deployments.
	jumboFrameOneLabel = "rds-core=jumbo-frame-one"
	jumboFrameTwoLabel = "rds-core=jumbo-frame-two"
	// RBAC names for the deployments.
	jumboFrameRBACName = "privileged-jumbo-frame"
	// ClusterRole to use with RBAC.
	jumboFrameRBACRole = "system:openshift:scc:privileged"
	// MTU values.
	mtuJumboFrame = 9000
	mtuStandard   = 1500
	// Ping payload sizes: MTU minus IPv4 (20) and ICMP (8) headers.
	pingSize9000 = 8972
	pingSize1500 = 1472
)

// VerifyJumboFrameOnSecondarySRIOVKernelMode verifies jumbo frame (9000 MTU) functionality
// over secondary SR-IOV kernel-mode interfaces and validates no negative impact on RAN KPIs.
//
// These SriovNetworks have empty IPAM. IPs are assigned in-container the same way as the
// other SR-IOV kernel tests (config.sh + VLAN), then MTU is set on net1 and the VLAN iface.
//
// CNF-24979: Add jumbo frame verification over secondary sriov (kernel mode) interfaces.
//
//nolint:funlen
func VerifyJumboFrameOnSecondarySRIOVKernelMode(ctx SpecContext) {
	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNetOne) == "" ||
		strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNetTwo) == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks cannot be empty")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNetOne)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNetTwo)

		Skip("SR-IOV networks cannot be empty")
	}

	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNetOne) == strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNetTwo) {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks are the same but should be different")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNetOne)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNetTwo)

		Skip("SR-IOV networks are the same but should be different")
	}

	if len(RDSCoreConfig.WlkdSRIOVDeployOneCmd) == 0 || len(RDSCoreConfig.WlkdSRIOVDeployTwoCmd) == 0 {
		Skip("SR-IOV deploy commands not configured")
	}

	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVDeployOneTargetAddress) == "" {
		Skip("SR-IOV deploy one target address not configured")
	}

	By("Retrieving SR-IOV Operator config")

	sriovOperatorConfig, oerr := getSRIOVOperatorConfig()

	Expect(oerr).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV Operator Config")

	By("Checking resourceInjectorMatchCondition is set")

	optionSet, ok := getSRIOVConfigOption(sriovOperatorConfig, "resourceInjectorMatchCondition")

	if !ok || !optionSet {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'resourceInjectorMatchCondition' not defined or disabled")

		Skip("Option 'resourceInjectorMatchCondition' not defined or enabled")
	}

	DeferCleanup(func() {
		By("Cleaning up jumbo frame test resources")

		cleanupJumboFrameResources()
	})

	By("Checking SR-IOV deployments don't exist")

	deleteDeployments(jumboFrameOneDeployName, RDSCoreConfig.WlkdSRIOVOneNS)
	deleteDeployments(jumboFrameTwoDeployName, RDSCoreConfig.WlkdSRIOVOneNS)

	assertPodsAreGone(RDSCoreConfig.WlkdSRIOVOneNS, jumboFrameOneLabel)
	assertPodsAreGone(RDSCoreConfig.WlkdSRIOVOneNS, jumboFrameTwoLabel)

	By("Removing ConfigMap")

	deleteConfigMap(jumboFrameCMName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Creating ConfigMap")

	createConfigMap(jumboFrameCMName,
		RDSCoreConfig.WlkdSRIOVOneNS, RDSCoreConfig.WlkdSRIOVConfigMapDataOne)

	By("Removing ServiceAccount")

	deleteServiceAccount(jumboFrameSAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Creating ServiceAccount")

	createServiceAccount(jumboFrameSAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Removing Cluster RBAC")

	deleteClusterRBAC(jumboFrameRBACName)

	By("Creating Cluster RBAC")

	createClusterRBAC(jumboFrameRBACName, jumboFrameRBACRole,
		jumboFrameSAName, RDSCoreConfig.WlkdSRIOVOneNS)

	pods := deployJumboFrameWorkloads(ctx)
	targetIP := strings.Fields(RDSCoreConfig.WlkdSRIOVDeployOneTargetAddress)[0]

	By("Verifying connectivity with standard MTU (1500)")

	setJumboFrameMTU(ctx, pods, mtuStandard)
	verifyJumboFrameConnectivity(ctx, pods[0], targetIP, pingSize1500)

	By("Measuring baseline CPU utilization with MTU 1500")

	baselineCPU, err := measureCPUUtilization(2 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), "Failed to measure baseline CPU")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Baseline CPU utilization: %.2f cores", baselineCPU)

	By("Verifying connectivity with jumbo frames (MTU 9000)")

	setJumboFrameMTU(ctx, pods, mtuJumboFrame)
	verifyJumboFrameConnectivity(ctx, pods[0], targetIP, pingSize9000)

	By("Measuring CPU utilization with MTU 9000")

	testCPU, err := measureCPUUtilization(2 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), "Failed to measure test CPU")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Test CPU utilization with jumbo frames: %.2f cores", testCPU)

	By("Comparing baseline and test metrics")

	err = compareMetrics(baselineCPU, testCPU)

	Expect(err).ToNot(HaveOccurred(), "Performance degradation detected with jumbo frames")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Jumbo frame verification completed successfully - Baseline: %.2f cores, Test: %.2f cores",
		baselineCPU, testCPU)
}

func deployJumboFrameWorkloads(ctx SpecContext) []*pod.Builder {
	By("Defining container configuration")

	containerOne := defineContainer(jumboFrameContainerName, RDSCoreConfig.WlkdSRIOVDeployOneImage,
		RDSCoreConfig.WlkdSRIOVDeployOneCmd, RDSCoreConfig.WldkSRIOVDeployOneResRequests,
		RDSCoreConfig.WldkSRIOVDeployOneResLimits)

	containerTwo := defineContainer(jumboFrameContainerName, RDSCoreConfig.WlkdSRIOVDeployTwoImage,
		RDSCoreConfig.WlkdSRIOVDeployTwoCmd, RDSCoreConfig.WldkSRIOVDeployTwoResRequests,
		RDSCoreConfig.WldkSRIOVDeployTwoResLimits)

	By("Obtaining container definition")

	cfgOne, err := containerOne.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	cfgTwo, err := containerTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining 1st deployment configuration")

	labelsOne := map[string]string{
		strings.Split(jumboFrameOneLabel, "=")[0]: strings.Split(jumboFrameOneLabel, "=")[1]}

	deployOne := defineDeployment(cfgOne,
		jumboFrameOneDeployName,
		RDSCoreConfig.WlkdSRIOVOneNS,
		RDSCoreConfig.WlkdSRIOVNetOne,
		jumboFrameCMName,
		jumboFrameSAName,
		labelsOne,
		RDSCoreConfig.WlkdSRIOVDeployOneSelector)

	By("Creating deployment one")

	_, err = deployOne.CreateAndWaitUntilReady(5 * time.Minute)
	if err != nil {
		dumpJumboFrameDeployFailure(ctx, RDSCoreConfig.WlkdSRIOVOneNS, jumboFrameOneLabel, err)
	}

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", jumboFrameOneDeployName, err))

	By("Defining 2nd deployment")

	labelsTwo := map[string]string{
		strings.Split(jumboFrameTwoLabel, "=")[0]: strings.Split(jumboFrameTwoLabel, "=")[1]}

	deployTwo := defineDeployment(cfgTwo,
		jumboFrameTwoDeployName,
		RDSCoreConfig.WlkdSRIOVOneNS,
		RDSCoreConfig.WlkdSRIOVNetTwo,
		jumboFrameCMName,
		jumboFrameSAName,
		labelsTwo,
		RDSCoreConfig.WlkdSRIOVDeployTwoSelector)

	By("Creating 2nd deployment")

	_, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	if err != nil {
		dumpJumboFrameDeployFailure(ctx, RDSCoreConfig.WlkdSRIOVOneNS, jumboFrameTwoLabel, err)
	}

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", jumboFrameTwoDeployName, err))

	return []*pod.Builder{
		findPodWithSelector(RDSCoreConfig.WlkdSRIOVOneNS, jumboFrameOneLabel)[0],
		findPodWithSelector(RDSCoreConfig.WlkdSRIOVOneNS, jumboFrameTwoLabel)[0],
	}
}

func setJumboFrameMTU(ctx context.Context, pods []*pod.Builder, mtu int) {
	vlanIface := jumboVLANIface()
	cmd := []string{"/bin/bash", "-c",
		fmt.Sprintf("ip link set net1 mtu %d && ip link set %s mtu %d", mtu, vlanIface, mtu)}

	for _, testPod := range pods {
		Eventually(func() error {
			_, execErr := testPod.ExecCommand(cmd, testPod.Definition.Spec.Containers[0].Name)
			if execErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to set MTU %d on %s (%s): %v", mtu, testPod.Definition.Name, vlanIface, execErr)
			}

			return execErr
		}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(2*time.Minute).Should(Succeed(),
			"failed to set MTU %d on pod %s iface %s", mtu, testPod.Definition.Name, vlanIface)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Set MTU %d on %s (net1, %s)", mtu, testPod.Definition.Name, vlanIface)
	}
}

func verifyJumboFrameConnectivity(ctx context.Context, sourcePod *pod.Builder, targetIP string, packetSize int) {
	By(fmt.Sprintf("Pinging %s with packet size %d (DF)", targetIP, packetSize))

	pingCmd := []string{"/bin/bash", "-c",
		fmt.Sprintf("ping -M do -s %d -c 5 -W 2 %s", packetSize, targetIP)}

	Eventually(func() error {
		output, pingErr := sourcePod.ExecCommand(pingCmd, sourcePod.Definition.Spec.Containers[0].Name)
		if pingErr != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Ping %s from %s failed: %v\n%s", targetIP, sourcePod.Definition.Name, pingErr, output.String())

			return pingErr
		}

		out := output.String()
		if !strings.Contains(out, "0% packet loss") && !strings.Contains(out, " 0 packets lost") {
			return fmt.Errorf("packet loss detected: %s", out)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Ping successful from %s to %s size %d:\n%s",
			sourcePod.Definition.Name, targetIP, packetSize, out)

		return nil
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(2*time.Minute).Should(Succeed(),
		"jumbo frame ping to %s size %d failed", targetIP, packetSize)
}

func jumboVLANIface() string {
	cmd := RDSCoreConfig.WlkdSRIOVDeployOneCmd
	parts := strings.Fields(cmd[len(cmd)-1])

	if len(parts) >= 2 {
		return "net1." + parts[1]
	}

	return "net1.3814"
}

func measureCPUUtilization(duration time.Duration) (float64, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Measuring CPU utilization for %v", duration)

	workerNodes, err := nodes.List(APIClient,
		metav1.ListOptions{LabelSelector: "node-role.kubernetes.io/worker="})
	if err != nil || len(workerNodes) == 0 {
		return 0.0, fmt.Errorf("failed to list worker nodes: %w", err)
	}

	var totalCPU float64

	nodeCount := 0

	for _, node := range workerNodes {
		nodeName := node.Object.Name
		query := fmt.Sprintf(rdscoreparams.NodeCPUTotalQuery, nodeName, duration.String())

		metrics, queryErr := ExecPromQuery(APIClient, query)
		if queryErr != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to query CPU for node %s: %v", nodeName, queryErr)

			continue
		}

		if len(metrics) > 0 && len(metrics[0].Value) > 1 {
			cpuCores, parseErr := ParseCPUValue(metrics[0].Value[1])
			if parseErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to parse CPU value for node %s: %v", nodeName, parseErr)

				continue
			}

			totalCPU += cpuCores
			nodeCount++

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Node %s CPU utilization: %.2f cores", nodeName, cpuCores)
		}
	}

	if nodeCount == 0 {
		return 0.0, fmt.Errorf("no CPU metrics collected from any worker node")
	}

	avgCPU := totalCPU / float64(nodeCount)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Average CPU utilization across %d nodes: %.2f cores", nodeCount, avgCPU)

	return avgCPU, nil
}

func compareMetrics(baselineCPU, testCPU float64) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Comparing metrics - Baseline CPU: %.2f cores, Test CPU: %.2f cores",
		baselineCPU, testCPU)

	thresholdPercent := 10.0
	cpuIncrease := testCPU - baselineCPU
	increasePercent := (cpuIncrease / baselineCPU) * 100.0

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"CPU increase: %.2f cores (%.2f%% relative increase, threshold: %.2f%%)",
		cpuIncrease, increasePercent, thresholdPercent)

	if increasePercent > thresholdPercent {
		return fmt.Errorf(
			"CPU utilization increased beyond acceptable threshold: baseline=%.2f cores, test=%.2f cores, "+
				"increase=%.2f cores (%.2f%%), threshold=%.2f%%",
			baselineCPU, testCPU, cpuIncrease, increasePercent, thresholdPercent)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Performance validation passed - CPU increase %.2f%% is within threshold %.2f%%",
		increasePercent, thresholdPercent)

	return nil
}

func dumpJumboFrameDeployFailure(ctx SpecContext, namespace, podLabel string, err error) {
	DumpDeploymentStatus(ctx, namespace)

	pods, listErr := pod.List(APIClient, namespace, metav1.ListOptions{LabelSelector: podLabel})
	if listErr != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods for %s: %v", podLabel, listErr)

		return
	}

	for _, testPod := range pods {
		DumpPodStatusOnFailure(testPod, err)
	}
}

func cleanupJumboFrameResources() {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cleaning up jumbo frame test resources")

	deleteDeployments(jumboFrameOneDeployName, RDSCoreConfig.WlkdSRIOVOneNS)
	deleteDeployments(jumboFrameTwoDeployName, RDSCoreConfig.WlkdSRIOVOneNS)
	deleteConfigMap(jumboFrameCMName, RDSCoreConfig.WlkdSRIOVOneNS)
	deleteClusterRBAC(jumboFrameRBACName)
	deleteServiceAccount(jumboFrameSAName, RDSCoreConfig.WlkdSRIOVOneNS)
}
