package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/network"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netnmstate"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	labelIPForwardingTestCases = "ipforwarding"
	ipFwdNNCPName              = "ip-fwd-policy"
	ipFwdBridgeNADName         = "internal"
	ipFwdFRRConfigMap1         = "frr-config-1"
	ipFwdFRRConfigMap2         = "frr-config-2"
	ipFwdBGPPeer1              = "peer-one"
	ipFwdBGPPeer2              = "peer-two"
	ipFwdAddrPool3             = "addresspool3"
	ipFwdAddrPool4             = "addresspool4"
	ipFwdBGPAdv1               = "bgpadvertisement-1"
	ipFwdBGPAdv2               = "bgpadvertisement-2"
	ipFwdRemoteBGPASN2         = 64502
	ipFwdRouterID              = "10.10.10.10"
	ipFwdV6NNCPName            = "ip-fwd-v6-policy"

	ipFwdWorkerVlanIP1 = "192.168.1.1"
	ipFwdWorkerVlanIP2 = "192.168.2.1"
	ipFwdFrrRouterIP1  = "192.168.1.2"
	ipFwdFrrRouterIP2  = "192.168.2.2"

	ipFwdClientInternalIP1 = "172.16.0.1"
	ipFwdClientInternalIP2 = "172.16.1.1"
	ipFwdRouterInternalIP1 = "172.16.0.254"
	ipFwdRouterInternalIP2 = "172.16.1.254"
	ipFwdInternalSubnet1   = "172.16.0.0/24"
	ipFwdInternalSubnet2   = "172.16.1.0/24"

	ipFwdServiceIP1  = "3.3.3.5"
	ipFwdServiceIP2  = "4.4.4.5"
	ipFwdCurlTimeout = "3"

	ipFwdV6WorkerVlanIP1     = "2001:192:1::1"
	ipFwdV6WorkerVlanIP2     = "2001:192:2::1"
	ipFwdV6FrrRouterIP1      = "2001:192:1::2"
	ipFwdV6FrrRouterIP2      = "2001:192:2::2"
	ipFwdV6ClientInternalIP1 = "2001:172:1::1"
	ipFwdV6ClientInternalIP2 = "2001:172:2::1"
	ipFwdV6RouterInternalIP1 = "2001:172:1::254"
	ipFwdV6RouterInternalIP2 = "2001:172:2::254"
	ipFwdV6InternalSubnet1   = "2001:172:1::/64"
	ipFwdV6InternalSubnet2   = "2001:172:2::/64"
	ipFwdV6ServiceIP1        = "2001:3::5"
	ipFwdV6ServiceIP2        = "2001:4::5"
)

