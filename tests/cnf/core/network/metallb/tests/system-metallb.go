package tests

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/network"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netnmstate"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	mlbcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	systemHTTPProtocol      = "http"
	systemICMPProtocol      = "ICMP"
	ovnNamespace            = "openshift-ovn-kubernetes"
	ovnkubeNodeDaemonSet    = "ovnkube-node"
	ovnkubeNodeContainer    = "ovnkube-controller"
	ovnV4JoinSubnetEnv      = "OVN_V4_JOIN_SUBNET"
	defaultOVNJoinSubnetV4  = "100.64.0.0/16"
	ovnNodeIDAnnotationName = "k8s.ovn.org/node-id"

	systemMetalLBInternalNAD   = "internal"
	systemTrafficDumpPath      = "/traffic"
	systemTCPDumpContainerName = "tcpdump"
	systemTCPDumpFilter        = "tcpdump -nnn --immediate-mode -l port 80 or proto 1"
)

var (
	systemCaptureTimeout = tsparams.DefaultTimeout
	systemCaptureRetry   = tsparams.DefaultRetryInterval
)

var _ = Describe("MetalLB system", Ordered, Label(tsparams.LabelSystemMetalLB), ContinueOnFailure, func() {
	BeforeAll(func() {
		By("Checking if cluster is SNO")

		if IsSNO {
			Skip("Skipping test on SNO (Single Node OpenShift) cluster - requires 2+ workers for 'different node' test case")
		}

		validateEnvVarAndGetNodeList()
		validateIPFamilySupport(netparam.IPV4Family)

		By("Creating a new instance of MetalLB Speakers on workers")

		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

		By("Activating SCTP module on master nodes")
		activateSCTPModuleOnMasterNodes()
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}

		resetOperatorAndTestNS()
	})

	BeforeEach(func() {
		setupTestEnv(ipv4, 32, false)
	})

	AfterEach(func() {
		By("Cleaning MetalLb operator namespace")

		metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
		err = metalLbNs.CleanObjects(
			tsparams.DefaultTimeout,
			metallb.GetBGPPeerGVR(),
			metallb.GetBFDProfileGVR(),
			metallb.GetBGPAdvertisementGVR(),
			metallb.GetIPAddressPoolGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

		By("Cleaning test namespace")

		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout,
			pod.GetGVR(),
			service.GetGVR(),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
	})

	DescribeTable("MetalLB Load balance external IP accessible to internal cluster IPs",
		func(diffNode bool) {
			By("Fetching LB IP assigned to service")

			lbSvc, err := service.Pull(APIClient, tsparams.MetallbServiceName, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName)
			Expect(lbSvc.Object.Status.LoadBalancer.Ingress).NotTo(BeEmpty(),
				"Load Balancer IP is not assigned to the tcp service")

			By("Fetching Nginx server pod IP address")

			nginxPod, err := pod.Pull(APIClient, "nginxpod1", tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull nginx server pod")
			Expect(nginxPod.Object.Status.PodIP).NotTo(BeEmpty(), "Pod IP is empty")

			By("Creating client pod")

			var clientPod *pod.Builder
			if !diffNode {
				clientPod, err = pod.NewBuilder(APIClient, "clientpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
					DefineOnNode(workerNodeList[0].Object.Name).
					CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create a client pod")
			} else {
				clientPod, err = pod.NewBuilder(APIClient, "clientpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
					DefineOnNode(workerNodeList[1].Object.Name).
					CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create a client pod")
			}

			By("Checking that client pod can run curl to Nginx server pod with LB IP address")
			By("Checking Nginx server Pod receives curl on the server pod IP address")
			// update client pod with latest status information with IP address.
			clientPod.Exists()
			output, err := clientPod.ExecCommand([]string{"/bin/bash", "-c",
				fmt.Sprintf("curl %s/serverip", lbSvc.Object.Status.LoadBalancer.Ingress[0].IP)})
			Expect(err).ToNot(HaveOccurred(), "Failed to curl to Nginx pod")
			Expect(output.String()).To(ContainSubstring(nginxPod.Object.Status.PodIP))

			By("Checking client IP seen by nginx server is same as client pod IP address")

			output, err = clientPod.ExecCommand([]string{"/bin/bash", "-c",
				fmt.Sprintf("curl %s/clientip", lbSvc.Object.Status.LoadBalancer.Ingress[0].IP)})
			Expect(err).ToNot(HaveOccurred(), "Failed to curl to Nginx pod")
			Expect(output.String()).To(ContainSubstring(clientPod.Object.Status.PodIP))
		},
		Entry("same node", reportxml.ID("53792"), false),
		Entry("different node", reportxml.ID("53766"), true))
})

var _ = Describe("MetalLB system", Ordered, Label(tsparams.LabelSystemMetalLB), ContinueOnFailure, func() {
	var (
		secInterfaces             []string
		originalRoutingViaHost    bool
		gatewayModeSetupSucceeded bool
		vlanID1                   uint16
		vlanID2                   uint16
		vlanNADName1              string
		vlanNADName2              string
		iface1Name                string
		iface2Name                string
		nodeRouterIP              string
		nncpPolicy                *nmstate.PolicyBuilder
		clientPod1                *pod.Builder
		clientPod2                *pod.Builder
		frrRouter1                *pod.Builder
		frrRouter2                *pod.Builder
		nodeCapturePod            *pod.Builder
	)

	BeforeAll(func() {
		By("Checking if cluster is SNO")

		if IsSNO {
			Skip("Skipping MetalLB system tests on SNO cluster - requires 2+ workers")
		}

		validateEnvVarAndGetNodeList()
		validateIPFamilySupport(netparam.IPV4Family)

		By("Collecting secondary interfaces for VLAN configuration")

		var err error

		secInterfaces, err = NetConfig.GetSriovInterfaces(1)
		if err != nil || len(secInterfaces) < 1 {
			Skip("Skipping: need at least 1 secondary interface for MetalLB system tests")
		}

		nodeRouterIP, err = getNodeOVNRouterIP(workerNodeList[1].Object)
		Expect(err).ToNot(HaveOccurred(), "Failed to determine OVN router IP for worker node %s",
			workerNodeList[1].Object.Name)

		By("Reading VLAN ID from environment configuration")

		vlanID1, err = NetConfig.GetVLAN()
		Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN ID from ECO_CNF_CORE_NET_VLAN")

		vlanID2 = vlanID1 + 1
		vlanNADName1 = fmt.Sprintf("external-%d", vlanID1)
		vlanNADName2 = fmt.Sprintf("external-%d", vlanID2)
		iface1Name = fmt.Sprintf("%s.%d", secInterfaces[0], vlanID1)
		iface2Name = fmt.Sprintf("%s.%d", secInterfaces[0], vlanID2)

		By("Saving original network operator settings")

		networkOperator, err := network.PullOperator(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull network operator")

		if networkOperator.Definition.Spec.DefaultNetwork.OVNKubernetesConfig != nil &&
			networkOperator.Definition.Spec.DefaultNetwork.OVNKubernetesConfig.GatewayConfig != nil {
			originalRoutingViaHost = networkOperator.Definition.Spec.DefaultNetwork.
				OVNKubernetesConfig.GatewayConfig.RoutingViaHost
		}

		By("Configuring local gateway mode with global IP forwarding")

		setLocalGWModeWithRetry(true)

		gatewayModeSetupSucceeded = true

		By("Creating a new instance of MetalLB Speakers on workers")

		err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

		By("Creating bridge and VLAN NetworkAttachmentDefinitions")

		createIPFwdBridgeNAD()
		createIPFwdVlanNAD(vlanNADName1, secInterfaces[0], vlanID1)
		createIPFwdVlanNAD(vlanNADName2, secInterfaces[0], vlanID2)

		By("Creating NMState policy with VLAN interfaces on the second worker node")

		nncpPolicy = nmstate.NewPolicyBuilder(APIClient, ipFwdNNCPName,
			map[string]string{corev1.LabelHostname: workerNodeList[1].Definition.Name}).
			WithVlanInterfaceIP(secInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, vlanID1).
			WithVlanInterfaceIP(secInterfaces[0], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, vlanID2).
			WithOptions(
				netnmstate.WithInterfaceIPv4Forwarding(iface1Name, true),
				netnmstate.WithInterfaceIPv4Forwarding(iface2Name, true)).
			WithStaticRoute(ipFwdInternalSubnet1, ipFwdFrrRouterIP1, iface1Name, 150, 254).
			WithStaticRoute(ipFwdInternalSubnet2, ipFwdFrrRouterIP2, iface2Name, 150, 254)
		err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpPolicy)
		Expect(err).ToNot(HaveOccurred(), "Failed to create NNCP with VLAN interfaces")

		By("Creating FRR ConfigMaps for the router pods")

		createIPFwdFRRConfigMap(tsparams.FRRDefaultConfigMapName, tsparams.RemoteBGPASN, ipFwdWorkerVlanIP1)
		createIPFwdFRRConfigMap(tsparams.FRRDefaultConfigMapName2, ipFwdRemoteBGPASN2, ipFwdWorkerVlanIP2)

		By("Creating FRR router pods on the first worker node")

		frrRouter1 = createIPFwdFRRRouterPod("frr-router-1", workerNodeList[0].Object.Name,
			tsparams.FRRDefaultConfigMapName, vlanNADName1, ipFwdFrrRouterIP1, ipFwdRouterInternalIP1,
			netparam.IPSubnetInt24)
		frrRouter2 = createIPFwdFRRRouterPod("frr-router-2", workerNodeList[0].Object.Name,
			tsparams.FRRDefaultConfigMapName2, vlanNADName2, ipFwdFrrRouterIP2, ipFwdRouterInternalIP2,
			netparam.IPSubnetInt24)

		By("Enabling IP forwarding on FRR router pod secondary interfaces")

		enableIPFwdOnPod(frrRouter1, false)
		enableIPFwdOnPod(frrRouter2, false)

		By("Verifying connectivity from FRR routers to node VLAN interfaces")

		verifyIPFwdPingConnectivity(frrRouter1, ipFwdWorkerVlanIP1)
		verifyIPFwdPingConnectivity(frrRouter2, ipFwdWorkerVlanIP2)

		By("Creating IPAddressPools and LoadBalancer services")

		ipPool1, err := metallb.NewIPAddressPoolBuilder(
			APIClient, tsparams.BGPAdvAndAddressPoolName, NetConfig.MlbOperatorNamespace,
			[]string{fmt.Sprintf("%s-%s", ipFwdServiceIP1, ipFwdServiceIP1)}).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool 1")

		ipPool2, err := metallb.NewIPAddressPoolBuilder(
			APIClient, tsparams.BGPAdvAndAddressPoolName2, NetConfig.MlbOperatorNamespace,
			[]string{fmt.Sprintf("%s-%s", ipFwdServiceIP2, ipFwdServiceIP2)}).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool 2")

		setupMetalLbService(tsparams.MetallbServiceName, netparam.IPV4Family, "server1",
			ipPool1, corev1.ServiceExternalTrafficPolicyTypeCluster)
		setupMetalLbService(tsparams.MetallbServiceName2, netparam.IPV4Family, "server2",
			ipPool2, corev1.ServiceExternalTrafficPolicyTypeCluster)

		primaryService, err := service.Pull(APIClient, tsparams.MetallbServiceName, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName)
		secondaryService, err := service.Pull(APIClient, tsparams.MetallbServiceName2, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName2)

		By("Creating nginx server pods with traffic capture on the second worker node")

		createSystemMetalLBServerPod("server1", workerNodeList[1].Object.Name, "server1")
		createSystemMetalLBServerPod("server2", workerNodeList[1].Object.Name, "server2")

		By("Creating client pods with traffic capture on the first worker node")

		clientPod1 = createSystemMetalLBClientPod("client-1", workerNodeList[0].Object.Name,
			ipFwdClientInternalIP1, ipFwdRouterInternalIP1, ipFwdServiceIP1, "192.168.1.0/24")
		clientPod2 = createSystemMetalLBClientPod("client-2", workerNodeList[0].Object.Name,
			ipFwdClientInternalIP2, ipFwdRouterInternalIP2, ipFwdServiceIP2, "192.168.2.0/24")

		By("Creating node traffic capture pod for VLAN verification")

		nodeCapturePod = createSystemMetalLBNodeCapturePod(
			workerNodeList[1].Object.Name, secInterfaces[0],
			primaryService.Object.Spec.ClusterIP, secondaryService.Object.Spec.ClusterIP)

		By("Creating BGP peers targeting the second worker node")

		workerNode1Name := workerNodeList[1].Definition.Name
		workerNode1Selector := map[string]string{corev1.LabelHostname: workerNode1Name}

		frrk8sPods := []*pod.Builder{}

		for _, frrk8sPod := range verifyAndCreateFRRk8sPodList() {
			if frrk8sPod.Object.Spec.NodeName == workerNode1Name {
				frrk8sPods = append(frrk8sPods, frrk8sPod)

				break
			}
		}

		Expect(frrk8sPods).ToNot(BeEmpty(), "Failed to find FRR k8s speaker pod on node %s", workerNode1Name)

		createBGPPeerAndVerifyIfItsReady(
			tsparams.BgpPeerName1, ipFwdFrrRouterIP1, "", ipFwdRouterID, tsparams.RemoteBGPASN,
			workerNode1Selector, false, 0, frrk8sPods)
		createBGPPeerAndVerifyIfItsReady(
			tsparams.BgpPeerName2, ipFwdFrrRouterIP2, "", ipFwdRouterID, ipFwdRemoteBGPASN2,
			workerNode1Selector, false, 0, frrk8sPods)

		By("Creating BGP advertisements")

		setupBgpAdvertisement(
			tsparams.BGPAdvAndAddressPoolName,
			tsparams.NoAdvertiseCommunity,
			tsparams.BGPAdvAndAddressPoolName,
			100,
			[]string{tsparams.BgpPeerName1},
			nil)
		setupBgpAdvertisement(
			tsparams.BGPAdvAndAddressPoolName2,
			tsparams.NoAdvertiseCommunity,
			tsparams.BGPAdvAndAddressPoolName2,
			100,
			[]string{tsparams.BgpPeerName2},
			nil)

		By("Verifying BGP sessions and per-interface forwarding are established")

		verifyIPFwdBGPSessionsEstablished(frrRouter1, frrRouter2, ipFwdWorkerVlanIP1, ipFwdWorkerVlanIP2)
		verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface1Name, "1")
		verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface2Name, "1")
	})

	AfterAll(func() {
		if gatewayModeSetupSucceeded {
			By("Restoring network operator settings")

			setLocalGWModeWithRetry(originalRoutingViaHost)
		}

		cleanupSystemMetalLBNNCP(nncpPolicy, ipFwdNNCPName, secInterfaces, vlanID1, vlanID2)

		if len(cnfWorkerNodeList) > 2 {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}

		resetOperatorAndTestNS()
	})

	It("Validates MetalLB access from secondary host interfaces with multiple VLANs",
		reportxml.ID("53894"), func() {
			verifySystemMetalLBSecondaryPath(
				clientPod1, "server1", tsparams.MetallbServiceName, ipFwdServiceIP1, ipFwdClientInternalIP1,
				nodeCapturePod, secInterfaces[0], vlanID1, ipFwdWorkerVlanIP1, nodeRouterIP)
			verifySystemMetalLBSecondaryPath(
				clientPod2, "server2", tsparams.MetallbServiceName2, ipFwdServiceIP2, ipFwdClientInternalIP2,
				nodeCapturePod, secInterfaces[0], vlanID2, ipFwdWorkerVlanIP2, nodeRouterIP)
		})

	It("Validates MetalLB secondary-interface traffic after node reboot",
		reportxml.ID("53947"), func() {
			By("Rebooting the worker node with VLAN interfaces")

			workerNodeName := workerNodeList[1].Definition.Name
			_, err := cluster.ExecCmdWithStdout(APIClient, "reboot -f",
				metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", corev1.LabelHostname, workerNodeName)})
			Expect(err).ToNot(HaveOccurred(), "Failed to reboot worker node %s", workerNodeName)

			By("Waiting for MachineConfigPool to stabilize after reboot")

			err = cluster.WaitForMcpStable(APIClient, netparam.MCOWaitTimeout, 1*time.Minute, NetConfig.CnfMcpLabel)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for MCP to be stable after reboot")

			By("Waiting for recreated pods and per-interface forwarding to recover")

			Expect(clientPod1.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred())
			Expect(clientPod2.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred())
			Expect(frrRouter1.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred())
			Expect(frrRouter2.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred())
			Expect(nodeCapturePod.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred())
			waitForIPForwardingOnWorker(workerNodeName, iface1Name, "1")
			waitForIPForwardingOnWorker(workerNodeName, iface2Name, "1")

			By("Verifying BGP sessions are re-established after reboot")

			verifyIPFwdBGPSessionsEstablished(frrRouter1, frrRouter2, ipFwdWorkerVlanIP1, ipFwdWorkerVlanIP2)

			By("Verifying traffic still passes over both VLAN-backed paths")

			verifySystemMetalLBSecondaryPath(
				clientPod1, "server1", tsparams.MetallbServiceName, ipFwdServiceIP1, ipFwdClientInternalIP1,
				nodeCapturePod, secInterfaces[0], vlanID1, ipFwdWorkerVlanIP1, nodeRouterIP)
			verifySystemMetalLBSecondaryPath(
				clientPod2, "server2", tsparams.MetallbServiceName2, ipFwdServiceIP2, ipFwdClientInternalIP2,
				nodeCapturePod, secInterfaces[0], vlanID2, ipFwdWorkerVlanIP2, nodeRouterIP)
		})
})

func cleanupSystemMetalLBNNCP(
	nncpPolicy *nmstate.PolicyBuilder, nncpName string,
	secInterfaces []string, vlanID1, vlanID2 uint16,
) {
	By("Reverting VLAN interfaces via NMState absent state")

	if nncpPolicy != nil && nncpPolicy.Exists() && len(secInterfaces) >= 1 {
		iface1Name := fmt.Sprintf("%s.%d", secInterfaces[0], vlanID1)
		iface2Name := fmt.Sprintf("%s.%d", secInterfaces[0], vlanID2)

		nncpPolicy = nmstate.NewPolicyBuilder(APIClient, nncpName,
			map[string]string{corev1.LabelHostname: workerNodeList[1].Definition.Name}).
			WithAbsentInterface(iface1Name).
			WithAbsentInterface(iface2Name)
		err := netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpPolicy)
		Expect(err).ToNot(HaveOccurred(), "Failed to set VLAN interfaces to absent")
	}

	By("Removing NMState policy")

	if nncpPolicy != nil && nncpPolicy.Exists() {
		_, err := nncpPolicy.Delete()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete NNCP")
	}
}

func verifySystemMetalLBSecondaryPath(
	clientPod *pod.Builder, serverPodName, serviceName, expectedLBIP, clientIP string,
	nodeCapturePod *pod.Builder, captureInterface string, vlanID uint16, nodeVlanIP, nodeRouterIP string,
) {
	GinkgoHelper()

	By(fmt.Sprintf("Verifying service %s is assigned the expected MetalLB IP", serviceName))

	lbSvc, err := service.Pull(APIClient, serviceName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", serviceName)
	Expect(lbSvc.Object.Status.LoadBalancer.Ingress).NotTo(BeEmpty(),
		"Load Balancer IP is not assigned to service %s", serviceName)
	Expect(lbSvc.Object.Status.LoadBalancer.Ingress[0].IP).To(Equal(expectedLBIP),
		"Unexpected Load Balancer IP assigned to service %s", serviceName)
	Expect(lbSvc.Object.Spec.ClusterIP).NotTo(BeEmpty(), "ClusterIP is not assigned to service %s", serviceName)

	By(fmt.Sprintf("Waiting for client pod %s to be running", clientPod.Definition.Name))

	Expect(clientPod.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred(),
		"Client pod %s is not running", clientPod.Definition.Name)
	Expect(nodeCapturePod.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred(),
		"Node capture pod is not running")

	serverPod, err := pod.Pull(APIClient, serverPodName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull server pod %s", serverPodName)
	Expect(serverPod.WaitUntilRunning(tsparams.DefaultTimeout)).ToNot(HaveOccurred(),
		"Server pod %s is not running", serverPodName)
	Expect(serverPod.Object.Status.PodIP).NotTo(BeEmpty(), "Pod IP is empty for server pod %s", serverPodName)

	By(fmt.Sprintf("Clearing stale traffic captures before validating path for service %s", serviceName))

	clearSystemTrafficCapture(clientPod, "")
	clearSystemTrafficCapture(nodeCapturePod, "")
	clearSystemTrafficCapture(nodeCapturePod, systemTCPDumpContainerName)
	clearSystemTrafficCapture(serverPod, systemTCPDumpContainerName)

	By(fmt.Sprintf("Generating HTTP connections to service %s from client %s", serviceName, clientPod.Definition.Name))

	generateSystemConnections(clientPod, clientIP, expectedLBIP)

	By(fmt.Sprintf("Checking source and destination in HTTP capture on client pod %s", clientPod.Definition.Name))

	verifySystemTrafficCaptureFromContainer(clientPod, "", clientIP, expectedLBIP, systemHTTPProtocol)

	By(fmt.Sprintf("Checking source and destination in HTTP capture on node VLAN interface %s", captureInterface))

	verifySystemTrafficCaptureFromContainer(nodeCapturePod, "", clientIP, expectedLBIP, systemHTTPProtocol, vlanID)

	By("Checking source and destination in HTTP capture on node br-ex interface")

	verifySystemTrafficCaptureFromContainer(nodeCapturePod, systemTCPDumpContainerName,
		clientIP, lbSvc.Object.Spec.ClusterIP, systemHTTPProtocol)

	By(fmt.Sprintf("Checking source and destination in HTTP capture on server pod %s", serverPodName))

	verifySystemTrafficCaptureFromContainer(serverPod, systemTCPDumpContainerName,
		nodeRouterIP, serverPod.Object.Status.PodIP, systemHTTPProtocol)

	By(fmt.Sprintf("Verifying reverse ICMP reachability from server %s to client IP %s", serverPodName, clientIP))

	verifyIPFwdPingConnectivity(serverPod, clientIP)

	By(fmt.Sprintf("Checking source and destination in ICMP capture on server pod %s", serverPodName))

	verifySystemTrafficCaptureFromContainer(serverPod, systemTCPDumpContainerName,
		serverPod.Object.Status.PodIP, clientIP, systemICMPProtocol)

	By(fmt.Sprintf("Checking source and destination in ICMP capture on node VLAN interface %s", captureInterface))

	verifySystemTrafficCaptureFromContainer(nodeCapturePod, "", nodeVlanIP, clientIP, systemICMPProtocol, vlanID)

	By(fmt.Sprintf("Checking source and destination in ICMP capture on client pod %s", clientPod.Definition.Name))

	verifySystemTrafficCaptureFromContainer(clientPod, "", nodeVlanIP, clientIP, systemICMPProtocol)
}

func createSystemMetalLBClientPod(name, nodeName, clientIP, gatewayIP, serviceIP, vlanSubnet string) *pod.Builder {
	GinkgoHelper()

	clientAnnotation := pod.StaticIPAnnotation(
		systemMetalLBInternalNAD, []string{fmt.Sprintf("%s/%d", clientIP, netparam.IPSubnetInt24)})

	cmd := fmt.Sprintf(
		"ip route del default 2>/dev/null; "+
			"ip route add %s/32 via %s; "+
			"ip route add %s via %s; "+
			"%s -i net1 > %s 2>&1",
		serviceIP, gatewayIP, vlanSubnet, gatewayIP, systemTCPDumpFilter, systemTrafficDumpPath)

	clientPod, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithSecondaryNetwork(clientAnnotation).
		RedefineDefaultCMD([]string{"/bin/bash", "-c", cmd}).
		WithPrivilegedFlag().
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod %s", name)

	return clientPod
}

func createSystemMetalLBServerPod(podName, nodeName, labelValue string) *pod.Builder {
	GinkgoHelper()

	privilegedTrue := true
	tcpdumpCmd := []string{
		"/bin/bash", "-c",
		fmt.Sprintf("%s -i eth0 > %s 2>&1", systemTCPDumpFilter, systemTrafficDumpPath),
	}

	tcpdumpContainer, err := pod.NewContainerBuilder(
		systemTCPDumpContainerName, NetConfig.CnfNetTestContainer, tcpdumpCmd).
		WithSecurityContext(&corev1.SecurityContext{Privileged: &privilegedTrue}).
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to build tcpdump container for server pod %s", podName)

	serverPod, err := pod.NewBuilder(
		APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithLabel("app", labelValue).
		RedefineDefaultCMD([]string{"/bin/bash", "-c", "nginx && sleep INF"}).
		WithPrivilegedFlag().
		WithAdditionalContainer(tcpdumpContainer).
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create nginx server pod %s", podName)

	return serverPod
}

func createSystemMetalLBNodeCapturePod(
	nodeName, captureInterface, serviceClusterIP1, serviceClusterIP2 string,
) *pod.Builder {
	GinkgoHelper()

	privilegedTrue := true
	primaryCaptureCmd := []string{
		"/bin/bash", "-c",
		fmt.Sprintf("%s -e -i %s > %s 2>&1", systemTCPDumpFilter, captureInterface, systemTrafficDumpPath),
	}
	brExCaptureCmd := []string{
		"/bin/bash", "-c",
		fmt.Sprintf("%s and host %s or host %s -i br-ex > %s 2>&1",
			systemTCPDumpFilter, serviceClusterIP1, serviceClusterIP2, systemTrafficDumpPath),
	}

	tcpdumpContainer, err := pod.NewContainerBuilder(
		systemTCPDumpContainerName, NetConfig.CnfNetTestContainer, brExCaptureCmd).
		WithSecurityContext(&corev1.SecurityContext{Privileged: &privilegedTrue}).
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to build br-ex tcpdump container")

	capturePod, err := pod.NewBuilder(
		APIClient, "system-vlan-capture", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithHostNetwork().
		RedefineDefaultCMD(primaryCaptureCmd).
		WithAdditionalContainer(tcpdumpContainer).
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to create node traffic capture pod on node %s for interface %s", nodeName, captureInterface)

	return capturePod
}

func generateSystemConnections(clientPod *pod.Builder, clientIP, targetIP string) {
	GinkgoHelper()

	for idx := 0; idx < 4; idx++ {
		_, err := mlbcmd.Curl(clientPod, clientIP, targetIP, netparam.IPV4Family)
		Expect(err).ToNot(HaveOccurred(), "Failed to generate HTTP connection %d from %s to %s",
			idx+1, clientIP, targetIP)
	}
}

func clearSystemTrafficCapture(capturePod *pod.Builder, containerName string) {
	GinkgoHelper()

	var err error
	if containerName == "" {
		_, err = capturePod.ExecCommand([]string{"truncate", "-s", "0", systemTrafficDumpPath})
	} else {
		_, err = capturePod.ExecCommand([]string{"truncate", "-s", "0", systemTrafficDumpPath}, containerName)
	}

	Expect(err).ToNot(HaveOccurred(), "Failed to clear traffic dump in container %q", containerName)
}

func verifySystemTrafficCaptureFromContainer(
	capturePod *pod.Builder, containerName, srcIP, dstIP, protocol string, vlanID ...uint16,
) {
	GinkgoHelper()

	Eventually(func() error {
		var (
			output bytes.Buffer
			err    error
		)

		if containerName == "" {
			output, err = capturePod.ExecCommand([]string{"cat", systemTrafficDumpPath})
		} else {
			output, err = capturePod.ExecCommand([]string{"cat", systemTrafficDumpPath}, containerName)
		}

		if err != nil {
			return fmt.Errorf("failed to read traffic dump from container %q: %w", containerName, err)
		}

		return matchSystemTrafficCapture(output.String(), srcIP, dstIP, protocol, vlanID...)
	}, systemCaptureTimeout, systemCaptureRetry).Should(Succeed(),
		"Failed to verify %s capture with src %s and dst %s in container %q",
		protocol, srcIP, dstIP, containerName)
}

func matchSystemTrafficCapture(capture, srcIP, dstIP, protocol string, vlanID ...uint16) error {
	GinkgoHelper()

	suffix := ".80"
	if protocol == systemICMPProtocol {
		suffix = ": ICMP"
	}

	for _, line := range strings.Split(capture, "\n") {
		if !strings.Contains(line, srcIP) || !strings.Contains(line, fmt.Sprintf("%s%s", dstIP, suffix)) {
			continue
		}

		if len(vlanID) > 0 && !strings.Contains(line, fmt.Sprintf("vlan %d", vlanID[0])) {
			return fmt.Errorf("failed to detect vlan %d in line %q", vlanID[0], line)
		}

		return nil
	}

	return fmt.Errorf("failed to detect source %s and destination %s in %s capture", srcIP, dstIP, protocol)
}

func getNodeOVNRouterIP(workerNode *corev1.Node) (string, error) {
	nodeIDValue, found := workerNode.Annotations[ovnNodeIDAnnotationName]
	if !found {
		return "", fmt.Errorf("annotation %s does not exist on node %s", ovnNodeIDAnnotationName, workerNode.Name)
	}

	if nodeIDValue == "" {
		return "", fmt.Errorf("annotation %s is empty on node %s", ovnNodeIDAnnotationName, workerNode.Name)
	}

	nodeID, err := strconv.ParseUint(nodeIDValue, 10, 32)
	if err != nil {
		return "", fmt.Errorf("failed to parse node id %q from annotation %s on node %s: %w",
			nodeIDValue, ovnNodeIDAnnotationName, workerNode.Name, err)
	}

	joinSubnet, err := getOVNJoinSubnetV4()
	if err != nil {
		return "", err
	}

	joinPrefix, err := netip.ParsePrefix(joinSubnet)
	if err != nil {
		return "", fmt.Errorf("failed to parse OVN join subnet %q: %w", joinSubnet, err)
	}

	if !joinPrefix.Addr().Is4() {
		return "", fmt.Errorf("OVN join subnet %q is not an IPv4 subnet", joinSubnet)
	}

	maskedIP := joinPrefix.Masked().Addr().As4()
	baseIP := binary.BigEndian.Uint32(maskedIP[:])

	var routerIP [4]byte
	binary.BigEndian.PutUint32(routerIP[:], baseIP+uint32(nodeID))

	return netip.AddrFrom4(routerIP).String(), nil
}

func getOVNJoinSubnetV4() (string, error) {
	ovnkubeNode, err := daemonset.Pull(APIClient, ovnkubeNodeDaemonSet, ovnNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to pull OVN DaemonSet %s/%s: %w", ovnNamespace, ovnkubeNodeDaemonSet, err)
	}

	for _, container := range ovnkubeNode.Definition.Spec.Template.Spec.Containers {
		if container.Name != ovnkubeNodeContainer {
			continue
		}

		for _, envVar := range container.Env {
			if envVar.Name == ovnV4JoinSubnetEnv {
				if envVar.Value == "" {
					return defaultOVNJoinSubnetV4, nil
				}

				return envVar.Value, nil
			}
		}

		return defaultOVNJoinSubnetV4, nil
	}

	return "", fmt.Errorf("container %q not found in OVN DaemonSet %s/%s",
		ovnkubeNodeContainer, ovnNamespace, ovnkubeNodeDaemonSet)
}
