package tests

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("rdmaMetricsAPI", Ordered, Label(tsparams.LabelRdmaMetricsAPITestCases, tsparams.LabelSriovHWEnabled),
	ContinueOnFailure, func() {
		var (
			workerNodeList           []*nodes.Builder
			sriovInterfacesUnderTest []string
			sriovNetNodeState        *sriov.PoolConfigBuilder
			tPol1, tPol2             *sriov.PolicyBuilder
			tNet1, tNet2             *sriov.NetworkBuilder
		)

		BeforeAll(func() {
			By("Verifying if Rdma Metrics API tests can be executed on given cluster")
			err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support Rdma Metrics test cases as it doesn't have enough nodes")

			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Fetching SR-IOV Vendor ID for interface under test")
			sriovVendor := discoverInterfaceUnderTestVendorID(sriovInterfacesUnderTest[0], workerNodeList[0].Definition.Name)
			Expect(sriovVendor).ToNot(BeEmpty(), "Expected Sriov Vendor not to be empty")

			By("Skipping test cases if the Sriov device is not of Mellanox")
			if sriovVendor != netparam.MlxVendorID {
				Skip("Rdma metrics is supported only on Mellanox devices")
			}
		})

		Context("Rdma metrics in exclusive mode", func() {
			BeforeAll(func() {
				By("Enable RDMA exclusive mode with NetworkPoolConfig")

				sriovNetNodeState = setRdmaMode("exclusive")

				By("Create Sriov Node Policy and Network")

				tPol1 = defineAndCreateNodePolicy("rdmapolicy1", "sriovpf1", sriovInterfacesUnderTest[0], 6, 1)
				tPol2 = defineAndCreateNodePolicy("rdmapolicy2", "sriovpf2", sriovInterfacesUnderTest[1], 6, 1)
				tNet1 = defineAndCreateSriovNetworkWithRdma("sriovnet1", tPol1.Object.Spec.ResourceName, true)
				tNet2 = defineAndCreateSriovNetworkWithRdma("sriovnet2", tPol2.Object.Spec.ResourceName, true)

				err := netenv.WaitForMcpStable(APIClient, tsparams.MCOWaitTimeout, 1*time.Minute, NetConfig.CnfMcpLabel)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for Sriov state to be stable")

				By("Ensure RDMA is in exclusive mode")

				verifyRdmaModeStatus("exclusive", workerNodeList)

			})
			It("1 Pod with 1 VF", reportxml.ID("77651"), func() {
				By("Define and Create Test Pod")
				testPod := defineAndCreatePod(tNet1.Object.Name, "")

				By("Verify allocatable devices doesn't change after sriov-device-plugin pod restart")
				verifyAllocatableResouces(testPod, tPol1.Object.Spec.ResourceName)

				By("Verify Rdma metrics are available inside Pod but not on Host")
				verifyRdmaMetrics(testPod, "net1")
			})
			It("1 Pod with 2 VF of same PF", reportxml.ID("77650"), func() {
				By("Define and Create Test Pod")
				testPod := defineAndCreatePod(tNet1.Object.Name, tNet1.Object.Name)

				By("Verify allocatable devices doesn't change after sriov-device-plugin pod restart")
				verifyAllocatableResouces(testPod, tPol1.Object.Spec.ResourceName)

				By("Verify Rdma metrics are available inside Pod but not on Host")
				verifyRdmaMetrics(testPod, "net1")
				verifyRdmaMetrics(testPod, "net2")
			})
			It("1 Pod with 2 VF of different PF", reportxml.ID("77649"), func() {
				By("Define and Create Test Pod")
				testPod := defineAndCreatePod(tNet1.Object.Name, tNet2.Object.Name)

				By("Verify allocatable devices doesn't change after sriov-device-plugin pod restart")
				verifyAllocatableResouces(testPod, tPol1.Object.Spec.ResourceName)
				verifyAllocatableResouces(testPod, tPol2.Object.Spec.ResourceName)

				By("Verify Rdma metrics are available inside Pod but not on Host")
				verifyRdmaMetrics(testPod, "net1")
				verifyRdmaMetrics(testPod, "net2")
			})

			AfterEach(func() {
				By("Cleaning test namespace")
				err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
			})

			AfterAll(func() {
				By("Delete SriovPoolConfig")
				err := sriovNetNodeState.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovPoolConfig")

				By("Removing SR-IOV configuration")
				err = netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
			})
		})

		Context("Rdma metrics in shared mode", func() {
			BeforeAll(func() {
				By("Set Rdma Mode to shared")

				sriovNetNodeState = setRdmaMode("shared")

				By("Create Sriov Node Policy and Network")

				tPol1 = defineAndCreateNodePolicy("rdmapolicy1", "sriovpf1", sriovInterfacesUnderTest[0], 1, 0)
				tNet1 = defineAndCreateSriovNetworkWithRdma("sriovnet1", tPol1.Object.Spec.ResourceName, false)

				err := netenv.WaitForMcpStable(APIClient, tsparams.MCOWaitTimeout, 1*time.Minute, NetConfig.CnfMcpLabel)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for Sriov state to be stable")

				By("Ensure RDMA is in shared mode")

				verifyRdmaModeStatus("shared", workerNodeList)

			})
			It("1 Pod with 1 VF", reportxml.ID("77653"), func() {
				By("Define and Create Test Pod")

				testPod := defineAndCreatePod(tNet1.Object.Name, "")

				By("Verify allocatable devices doesn't change after sriov-device-plugin pod restart")
				verifyAllocatableResouces(testPod, tPol1.Object.Spec.ResourceName)

				By("Fetch PCI Address and Rdma device from Pod Annotations")
				pciAddress, rdmaDevice, err := getInterfacePci(testPod, "net1")
				Expect(err).ToNot(HaveOccurred(),
					"Could not get PCI Address and/or Rdma device from Pod Annotations")
				Expect(pciAddress).To(Not(BeEmpty()), "pci-address field is empty")
				Expect(rdmaDevice).To(Not(BeEmpty()), "rdma-device field is empty")

				By("Verify Rdma Metrics should not be present inside Pod")
				podOutput, err := testPod.ExecCommand(
					[]string{"bash", "-c", fmt.Sprintf("ls /sys/bus/pci/devices/%s/infiniband/%s/ports/1/hw_counters",
						pciAddress, rdmaDevice)})
				Expect(err).To(HaveOccurred(), "Expected command to fail as the path is not present on Pod")
				Expect(podOutput.String()).To(ContainSubstring("No such file or directory"),
					fmt.Sprintf("Expected error 'No such file or directory' in the output %s", podOutput.String()))

				By("Verify Rdma Metrics should be present on Host")
				nodeOutput, err := cluster.ExecCmdWithStdout(APIClient,
					fmt.Sprintf("ls /sys/bus/pci/devices/%s/infiniband/%s/ports/1/hw_counters", pciAddress, rdmaDevice),
					metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", testPod.Object.Spec.NodeName)})
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to run command %s on node %s",
						fmt.Sprintf("ls /sys/bus/pci/devices/%s/infiniband/%s/ports/1/hw_counters",
							pciAddress, rdmaDevice), testPod.Object.Spec.NodeName))
				Expect(nodeOutput[testPod.Object.Spec.NodeName]).To(ContainSubstring("out_of_buffer"),
					fmt.Sprintf("Failed to find the counters in the output %s", nodeOutput[testPod.Object.Spec.NodeName]))
			})

			AfterAll(func() {
				By("Removing SR-IOV configuration")
				err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

				By("Cleaning test namespace")
				err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")

				By("Delete SriovPoolConfig")

				err = sriovNetNodeState.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovPoolConfig")

				err = netenv.WaitForMcpStable(
					APIClient, tsparams.MCOWaitTimeout, 1*time.Minute, NetConfig.CnfMcpLabel)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for SriovNodeState to be stable")
			})
		})
	})

