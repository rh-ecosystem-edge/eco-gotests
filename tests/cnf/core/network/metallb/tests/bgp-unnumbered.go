package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netnmstate"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/frr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BGP Unnumbered", Ordered, Label(tsparams.LabelBGPUnnumbered),
	ContinueOnFailure, func() {
		var (
			frrk8sPods                   []*pod.Builder
			interfacesUnderTest          []string
			linkLocalAddress             string
			nodeNetConfigPolicyName      = "eth-int-worker0"
			ipAddressPoolRange           = []string{"3.3.3.1", "3.3.3.240"}
			externalAdvertisedIPv4Routes = []string{"192.168.100.0/24", "192.168.200.0/24"}
			externalAdvertisedIPv6Routes = []string{"2001:100::0/64", "2001:200::0/64"}
		)

		BeforeAll(func() {
			validateEnvVarAndGetNodeList()

			By("Creating a new instance of MetalLB Speakers on workers")
			err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to create metalLb daemonset")

			By("Collecting interface information to enable a node ethernet interface")
			interfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By(fmt.Sprintf("nmstate policy configuring ethernet interface %s on worker %s",
				interfacesUnderTest[0], workerNodeList[0].Definition.Name))
			enableEthernetInterfaceWithIP(nodeNetConfigPolicyName, workerNodeList[0].Definition.Name,
				interfacesUnderTest[0])

			By("Collecting the frrk8sPod Pod list for worker nodes in list cnfWorkerNodeList")
			frrk8sPods = verifyAndCreateFRRk8sPodList()

			By(fmt.Sprintf("Verify the frrk8s pod's link local IP address from interface %s on worker node %s",
				interfacesUnderTest[0], workerNodeList[0].Definition.Name))
			linkLocalAddress = getLinkLocalAddress(frrk8sPods[0], interfacesUnderTest[0], tsparams.FRRContainerName)

			By("Creating BFD profile.")
			createBFDProfileUnnumberedAndVerifyIfItsReady(frrk8sPods[0])

			By("Creating host-device NAD for external FRR pod")
			_, err = define.HostDeviceNad(APIClient, frrconfig.ExternalMacVlanNADName, interfacesUnderTest[0],
				tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

			By("Create a single IPAddressPool")
			ipAddressPool := createIPAddressPool("ipaddresspool", ipAddressPoolRange)

			By("Creating a MetalLB service")
			setupMetalLbService(tsparams.MetallbServiceName, netparam.IPV4Family, tsparams.LabelValue1,
				ipAddressPool, "Cluster")

			By(fmt.Sprintf(
				"Creating a BGPAdvertisement with the nodeSelector to %s", workerNodeList[0].Definition.Name))
			worker0NodeLabel := []metav1.LabelSelector{
				{MatchLabels: map[string]string{corev1.LabelHostname: workerNodeList[0].Definition.Name}},
			}

			setupBgpAdvertisement("bgpadvertisment", tsparams.CustomCommunity, ipAddressPool.Definition.Name,
				200, []string{tsparams.BgpPeerName1}, worker0NodeLabel)
		})

		AfterAll(func() {
			By("Remove custom MetalLB test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)

			By(fmt.Sprintf("Disabling ethernet interface %s on worker node %s",
				interfacesUnderTest[0], workerNodeList[0].Definition.Name))
			ethIntWorker0Policy := nmstate.NewPolicyBuilder(APIClient, nodeNetConfigPolicyName, NetConfig.WorkerLabelMap).
				WithAbsentInterface(interfacesUnderTest[0])
			err := netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, ethIntWorker0Policy)
			Expect(err).ToNot(HaveOccurred(), "Failed to update NMState network policy")

			By("Removing NMState policies")
			err = nmstate.CleanAllNMStatePolicies(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")

			By("Clean MetalLB operator and test namespaces")
			resetOperatorAndTestNS()
		})

		AfterEach(func() {
			By("Reset MetalLB operator and test namespaces for next test.")
			resetTestNsAndMlbBetweenTestCases()
		})

		It("Verify IBGP peering established with BGP Unnumbered and BFD",
			reportxml.ID("80393"), func() {
				By("Create a frr config-map")
				frrConfigMap := createConfigMapWithUnnumbered(tsparams.LocalBGPASN, tsparams.LocalBGPASN,
					interfacesUnderTest[0], externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes,
					false, true)

				By("Creating static ip annotation for the external FRR pod")
				frrStaticAnnotation := pod.StaticIPAnnotationWithInterfaceAndNamespace(
					tsparams.ExternalMacVlanNADName, tsparams.TestNamespaceName, interfacesUnderTest[0],
					[]string{})

				frrPod := createFrrPod(
					workerNodeList[1].Object.Name, frrConfigMap.Definition.Name, []string{}, frrStaticAnnotation)

				By("Creating BGP Peers")
				createBGPPeerUnnumberedAndVerifyIfItsReady(tsparams.BgpPeerName1, tsparams.BgpPeerDynamicASiBGP,
					interfacesUnderTest[0], tsparams.BfdProfileName, tsparams.LocalBGPASN, 0, false,
					0, frrk8sPods[0], map[string]string{netparam.LabelHostName: cnfWorkerNodeList[0].Definition.Name})

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, []string{interfacesUnderTest[0]})
			})

		It("Verify EBGP peering established and advertised IPv4 prefixes received by the external FRR pod with BFD",
			reportxml.ID("80394"), func() {
				By("Creating nginx test pod on worker node 0")
				setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
					workerNodeList[0].Definition.Name,
					tsparams.LabelValue1)

				By("Creating static ip annotation for the external FRR pod")
				frrStaticAnnotation := pod.StaticIPAnnotationWithInterfaceAndNamespace(
					tsparams.ExternalMacVlanNADName, tsparams.TestNamespaceName, interfacesUnderTest[0],
					[]string{})

				By("Create a frr config-map")
				frrConfigMap := createConfigMapWithUnnumbered(tsparams.RemoteBGPASN, tsparams.LocalBGPASN,
					interfacesUnderTest[0], externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes,
					false, true)

				By("Creating BGP Peers")
				createBGPPeerUnnumberedAndVerifyIfItsReady(tsparams.BgpPeerName1, tsparams.BgpPeerDynamicASeBGP,
					interfacesUnderTest[0], tsparams.BfdProfileName, tsparams.LocalBGPASN, 0, false,
					0, frrk8sPods[0],
					map[string]string{netparam.LabelHostName: cnfWorkerNodeList[0].Definition.Name})

				frrPod := createFrrPod(
					workerNodeList[1].Object.Name, frrConfigMap.Definition.Name, []string{}, frrStaticAnnotation)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, []string{interfacesUnderTest[0]})

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32, []string{linkLocalAddress},
					ipAddressPoolRange)
			})

		It("Verify EBGP peering established and advertised IPv4 prefixes received by the external FRR pod without "+
			"BFD",
			reportxml.ID("81916"), func() {
				By("Creating nginx test pod on worker node 0")
				setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
					workerNodeList[0].Definition.Name,
					tsparams.LabelValue1)

				By("Creating static ip annotation for the external FRR pod")
				frrStaticAnnotation := pod.StaticIPAnnotationWithInterfaceAndNamespace(
					tsparams.ExternalMacVlanNADName, tsparams.TestNamespaceName, interfacesUnderTest[0],
					[]string{})

				By("Create a frr config-map")
				frrConfigMap := createConfigMapWithUnnumbered(tsparams.RemoteBGPASN, tsparams.LocalBGPASN,
					interfacesUnderTest[0], externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes,
					false, false)

				By("Creating BGP Peers")
				createBGPPeerUnnumberedAndVerifyIfItsReady(tsparams.BgpPeerName1, tsparams.BgpPeerDynamicASeBGP,
					interfacesUnderTest[0], "", tsparams.LocalBGPASN, 0, false,
					0, frrk8sPods[0],
					map[string]string{netparam.LabelHostName: cnfWorkerNodeList[0].Definition.Name})

				frrPod := createFrrPod(
					workerNodeList[1].Object.Name, frrConfigMap.Definition.Name, []string{}, frrStaticAnnotation)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, []string{interfacesUnderTest[0]})

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32, []string{linkLocalAddress},
					ipAddressPoolRange)
			})

		It("Verify IBGP peering established with configured peerASN and unnumbered and BFD",
			reportxml.ID("80395"), func() {
				By("Creating nginx test pod on worker node 0")
				setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
					workerNodeList[0].Definition.Name,
					tsparams.LabelValue1)

				By("Creating static ip annotation for the external FRR pod")
				frrStaticAnnotation := pod.StaticIPAnnotationWithInterfaceAndNamespace(
					tsparams.ExternalMacVlanNADName, tsparams.TestNamespaceName, interfacesUnderTest[0],
					[]string{})

				By("Create a frr config-map")
				frrConfigMap := createConfigMapWithUnnumbered(tsparams.LocalBGPASN, tsparams.LocalBGPASN,
					interfacesUnderTest[0], externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes,
					false, true)

				By("Creating BGP Peers")
				createBGPPeerUnnumberedAndVerifyIfItsReady(tsparams.BgpPeerName1, "",
					interfacesUnderTest[0], tsparams.BfdProfileName, tsparams.LocalBGPASN, tsparams.LocalBGPASN,
					false, 0, frrk8sPods[0], map[string]string{netparam.LabelHostName: cnfWorkerNodeList[0].Definition.Name})

				frrPod := createFrrPod(
					workerNodeList[1].Object.Name, frrConfigMap.Definition.Name, []string{}, frrStaticAnnotation)

				By("Checking that BGP session is established and up")
				verifyMetalLbBGPSessionsAreUPOnFrrPod(frrPod, []string{interfacesUnderTest[0]})

				By("Validating BGP route prefix")
				validatePrefix(frrPod, netparam.IPV4Family, netparam.IPSubnetInt32, []string{linkLocalAddress},
					ipAddressPoolRange)
			})
	})