var _ = Describe("IP Forwarding per Interface", Ordered,
	Label(labelIPForwardingTestCases), ContinueOnFailure, func() {
		var (
			sriovInterfaces        []string
			originalIPForwarding   operatorv1.IPForwardingMode
			originalRoutingViaHost bool
			ipFwdVlanID1           uint16
			ipFwdVlanID2           uint16
			ipFwdVlanNADName1      string
			ipFwdVlanNADName2      string
			iface1Name             string
			iface2Name             string
			clientPod1             *pod.Builder
			clientPod2             *pod.Builder
			frrRouter1             *pod.Builder
			frrRouter2             *pod.Builder
		)

		BeforeAll(func() {
			By("Checking if cluster is SNO")

			if IsSNO {
				Skip("Skipping IP forwarding test on SNO cluster - requires 2+ workers")
			}

			validateEnvVarAndGetNodeList()

			By("Collecting SR-IOV interfaces for VLAN configuration")

			var err error

			sriovInterfaces, err = NetConfig.GetSriovInterfaces(2)
			if err != nil || len(sriovInterfaces) < 2 {
				Skip("Skipping: need at least 2 SR-IOV interfaces for IP forwarding test")
			}

			By("Reading VLAN ID from environment configuration")

			ipFwdVlanID1, err = NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN ID from ECO_CNF_CORE_NET_VLAN")

			ipFwdVlanID2 = ipFwdVlanID1 + 1
			ipFwdVlanNADName1 = fmt.Sprintf("external-%d", ipFwdVlanID1)
			ipFwdVlanNADName2 = fmt.Sprintf("external-%d", ipFwdVlanID2)
			iface1Name = fmt.Sprintf("%s.%d", sriovInterfaces[0], ipFwdVlanID1)
			iface2Name = fmt.Sprintf("%s.%d", sriovInterfaces[1], ipFwdVlanID2)

			By("Saving original network operator settings")

			networkOperator, err := network.PullOperator(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull network operator")

			if networkOperator.Definition.Spec.DefaultNetwork.OVNKubernetesConfig != nil &&
				networkOperator.Definition.Spec.DefaultNetwork.OVNKubernetesConfig.GatewayConfig != nil {
				originalIPForwarding = networkOperator.Definition.Spec.DefaultNetwork.
					OVNKubernetesConfig.GatewayConfig.IPForwarding
				originalRoutingViaHost = networkOperator.Definition.Spec.DefaultNetwork.
					OVNKubernetesConfig.GatewayConfig.RoutingViaHost
			}

			By("Enabling routingViaHost on the cluster network operator")

			_, err = networkOperator.SetLocalGWMode(true, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to enable routingViaHost")

			By("Ensuring global IP forwarding is enabled")

			if originalIPForwarding != operatorv1.IPForwardingGlobal {
				networkOperator, err = network.PullOperator(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull network operator")

				_, err = networkOperator.SetIPForwarding(operatorv1.IPForwardingGlobal, 10*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to set ipForwarding to Global")
			}

			By("Creating a new instance of MetalLB Speakers on workers")

			err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(
				tsparams.DefaultTimeout, workerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to recreate MetalLB daemonset")

			By("Creating bridge NetworkAttachmentDefinition for internal connectivity")

			createIPFwdBridgeNAD()

			By("Creating VLAN NetworkAttachmentDefinitions for external connectivity")

			createIPFwdVlanNAD(ipFwdVlanNADName1, sriovInterfaces[0], ipFwdVlanID1)
			createIPFwdVlanNAD(ipFwdVlanNADName2, sriovInterfaces[1], ipFwdVlanID2)

			By("Creating nginx server pods on the second worker node")

			setupNGNXPod("server1", workerNodeList[1].Object.Name, "server1")
			setupNGNXPod("server2", workerNodeList[1].Object.Name, "server2")
		})

		AfterAll(func() {
			By("Restoring network operator settings")

			networkOperator, err := network.PullOperator(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull network operator for restore")

			_, err = networkOperator.SetIPForwarding(originalIPForwarding, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore ipForwarding")

			_, err = networkOperator.SetLocalGWMode(originalRoutingViaHost, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore routingViaHost")

			resetOperatorAndTestNS()
		})

		Context("IPv4", Ordered, func() {
			var nncpPolicy *nmstate.PolicyBuilder

			BeforeAll(func() {
				By("Creating NMState policy with VLAN interfaces on the second worker node")

				nncpPolicy = nmstate.NewPolicyBuilder(APIClient, ipFwdNNCPName,
					map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
					WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
					WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
					WithStaticRoute(ipFwdInternalSubnet1, ipFwdFrrRouterIP1, iface1Name, 150, 254).
					WithStaticRoute(ipFwdInternalSubnet2, ipFwdFrrRouterIP2, iface2Name, 150, 254)
				err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpPolicy)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NNCP with VLAN interfaces")

				By("Creating FRR ConfigMaps for the router pods")

				createIPFwdFRRConfigMap(ipFwdFRRConfigMap1,
					tsparams.RemoteBGPASN, ipFwdWorkerVlanIP1)
				createIPFwdFRRConfigMap(ipFwdFRRConfigMap2,
					ipFwdRemoteBGPASN2, ipFwdWorkerVlanIP2)

				By("Creating FRR router pods on the first worker node")

				frrRouter1 = createIPFwdFRRRouterPod("frr-router-1", workerNodeList[0].Object.Name,
					ipFwdFRRConfigMap1, ipFwdVlanNADName1, ipFwdFrrRouterIP1, ipFwdRouterInternalIP1,
					netparam.IPSubnetInt24)
				frrRouter2 = createIPFwdFRRRouterPod("frr-router-2", workerNodeList[0].Object.Name,
					ipFwdFRRConfigMap2, ipFwdVlanNADName2, ipFwdFrrRouterIP2, ipFwdRouterInternalIP2,
					netparam.IPSubnetInt24)

				By("Enabling IP forwarding on FRR router pod secondary interfaces")

				enableIPFwdOnPod(frrRouter1, false)
				enableIPFwdOnPod(frrRouter2, false)

				By("Verifying connectivity from FRR routers to node VLAN interfaces")

				verifyIPFwdPingConnectivity(frrRouter1, ipFwdWorkerVlanIP1)
				verifyIPFwdPingConnectivity(frrRouter2, ipFwdWorkerVlanIP2)

				By("Creating client pods on the first worker node")

				clientPod1 = createIPFwdClientPod("client-1", workerNodeList[0].Object.Name,
					ipFwdClientInternalIP1, ipFwdRouterInternalIP1,
					ipFwdServiceIP1, "192.168.1.0/24")
				clientPod2 = createIPFwdClientPod("client-2", workerNodeList[0].Object.Name,
					ipFwdClientInternalIP2, ipFwdRouterInternalIP2,
					ipFwdServiceIP2, "192.168.2.0/24")

				By("Creating IPAddressPools")

				ipPool3, err := metallb.NewIPAddressPoolBuilder(
					APIClient, ipFwdAddrPool3, NetConfig.MlbOperatorNamespace,
					[]string{fmt.Sprintf("%s-%s", ipFwdServiceIP1, ipFwdServiceIP1)}).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool 3")

				ipPool4, err := metallb.NewIPAddressPoolBuilder(
					APIClient, ipFwdAddrPool4, NetConfig.MlbOperatorNamespace,
					[]string{fmt.Sprintf("%s-%s", ipFwdServiceIP2, ipFwdServiceIP2)}).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPAddressPool 4")

				By("Creating LoadBalancer services")

				setupMetalLbService("service1", netparam.IPV4Family, "server1",
					ipPool3, corev1.ServiceExternalTrafficPolicyTypeCluster)
				setupMetalLbService("service2", netparam.IPV4Family, "server2",
					ipPool4, corev1.ServiceExternalTrafficPolicyTypeCluster)

				By("Creating BGP peers targeting the second worker node")

				workerNode1Selector := map[string]string{
					netparam.LabelHostName: workerNodeList[1].Definition.Name}

				_, err = metallb.NewBPGPeerBuilder(
					APIClient, ipFwdBGPPeer1, NetConfig.MlbOperatorNamespace,
					ipFwdFrrRouterIP1, tsparams.LocalBGPASN, tsparams.RemoteBGPASN).
					WithRouterID(ipFwdRouterID).
					WithNodeSelector(workerNode1Selector).
					WithPassword(tsparams.BGPPassword).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create BGP peer 1")

				_, err = metallb.NewBPGPeerBuilder(
					APIClient, ipFwdBGPPeer2, NetConfig.MlbOperatorNamespace,
					ipFwdFrrRouterIP2, tsparams.LocalBGPASN, ipFwdRemoteBGPASN2).
					WithRouterID(ipFwdRouterID).
					WithNodeSelector(workerNode1Selector).
					WithPassword(tsparams.BGPPassword).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create BGP peer 2")

				By("Creating BGP advertisements")

				_, err = metallb.NewBGPAdvertisementBuilder(
					APIClient, ipFwdBGPAdv1, NetConfig.MlbOperatorNamespace).
					WithIPAddressPools([]string{ipFwdAddrPool3}).
					WithPeers([]string{ipFwdBGPPeer1}).
					WithCommunities([]string{tsparams.NoAdvertiseCommunity}).
					WithAggregationLength4(32).
					WithLocalPref(100).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create BGPAdvertisement 1")

				_, err = metallb.NewBGPAdvertisementBuilder(
					APIClient, ipFwdBGPAdv2, NetConfig.MlbOperatorNamespace).
					WithIPAddressPools([]string{ipFwdAddrPool4}).
					WithPeers([]string{ipFwdBGPPeer2}).
					WithCommunities([]string{tsparams.NoAdvertiseCommunity}).
					WithAggregationLength4(32).
					WithLocalPref(100).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create BGPAdvertisement 2")

				By("Verifying BGP sessions are established on the FRR router pods")

				verifyIPFwdBGPSessionsEstablished(frrRouter1, frrRouter2,
					ipFwdWorkerVlanIP1, ipFwdWorkerVlanIP2)

				By("Verifying initial connectivity with global IP forwarding enabled")

				verifyIPFwdTrafficPasses(clientPod1, ipFwdServiceIP1)
				verifyIPFwdTrafficPasses(clientPod2, ipFwdServiceIP2)

				By("Disabling global IP forwarding on the network operator")

				disableGlobalIPForwarding()
			})

			AfterAll(func() {
				cleanupIPFwdNNCP(nncpPolicy, ipFwdNNCPName, sriovInterfaces,
					ipFwdVlanID1, ipFwdVlanID2)
				cleanupIPFwdContextResources()
			})

			It("Basic functionality with MetalLB",
				reportxml.ID("80340"), func() {
					By("Verifying no traffic passes with global IP forwarding disabled")

					verifyIPFwdTrafficFails(clientPod1, ipFwdServiceIP1)
					verifyIPFwdTrafficFails(clientPod2, ipFwdServiceIP2)

					By("Enabling IP forwarding on the first VLAN interface via NNCP update")

					nncpPolicy = nmstate.NewPolicyBuilder(APIClient, ipFwdNNCPName,
						map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
						WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
						WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
						WithOptions(netnmstate.WithInterfaceIPv4Forwarding(iface1Name, true)).
						WithStaticRoute(ipFwdInternalSubnet1, ipFwdFrrRouterIP1, iface1Name, 150, 254).
						WithStaticRoute(ipFwdInternalSubnet2, ipFwdFrrRouterIP2, iface2Name, 150, 254)
					err := netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpPolicy)
					Expect(err).ToNot(HaveOccurred(), "Failed to update NNCP with forwarding on interface 1")

					By("Verifying IP forwarding is enabled on the first VLAN interface")

					verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface1Name, "1")

					By("Verifying traffic passes on the first path only")

					verifyIPFwdTrafficPasses(clientPod1, ipFwdServiceIP1)
					verifyIPFwdTrafficFails(clientPod2, ipFwdServiceIP2)

					By("Enabling IP forwarding on both VLAN interfaces via NNCP update")

					nncpPolicy = nmstate.NewPolicyBuilder(APIClient, ipFwdNNCPName,
						map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
						WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
						WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
						WithOptions(
							netnmstate.WithInterfaceIPv4Forwarding(iface1Name, true),
							netnmstate.WithInterfaceIPv4Forwarding(iface2Name, true)).
						WithStaticRoute(ipFwdInternalSubnet1, ipFwdFrrRouterIP1, iface1Name, 150, 254).
						WithStaticRoute(ipFwdInternalSubnet2, ipFwdFrrRouterIP2, iface2Name, 150, 254)
					err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpPolicy)
					Expect(err).ToNot(HaveOccurred(), "Failed to update NNCP with forwarding on both interfaces")

					By("Verifying IP forwarding is enabled on both VLAN interfaces")

					verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface1Name, "1")
					verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface2Name, "1")

					By("Verifying traffic passes on both paths")

					verifyIPFwdTrafficPasses(clientPod1, ipFwdServiceIP1)
					verifyIPFwdTrafficPasses(clientPod2, ipFwdServiceIP2)

					By("Disabling IP forwarding on both VLAN interfaces via NNCP update")

					nncpPolicy = nmstate.NewPolicyBuilder(APIClient, ipFwdNNCPName,
						map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
						WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
						WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
						WithOptions(
							netnmstate.WithInterfaceIPv4Forwarding(iface1Name, false),
							netnmstate.WithInterfaceIPv4Forwarding(iface2Name, false)).
						WithStaticRoute(ipFwdInternalSubnet1, ipFwdFrrRouterIP1, iface1Name, 150, 254).
						WithStaticRoute(ipFwdInternalSubnet2, ipFwdFrrRouterIP2, iface2Name, 150, 254)
					err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpPolicy)
					Expect(err).ToNot(HaveOccurred(), "Failed to update NNCP to disable forwarding")

					By("Verifying IP forwarding is disabled on both VLAN interfaces")

					verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface1Name, "0")
					verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface2Name, "0")

					By("Verifying no traffic passes after disabling IP forwarding")

					verifyIPFwdTrafficFails(clientPod1, ipFwdServiceIP1)
					verifyIPFwdTrafficFails(clientPod2, ipFwdServiceIP2)
				})

			It("Persistence after reboot",
				reportxml.ID("80383"), func() {
					By("Enabling IP forwarding on the first VLAN interface via NNCP update")

					nncpPolicy = nmstate.NewPolicyBuilder(APIClient, ipFwdNNCPName,
						map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
						WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
						WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
						WithOptions(netnmstate.WithInterfaceIPv4Forwarding(iface1Name, true)).
						WithStaticRoute(ipFwdInternalSubnet1, ipFwdFrrRouterIP1, iface1Name, 150, 254).
						WithStaticRoute(ipFwdInternalSubnet2, ipFwdFrrRouterIP2, iface2Name, 150, 254)
					err := netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpPolicy)
					Expect(err).ToNot(HaveOccurred(), "Failed to update NNCP with forwarding on interface 1")

					By("Verifying traffic passes on the first path and fails on the second")

					verifyIPFwdTrafficPasses(clientPod1, ipFwdServiceIP1)
					verifyIPFwdTrafficFails(clientPod2, ipFwdServiceIP2)

					By("Rebooting the worker node with VLAN interfaces")

					workerNode1Name := workerNodeList[1].Definition.Name
					_, err = cluster.ExecCmdWithStdout(APIClient, "reboot -f",
						metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s",
							netparam.LabelHostName, workerNode1Name)})
					Expect(err).ToNot(HaveOccurred(), "Failed to reboot worker node %s", workerNode1Name)

					By("Waiting for MachineConfigPool to stabilize after reboot")

					err = cluster.WaitForMcpStable(APIClient, 35*time.Minute, 1*time.Minute, NetConfig.CnfMcpLabel)
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for MCP to be stable after reboot")

					By("Verifying IP forwarding persisted on the first VLAN interface after reboot")

					verifyIPForwardingOnWorker(workerNode1Name, iface1Name, "1")

					By("Verifying BGP sessions are re-established after reboot")

					verifyIPFwdBGPSessionsEstablished(frrRouter1, frrRouter2,
						ipFwdWorkerVlanIP1, ipFwdWorkerVlanIP2)

					By("Verifying traffic still passes on the first path after reboot")

					verifyIPFwdTrafficPasses(clientPod1, ipFwdServiceIP1)

					By("Verifying traffic still fails on the second path after reboot")

					verifyIPFwdTrafficFails(clientPod2, ipFwdServiceIP2)
				})
		})

		Context("IPv6", Ordered, func() {
			var nncpV6Policy *nmstate.PolicyBuilder

			BeforeAll(func() {
				By("Ensuring global IP forwarding is Restricted for IPv6 non-interference tests")

				disableGlobalIPForwarding()

				By("Creating NMState policy with IPv6 VLAN interfaces on the second worker node")

				nncpV6Policy = nmstate.NewPolicyBuilder(APIClient, ipFwdV6NNCPName,
					map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
					WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
					WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
					WithStaticRoute(ipFwdV6InternalSubnet1, ipFwdV6FrrRouterIP1, iface1Name, 150, 254).
					WithStaticRoute(ipFwdV6InternalSubnet2, ipFwdV6FrrRouterIP2, iface2Name, 150, 254)
				err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpV6Policy)
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 NNCP with VLAN interfaces")

				By("Creating FRR ConfigMaps for the IPv6 router pods")

				createIPFwdFRRConfigMap(ipFwdFRRConfigMap1,
					tsparams.RemoteBGPASN, ipFwdV6WorkerVlanIP1)
				createIPFwdFRRConfigMap(ipFwdFRRConfigMap2,
					ipFwdRemoteBGPASN2, ipFwdV6WorkerVlanIP2)

				By("Creating FRR router pods on the first worker node with IPv6 addresses")

				frrRouter1 = createIPFwdFRRRouterPod("frr-router-1", workerNodeList[0].Object.Name,
					ipFwdFRRConfigMap1, ipFwdVlanNADName1, ipFwdV6FrrRouterIP1, ipFwdV6RouterInternalIP1,
					netparam.IPSubnetInt64)
				frrRouter2 = createIPFwdFRRRouterPod("frr-router-2", workerNodeList[0].Object.Name,
					ipFwdFRRConfigMap2, ipFwdVlanNADName2, ipFwdV6FrrRouterIP2, ipFwdV6RouterInternalIP2,
					netparam.IPSubnetInt64)

				By("Enabling IPv6 forwarding on FRR router pod secondary interfaces")

				enableIPFwdOnPod(frrRouter1, true)
				enableIPFwdOnPod(frrRouter2, true)

				By("Verifying IPv6 connectivity from FRR routers to node VLAN interfaces")

				verifyIPFwdPingConnectivity(frrRouter1, ipFwdV6WorkerVlanIP1)
				verifyIPFwdPingConnectivity(frrRouter2, ipFwdV6WorkerVlanIP2)

				By("Creating IPv6 client pods on the first worker node")

				clientPod1 = createIPFwdClientPod("client-1", workerNodeList[0].Object.Name,
					ipFwdV6ClientInternalIP1, ipFwdV6RouterInternalIP1,
					ipFwdV6ServiceIP1, ipFwdV6InternalSubnet1)
				clientPod2 = createIPFwdClientPod("client-2", workerNodeList[0].Object.Name,
					ipFwdV6ClientInternalIP2, ipFwdV6RouterInternalIP2,
					ipFwdV6ServiceIP2, ipFwdV6InternalSubnet2)

				By("Creating IPv6 IPAddressPools")

				ipPool3, err := metallb.NewIPAddressPoolBuilder(
					APIClient, ipFwdAddrPool3, NetConfig.MlbOperatorNamespace,
					[]string{fmt.Sprintf("%s-%s", ipFwdV6ServiceIP1, ipFwdV6ServiceIP1)}).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 IPAddressPool 3")

				ipPool4, err := metallb.NewIPAddressPoolBuilder(
					APIClient, ipFwdAddrPool4, NetConfig.MlbOperatorNamespace,
					[]string{fmt.Sprintf("%s-%s", ipFwdV6ServiceIP2, ipFwdV6ServiceIP2)}).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 IPAddressPool 4")

				By("Creating IPv6 LoadBalancer services")

				setupMetalLbService("service1", netparam.IPV6Family, "server1",
					ipPool3, corev1.ServiceExternalTrafficPolicyTypeCluster)
				setupMetalLbService("service2", netparam.IPV6Family, "server2",
					ipPool4, corev1.ServiceExternalTrafficPolicyTypeCluster)

				By("Creating BGP peers targeting the second worker node with IPv6 addresses")

				workerNode1Selector := map[string]string{
					netparam.LabelHostName: workerNodeList[1].Definition.Name}

				_, err = metallb.NewBPGPeerBuilder(
					APIClient, ipFwdBGPPeer1, NetConfig.MlbOperatorNamespace,
					ipFwdV6FrrRouterIP1, tsparams.LocalBGPASN, tsparams.RemoteBGPASN).
					WithRouterID(ipFwdRouterID).
					WithNodeSelector(workerNode1Selector).
					WithPassword(tsparams.BGPPassword).
					WithEBGPMultiHop(true).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 BGP peer 1")

				_, err = metallb.NewBPGPeerBuilder(
					APIClient, ipFwdBGPPeer2, NetConfig.MlbOperatorNamespace,
					ipFwdV6FrrRouterIP2, tsparams.LocalBGPASN, ipFwdRemoteBGPASN2).
					WithRouterID(ipFwdRouterID).
					WithNodeSelector(workerNode1Selector).
					WithPassword(tsparams.BGPPassword).
					WithEBGPMultiHop(true).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 BGP peer 2")

				By("Creating BGP advertisements with IPv6 aggregation")

				_, err = metallb.NewBGPAdvertisementBuilder(
					APIClient, ipFwdBGPAdv1, NetConfig.MlbOperatorNamespace).
					WithIPAddressPools([]string{ipFwdAddrPool3}).
					WithPeers([]string{ipFwdBGPPeer1}).
					WithCommunities([]string{tsparams.NoAdvertiseCommunity}).
					WithAggregationLength6(128).
					WithLocalPref(100).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 BGPAdvertisement 1")

				_, err = metallb.NewBGPAdvertisementBuilder(
					APIClient, ipFwdBGPAdv2, NetConfig.MlbOperatorNamespace).
					WithIPAddressPools([]string{ipFwdAddrPool4}).
					WithPeers([]string{ipFwdBGPPeer2}).
					WithCommunities([]string{tsparams.NoAdvertiseCommunity}).
					WithAggregationLength6(128).
					WithLocalPref(100).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 BGPAdvertisement 2")

				By("Verifying BGP sessions are established on the FRR router pods")

				verifyIPFwdBGPSessionsEstablished(frrRouter1, frrRouter2,
					ipFwdV6WorkerVlanIP1, ipFwdV6WorkerVlanIP2)

				By("Verifying initial IPv6 connectivity")

				verifyIPFwdTrafficPasses(clientPod1, ipFwdV6ServiceIP1)
				verifyIPFwdTrafficPasses(clientPod2, ipFwdV6ServiceIP2)
			})

			AfterAll(func() {
				cleanupIPFwdNNCP(nncpV6Policy, ipFwdV6NNCPName, sriovInterfaces,
					ipFwdVlanID1, ipFwdVlanID2)
				cleanupIPFwdContextResources()
			})

			It("IPv6 is not affected",
				reportxml.ID("80457"), func() {
					By("Attempting to enable IPv6 forwarding on the first VLAN interface via NNCP update")

					nncpV6Policy = nmstate.NewPolicyBuilder(APIClient, ipFwdV6NNCPName,
						map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
						WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
						WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
						WithOptions(netnmstate.WithInterfaceIPv6Forwarding(iface1Name, true)).
						WithStaticRoute(ipFwdV6InternalSubnet1, ipFwdV6FrrRouterIP1, iface1Name, 150, 254).
						WithStaticRoute(ipFwdV6InternalSubnet2, ipFwdV6FrrRouterIP2, iface2Name, 150, 254)

					var err error

					nncpV6Policy, err = netnmstate.UpdatePolicyAndWaitUntilItsDegraded(
						netparam.DefaultTimeout, nncpV6Policy)
					Expect(err).ToNot(HaveOccurred(),
						"NNCP should become Degraded when IPv6 forwarding is configured")

					By("Restoring NNCP by removing IPv6 forwarding configuration")

					nncpV6Policy = nmstate.NewPolicyBuilder(APIClient, ipFwdV6NNCPName,
						map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
						WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
						WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
						WithStaticRoute(ipFwdV6InternalSubnet1, ipFwdV6FrrRouterIP1, iface1Name, 150, 254).
						WithStaticRoute(ipFwdV6InternalSubnet2, ipFwdV6FrrRouterIP2, iface2Name, 150, 254)
					err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpV6Policy)
					Expect(err).ToNot(HaveOccurred(), "Failed to restore NNCP after IPv6 forwarding test")

					By("Verifying IPv6 traffic still passes after restoring NNCP")

					verifyIPFwdTrafficPasses(clientPod1, ipFwdV6ServiceIP1)
					verifyIPFwdTrafficPasses(clientPod2, ipFwdV6ServiceIP2)

					By("Enabling IPv4 forwarding on the first VLAN interface via NNCP update")

					nncpV6Policy = nmstate.NewPolicyBuilder(APIClient, ipFwdV6NNCPName,
						map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
						WithVlanInterfaceIP(sriovInterfaces[0], ipFwdWorkerVlanIP1, ipFwdV6WorkerVlanIP1, ipFwdVlanID1).
						WithVlanInterfaceIP(sriovInterfaces[1], ipFwdWorkerVlanIP2, ipFwdV6WorkerVlanIP2, ipFwdVlanID2).
						WithOptions(netnmstate.WithInterfaceIPv4Forwarding(iface1Name, true)).
						WithStaticRoute(ipFwdV6InternalSubnet1, ipFwdV6FrrRouterIP1, iface1Name, 150, 254).
						WithStaticRoute(ipFwdV6InternalSubnet2, ipFwdV6FrrRouterIP2, iface2Name, 150, 254)
					err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncpV6Policy)
					Expect(err).ToNot(HaveOccurred(), "Failed to update NNCP with IPv4 forwarding")

					By("Verifying IPv4 forwarding is enabled on the first VLAN interface")

					verifyIPForwardingOnWorker(workerNodeList[1].Object.Name, iface1Name, "1")

					By("Verifying IPv6 traffic still passes on both paths after enabling IPv4 forwarding")

					verifyIPFwdTrafficPasses(clientPod1, ipFwdV6ServiceIP1)
					verifyIPFwdTrafficPasses(clientPod2, ipFwdV6ServiceIP2)
				})
		})
	})

func cleanupIPFwdNNCP(
	nncpPolicy *nmstate.PolicyBuilder, nncpName string,
	sriovInterfaces []string, vlanID1, vlanID2 uint16,
) {
	By("Reverting VLAN interfaces via NMState absent state")

	if nncpPolicy != nil && nncpPolicy.Exists() && len(sriovInterfaces) >= 2 {
		iface1Name := fmt.Sprintf("%s.%d", sriovInterfaces[0], vlanID1)
		iface2Name := fmt.Sprintf("%s.%d", sriovInterfaces[1], vlanID2)

		nncpPolicy = nmstate.NewPolicyBuilder(APIClient, nncpName,
			map[string]string{netparam.LabelHostName: workerNodeList[1].Definition.Name}).
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

func cleanupIPFwdContextResources() {
	By("Cleaning MetalLB operator resources")

	metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")

	err = metalLbNs.CleanObjects(
		tsparams.DefaultTimeout,
		metallb.GetBGPPeerGVR(),
		metallb.GetBFDProfileGVR(),
		metallb.GetL2AdvertisementGVR(),
		metallb.GetBGPAdvertisementGVR(),
		metallb.GetIPAddressPoolGVR(),
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to clean metallb operator resources")

	By("Cleaning test namespace services and configmaps")

	err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
		tsparams.DefaultTimeout,
		service.GetGVR(),
		configmap.GetGVR(),
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace services and configmaps")

	By("Deleting FRR router and client pods")

	for _, podName := range []string{"frr-router-1", "frr-router-2", "client-1", "client-2"} {
		podObj, pullErr := pod.Pull(APIClient, podName, tsparams.TestNamespaceName)
		if pullErr == nil {
			_, _ = podObj.DeleteAndWait(tsparams.DefaultTimeout)
		}
	}
}

func disableGlobalIPForwarding() {
	networkOperator, err := network.PullOperator(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull network operator")

	_, err = networkOperator.SetIPForwarding(operatorv1.IPForwardingRestricted, 10*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to set ipForwarding to Restricted")
}

func createIPFwdBridgeNAD() {
	bridgePlugin, err := nad.NewMasterBridgePlugin(ipFwdBridgeNADName, "br0").
		WithIPAM(&nad.IPAM{Type: "static"}).
		GetMasterPluginConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to build bridge plugin config")

	_, err = nad.NewBuilder(APIClient, ipFwdBridgeNADName, tsparams.TestNamespaceName).
		WithMasterPlugin(bridgePlugin).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create bridge NAD")
}

func createIPFwdVlanNAD(name, masterInterface string, vlanID uint16) {
	vlanPlugin, err := nad.NewMasterVlanPlugin(name, vlanID).
		WithMasterInterface(masterInterface).
		WithIPAM(&nad.IPAM{Type: "static"}).
		GetMasterPluginConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to build VLAN plugin config for %s", name)

	_, err = nad.NewBuilder(APIClient, name, tsparams.TestNamespaceName).
		WithMasterPlugin(vlanPlugin).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create VLAN NAD %s", name)
}

func createIPFwdFRRConfigMap(name string, bgpASN int, neighborIP string) {
	frrConf := frr.DefineBGPConfig(
		bgpASN, tsparams.LocalBGPASN, []string{neighborIP}, false, false)

	configMapData := frrconfig.DefineBaseConfig(frrconfig.DaemonsFile, frrConf, "")

	_, err := configmap.NewBuilder(APIClient, name, tsparams.TestNamespaceName).
		WithData(configMapData).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create FRR configmap %s", name)
}

func enableIPFwdOnPod(frrPod *pod.Builder, ipv6 bool) {
	var cmd string
	if ipv6 {
		cmd = "sysctl -w net.ipv6.conf.all.forwarding=1"
	} else {
		cmd = "echo 1 > /proc/sys/net/ipv4/conf/net1/forwarding && " +
			"echo 1 > /proc/sys/net/ipv4/conf/net2/forwarding"
	}

	_, err := frrPod.ExecCommand(
		[]string{"/bin/sh", "-c", cmd},
		tsparams.FRRSecondContainerName)
	Expect(err).ToNot(HaveOccurred(), "Failed to enable IP forwarding on pod %s", frrPod.Definition.Name)
}

func createIPFwdClientPod(name, nodeName, clientIP, gatewayIP, serviceIP, vlanSubnet string) *pod.Builder {
	isIPv6 := strings.Contains(clientIP, ":")

	prefixLen := netparam.IPSubnetInt24
	ipCmd := "ip"
	hostPrefix := "/32"

	if isIPv6 {
		prefixLen = netparam.IPSubnetInt64
		ipCmd = "ip -6"
		hostPrefix = "/128"
	}

	clientAnnotation := pod.StaticIPAnnotation(
		ipFwdBridgeNADName, []string{fmt.Sprintf("%s/%d", clientIP, prefixLen)})

	cmd := fmt.Sprintf(
		"%s route del default 2>/dev/null; "+
			"%s route add %s%s via %s; "+
			"%s route add %s via %s; "+
			"sleep infinity",
		ipCmd, ipCmd, serviceIP, hostPrefix, gatewayIP, ipCmd, vlanSubnet, gatewayIP)

	clientPod, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithSecondaryNetwork(clientAnnotation).
		RedefineDefaultCMD([]string{"/bin/sh", "-c", cmd}).
		WithPrivilegedFlag().
		CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod %s", name)

	return clientPod
}

func verifyIPFwdTrafficPasses(clientPod *pod.Builder, serviceIP string) {
	curlTarget := formatCurlTarget(serviceIP)

	Eventually(func() error {
		output, err := clientPod.ExecCommand(
			[]string{"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
				curlTarget, "--connect-timeout", ipFwdCurlTimeout})
		if err != nil {
			return fmt.Errorf("curl failed: %w", err)
		}

		if strings.TrimSpace(output.String()) != "200" {
			return fmt.Errorf("expected HTTP 200, got %s", output.String())
		}

		return nil
	}, 15*time.Second, 5*time.Second).Should(Succeed(),
		"Traffic should pass from %s to %s", clientPod.Definition.Name, serviceIP)
}

func verifyIPFwdTrafficFails(clientPod *pod.Builder, serviceIP string) {
	curlTarget := formatCurlTarget(serviceIP)

	Consistently(func() error {
		_, err := clientPod.ExecCommand(
			[]string{"curl", "-s", "-o", "/dev/null",
				curlTarget, "--connect-timeout", ipFwdCurlTimeout})

		return err
	}, 12*time.Second, 4*time.Second).Should(HaveOccurred(),
		"Traffic should NOT pass from %s to %s", clientPod.Definition.Name, serviceIP)
}

func formatCurlTarget(ip string) string {
	if strings.Contains(ip, ":") {
		return fmt.Sprintf("http://[%s]", ip)
	}

	return ip
}

func createIPFwdFRRRouterPod(
	name, nodeName, configMapName, vlanNADName, routerExternalIP, routerInternalIP string,
	prefixLen int,
) *pod.Builder {
	annotation := pod.StaticIPAnnotation(
		vlanNADName, []string{fmt.Sprintf("%s/%d", routerExternalIP, prefixLen)})
	annotation = append(annotation, pod.StaticIPAnnotation(
		ipFwdBridgeNADName, []string{fmt.Sprintf("%s/%d", routerInternalIP, prefixLen)})...)

	return createFrrPod(nodeName, configMapName, []string{}, annotation, name)
}

func verifyIPFwdPingConnectivity(frrPod *pod.Builder, targetIP string) {
	Eventually(func() error {
		output, err := frrPod.ExecCommand(
			[]string{"ping", "-c", "3", "-W", "2", targetIP},
			tsparams.FRRSecondContainerName)
		if err != nil {
			return fmt.Errorf("ping from %s to %s failed: %s %w",
				frrPod.Definition.Name, targetIP, output.String(), err)
		}

		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed(),
		"%s cannot reach %s", frrPod.Definition.Name, targetIP)
}

func verifyIPFwdBGPSessionsEstablished(
	frrRouter1, frrRouter2 *pod.Builder, neighborIP1, neighborIP2 string,
) {
	Eventually(func() bool {
		state1, _ := frr.BGPNeighborshipHasState(frrRouter1, neighborIP1, "Established")
		state2, _ := frr.BGPNeighborshipHasState(frrRouter2, neighborIP2, "Established")

		return state1 && state2
	}, 4*time.Minute, tsparams.DefaultRetryInterval).Should(BeTrue(),
		"BGP sessions not established on FRR router pods")
}

func verifyIPForwardingOnWorker(nodeName, ifaceName, expectedValue string) {
	speakerPods, err := pod.List(APIClient, NetConfig.MlbOperatorNamespace,
		metav1.ListOptions{LabelSelector: tsparams.SpeakerLabel})
	Expect(err).ToNot(HaveOccurred(), "Failed to list speaker pods")

	for _, speakerPod := range speakerPods {
		if speakerPod.Object.Spec.NodeName == nodeName {
			output, err := speakerPod.ExecCommand(
				[]string{"cat", fmt.Sprintf("/proc/sys/net/ipv4/conf/%s/forwarding", ifaceName)},
				"speaker")
			Expect(err).ToNot(HaveOccurred(),
				"Failed to read ip forwarding flag on %s", ifaceName)
			Expect(strings.TrimSpace(output.String())).To(Equal(expectedValue),
				"IP forwarding on %s should be %s", ifaceName, expectedValue)

			return
		}
	}

	Fail(fmt.Sprintf("Speaker pod not found on node %s", nodeName))
}
