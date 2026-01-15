package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	mlbcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("BGP", Ordered, Label("pool-selector"), ContinueOnFailure, func() {
	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

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
			configmap.GetGVR(),
			nad.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
	})

	DescribeTable("Allow single pool to BGP Peers", reportxml.ID("49838"),
		func(ipStack string, bgpASN int, trafficPolicy string) {

			validateIPFamilySupport(ipStack)

			if ipStack == netparam.DualIPFamily {
				runPoolSelectorTestsDualStack(ipStack, trafficPolicy, bgpASN, false)
			} else {
				runPoolSelectorTests(ipStack, trafficPolicy, bgpASN, false)
			}
		},
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", netparam.IPV4Family, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", netparam.IPV4Family, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
	)

	DescribeTable("Allow two specific pools to BGP Peers", reportxml.ID("49837"),
		func(ipStack string, bgpASN int, trafficPolicy string) {
			validateIPFamilySupport(ipStack)

			if ipStack == netparam.DualIPFamily {
				runPoolSelectorTestsDualStack(ipStack, trafficPolicy, bgpASN, true)
			} else {
				runPoolSelectorTests(ipStack, trafficPolicy, bgpASN, true)
			}
		},
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.IPV4Family, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", netparam.IPV4Family, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", Label(tsparams.MetalLBIPv6), netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", Label(tsparams.MetalLBDual), netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
	)

	It("IPAddressPool: IPv4 and IPv6 routes simultaneously", reportxml.ID("85989"), func() {
		validateIPFamilySupport(netparam.DualIPFamily)

		By("Setting up test environment")
		_, extFrrPod, _ := setupTestEnv(netparam.IPV4Family, 32, false)

		By("Pulling IPAddressPool and adding IPv6 address range")
		ipAddressPool, err := metallb.PullAddressPool(APIClient, "address-pool", NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull IPAddressPool")
		ipv6Address := tsparams.LBipRange1[netparam.IPV6Family][0] + "/64"
		ipAddressPool.Definition.Spec.Addresses = append(ipAddressPool.Definition.Spec.Addresses, ipv6Address)
		ipAddressPool, err = ipAddressPool.Update(true)
		Expect(err).ToNot(HaveOccurred(), "Failed to update IPAddressPool")
		validateAddressPool("address-pool", mlbtypes.IPAddressPoolStatus{
			AvailableIPv4: 239,
			AvailableIPv6: 9223372036854775807,
			AssignedIPv4:  1,
			AssignedIPv6:  0,
		})

		By("Creating 10 IPv4 and 10 IPv6 services using the same IPAddressPool")
		for serviceIndex := 0; serviceIndex < 10; serviceIndex++ {
			ipv4ServiceName := fmt.Sprintf("%s-%d", tsparams.MetallbServiceName, serviceIndex)
			setupMetalLbService(ipv4ServiceName, netparam.IPV4Family, tsparams.LabelValue1, ipAddressPool,
				corev1.ServiceExternalTrafficPolicyTypeCluster)
			ipv6ServiceName := fmt.Sprintf("%s-%d", tsparams.MetallbServiceName2, serviceIndex)
			setupMetalLbService(ipv6ServiceName, netparam.IPV6Family, tsparams.LabelValue1, ipAddressPool,
				corev1.ServiceExternalTrafficPolicyTypeCluster)
		}

		By("Validating the service BGP statuses")
		for serviceIndex := 0; serviceIndex < 10; serviceIndex++ {
			ipv4ServiceName := fmt.Sprintf("%s-%d", tsparams.MetallbServiceName, serviceIndex)
			validateServiceBGPStatus(
				workerNodeList,
				ipv4ServiceName,
				tsparams.TestNamespaceName,
				[]string{tsparams.BgpPeerName1})
			ipv6ServiceName := fmt.Sprintf("%s-%d", tsparams.MetallbServiceName2, serviceIndex)
			validateServiceBGPStatus(
				workerNodeList,
				ipv6ServiceName,
				tsparams.TestNamespaceName,
				[]string{tsparams.BgpPeerName1})
		}

		By("Validating IPAddressPool status after creating services")
		validateAddressPool("address-pool", mlbtypes.IPAddressPoolStatus{
			AvailableIPv4: 229,
			AvailableIPv6: 9223372036854775797,
			AssignedIPv4:  11,
			AssignedIPv6:  10,
		})

		By("Curl the nginx pod from the external FRR pod")
		httpOutput, err := mlbcmd.Curl(extFrrPod, ipv4metalLbIPList[0],
			tsparams.LBipRange1[netparam.IPV4Family][0],
			netparam.IPV4Family,
			tsparams.FRRSecondContainerName)
		Expect(err).ToNot(HaveOccurred(), httpOutput)

		By("Deleting 5 IPv4 services")
		for i := 0; i < 5; i++ {
			svc, err := service.Pull(APIClient, fmt.Sprintf("%s-%d", tsparams.MetallbServiceName, i), tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull service")
			err = svc.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete service")
		}

		By("Validating IPAddressPool status after deleting services")
		validateAddressPool("address-pool", mlbtypes.IPAddressPoolStatus{
			AvailableIPv4: 234,
			AvailableIPv6: 9223372036854775797,
			AssignedIPv4:  6,
			AssignedIPv6:  10,
		})
	})
})

