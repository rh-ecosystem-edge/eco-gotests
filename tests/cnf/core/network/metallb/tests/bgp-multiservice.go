package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	netcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("MetalLB BGP", Ordered, Label(tsparams.LabelBGPTestCases), ContinueOnFailure, func() {
	var (
		err           error
		AddressPoolS1 = []string{"4.4.4.100", "4.4.4.101"}
		AddressPoolS2 = []string{"5.5.5.100", "5.5.5.101"}
	)

	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

		By("Creating a new instance of MetalLB Speakers on workers")
		err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}

		By("Cleaning MetalLb operator namespace")
		metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
		err = metalLbNs.CleanObjects(
			tsparams.DefaultTimeout,
			metallb.GetBGPPeerGVR(),
			metallb.GetBFDProfileGVR(),
			metallb.GetBGPAdvertisementGVR(),
			metallb.GetIPAddressPoolGVR(),
			metallb.GetMetalLbIoGVR())
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

	It("Multi-Service Validation", reportxml.ID("47182"), func() {
		By("Creating an IPAddressPool and BGPAdvertisement for service 1")
		ipAddressPool1 := setupBgpAdvertisementAndIPAddressPool(
			tsparams.BGPAdvAndAddressPoolName, AddressPoolS1, netparam.IPSubnetInt32)

		By("Creating an IPAddressPool and BGPAdvertisement for service 2")
		ipAddressPool2 := setupBgpAdvertisementAndIPAddressPool("bgp-test2", AddressPoolS2, netparam.IPSubnetInt32)

		By("Creating service 1 with 2 backend pods")
		setupMetalLbService(
			tsparams.MetallbServiceName,
			netparam.IPV4Family,
			tsparams.LabelValue1,
			ipAddressPool1,
			corev1.ServiceExternalTrafficPolicyTypeCluster)

		setupNGNXPod("mlbnginxtpod1"+workerNodeList[0].Definition.Name,
			workerNodeList[0].Definition.Name,
			tsparams.LabelValue1)
		setupNGNXPod("mlbnginxtpod1"+workerNodeList[1].Definition.Name,
			workerNodeList[1].Definition.Name,
			tsparams.LabelValue1)

		By("Creating service 2 with 2 backend pods")
		setupMetalLbService(
			tsparams.MetallbServiceName2,
			netparam.IPV4Family,
			tsparams.LabelValue2,
			ipAddressPool2,
			corev1.ServiceExternalTrafficPolicyTypeCluster)

		setupNGNXPod("mlbnginxtpod2"+workerNodeList[0].Definition.Name,
			workerNodeList[0].Definition.Name,
			tsparams.LabelValue2)
		setupNGNXPod("mlbnginxtpod2"+workerNodeList[1].Definition.Name,
			workerNodeList[1].Definition.Name,
			tsparams.LabelValue2)

		By("Creating an IBGP BGP Peer")
		frrk8sPods := verifyAndCreateFRRk8sPodList()
		createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], "",
			tsparams.LocalBGPASN, false, 0,
			frrk8sPods)

		By("Creating configMap for external FRR Pod")
		masterConfigMap := createConfigMap(tsparams.LocalBGPASN, ipv4NodeAddrList, false, false)

		By("Creating External NAD for external FRR pod")
		createExternalNad(frrconfig.ExternalMacVlanNADName)

		By("Creating static ip annotation for external FRR pod")
		staticIPAnnotation := pod.StaticIPAnnotation(
			frrconfig.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[0], netparam.IPSubnet24)})

		By("Creating external FRR Pod")
		frrPod := createFrrPod(
			masterNodeList[0].Object.Name, masterConfigMap.Definition.Name, []string{}, staticIPAnnotation)

		By("Checking that BGP session is established and up")
		verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, netcmd.RemovePrefixFromIPList(ipv4NodeAddrList))

		By("Validating BGP routes to service")
		validatePrefix(
			frrPod, netparam.IPV4Family, netparam.IPSubnetInt32, removePrefixFromIPList(ipv4NodeAddrList), AddressPoolS1)
		validatePrefix(
			frrPod, netparam.IPV4Family, netparam.IPSubnetInt32, removePrefixFromIPList(ipv4NodeAddrList), AddressPoolS2)

		By("Validating curl to service 1")
		httpTrafficValidation(frrPod, ipaddr.RemovePrefix(ipv4metalLbIPList[0]), AddressPoolS1[0])

		By("Validating curl to service 2")
		httpTrafficValidation(frrPod, ipaddr.RemovePrefix(ipv4metalLbIPList[0]), AddressPoolS2[0])
	})
})