func enableEthernetInterfaceWithIP(policyName, nodeName, interfaceName string) {
	ethernetInterface := nmstate.NewPolicyBuilder(APIClient, policyName, map[string]string{
		corev1.LabelHostname: nodeName,
	})

	ethernetInterface.WithEthernetIPv6LinkLocalInterface(interfaceName)

	_, err := ethernetInterface.Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("fail to add an IP address to interface: %s", interfaceName))
}

//nolint:unparam
func createConfigMapWithUnnumbered(localAS, remoteAS int, interfaceName string,
	externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes []string, multiHop, bfd bool) *configmap.Builder {
	frrBFDConfig := frr.DefineBGPConfigWithUnnumbered(localAS, remoteAS, interfaceName,
		externalAdvertisedIPv4Routes, externalAdvertisedIPv6Routes, multiHop, bfd)
	configMapData := frrconfig.DefineBaseConfig(frrconfig.DaemonsFile, frrBFDConfig, "")
	frrConfigMap, err := configmap.NewBuilder(APIClient, "frr-external-configmap", tsparams.TestNamespaceName).
		WithData(configMapData).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create config map")

	return frrConfigMap
}

func getLinkLocalAddress(frrPod *pod.Builder, interfaceName, containerName string) string {
	GinkgoHelper()

	var linkLocal string

	Eventually(func() string {
		interfaceStatus, err := frr.GetInterfaceStatus(frrPod, interfaceName, []string{containerName})
		Expect(err).ToNot(HaveOccurred(), "Failed to verify interface details")

		for _, addr := range interfaceStatus.IPAddresses {
			if strings.HasPrefix(addr.Address, "fe80::") {
				linkLocal = strings.Split(addr.Address, "/")[0]

				return linkLocal
			}
		}

		return ""
	}, 30*time.Second, tsparams.DefaultRetryInterval).Should(Not(BeEmpty()),
		"Failed to find a link-local address starting with fe80::")

	return linkLocal
}