// validateIPFamilySupport checks if the cluster supports the requested IP family and skips the test if not.
func validateIPFamilySupport(ipStack string) {
	switch ipStack {
	case netparam.DualIPFamily:
		if !clusterSupportsIPv4() || !clusterSupportsIPv6() {
			Skip(fmt.Sprintf("Cluster does not support dual-stack (IPv4 + IPv6) - required for %s tests", ipStack))
		}
	case netparam.IPV6Family:
		if !clusterSupportsIPv6() {
			Skip(fmt.Sprintf("Cluster does not support IPv6 - required for %s tests", ipStack))
		}
	case netparam.IPV4Family:
		if !clusterSupportsIPv4() {
			Skip(fmt.Sprintf("Cluster does not support IPv4 - required for %s tests", ipStack))
		}
	default:
		Skip(fmt.Sprintf("Unknown IP stack type: %s", ipStack))
	}
}

func activateSCTPModuleOnMasterNodes() {
	_, err := cluster.ExecCmdWithStdout(APIClient, "modprobe sctp",
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.ControlPlaneLabelMap).String()})
	Expect(err).ToNot(HaveOccurred(), "Failed to activate sctp module on master nodes")

	By("Verifying SCTP module is active on master nodes")

	nodeOutputs, err := cluster.ExecCmdWithStdout(APIClient, "lsmod | grep sctp",
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.ControlPlaneLabelMap).String()})
	Expect(err).ToNot(HaveOccurred(), "Failed to verify sctp module status on master nodes")

	for node, output := range nodeOutputs {
		Expect(output).To(ContainSubstring("libcrc32c"), fmt.Sprintf("SCTP module is not active on %s", node))
	}
}

//nolint:unparam
func sctpTrafficValidation(testPod *pod.Builder, dstIPAddress, port string, containerName ...string) {
	Eventually(func() error {
		_, err := testPod.ExecCommand([]string{"testcmd", "-protocol=sctp", "-mtu=1200", "-interface=net1",
			fmt.Sprintf("-server=%s", dstIPAddress), fmt.Sprintf("-port=%s", port)}, containerName...)

		return err
	}, 15*time.Second, 5*time.Second).ShouldNot(HaveOccurred(), "SCTP traffic validation failure")
}

