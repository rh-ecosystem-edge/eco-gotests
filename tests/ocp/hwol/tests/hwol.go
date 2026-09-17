package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/hwolenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

type networkStatusEntry struct {
	Name       string   `json:"name"`
	IPs        []string `json:"ips"`
	DeviceInfo *struct {
		Type string `json:"type"`
		Pci  *struct {
			PciAddress string `json:"pci-address"`
		} `json:"pci"`
	} `json:"device-info,omitempty"`
}

var _ = Describe(
	"HWOL",
	Ordered,
	ContinueOnFailure,
	func() {
		var (
			operatorNS string
			mcpLabel   string
			pfName     string
			vfNum      int
			workerNode string
			ovsBridge  string
		)

		BeforeAll(func() {
			operatorNS = HwolOcpConfig.OcpHwolOperatorNamespace
			mcpLabel = HwolOcpConfig.MCPLabel

			By("Loading HWOL device configuration from ECO_OCP_HWOL_DEVICES")

			devices, err := HwolOcpConfig.GetHwolDevices()
			Expect(err).ToNot(HaveOccurred(), "Failed to get HWOL devices")
			Expect(devices).ToNot(BeEmpty(), "No HWOL devices configured")
			pfName = devices[0].InterfaceName

			vfNum, err = HwolOcpConfig.GetVFNum()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VF count")

			By("Ensuring switchdev foundation (operator config, pool, policy, wait)")

			Expect(hwolenv.EnsureSwitchdevFoundation(
				operatorNS, mcpLabel, devices[0], vfNum,
			)).To(Succeed(), "Failed to ensure switchdev foundation")

			mcpNodes, err := hwolenv.ListMCPWorkerNodes(mcpLabel)
			Expect(err).ToNot(HaveOccurred(), "Failed to list MCP nodes")
			Expect(mcpNodes).ToNot(BeEmpty(), "No MCP-labeled workers")
			workerNode = mcpNodes[0].Object.Name

			ovsBridge, err = hwolenv.LookupManagedOVSBridge(operatorNS, mcpLabel, pfName)
			Expect(err).ToNot(HaveOccurred(), "Failed to look up managed OVS bridge")
			Expect(ovsBridge).ToNot(BeEmpty(), "Managed OVS bridge name is empty")
		})

		AfterAll(func() {
			By("Cleaning HWOL resources")

			err := hwolenv.CleanupHwolResources(
				operatorNS,
				mcpLabel,
				pfName,
				tsparams.CleanupWaitTimeout,
				tsparams.DefaultStableDuration,
			)
			if err != nil {
				AddReportEntry("hwol-cleanup-failure", err.Error())
			}

			Expect(err).ToNot(HaveOccurred(), "Failed to clean HWOL resources")
		})

		It("verifies switchdev mode and managed OVS bridge on MCP nodes",
			Label(tsparams.LabelSwitchdev),
			reportxml.ID("85001"),
			func() {
				By("Asserting eSwitchMode=switchdev and OVS bridge uplink for PF")

				Expect(hwolenv.AssertSwitchdevAndOVSBridge(operatorNS, mcpLabel, pfName)).To(Succeed())
			})

		DescribeTable("attach path",
			func(cniType sriovoperator.CNIType, networkName string) {
				By(fmt.Sprintf("Creating %s network %s", cniType, networkName))

				err := createAttachNetwork(cniType, networkName, operatorNS, ovsBridge)
				Expect(err).ToNot(HaveOccurred(), "Failed to create attach network")

				DeferCleanup(func() {
					By(fmt.Sprintf("Deleting %s network %s", cniType, networkName))

					Expect(deleteAttachNetwork(cniType, networkName, operatorNS)).To(Succeed(),
						"Failed to delete attach network")
					Expect(sriovoperator.WaitForNADDeletion(
						APIClient, networkName, tsparams.TestNamespaceName, tsparams.DefaultTimeout,
					)).To(Succeed(), "NAD was not deleted")
				})

				By("Waiting for NAD creation")

				Expect(sriovoperator.WaitForNADCreation(
					APIClient, networkName, tsparams.TestNamespaceName, tsparams.DefaultTimeout,
				)).To(Succeed(), "NAD was not created")

				By("Asserting NAD CNI type and resource annotation")

				wantBridge := ""
				if cniType == sriovoperator.CNITypeOVS {
					wantBridge = ovsBridge
				}

				Expect(sriovoperator.AssertNAD(
					APIClient,
					networkName,
					tsparams.TestNamespaceName,
					cniType,
					tsparams.ResourceNamePrefix+tsparams.ResourceName,
					wantBridge,
				)).To(Succeed(), "NAD assertion failed")

				By("Creating sleep pod with secondary network")

				attachPod, err := createTrafficPod(
					fmt.Sprintf("hwol-attach-%s", cniType), networkName, "", nil)
				Expect(err).ToNot(HaveOccurred(), "Failed to create attach pod")

				DeferCleanup(func() {
					By("Deleting attach pod")

					_, delErr := attachPod.Delete()
					Expect(delErr).ToNot(HaveOccurred(), "Failed to delete attach pod")
				})

				By("Asserting pod network-status includes the attach NAD with an IP")

				_, err = waitForNetworkStatusIP(attachPod, networkName)
				Expect(err).ToNot(HaveOccurred(),
					"network-status should reference attach NAD with an assigned IP")
			},
			Entry("ovs CNI", Label(tsparams.LabelOvsNetwork), reportxml.ID("85002"),
				sriovoperator.CNITypeOVS, tsparams.OvsNetworkName),
			Entry("sriov CNI", Label(tsparams.LabelSriovNetwork), reportxml.ID("85003"),
				sriovoperator.CNITypeSriov, tsparams.SriovNetworkName),
		)

		DescribeTable("offload path",
			func(cniType sriovoperator.CNIType, networkName string) {
				Expect(vfNum).To(BeNumerically(">=", tsparams.MinVFNumForOffload),
					"offload needs ≥%d VFs (VF0 reserved + server/client); set ECO_OCP_HWOL_VF_NUM",
					tsparams.MinVFNumForOffload)

				By(fmt.Sprintf("Creating %s network %s for offload", cniType, networkName))

				err := createAttachNetwork(cniType, networkName, operatorNS, ovsBridge)
				Expect(err).ToNot(HaveOccurred(), "Failed to create offload network")

				DeferCleanup(func() {
					By(fmt.Sprintf("Deleting %s network %s", cniType, networkName))

					Expect(deleteAttachNetwork(cniType, networkName, operatorNS)).To(Succeed(),
						"Failed to delete offload network")
					Expect(sriovoperator.WaitForNADDeletion(
						APIClient, networkName, tsparams.TestNamespaceName, tsparams.DefaultTimeout,
					)).To(Succeed(), "NAD was not deleted")
				})

				By("Waiting for NAD creation")

				Expect(sriovoperator.WaitForNADCreation(
					APIClient, networkName, tsparams.TestNamespaceName, tsparams.DefaultTimeout,
				)).To(Succeed(), "NAD was not created")

				By("Asserting NAD CNI type and resource annotation")

				wantBridge := ""
				if cniType == sriovoperator.CNITypeOVS {
					wantBridge = ovsBridge
				}

				Expect(sriovoperator.AssertNAD(
					APIClient,
					networkName,
					tsparams.TestNamespaceName,
					cniType,
					tsparams.ResourceNamePrefix+tsparams.ResourceName,
					wantBridge,
				)).To(Succeed(), "NAD assertion failed")

				By(fmt.Sprintf("Creating iperf server/client pods on node %s", workerNode))

				// Prior table entry may still hold VFs until the device plugin frees them.
				Expect(waitForHwolResource(workerNode, 2, tsparams.DefaultTimeout)).To(Succeed(),
					"need ≥2 openshift.io/hwolresource before creating iperf pods")

				// Run iperf3 as the server container PID 1. Starting it via kubectl exec
				// (even with -D / background) does not leave a listener in this image.
				serverPod, err := createTrafficPod(
					fmt.Sprintf("hwol-iperf-srv-%s", cniType), networkName, workerNode,
					iperfServerCMD())
				Expect(err).ToNot(HaveOccurred(), "Failed to create iperf server pod")

				DeferCleanup(func() {
					_, delErr := serverPod.Delete()
					Expect(delErr).ToNot(HaveOccurred(), "Failed to delete iperf server pod")
				})

				Expect(waitForHwolResource(workerNode, 1, tsparams.DefaultTimeout)).To(Succeed(),
					"need ≥1 openshift.io/hwolresource for iperf client")

				clientPod, err := createTrafficPod(
					fmt.Sprintf("hwol-iperf-cli-%s", cniType), networkName, workerNode, nil)
				Expect(err).ToNot(HaveOccurred(), "Failed to create iperf client pod")

				DeferCleanup(func() {
					_, delErr := clientPod.Delete()
					Expect(delErr).ToNot(HaveOccurred(), "Failed to delete iperf client pod")
				})

				serverIP, serverPCI, err := waitForNetworkStatusIPAndPCI(serverPod, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get server secondary IP/PCI")

				clientIP, clientPCI, err := waitForNetworkStatusIPAndPCI(clientPod, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get client secondary IP/PCI")

				if cniType == sriovoperator.CNITypeOVS {
					By("Asserting VF representors are ports on the managed OVS bridge")

					Expect(hwolenv.AssertVFRepresentorsOnBridge(
						workerNode, ovsBridge, HwolOcpConfig.OcpHwolTestContainer,
						serverPCI, clientPCI,
					)).To(Succeed(), "expected server and client VF representors on bridge")
				}

				By("Running iperf3 between pods on secondary network")

				Expect(runIperfBetweenPods(
					clientPod, serverIP, clientIP, tsparams.IperfDuration,
				)).To(Succeed(), "iperf3 traffic failed")

				By("Asserting OVS datapath has type=offloaded flows")

				Expect(hwolenv.AssertOvsOffloadedFlows(
					workerNode, HwolOcpConfig.OcpHwolTestContainer,
				)).To(Succeed(), "expected non-empty offloaded flows after iperf")
			},
			Entry("ovs CNI",
				Label(tsparams.LabelOffload), Label(tsparams.LabelOvsNetwork), reportxml.ID("85004"),
				sriovoperator.CNITypeOVS, tsparams.OvsNetworkName),
			// Sriov CNI same-node HWOL is deferred: VF representors are not on the managed OVS bridge.
			PEntry("sriov CNI",
				Label(tsparams.LabelOffload), Label(tsparams.LabelSriovNetwork), reportxml.ID("85005"),
				sriovoperator.CNITypeSriov, tsparams.SriovNetworkName),
		)
	})

func createAttachNetwork(
	cniType sriovoperator.CNIType, networkName, operatorNS, ovsBridge string,
) error {
	switch cniType {
	case sriovoperator.CNITypeOVS:
		_, err := hwolenv.CreateOvsNetwork(
			networkName, operatorNS, tsparams.ResourceName, tsparams.TestNamespaceName,
			hwolenv.HostLocalIPAM, ovsBridge)

		return err
	case sriovoperator.CNITypeSriov:
		// Sriov CNI same-node HWOL is deferred: VF representors are not attached to the
		// managed OVS bridge; the offload Entry is Pending until a supported plumbing path exists.
		_, err := hwolenv.CreateSriovNetwork(
			networkName, operatorNS, tsparams.ResourceName, tsparams.TestNamespaceName,
			hwolenv.HostLocalIPAM)

		return err
	default:
		return fmt.Errorf("unsupported CNI type %q", cniType)
	}
}

func deleteAttachNetwork(cniType sriovoperator.CNIType, networkName, operatorNS string) error {
	switch cniType {
	case sriovoperator.CNITypeOVS:
		builder := hwolenv.NewOvsNetworkBuilder(
			APIClient, networkName, operatorNS, tsparams.TestNamespaceName, tsparams.ResourceName)
		if builder == nil {
			return fmt.Errorf("failed to init OVSNetwork builder for delete")
		}

		return builder.Delete()
	case sriovoperator.CNITypeSriov:
		builder, err := sriov.PullNetwork(APIClient, networkName, operatorNS)
		if err != nil {
			if k8serrors.IsNotFound(err) || strings.Contains(err.Error(), "does not exist") {
				return nil
			}

			return err
		}

		return builder.Delete()
	default:
		return fmt.Errorf("unsupported CNI type %q", cniType)
	}
}

// waitForNetworkStatusIP refreshes the pod and waits until Multus network-status has an IP
// for networkName.
func waitForNetworkStatusIP(trafficPod *pod.Builder, networkName string) (string, error) {
	var netIP string

	var lastErr error

	Eventually(func() error {
		if !trafficPod.Exists() {
			lastErr = fmt.Errorf("pod %s/%s does not exist",
				trafficPod.Definition.Namespace, trafficPod.Definition.Name)

			return lastErr
		}

		status := trafficPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
		netIP, _, lastErr = networkStatusIPAndPCI(status, networkName, false)

		return lastErr
	}).WithTimeout(tsparams.DefaultTimeout).WithPolling(tsparams.RetryInterval).Should(Succeed())

	return netIP, lastErr
}

// waitForNetworkStatusIPAndPCI waits until Multus network-status has an IP and PCI address
// for networkName.
func waitForNetworkStatusIPAndPCI(trafficPod *pod.Builder, networkName string) (string, string, error) {
	var netIP, pci string

	var lastErr error

	Eventually(func() error {
		if !trafficPod.Exists() {
			lastErr = fmt.Errorf("pod %s/%s does not exist",
				trafficPod.Definition.Namespace, trafficPod.Definition.Name)

			return lastErr
		}

		status := trafficPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
		netIP, pci, lastErr = networkStatusIPAndPCI(status, networkName, true)

		return lastErr
	}).WithTimeout(tsparams.DefaultTimeout).WithPolling(tsparams.RetryInterval).Should(Succeed())

	return netIP, pci, lastErr
}

// networkStatusIPAndPCI returns the first IP (and optionally PCI) for a Multus network-status
// entry matching networkName. When requirePCI is true, device-info pci-address must be set.
func networkStatusIPAndPCI(
	networkStatusJSON, networkName string, requirePCI bool,
) (string, string, error) {
	var entries []networkStatusEntry
	if err := json.Unmarshal([]byte(networkStatusJSON), &entries); err != nil {
		return "", "", fmt.Errorf("failed to parse network-status: %w", err)
	}

	for _, entry := range entries {
		if entry.Name == networkName ||
			strings.HasSuffix(entry.Name, "/"+networkName) ||
			strings.Contains(entry.Name, networkName) {
			if len(entry.IPs) == 0 {
				return "", "", fmt.Errorf("network-status entry %q has no ips", entry.Name)
			}

			netIP := entry.IPs[0]
			if idx := strings.Index(netIP, "/"); idx >= 0 {
				netIP = netIP[:idx]
			}

			pci := ""
			if entry.DeviceInfo != nil && entry.DeviceInfo.Pci != nil {
				pci = entry.DeviceInfo.Pci.PciAddress
			}

			if requirePCI && pci == "" {
				return "", "", fmt.Errorf("network-status entry %q has no device-info PCI", entry.Name)
			}

			return netIP, pci, nil
		}
	}

	return "", "", fmt.Errorf("network-status has no entry for network %q", networkName)
}

// createTrafficPod creates a sleep pod with the HWOL secondary network.
// If nodeName is non-empty, the pod is pinned to that node (required for host-local IPAM pairs).
func createTrafficPod(podName, networkName, nodeName string, cmd []string) (*pod.Builder, error) {
	resName := corev1.ResourceName(tsparams.ResourceNamePrefix + tsparams.ResourceName)
	secNetwork := pod.StaticIPAnnotation(networkName, nil)

	if secNetwork == nil {
		return nil, fmt.Errorf("failed to build secondary network annotation for %s", networkName)
	}

	// openshifttest/iperf3 is BusyBox: GNU "sleep infinity" is rejected ("invalid number").
	if len(cmd) == 0 {
		cmd = []string{"sleep", "86400000"}
	}

	builder := pod.NewBuilder(
		APIClient,
		podName,
		tsparams.TestNamespaceName,
		HwolOcpConfig.OcpHwolTestContainer,
	).RedefineDefaultCMD(cmd).
		WithSecondaryNetwork(secNetwork)

	if nodeName != "" {
		builder = builder.DefineOnNode(nodeName)
	}

	if builder == nil || builder.Definition == nil || len(builder.Definition.Spec.Containers) == 0 {
		return nil, fmt.Errorf("failed to init traffic pod builder")
	}

	builder.Definition.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{resName: resource.MustParse("1")},
		Limits:   corev1.ResourceList{resName: resource.MustParse("1")},
	}

	return builder.CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
}

// iperfServerCMD returns a shell command that waits for net1, then runs iperf3 bound to that IP.
// VF/sriov pods need -B on the secondary iface; plain iperf3 -s may not accept on net1.
func iperfServerCMD() []string {
	return []string{"sh", "-c", `
iface=net1
for i in $(seq 1 60); do
  ip=$(ip -4 -o addr show dev "$iface" 2>/dev/null | awk '{print $4}' | cut -d/ -f1)
  [ -n "$ip" ] && break
  sleep 1
done
[ -n "$ip" ] || { echo "no IPv4 on $iface"; exit 1; }
exec iperf3 -s -B "$ip"
`}
}

// waitForHwolResource waits until available openshift.io/hwolresource on the node
// (allocatable minus non-terminated pod requests) is at least minAvail.
func waitForHwolResource(nodeName string, minAvail int64, timeout time.Duration) error {
	resName := corev1.ResourceName(tsparams.ResourceNamePrefix + tsparams.ResourceName)

	return wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			node, err := nodes.Pull(APIClient, nodeName)
			if err != nil {
				return false, fmt.Errorf("failed to pull node %s: %w", nodeName, err)
			}

			allocatable := node.Object.Status.Allocatable[resName]

			nodePods, err := pod.ListInAllNamespaces(APIClient, metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
			})
			if err != nil {
				return false, fmt.Errorf("failed to list pods on node %s: %w", nodeName, err)
			}

			var requested resource.Quantity

			for _, nodePod := range nodePods {
				if nodePod.Object == nil {
					continue
				}

				phase := nodePod.Object.Status.Phase
				if phase == corev1.PodSucceeded || phase == corev1.PodFailed {
					continue
				}

				for i := range nodePod.Object.Spec.Containers {
					if qty, ok := nodePod.Object.Spec.Containers[i].Resources.Requests[resName]; ok {
						requested.Add(qty)
					}
				}
			}

			available := allocatable.DeepCopy()
			available.Sub(requested)

			return available.Value() >= minAvail, nil
		})
}

