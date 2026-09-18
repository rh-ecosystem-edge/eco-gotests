package rdscorecommon

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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
	// RAN KPI sample window for CPU rate, oslat, and cyclictest.
	jumboKPIDuration = 2 * time.Minute
	// Allowed relative increase from MTU 1500 to MTU 9000.
	jumboKPIThresholdPercent = 10.0
)

// jumboKPIMetrics is the RAN KPI snapshot collected at one MTU.
type jumboKPIMetrics struct {
	CPU       float64
	OslatMax  float64
	CyclicMax float64
}

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

	By("Collecting RAN KPIs at MTU 1500")

	setJumboFrameMTU(ctx, pods, mtuStandard)
	verifyJumboFrameConnectivity(ctx, pods[0], targetIP, pingSize1500)

	baselineKPI := collectJumboKPIMetrics(pods[0], jumboKPIDuration)

	By("Collecting RAN KPIs at MTU 9000")

	setJumboFrameMTU(ctx, pods, mtuJumboFrame)
	verifyJumboFrameConnectivity(ctx, pods[0], targetIP, pingSize9000)

	jumboKPI := collectJumboKPIMetrics(pods[0], jumboKPIDuration)

	By("Comparing RAN KPIs (CPU, oslat, cyclictest) between MTU 1500 and 9000")

	err := compareJumboKPIMetrics(baselineKPI, jumboKPI)

	Expect(err).ToNot(HaveOccurred(), "Performance degradation detected with jumbo frames")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Jumbo frame verification completed successfully - "+
			"baseline CPU=%.2f oslat=%.0fus cyclic=%.0fus, jumbo CPU=%.2f oslat=%.0fus cyclic=%.0fus",
		baselineKPI.CPU, baselineKPI.OslatMax, baselineKPI.CyclicMax,
		jumboKPI.CPU, jumboKPI.OslatMax, jumboKPI.CyclicMax)
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

func collectJumboKPIMetrics(sourcePod *pod.Builder, duration time.Duration) jumboKPIMetrics {
	By("Measuring CPU utilization")

	cpuCores, err := measureCPUUtilization(duration)

	Expect(err).ToNot(HaveOccurred(), "Failed to measure CPU utilization")

	By("Running oslat")

	oslatMax := runJumboLatencyTool(sourcePod, jumboOslatCmd(duration), duration)

	By("Running cyclictest")

	cyclicMax := runJumboLatencyTool(sourcePod, jumboCyclictestCmd(duration), duration)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Collected RAN KPIs CPU=%.2f oslat_max=%.0fus cyclictest_max=%.0fus",
		cpuCores, oslatMax, cyclicMax)

	return jumboKPIMetrics{CPU: cpuCores, OslatMax: oslatMax, CyclicMax: cyclicMax}
}

func jumboOslatCmd(duration time.Duration) []string {
	return []string{"oslat", "-D", fmt.Sprintf("%ds", int(duration.Seconds()))}
}

func jumboCyclictestCmd(duration time.Duration) []string {
	return []string{"cyclictest", "-q", "-m", "-p", "95", "-t", "1",
		"-D", fmt.Sprintf("%d", int(duration.Seconds()))}
}

func runJumboLatencyTool(sourcePod *pod.Builder, command []string, duration time.Duration) float64 {
	output, err := sourcePod.ExecCommandWithTimeout(
		command, duration+30*time.Second, sourcePod.Definition.Spec.Containers[0].Name)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to run %s in pod %s: %v\n%s",
			command[0], sourcePod.Definition.Name, err, output.String()))

	maxUs, parseErr := parseMaxLatencyUs(output.String())

	Expect(parseErr).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse max latency from %s output:\n%s", command[0], output.String()))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("%s max latency: %.0f us\n%s",
		command[0], maxUs, output.String())

	return maxUs
}

func parseMaxLatencyUs(output string) (float64, error) {
	maxRe := regexp.MustCompile(`(?i)max(?:imum)?[:\s]+(\d+)`)
	matches := maxRe.FindAllStringSubmatch(output, -1)

	if len(matches) == 0 {
		return 0, fmt.Errorf("no max latency value in output")
	}

	maxVal := 0.0

	for _, match := range matches {
		value, convErr := strconv.ParseFloat(match[1], 64)
		if convErr != nil {
			continue
		}

		if value > maxVal {
			maxVal = value
		}
	}

	return maxVal, nil
}

func compareJumboKPIMetrics(baseline, jumbo jumboKPIMetrics) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Comparing RAN KPIs - baseline CPU=%.2f oslat=%.0fus cyclic=%.0fus, "+
			"jumbo CPU=%.2f oslat=%.0fus cyclic=%.0fus (threshold %.1f%%)",
		baseline.CPU, baseline.OslatMax, baseline.CyclicMax,
		jumbo.CPU, jumbo.OslatMax, jumbo.CyclicMax, jumboKPIThresholdPercent)

	var failures []string

	failures = appendKPIFailure(failures, "CPU", baseline.CPU, jumbo.CPU)
	failures = appendKPIFailure(failures, "oslat", baseline.OslatMax, jumbo.OslatMax)
	failures = appendKPIFailure(failures, "cyclictest", baseline.CyclicMax, jumbo.CyclicMax)

	if len(failures) > 0 {
		return fmt.Errorf("RAN KPI degradation: %s", strings.Join(failures, "; "))
	}

	return nil
}

func appendKPIFailure(failures []string, name string, baseline, jumbo float64) []string {
	increasePercent := kpiIncreasePercent(baseline, jumbo)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"%s: baseline=%.2f jumbo=%.2f increase=%.2f%%", name, baseline, jumbo, increasePercent)

	if increasePercent > jumboKPIThresholdPercent {
		return append(failures, fmt.Sprintf(
			"%s increased beyond threshold: baseline=%.2f jumbo=%.2f increase=%.2f%% threshold=%.1f%%",
			name, baseline, jumbo, increasePercent, jumboKPIThresholdPercent))
	}

	return failures
}

func kpiIncreasePercent(baseline, jumbo float64) float64 {
	if baseline == 0 {
		if jumbo == 0 {
			return 0
		}

		return 100
	}

	return ((jumbo - baseline) / baseline) * 100
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
