package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/hwolenv"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
)

type networkStatusEntry struct {
	Name       string   `json:"name"`
	IPs        []string `json:"ips"`
	MAC        string   `json:"mac"`
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
			operatorNS     string
			mcpLabel       string
			pfName         string
			device         hwolconfig.DeviceConfig
			vfNum          int
			workerNode     string
			ovsBridge      string
			baseline       map[string]hwolenv.OvsOffloadBaseline
			cleanup        bool
			cleanupBlocked bool
			specPods       []*pod.Builder
			specNetwork    *specNetworkIdentity
			runID          string
		)

		BeforeAll(func() {
			operatorNS = HwolOcpConfig.OcpHwolOperatorNamespace
			mcpLabel = HwolOcpConfig.MCPLabel

			By("Loading HWOL device configuration from ECO_OCP_HWOL_DEVICES")

			devices, err := HwolOcpConfig.GetHwolDevices()
			Expect(err).ToNot(HaveOccurred(), "Failed to get HWOL devices")
			Expect(devices).ToNot(BeEmpty(), "No HWOL devices configured")
			device = devices[0]
			pfName = device.InterfaceName

			vfNum, err = HwolOcpConfig.GetVFNum()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VF count")

			By("Checking test resources are absent and recording the OVS offload baseline")

			Expect(hwolenv.EnsureTestResourcesAbsent(operatorNS)).To(Succeed())
			Expect(hwolenv.ValidateSwitchdevPreconditions(operatorNS, mcpLabel, device, vfNum)).To(Succeed(),
				"HWOL requires an unused PF and no other SR-IOV policy on the selected node")
			runID = string(uuid.NewUUID())
			baseline, err = hwolenv.CaptureOvsOffloadBaseline(mcpLabel, HwolOcpConfig.OcpHwolTestContainer)
			Expect(err).ToNot(HaveOccurred(), "Failed to capture OVS offload baseline")
			cleanup = true

			By("Ensuring switchdev foundation (operator config, pool, policy, wait)")

			Expect(hwolenv.EnsureSwitchdevFoundation(
				operatorNS, mcpLabel, device, vfNum, HwolOcpConfig.OvsDocaEnabled, runID,
			)).To(Succeed(), "Failed to ensure switchdev foundation")

			if HwolOcpConfig.OvsDocaEnabled {
				By("Verifying OVS-DOCA is enabled on HWOL MCP nodes")

				Expect(hwolenv.AssertOvsDocaEnabled(
					mcpLabel, HwolOcpConfig.OcpHwolTestContainer,
				)).To(Succeed(), "OVS-DOCA is not enabled on HWOL MCP nodes")
			}

			mcpNodes, err := hwolenv.ListMCPWorkerNodes(mcpLabel)
			Expect(err).ToNot(HaveOccurred(), "Failed to list MCP nodes")
			Expect(mcpNodes).ToNot(BeEmpty(), "No MCP-labeled workers")
			workerNode = mcpNodes[0].Object.Name

			ovsBridge, err = hwolenv.LookupManagedOVSBridge(operatorNS, mcpLabel, pfName)
			Expect(err).ToNot(HaveOccurred(), "Failed to look up managed OVS bridge")
			Expect(ovsBridge).ToNot(BeEmpty(), "Managed OVS bridge name is empty")

			if HwolOcpConfig.OvsDocaEnabled {
				By("Verifying the managed bridge and PF use the DOCA datapath")
				Expect(hwolenv.AssertDocaBridge(workerNode, ovsBridge, pfName,
					HwolOcpConfig.OcpHwolTestContainer)).To(Succeed())
			}
		})

		BeforeEach(func() {
			specPods = nil
			specNetwork = nil
			if cleanupBlocked {
				Skip("previous workload cleanup failed; refusing further HWOL mutations")
			}
		})

		AfterEach(func() {
			By("Cleaning workload pods before deleting their network")
			err := cleanupSpecResources(specPods, func(p *pod.Builder) error {
				return hwolenv.DeleteOwnedPod(p, tsparams.DefaultTimeout)
			}, func() error {
				if cleanupBlocked || specNetwork == nil {
					return nil
				}
				if err := deleteAttachNetwork(specNetwork.cniType, specNetwork.name, operatorNS, runID); err != nil {
					return err
				}
				return sriovoperator.WaitForNADDeletion(APIClient, specNetwork.name,
					tsparams.TestNamespaceName, tsparams.DefaultTimeout)
			})
			if err != nil {
				cleanupBlocked = true
				AddReportEntry("hwol-workload-cleanup-failure", err.Error())
			}
			Expect(err).ToNot(HaveOccurred(), "workload cleanup failed; foundation will be retained")
		})

		AfterAll(func() {
			if !cleanup {
				return
			}
			if cleanupBlocked {
				Fail("refusing to remove HWOL foundation after workload cleanup failure; inspect owned resources")
				return
			}

			By("Cleaning HWOL resources")

			err := hwolenv.CleanupHwolResources(
				operatorNS,
				mcpLabel,
				pfName,
				runID,
				tsparams.CleanupWaitTimeout,
				tsparams.DefaultStableDuration,
			)
			if err != nil {
				AddReportEntry("hwol-cleanup-failure", err.Error())

				if HwolOcpConfig.RecoverOnCleanupFailure {
					By("Recovering HWOL nodes after cleanup failure")

					recoveryErr := hwolenv.RecoverHwolCleanup(
						operatorNS,
						mcpLabel,
						pfName,
						tsparams.MCOWaitTimeout,
						tsparams.DefaultStableDuration,
					)
					if recoveryErr != nil {
						AddReportEntry("hwol-cleanup-recovery-failure", recoveryErr.Error())
					} else {
						AddReportEntry(
							"hwol-cleanup-recovery",
							"reboot recovery completed; preserving original cleanup failure",
						)
					}
				}
			}
			if err == nil {
				By("Verifying the original OVS offload configuration was restored")

				offloadErr := hwolenv.VerifyOvsOffloadBaseline(mcpLabel, HwolOcpConfig.OcpHwolTestContainer, baseline)
				if offloadErr != nil && HwolOcpConfig.RestoreOffloadOnCleanup {
					By("Restoring the OVS offload baseline on dedicated HWOL nodes")

					if restoreErr := hwolenv.RestoreOvsOffloadBaseline(
						operatorNS, mcpLabel, HwolOcpConfig.OcpHwolTestContainer, baseline,
					); restoreErr != nil {
						offloadErr = errors.Join(offloadErr, fmt.Errorf("OVS offload recovery failed: %w", restoreErr))
					} else {
						offloadErr = nil
						AddReportEntry("hwol-offload-baseline-restored", "OVS offload baseline restored on dedicated nodes")
					}
				}

				if offloadErr != nil {
					AddReportEntry("hwol-offload-baseline-mismatch", offloadErr.Error())
					err = errors.Join(err, offloadErr)
				}
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

				err := createAttachNetwork(cniType, networkName, operatorNS, ovsBridge, runID)
				if err != nil {
					cleanupBlocked = true
				} else {
					specNetwork = &specNetworkIdentity{cniType: cniType, name: networkName}
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to create attach network")

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
				if cniType == sriovoperator.CNITypeOVS && HwolOcpConfig.OvsDocaEnabled {
					Expect(sriovoperator.AssertNADInterfaceType(
						APIClient, networkName, tsparams.TestNamespaceName, "doca",
					)).To(Succeed(), "NAD must request DOCA representors")
				}

				By("Creating sleep pod with secondary network")

				attachPod, err := createTrafficPod(
					fmt.Sprintf("hwol-attach-%s", cniType), networkName, workerNode, runID, nil)
				if attachPod != nil {
					specPods = append(specPods, attachPod)
				}
				if err != nil {
					cleanupBlocked = true
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to create attach pod")

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
				var providerCursor hwolenv.DocaProviderCursor
				if cniType == sriovoperator.CNITypeOVS && HwolOcpConfig.OvsDocaEnabled {
					By("Recording the OVS provider log position before creating workload interfaces")
					var err error
					providerCursor, err = hwolenv.CaptureDocaProviderCursor(workerNode, HwolOcpConfig.OcpHwolTestContainer)
					Expect(err).ToNot(HaveOccurred(), "Failed to record OVS provider log position")
				}

				By(fmt.Sprintf("Creating %s network %s for offload", cniType, networkName))

				err := createAttachNetwork(cniType, networkName, operatorNS, ovsBridge, runID)
				if err != nil {
					cleanupBlocked = true
				} else {
					specNetwork = &specNetworkIdentity{cniType: cniType, name: networkName}
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to create offload network")

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
				if cniType == sriovoperator.CNITypeOVS && HwolOcpConfig.OvsDocaEnabled {
					Expect(sriovoperator.AssertNADInterfaceType(
						APIClient, networkName, tsparams.TestNamespaceName, "doca",
					)).To(Succeed(), "NAD must request DOCA representors")
				}

				By(fmt.Sprintf("Creating iperf server/client pods on node %s", workerNode))

				// Prior table entry may still hold VFs until the device plugin frees them.
				Expect(waitForHwolResource(workerNode, 2, tsparams.DefaultTimeout)).To(Succeed(),
					"need ≥2 openshift.io/hwolresource before creating iperf pods")

				// Run iperf3 as the server container PID 1. Starting it via kubectl exec
				// (even with -D / background) does not leave a listener in this image.
				serverPod, err := createTrafficPod(
					fmt.Sprintf("hwol-iperf-srv-%s", cniType), networkName, workerNode, runID,
					iperfServerCMD())
				if serverPod != nil {
					specPods = append(specPods, serverPod)
				}
				if err != nil {
					cleanupBlocked = true
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to create iperf server pod")

				Expect(waitForHwolResource(workerNode, 1, tsparams.DefaultTimeout)).To(Succeed(),
					"need ≥1 openshift.io/hwolresource for iperf client")

				clientPod, err := createTrafficPod(
					fmt.Sprintf("hwol-iperf-cli-%s", cniType), networkName, workerNode, runID, nil)
				if clientPod != nil {
					specPods = append(specPods, clientPod)
				}
				if err != nil {
					cleanupBlocked = true
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to create iperf client pod")

				serverIP, serverPCI, err := waitForNetworkStatusIPAndPCI(serverPod, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get server secondary IP/PCI")

				clientIP, clientPCI, err := waitForNetworkStatusIPAndPCI(clientPod, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get client secondary IP/PCI")

				serverMAC, err := waitForNetworkStatusMAC(serverPod, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get server secondary MAC")

				clientMAC, err := waitForNetworkStatusMAC(clientPod, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get client secondary MAC")

				if cniType == sriovoperator.CNITypeOVS {
					By("Asserting VF representors are ports on the managed OVS bridge")

					if HwolOcpConfig.OvsDocaEnabled {
						Expect(hwolenv.AssertDocaVFRepresentorsOnBridge(
							workerNode, ovsBridge, HwolOcpConfig.OcpHwolTestContainer,
							providerCursor, serverPCI, clientPCI,
						)).To(Succeed(), "expected operational DOCA VF representors assigned to doca_flow_api")
					} else {
						Expect(hwolenv.AssertVFRepresentorsOnBridge(
							workerNode, ovsBridge, HwolOcpConfig.OcpHwolTestContainer,
							serverPCI, clientPCI,
						)).To(Succeed(), "expected server and client VF representors on bridge")
					}
				}

				By("Recording offloaded packet counters for the workload MAC pair")

				datapathType := "system"
				if HwolOcpConfig.OvsDocaEnabled {
					datapathType = "doca"
				}

				before, err := hwolenv.OvsOffloadedTrafficPackets(
					workerNode, HwolOcpConfig.OcpHwolTestContainer, datapathType, clientMAC, serverMAC)
				Expect(err).ToNot(HaveOccurred(), "Failed to record offload baseline")

				By("Running iperf3 between pods on secondary network")

				Expect(runIperfBetweenPods(
					clientPod, serverIP, clientIP, tsparams.IperfDuration,
				)).To(Succeed(), "iperf3 traffic failed")

				By("Asserting offloaded IPv4 packet counters increased for the workload MAC pair")

				after, err := hwolenv.OvsOffloadedTrafficPackets(
					workerNode, HwolOcpConfig.OcpHwolTestContainer, datapathType, clientMAC, serverMAC)
				Expect(err).ToNot(HaveOccurred(), "Failed to read workload offloaded flows")
				Expect(after).To(BeNumerically(">", before), "workload offloaded counters did not increase")
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

type specNetworkIdentity struct {
	cniType sriovoperator.CNIType
	name    string
}

func cleanupSpecResources(pods []*pod.Builder, deletePod func(*pod.Builder) error, deleteNetwork func() error) error {
	var podErr error
	for i := len(pods) - 1; i >= 0; i-- {
		if err := deletePod(pods[i]); err != nil {
			podErr = errors.Join(podErr, err)
		}
	}
	if podErr != nil {
		return fmt.Errorf("cannot safely delete network while workload pods remain: %w", podErr)
	}
	return deleteNetwork()
}

func createAttachNetwork(
	cniType sriovoperator.CNIType, networkName, operatorNS, ovsBridge, runID string,
) error {
	switch cniType {
	case sriovoperator.CNITypeOVS:
		interfaceType := ""
		if HwolOcpConfig.OvsDocaEnabled {
			interfaceType = "doca"
		}

		_, err := hwolenv.CreateOvsNetwork(
			networkName, operatorNS, tsparams.ResourceName, tsparams.TestNamespaceName,
			hwolenv.HostLocalIPAM, ovsBridge, interfaceType, runID)

		return err
	case sriovoperator.CNITypeSriov:
		// Sriov CNI same-node HWOL is deferred: VF representors are not attached to the
		// managed OVS bridge; the offload Entry is Pending until a supported plumbing path exists.
		_, err := hwolenv.CreateSriovNetwork(
			networkName, operatorNS, tsparams.ResourceName, tsparams.TestNamespaceName,
			hwolenv.HostLocalIPAM, runID)

		return err
	default:
		return fmt.Errorf("unsupported CNI type %q", cniType)
	}
}

func deleteAttachNetwork(cniType sriovoperator.CNIType, networkName, operatorNS, runID string) error {
	switch cniType {
	case sriovoperator.CNITypeOVS:
		return hwolenv.DeleteOwnedTestResource(operatorNS, networkName, runID, &sriovv1.OVSNetwork{})
	case sriovoperator.CNITypeSriov:
		return hwolenv.DeleteOwnedTestResource(operatorNS, networkName, runID, &sriovv1.SriovNetwork{})
	default:
		return fmt.Errorf("unsupported CNI type %q", cniType)
	}
}

// waitForNetworkStatusMAC waits for the workload MAC on the requested secondary NAD.
func waitForNetworkStatusMAC(trafficPod *pod.Builder, networkName string) (string, error) {
	var mac string

	var lastErr error

	Eventually(func() error {
		if lastErr = hwolenv.RefreshOwnedPod(trafficPod); lastErr != nil {
			return lastErr
		}

		var entries []networkStatusEntry
		status := trafficPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
		if err := json.Unmarshal([]byte(status), &entries); err != nil {
			lastErr = fmt.Errorf("failed to parse network-status: %w", err)

			return lastErr
		}

		for _, entry := range entries {
			if matchesNetworkName(entry.Name, networkName) {
				if entry.MAC == "" {
					lastErr = fmt.Errorf("network-status entry %q has no MAC", entry.Name)

					return lastErr
				}

				mac, lastErr = entry.MAC, nil

				return nil
			}
		}

		lastErr = fmt.Errorf("network-status has no entry for network %q", networkName)

		return lastErr
	}).WithTimeout(tsparams.DefaultTimeout).WithPolling(tsparams.RetryInterval).Should(Succeed())

	return mac, lastErr
}

// waitForNetworkStatusIP refreshes the pod and waits until Multus network-status has an IP
// for networkName.
func waitForNetworkStatusIP(trafficPod *pod.Builder, networkName string) (string, error) {
	var netIP string

	var lastErr error

	Eventually(func() error {
		if lastErr = hwolenv.RefreshOwnedPod(trafficPod); lastErr != nil {
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
		if lastErr = hwolenv.RefreshOwnedPod(trafficPod); lastErr != nil {
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
		if matchesNetworkName(entry.Name, networkName) {
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

func matchesNetworkName(entry, name string) bool {
	if name == "" {
		return false
	}
	return entry == name || (!strings.Contains(name, "/") && entry == tsparams.TestNamespaceName+"/"+name)
}

// createTrafficPod creates a workload on the selected HWOL node.
func createTrafficPod(podName, networkName, nodeName, runID string, cmd []string) (*pod.Builder, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("HWOL workload node is required")
	}
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
		WithSecondaryNetwork(secNetwork).
		DefineOnNode(nodeName)

	if builder == nil || builder.Definition == nil || len(builder.Definition.Spec.Containers) == 0 {
		return nil, fmt.Errorf("failed to init traffic pod builder")
	}

	builder.Definition.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{resName: resource.MustParse("1")},
		Limits:   corev1.ResourceList{resName: resource.MustParse("1")},
	}

	return hwolenv.CreateOwnedPod(builder, runID, tsparams.DefaultTimeout)
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

	if err := hwolenv.RefreshOwnedPod(clientPod); err != nil {
		return fmt.Errorf("cannot verify iperf client ownership: %w", err)
	}
	if _, err := clientPod.ExecCommand([]string{
		"ping", "-I", clientIP, "-c", "3", "-W", "2", serverIP,
	}); err != nil {
		return fmt.Errorf("ping from %s to %s failed: %w", clientIP, serverIP, err)
	}

	if err := hwolenv.RefreshOwnedPod(clientPod); err != nil {
		return fmt.Errorf("cannot verify iperf client ownership: %w", err)
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
