package tests

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	mlbcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MetalLb New CRDs", Ordered, Label("newcrds"), ContinueOnFailure, func() {
	var (
		addressPool              = []string{"3.3.3.1", "3.3.3.240"}
		ipAddressPool            *metallb.IPAddressPoolBuilder
		l3ClientPod              *pod.Builder
		sriovInterfacesUnderTest []string
		ipSecondaryInterface1    = "3.3.3.10/24"
		ipSecondaryInterface2    = "3.3.3.20/24"
	)

	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

		firstMasterNode := masterNodeList[0]
		By("Setup MetalLB CR")
		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed create MetalLB CR")

		By("Creating nginx test pod")
		setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
			workerNodeList[0].Definition.Name,
			tsparams.LabelValue1)

		By("Generating ConfigMap configuration for the external FRR pod")
		masterConfigMap := createConfigMap(tsparams.LocalBGPASN, ipv4NodeAddrList, false, true)

		By("Creating External NAD")
		err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the external network-attachment-definition")

		By("Creating FRR-L3client pod on a Master node")
		staticIPAnnotation := pod.StaticIPAnnotation(
			frrconfig.ExternalMacVlanNADName, []string{fmt.Sprintf("%s/%d", ipv4metalLbIPList[0], 24)})

		l3ClientPod = createFrrPod(
			firstMasterNode.Object.Name, masterConfigMap.Definition.Name, []string{}, staticIPAnnotation)

		By("Verifying that the frrk8sPod deployment is in Ready state and create a list of the pods on " +
			"worker nodes.")
		frrk8sPods := verifyAndCreateFRRk8sPodList()

		By("Configuring BGP and BFD")
		bfdProfile := createBFDProfileAndVerifyIfItsReady(frrk8sPods)

		createBGPPeerAndVerifyIfItsReady(tsparams.BgpPeerName1, ipv4metalLbIPList[0], bfdProfile.Definition.Name,
			tsparams.LocalBGPASN, false, 0, frrk8sPods)

		By("Checking that BGP and BFD sessions are established and up")
		verifyMetalLbBFDAndBGPSessionsAreUPOnFrrPod(l3ClientPod, ipv4NodeAddrList)

		By("Configuring Local GW mode")
		setLocalGWMode(true)

		By("Adding IP to a secondary interface on the worker 0")
		sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		addOrDeleteNodeSecIPAddViaFRRK8S("add", workerNodeList[0].Object.Name,
			ipSecondaryInterface1, sriovInterfacesUnderTest[0])

		By("Creating an IPAddressPool and BGPAdvertisement")
		ipAddressPool = setupBgpAdvertisementAndIPAddressPool(
			tsparams.BGPAdvAndAddressPoolName, addressPool, netparam.IPSubnetInt32)

		By("Creating a L2Advertisement")
		_, err = metallb.NewL2AdvertisementBuilder(
			APIClient, "l2advertisement", NetConfig.MlbOperatorNamespace).
			WithIPAddressPools([]string{ipAddressPool.Definition.Name}).
			WithInterfaces([]string{sriovInterfacesUnderTest[0]}).
			Create()
		Expect(err).ToNot(HaveOccurred(), "An unexpected error occurred while creating L2Advertisement.")

		By("Enabling IPForwarding on a DUT interface")
		output, err := cluster.ExecCmdWithStdout(APIClient,
			fmt.Sprintf("sudo sysctl -w net.ipv4.conf.%s.forwarding=1", sriovInterfacesUnderTest[0]),
			metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", workerNodeList[0].Object.Name)})
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to enable IPForwarding on a DUT interface: %s", output))
	})

	AfterEach(func() {
		By("Removing IP to a secondary interface from the worker 0")
		addOrDeleteNodeSecIPAddViaFRRK8S("del", workerNodeList[0].Object.Name,
			ipSecondaryInterface1, sriovInterfacesUnderTest[0])

		By("Removing MetalLB CRs and cleaning the test ns")
		resetOperatorAndTestNS()
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}

		By("Disabling IPForwarding on a DUT interface")
		output, err := cluster.ExecCmdWithStdout(APIClient,
			fmt.Sprintf("sudo sysctl -w net.ipv4.conf.%s.forwarding=0", sriovInterfacesUnderTest[0]),
			metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", workerNodeList[0].Object.Name)})
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to disable IPForwarding on a DUT interface: %s", output))

		By("Reverting GW mode to the Sharing")
		setLocalGWMode(false)
	})

	It("Concurrent Layer2 and Layer3 should work concurrently Layer 2 and Layer 3", reportxml.ID("50059"), func() {
		By("Creating MetalLB service")
		setupMetalLbService(
			tsparams.MetallbServiceName,
			netparam.IPV4Family,
			tsparams.LabelValue1,
			ipAddressPool,
			corev1.ServiceExternalTrafficPolicyTypeLocal)

		By(fmt.Sprintf("Creating macvlan NAD with the secondary interface %s", sriovInterfacesUnderTest[0]))
		createExternalNadWithMasterInterface("l2nad", sriovInterfacesUnderTest[0])

		By("Creating L2 client")
		staticIPAnnotation := pod.StaticIPAnnotation("l2nad", []string{ipSecondaryInterface2})

		l2ClientPod, err := pod.NewBuilder(APIClient, "l2client", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
			DefineOnNode(workerNodeList[1].Object.Name).
			WithSecondaryNetwork(staticIPAnnotation).
			CreateAndWaitUntilRunning(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to create l2 client pod")

		By("Validating that l2 client can curl to LB address")
		httpTrafficValidation(l2ClientPod, ipaddr.RemovePrefix(ipSecondaryInterface2), addressPool[0])

		By("Validating that l3 client can curl to LB address")
		httpTrafficValidation(l3ClientPod, ipv4metalLbIPList[0], addressPool[0], tsparams.FRRSecondContainerName)
	})
})

func addOrDeleteNodeSecIPAddViaFRRK8S(action string,
	workerNodeName string,
	ipaddress string,
	secInterface string) {
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", workerNodeName)

	frrk8sPods, err := pod.List(APIClient, NetConfig.Frrk8sNamespace, metav1.ListOptions{
		LabelSelector: tsparams.FRRK8sDefaultLabel, FieldSelector: fieldSelector},
	)
	Expect(err).ToNot(HaveOccurred(), "Failed to list frrk8s pods")

	buffer, err := frrk8sPods[0].ExecCommand([]string{"ip", "add", action,
		ipaddress, "dev", secInterface}, "frr")

	if err != nil && strings.Contains(buffer.String(), "already assigned") {
		glog.V(90).Infof("Warning: Address %s is already assigned to %s", ipaddress, secInterface)

		return
	}

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to %s sec IP address: %s", action, buffer.String()))
}

func httpTrafficValidation(testPod *pod.Builder, srcIPAddress, dstIPAddress string, secContainerName ...string) {
	Eventually(func() error {
		var err error

		if net.ParseIP(dstIPAddress).To4() == nil {
			_, err = mlbcmd.Curl(
				testPod, srcIPAddress, dstIPAddress, netparam.IPV6Family, secContainerName...)

		} else {
			_, err = mlbcmd.Curl(
				testPod, srcIPAddress, dstIPAddress, netparam.IPV4Family, secContainerName...)
		}

		return err
	}, 15*time.Second, 5*time.Second).ShouldNot(HaveOccurred(),
		fmt.Sprintf("Client %s can not curl LB IP address %s",
			testPod.Definition.Name, dstIPAddress))
}