//nolint:funlen
func runPoolSelectorTests(ipStack, trafficPolicy string, bgpASN int, twoPools bool) {
	frrk8sPods := verifyAndCreateFRRk8sPodList()

	By("Creating two IPAddressPools")

	ipPool1 := createIPAddressPool("pool1", tsparams.LBipRange1[ipStack])
	validateAddressPool(ipPool1.Definition.Name, mlbtypes.IPAddressPoolStatus{
		AvailableIPv4: 240,
		AvailableIPv6: 0,
		AssignedIPv4:  0,
		AssignedIPv6:  0,
	})

	var ipPool2 *metallb.IPAddressPoolBuilder

	if twoPools {
		ipPool2 = createIPAddressPool("pool2", tsparams.LBipRange2[ipStack])
		validateAddressPool(ipPool2.Definition.Name, mlbtypes.IPAddressPoolStatus{
			AvailableIPv4: 239,
			AvailableIPv6: 0,
			AssignedIPv4:  0,
			AssignedIPv6:  0,
		})
	}

	By("Creating two BGPAdvertisements")

	setupBgpAdvertisement("bgpadv1", tsparams.NoAdvertiseCommunity, ipPool1.Object.Name,
		100, []string{tsparams.BgpPeerName1}, nil)

	if !twoPools {
		setupBgpAdvertisement("bgpadv2", tsparams.NoAdvertiseCommunity, ipPool1.Object.Name,
			400, []string{tsparams.BgpPeerName2}, nil)
	} else {
		setupBgpAdvertisement("bgpadv2", tsparams.NoAdvertiseCommunity, ipPool2.Object.Name,
			400, []string{tsparams.BgpPeerName2}, nil)
	}

	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, metallbAddrList[ipStack][0], "", uint32(bgpASN),
		false, 0, frrk8sPods)
	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName2, metallbAddrList[ipStack][1], "", uint32(bgpASN),
		false, 0, frrk8sPods)

	By("Deploy test pods that runs Nginx server and SCTP server on worker0 & worker-1")

	setupNGNXPodAndSCTPServer("nginxpod1worker0", workerNodeList[0].Object.Name, tsparams.LabelValue1,
		ipStack == netparam.IPV6Family)
	setupNGNXPodAndSCTPServer("nginxpod1worker1", workerNodeList[1].Object.Name, tsparams.LabelValue1,
		ipStack == netparam.IPV6Family)

	By("Creating 2 Services for TCP and SCTP which has Nginx/SCTP server pods as endpoints")

	if !twoPools {
		setupLoadBalancerService(tsparams.MetallbServiceName, ipStack, tsparams.LabelValue1, ipPool1,
			corev1.ServiceExternalTrafficPolicyType(trafficPolicy), corev1.ProtocolTCP, 80, 80)
	} else {
		setupLoadBalancerService(tsparams.MetallbServiceName, ipStack, tsparams.LabelValue1, ipPool2,
			corev1.ServiceExternalTrafficPolicyType(trafficPolicy), corev1.ProtocolTCP, 80, 80)
	}

	setupLoadBalancerService(tsparams.MetallbServiceName2, ipStack, tsparams.LabelValue1, ipPool1,
		corev1.ServiceExternalTrafficPolicyType(trafficPolicy), corev1.ProtocolSCTP, 50000, 50000)

	tcpSvc, err := service.Pull(APIClient, tsparams.MetallbServiceName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName)

	sctpSvc, err := service.Pull(APIClient, tsparams.MetallbServiceName2, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName2)

	By("Creating Configmap for external FRR Pods")

	masterConfigMap := createConfigMap(bgpASN, nodeAddrList[ipStack], false, false)

	By("Creating macvlan NAD for external FRR Pods")

	err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a macvlan NAD")

	By("Creating FRR Pods on master-0 & master-1")

	extFrrPod1 := createFrrPod(masterNodeList[0].Object.Name, masterConfigMap.Object.Name, []string{},
		pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
			[]string{fmt.Sprintf("%s/%s", metallbAddrList[ipStack][0], frrPodSubnet[ipStack])}), "frr1")

	extFrrPod2 := createFrrPod(masterNodeList[1].Object.Name, masterConfigMap.Object.Name, []string{},
		pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
			[]string{fmt.Sprintf("%s/%s", metallbAddrList[ipStack][1], frrPodSubnet[ipStack])}), "frr2")

	By("Checking that BGP session is established on external FRR Pod")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(extFrrPod1, nodeAddrList[ipStack])
	validateBGPSessionState("Established", "N/A", metallbAddrList[ipStack][0], []*nodes.Builder{workerNodeList[0]})
	verifyMetalLbBGPSessionsAreUPOnFrrPod(extFrrPod2, nodeAddrList[ipStack])
	validateBGPSessionState("Established", "N/A", metallbAddrList[ipStack][1], []*nodes.Builder{workerNodeList[1]})

	By("Checking HTTP traffic and SCTP traffic is running and Validating Prefixs on external FRR Pod")
	// Update service builders with latest status that includes LB IP.
	tcpSvc.Exists()
	sctpSvc.Exists()
	Expect(tcpSvc.Object.Status.LoadBalancer.Ingress).NotTo(BeEmpty(),
		"Load Balancer IP is not assigned to the tcp service")
	Expect(sctpSvc.Object.Status.LoadBalancer.Ingress).NotTo(BeEmpty(),
		"Load Balancer IP is not assigned to the sctp service")

	sctpTrafficValidation(extFrrPod1, sctpSvc.Object.Status.LoadBalancer.Ingress[0].IP,
		"50000", tsparams.FRRSecondContainerName)
	httpTrafficValidation(extFrrPod2, ipaddr.RemovePrefix(metallbAddrList[ipStack][1]),
		tcpSvc.Object.Status.LoadBalancer.Ingress[0].IP)
	validatePrefix(extFrrPod1, ipStack, defaultAggLen[ipStack], removePrefixFromIPList(nodeAddrList[ipStack]),
		tsparams.LBipRange1[ipStack])

	if !twoPools {
		httpTrafficValidation(extFrrPod1, ipaddr.RemovePrefix(metallbAddrList[ipStack][0]),
			tcpSvc.Object.Status.LoadBalancer.Ingress[0].IP)
		sctpTrafficValidation(extFrrPod2, sctpSvc.Object.Status.LoadBalancer.Ingress[0].IP,
			"50000", tsparams.FRRSecondContainerName)
		validatePrefix(extFrrPod2, ipStack, defaultAggLen[ipStack], removePrefixFromIPList(nodeAddrList[ipStack]),
			tsparams.LBipRange1[ipStack])
	} else {
		validatePrefix(extFrrPod2, ipStack, defaultAggLen[ipStack], removePrefixFromIPList(nodeAddrList[ipStack]),
			tsparams.LBipRange2[ipStack])
	}
}

