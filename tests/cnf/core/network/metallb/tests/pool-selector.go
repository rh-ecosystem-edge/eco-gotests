package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
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
			// To-Do: This should be removed once we have dual stack clusters for testing.
			// Also, the test procedure for IPv6 should be supported.
			if ipStack != netparam.IPV4Family {
				Skip("bgp test cases doesn't support ipv6 yet")
			}

			runPoolSelectorTests(ipStack, trafficPolicy, bgpASN, false)

		},
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPCluster,
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
		Entry("", netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
		Entry("", netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal)),
		Entry("", netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN)),
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster)),
	)

	DescribeTable("Allow two specific pools to BGP Peers", reportxml.ID("49837"),
		func(ipStack string, bgpASN int, trafficPolicy string) {
			// To-Do: This should be removed once we have dual stack clusters for testing.
			// Also, the test procedure for IPv6 should be supported.
			if ipStack != netparam.IPV4Family {
				Skip("bgp test cases doesn't support ipv6 yet")
			}

			runPoolSelectorTests(ipStack, trafficPolicy, bgpASN, true)
		},
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.IPV4Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.IPV4Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.IPV6Family, tsparams.LocalBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.LocalBGPASN))),
		Entry("", netparam.DualIPFamily, tsparams.LocalBGPASN, tsparams.ETPCluster,
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
		Entry("", netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", netparam.IPV6Family, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.IPV6Family),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPLocal,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPLocal),
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
		Entry("", netparam.DualIPFamily, tsparams.RemoteBGPASN, tsparams.ETPCluster,
			reportxml.SetProperty("TrafficPolicy", tsparams.ETPCluster),
			reportxml.SetProperty("IPStack", netparam.DualIPFamily),
			reportxml.SetProperty("BGPASN", fmt.Sprintf("%d", tsparams.RemoteBGPASN))),
	)
})

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

	ipPool1 := createIPAddressPool("pool1", tsparams.LBipv4Range1)

	var ipPool2 *metallb.IPAddressPoolBuilder

	if twoPools {
		ipPool2 = createIPAddressPool("pool2", tsparams.LBipv4Range2)
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

	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "", uint32(bgpASN),
		false, 0, frrk8sPods)
	createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName2, ipv4metalLbIPList[1], "", uint32(bgpASN),
		false, 0, frrk8sPods)

	By("Deploy test pods that runs Nginx server and SCTP server on worker0 & worker-1")

	setupNGNXPodAndSCTPServer("nginxpod1worker0", workerNodeList[0].Object.Name, tsparams.LabelValue1)
	setupNGNXPodAndSCTPServer("nginxpod1worker1", workerNodeList[1].Object.Name, tsparams.LabelValue1)

	By("Creating 2 Services for TCP and SCTP which has Nginx/SCTP server pods as endpoints")

	if !twoPools {
		setupMetalLbService(tsparams.MetallbServiceName, netparam.IPV4Family, tsparams.LabelValue1, ipPool1,
			corev1.ServiceExternalTrafficPolicyType(trafficPolicy))
	} else {
		setupMetalLbService(tsparams.MetallbServiceName, netparam.IPV4Family, tsparams.LabelValue1, ipPool2,
			corev1.ServiceExternalTrafficPolicyType(trafficPolicy))
	}

	tcpSvc, err := service.Pull(APIClient, tsparams.MetallbServiceName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName)

	sctpSvcPort, err := service.DefineServicePort(50000, 50000, "SCTP")
	Expect(err).ToNot(HaveOccurred(), "Failed to define service port")

	sctpSvc, err := service.NewBuilder(APIClient, tsparams.MetallbServiceName2, tsparams.TestNamespaceName,
		map[string]string{"app": tsparams.LabelValue1}, *sctpSvcPort).
		WithExternalTrafficPolicy(corev1.ServiceExternalTrafficPolicyType(trafficPolicy)).
		WithIPFamily([]corev1.IPFamily{corev1.IPFamily(ipStack)}, corev1.IPFamilyPolicySingleStack).
		WithAnnotation(map[string]string{"metallb.universe.tf/address-pool": ipPool1.Definition.Name}).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create LB Service")

	By("Creating Configmap for external FRR Pods")

	masterConfigMap := createConfigMap(bgpASN, ipv4NodeAddrList, false, false)

	By("Creating macvlan NAD for external FRR Pods")

	err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create a macvlan NAD")

	By("Creating FRR Pods on master-0 & master-1")

	extFrrPod1 := createFrrPod(masterNodeList[0].Object.Name, masterConfigMap.Object.Name, []string{},
		pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
			[]string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0], "24")}), "frr1")

	extFrrPod2 := createFrrPod(masterNodeList[1].Object.Name, masterConfigMap.Object.Name, []string{},
		pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
			[]string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[1], "24")}), "frr2")

	By("Checking that BGP session is established on external FRR Pod")
	verifyMetalLbBGPSessionsAreUPOnFrrPod(extFrrPod1, ipv4NodeAddrList)
	verifyMetalLbBGPSessionsAreUPOnFrrPod(extFrrPod2, ipv4NodeAddrList)

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
	httpTrafficValidation(extFrrPod2, ipaddr.RemovePrefix(ipv4metalLbIPList[1]),
		tcpSvc.Object.Status.LoadBalancer.Ingress[0].IP)
	validatePrefix(extFrrPod1, ipStack, 32, removePrefixFromIPList(ipv4NodeAddrList), tsparams.LBipv4Range1)

	if !twoPools {
		httpTrafficValidation(extFrrPod1, ipaddr.RemovePrefix(ipv4metalLbIPList[0]),
			tcpSvc.Object.Status.LoadBalancer.Ingress[0].IP)
		sctpTrafficValidation(extFrrPod2, sctpSvc.Object.Status.LoadBalancer.Ingress[0].IP,
			"50000", tsparams.FRRSecondContainerName)
		validatePrefix(extFrrPod2, ipStack, 32, removePrefixFromIPList(ipv4NodeAddrList), tsparams.LBipv4Range1)
	} else {
		validatePrefix(extFrrPod2, ipStack, 32, removePrefixFromIPList(ipv4NodeAddrList), tsparams.LBipv4Range2)
	}
}