func resetTestNsAndMlbBetweenTestCases() {
	// GinkgoHelper used for better stack traces.
	GinkgoHelper()

	By("Cleaning MetalLb operator namespace")

	metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
	err = metalLbNs.CleanObjects(
		tsparams.DefaultTimeout,
		metallb.GetBGPPeerGVR(),
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

	By("Cleaning test namespace")

	err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
		tsparams.DefaultTimeout,
		pod.GetGVR(),
		configmap.GetGVR())
	Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
}

func createBFDProfileUnnumberedAndVerifyIfItsReady(frrk8sPod *pod.Builder) *metallb.BFDBuilder {
	By("Creating BFD profile")

	bfdProfile, err := metallb.NewBFDBuilder(APIClient, "bfdprofile", NetConfig.MlbOperatorNamespace).
		WithRcvInterval(300).WithTransmitInterval(300).
		WithEchoMode(false).WithPassiveMode(false).WithMinimumTTL(5).WithEchoMode(true).
		WithMultiplier(3).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create BFD profile")

	Eventually(frr.IsProtocolConfigured,
		time.Minute, tsparams.DefaultRetryInterval).WithArguments(frrk8sPod, "bfd").
		Should(BeTrue(), "BFD is not configured on the FRR node pod")

	return bfdProfile
}