func setRdmaMode(mode string) *sriov.PoolConfigBuilder {
	sriovPoolConfig, err := sriov.NewPoolConfigBuilder(APIClient, "rdmasubsystem", NetConfig.SriovOperatorNamespace).
		WithNodeSelector(NetConfig.WorkerLabelMap).
		WithMaxUnavailable(intstr.FromInt32(1)).
		WithRDMAMode(mode).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig")

	return sriovPoolConfig
}

func verifyRdmaModeStatus(mode string, nodelist []*nodes.Builder) {
	By("Verify RdmaMode status through SriovNetworkNodeState API")

	var allNodes []string

	for _, node := range nodelist {
		allNodes = append(allNodes, node.Object.Name)
	}

	sriovNodeStates, err := sriov.ListNetworkNodeState(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch SriovNetworkNodeStates")

	for _, nodeState := range sriovNodeStates {
		if slices.Contains(allNodes, nodeState.Objects.Name) {
			Expect(nodeState.Objects.Spec.System.RdmaMode).To(Equal(mode), "SriovNodeState Spec is not correctly updated")
			Expect(nodeState.Objects.Status.System.RdmaMode).To(Equal(mode), "SriovNodeState Spec is not correctly updated")
		}
	}

	By("Verify RdmaMode status through host cli")

	outputNodes, err := cluster.ExecCmdWithStdout(
		APIClient, "rdma system show", metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
	Expect(err).ToNot(HaveOccurred(), "Failed to get output of rdma system show")

	for node, output := range outputNodes {
		Expect(output).To(ContainSubstring(mode), fmt.Sprintf("RDMA is not set to shared on node %s", node))
	}
}

func verifyAllocatableResouces(tPod *pod.Builder, resName string) {
	testPodNode, err := nodes.Pull(APIClient, tPod.Object.Spec.NodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull test pod's host worker node")

	oldAllocatableSriovDevices := testPodNode.Object.Status.Allocatable[corev1.ResourceName("openshift.io/"+resName)]
	oldNum, _ := oldAllocatableSriovDevices.AsInt64()

	sriovDevicePluginPods, err := pod.List(APIClient, NetConfig.SriovOperatorNamespace, metav1.ListOptions{
		LabelSelector: labels.Set{"app": "sriov-device-plugin"}.String(),
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": tPod.Object.Spec.NodeName}).String()})
	Expect(err).ToNot(HaveOccurred(), "Failed to list sriov device plugin pods")
	Expect(len(sriovDevicePluginPods)).To(Equal(1), "Failed to fetch the sriov device plugin pod")

	_, err = sriovDevicePluginPods[0].DeleteAndWait(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete the sriov device plugin pod")

	Eventually(func() bool {
		sriovDevicePluginPods, err = pod.List(APIClient, NetConfig.SriovOperatorNamespace, metav1.ListOptions{
			LabelSelector: labels.Set{"app": "sriov-device-plugin"}.String(),
			FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": tPod.Object.Spec.NodeName}).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list sriov device plugin pods")

		return len(sriovDevicePluginPods) == 1
	}, 1*time.Minute, 2*time.Second).Should(BeTrue(), "Failed to find the new sriov device plugin pod")

	Eventually(func() bool {
		testNode, err := nodes.Pull(APIClient, tPod.Object.Spec.NodeName)
		Expect(err).NotTo(HaveOccurred(), "Failed to pull test pod's host worker node")

		newAllocatableSriovDevices := testNode.Object.Status.Allocatable[corev1.ResourceName("openshift.io/"+resName)]
		newNum, _ := newAllocatableSriovDevices.AsInt64()

		return oldNum == newNum
	}, 1*time.Minute, 2*time.Second).Should(BeTrue(), "New allocatable resource is not same as old")
}

func getInterfacePci(tPod *pod.Builder, podInterface string) (string, string, error) {
	type PodNetworkStatusAnnotation struct {
		Name       string   `json:"name"`
		Interface  string   `json:"interface"`
		Ips        []string `json:"ips,omitempty"`
		Mac        string   `json:"mac,omitempty"`
		Default    bool     `json:"default,omitempty"`
		Mtu        int      `json:"mtu,omitempty"`
		DeviceInfo struct {
			Type    string `json:"type"`
			Version string `json:"version"`
			Pci     struct {
				PciAddress string `json:"pci-address"`
				RdmaDevice string `json:"rdma-device"`
			} `json:"pci"`
		} `json:"device-info,omitempty"`
	}

	var annotation []PodNetworkStatusAnnotation

	err := json.Unmarshal([]byte(tPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]), &annotation)
	Expect(err).ToNot(HaveOccurred(), "Failed to Unmarshal Pod Network Status annotation")

	for _, annotationObject := range annotation {
		if annotationObject.Interface == podInterface {
			if annotationObject.DeviceInfo.Pci.PciAddress != "" || annotationObject.DeviceInfo.Pci.RdmaDevice != "" {
				return annotationObject.DeviceInfo.Pci.PciAddress, annotationObject.DeviceInfo.Pci.RdmaDevice, nil
			}

			return "", "", fmt.Errorf("PCI Address or Rdma Device are not present in the Interface %v", annotationObject)
		}
	}

	return "", "", fmt.Errorf("interface %s not found", podInterface)
}

func defineAndCreateNodePolicy(polName, resName, sriovInterface string, numVfs, endVf int) *sriov.PolicyBuilder {
	testPolicy, err := sriov.NewPolicyBuilder(APIClient,
		polName, NetConfig.SriovOperatorNamespace, resName, numVfs, []string{sriovInterface}, NetConfig.WorkerLabelMap).
		WithDevType("netdevice").
		WithVFRange(0, endVf).
		WithRDMA(true).
		Create()

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SriovNodePolicy %s", polName))

	return testPolicy
}

func defineAndCreateSriovNetworkWithRdma(netName, resName string, withRdma bool) *sriov.NetworkBuilder {
	testNetBuilder := sriov.NewNetworkBuilder(
		APIClient, netName, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName, resName).
		WithMacAddressSupport().
		WithIPAddressSupport().
		WithStaticIpam().
		WithLogLevel(netparam.LogLevelDebug)

	if withRdma {
		testNetBuilder.WithMetaPluginRdma()
	}

	testNetwork, err := testNetBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create Sriov Network %s", netName))

	return testNetwork
}

func defineAndCreatePod(netName1, netName2 string) *pod.Builder {
	netAnnotations := []*types.NetworkSelectionElement{
		{
			Name:       netName1,
			MacRequest: tsparams.ServerMacAddress,
			IPRequest:  []string{tsparams.ServerIPv4IPAddress},
		},
	}

	if len(netName2) != 0 {
		netAnnotations = append(netAnnotations, &types.NetworkSelectionElement{
			Name:       netName2,
			MacRequest: tsparams.ClientMacAddress,
			IPRequest:  []string{tsparams.ClientIPv4IPAddress},
		})
	}

	for _, net := range netAnnotations {
		Eventually(func() error {
			_, err := nad.Pull(APIClient, net.Name, tsparams.TestNamespaceName)

			return err
		}, 10*time.Second, 1*time.Second).Should(BeNil(), fmt.Sprintf("Failed to pull NAD %s", net.Name))
	}

	tPod, err := pod.NewBuilder(APIClient, "testpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithSecondaryNetwork(netAnnotations).
		WithPrivilegedFlag().
		CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Pod %s")

	tPod.Exists()

	return tPod
}

func verifyRdmaMetrics(inputPod *pod.Builder, iName string) {
	By("Fetch PCI Address and Rdma device from Pod Annotations")

	pciAddress, rdmaDevice, err := getInterfacePci(inputPod, iName)
	Expect(err).ToNot(HaveOccurred(),
		"Could not get PCI Address and/or Rdma device from Pod Annotations")
	Expect(pciAddress).To(Not(BeEmpty()), "pci-address field is empty")
	Expect(rdmaDevice).To(Not(BeEmpty()), "rdma-device field is empty")

	By("Rdma metrics should be present inside the Pod")

	podOutput, err := inputPod.ExecCommand(
		[]string{"bash", "-c", fmt.Sprintf("ls /sys/bus/pci/devices/%s/infiniband/%s/ports/1/hw_counters",
			pciAddress, rdmaDevice)})
	Expect(err).ToNot(HaveOccurred(), "Failed to check counters directory on Pod")
	Expect(podOutput.String()).To(ContainSubstring("out_of_buffer"),
		fmt.Sprintf("Failed to find the counters in the output %s", podOutput.String()))

	By("Rdma metrics should not be present in the Host")

	_, err = cluster.ExecCmdWithStdout(APIClient,
		fmt.Sprintf("ls /sys/bus/pci/devices/%s/infiniband/%s/ports/1/hw_counters", pciAddress, rdmaDevice),
		metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", inputPod.Object.Spec.NodeName)})
	Expect(err).To(HaveOccurred(), "Failed to check counters directory on Host")
	Expect(err.Error()).To(ContainSubstring("command terminated with exit code 2"), "Failure is not as expected")
}

func discoverInterfaceUnderTestVendorID(srIovInterfaceUnderTest, workerNodeName string) string {
	sriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(
		APIClient, workerNodeName, NetConfig.SriovOperatorNamespace).GetUpNICs()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("fail to discover Vendor ID for network interface %s",
		srIovInterfaceUnderTest))

	for _, srIovInterface := range sriovInterfaces {
		if srIovInterface.Name == srIovInterfaceUnderTest {
			return srIovInterface.Vendor
		}
	}

	return ""
}