// runIperfBetweenPods runs the iperf3 client against a server already listening in serverPod
// (container command iperf3 -s). Client binds to the secondary-network IP so traffic does not
// egress via the pod primary NIC.
func runIperfBetweenPods(
	clientPod *pod.Builder,
	serverIP, clientIP string,
	duration time.Duration,
) error {
	if serverIP == "" || clientIP == "" {
		return fmt.Errorf("serverIP and clientIP cannot be empty")
	}

	seconds := int(duration.Seconds())
	if seconds < 1 {
		seconds = 1
	}

	// Brief settle, then verify L2 reachability before iperf (sriov VF paths need ARP).
	time.Sleep(3 * time.Second)

	if _, err := clientPod.ExecCommand([]string{
		"ping", "-I", clientIP, "-c", "3", "-W", "2", serverIP,
	}); err != nil {
		return fmt.Errorf("ping from %s to %s failed: %w", clientIP, serverIP, err)
	}

	out, err := clientPod.ExecCommandWithTimeout(
		[]string{
			"iperf3", "-c", serverIP, "-B", clientIP,
			"-t", fmt.Sprintf("%d", seconds),
		},
		duration+30*time.Second,
	)
	if err != nil {
		return fmt.Errorf("iperf3 client failed: %w (out=%s)", err, out.String())
	}

	return nil
}