//nolint:funlen
func runPoolSelectorTestsDualStack(ipStack, trafficPolicy string, bgpASN int, twoPools bool) {
	frrk8sPods := verifyAndCreateFRRk8sPodList()

	By("Creating IPAddressPool for dual stack")

	ipPool1 := createIPAddressPool("pool1", tsparams.LBipRange1[ipStack])
	validateAddressPool(ipPool1.Definition.Name, mlbtypes.IPAddressPoolStatus{
		AvailableIPv4: 240,
		AvailableIPv6: 0,
		AssignedIPv4:  0,
		AssignedIPv6:  0,
	})

	var ipPool2 *metallb.IPAddressPoolBuilder

	if twoPools {
		ipPool2 = createIPAddressPool("pool2", tsparams.LBipRange2[ipStack])
		validateAddressPool(ipPool2.Definition.Name, mlbtypes.IPAddressPoolStatus{
			AvailableIPv4: 240,
			AvailableIPv6: 0,
			AssignedIPv4:  0,
			AssignedIPv6:  0,
		})
	}

	By("Creating four BGPAdvertisements")

	setupBgpAdvertisement("bgpadv1", tsparams.NoAdvertiseCommunity, ipPool1.Object.Name,
		100, []string{tsparams.BgpPeerName1}, nil)
	setupBgpAdvertisement("bgpadv2", tsparams.NoAdvertiseCommunity, ipPool1.Object.Name,
		100, []string{tsparams.BgpPeerName2}, nil)

	if !twoPools {
		setupBgpAdvertisement("bgpadv3", tsparams.NoAdvertiseCommunity, ipPool1.Object.Name,
			100, []string{"bgppeer3"}, nil)
		setupBgpAdvertisement("bgpadv4", tsparams.NoAdvertiseCommunity, ipPool1.Object.Name,
			100, []string{"bgppeer4"}, nil)
	} else {
		setupBgpAdvertisement("bgpadv3", tsparams.NoAdvertiseCommunity, ipPool2.Object.Name,
			100, []string{"bgppeer3"}, nil)
		setupBgpAdvertisement("bgpadv4", tsparams.NoAdvertiseCommunity, ipPool2.Object.Name,
			100, []string{"bgppeer4"}, nil)
	}

	By("Creating BGP Peers")
	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, metallbAddrList[ipv4][0], "", uint32(bgpASN),
		false, 0, frrk8sPods)
	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName2, metallbAddrList[ipv6][0], "", uint32(bgpASN),
		false, 0, frrk8sPods)
	createBGPPeerAndVerifyIfItsReady("bgppeer3", metallbAddrList[ipv4][1], "", uint32(bgpASN),
		false, 0, frrk8sPods)
	createBGPPeerAndVerifyIfItsReady("bgppeer4", metallbAddrList[ipv6][1], "", uint32(bgpASN),
		false, 0, frrk8sPods)

	By("Deploy test pods that runs Nginx server and SCTP server on worker0 & worker1")

	setupNGNXPodAndSCTPServer("nginxpod1worker0", workerNodeList[0].Object.Name, tsparams.LabelValue1,
		ipStack == netparam.IPV6Family || ipStack == netparam.DualIPFamily)
	setupNGNXPodAndSCTPServer("nginxpod1worker1", workerNodeList[1].Object.Name, tsparams.LabelValue1,
		ipStack == netparam.IPV6Family || ipStack == netparam.DualIPFamily)

	By("Creating 2 Services for TCP and SCTP which has Nginx/SCTP server pods as endpoints")

	if !twoPools {
		setupLoadBalancerService(tsparams.MetallbServiceName, ipStack, tsparams.LabelValue1, ipPool1,
			corev1.ServiceExternalTrafficPolicyType(trafficPolicy), corev1.ProtocolTCP, 80, 80)
	} else {
		setupLoadBalancerService(tsparams.MetallbServiceName, ipStack, tsparams.LabelValue1, ipPool2,
			corev1.ServiceExternalTrafficPolicyType(trafficPolicy), corev1.ProtocolTCP, 80, 80)
	}

	setupLoadBalancerService(tsparams.MetallbServiceName2, ipStack, tsparams.LabelValue1, ipPool1,
		corev1.ServiceExternalTrafficPolicyType(trafficPolicy), corev1.ProtocolSCTP, 50000, 50000)

	tcpSvc, err := service.Pull(APIClient, tsparams.MetallbServiceName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName)

	sctpSvc, err := service.Pull(APIClient, tsparams.MetallbServiceName2, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName2)

	By("Creating Configmap for external FRR Pods")

	masterConfigMap := createConfigMap(bgpASN,
		[]string{nodeAddrList[ipv4][0], nodeAddrList[ipv4][1], nodeAddrList[ipv6][0], nodeAddrList[ipv6][1]},
		false, false)

	By("Creating macvlan NAD for external FRR Pods")

	err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a macvlan NAD")

	By("Creating FRR Pods on master-0 & master-1")

	extFrrPod1 := createFrrPod(masterNodeList[0].Object.Name, masterConfigMap.Object.Name, []string{},
		pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
			[]string{fmt.Sprintf("%s/%s", metallbAddrList[ipv4][0], "24"),
				fmt.Sprintf("%s/%s", metallbAddrList[ipv6][0], "64")}), "frr1")

	extFrrPod2 := createFrrPod(masterNodeList[1].Object.Name, masterConfigMap.Object.Name, []string{},
		pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
			[]string{fmt.Sprintf("%s/%s", metallbAddrList[ipv4][1], "24"),
				fmt.Sprintf("%s/%s", metallbAddrList[ipv6][1], "64")}), "frr2")

	By("Checking that BGP session is established on external FRR Pod")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(extFrrPod1, []string{nodeAddrList[ipv4][0], nodeAddrList[ipv6][0]})
	validateBGPSessionState("Established", "N/A", metallbAddrList[ipStack][0], []*nodes.Builder{workerNodeList[0]})
	verifyMetalLbBGPSessionsAreUPOnFrrPod(extFrrPod2, []string{nodeAddrList[ipv4][1], nodeAddrList[ipv6][1]})
	validateBGPSessionState("Established", "N/A", metallbAddrList[ipStack][1], []*nodes.Builder{workerNodeList[1]})

	By("Checking HTTP traffic and SCTP traffic is running and Validating Prefixs on external FRR Pod")
	// Update service builders with latest status that includes LB IP.
	tcpSvc.Exists()
	sctpSvc.Exists()
	Expect(len(tcpSvc.Object.Status.LoadBalancer.Ingress)).To(Equal(2),
		"Load Balancer IP is not assigned to the tcp service")
	Expect(len(sctpSvc.Object.Status.LoadBalancer.Ingress)).To(Equal(2),
		"Load Balancer IP is not assigned to the sctp service")

	for _, ingress := range sctpSvc.Object.Status.LoadBalancer.Ingress {
		if strings.Contains(ingress.IP, ":") {
			sctpTrafficValidation(extFrrPod1, ingress.IP,
				"50000", tsparams.FRRSecondContainerName)
		} else {
			// TO:DO: Remove this once we have a way to validate IPv4 traffic with sctp server listening on both ipv4 and ipv6
			continue
		}
	}

	for _, ingress := range tcpSvc.Object.Status.LoadBalancer.Ingress {
		if strings.Contains(ingress.IP, ":") {
			httpTrafficValidation(extFrrPod2, ipaddr.RemovePrefix(metallbAddrList[ipv6][1]),
				ingress.IP)
		} else {
			httpTrafficValidation(extFrrPod2, ipaddr.RemovePrefix(metallbAddrList[ipv4][1]),
				ingress.IP)
		}
	}

	validatePrefix(extFrrPod1, ipv4, defaultAggLen[ipv4], removePrefixFromIPList(nodeAddrList[ipv4]),
		tsparams.LBipRange1[ipv4])
	validatePrefix(extFrrPod1, ipv6, defaultAggLen[ipv6], removePrefixFromIPList(nodeAddrList[ipv6]),
		tsparams.LBipRange1[ipv6])

	if !twoPools {
		for _, ingress := range tcpSvc.Object.Status.LoadBalancer.Ingress {
			if strings.Contains(ingress.IP, ":") {
				httpTrafficValidation(extFrrPod1, ipaddr.RemovePrefix(metallbAddrList[ipv6][0]),
					ingress.IP)
			} else {
				httpTrafficValidation(extFrrPod1, ipaddr.RemovePrefix(metallbAddrList[ipv4][0]),
					ingress.IP)
			}
		}

		for _, ingress := range sctpSvc.Object.Status.LoadBalancer.Ingress {
			if strings.Contains(ingress.IP, ":") {
				sctpTrafficValidation(extFrrPod2, ingress.IP,
					"50000", tsparams.FRRSecondContainerName)
			} else {
				// TO:DO: Remove this once we have a way to validate IPv4 traffic with sctp server listening on both ipv4 and ipv6
				continue
			}
		}

		validatePrefix(extFrrPod2, ipv4, defaultAggLen[ipv4], removePrefixFromIPList(nodeAddrList[ipv4]),
			tsparams.LBipRange1[ipv4])
		validatePrefix(extFrrPod2, ipv6, defaultAggLen[ipv6], removePrefixFromIPList(nodeAddrList[ipv6]),
			tsparams.LBipRange1[ipv6])
	} else {
		validatePrefix(extFrrPod2, ipv4, defaultAggLen[ipv4], removePrefixFromIPList(nodeAddrList[ipv4]),
			tsparams.LBipRange2[ipv4])
		validatePrefix(extFrrPod2, ipv6, defaultAggLen[ipv6], removePrefixFromIPList(nodeAddrList[ipv6]),
			tsparams.LBipRange2[ipv6])
	}
}
