package tests

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	srIovPolicyNode1Name          = "sriov-policy-node-1"
	srIovPolicyNode2Name          = "sriov-policy-node-2"
	srIovPolicyNode0ResName       = "sriovpolicynode0"
	srIovPolicyNode1ResName       = "sriovpolicynode1"
	srIovNetworkAllMultiNode1     = "sriovnet-allmulti-node1"
	srIovNetworkDefaultNode1      = "sriovnet-default-node1"
	srIovNetworkDefaultNode2      = "sriovnet-default-node2"
	srIovNetworkDefaultBondNet1   = "sriovnet-default-node1-net1"
	srIovNetworkDefaultBondNet2   = "sriovnet-default-node1-net2"
	srIovNetworkAllMultiBondNet1  = "sriovnet-allmulti-node1-net1"
	srIovNetworkAllMultiBondNet2  = "sriovnet-allmulti-node1-net2"
	clientDefaultName             = "client-default"
	clientAllmultiEnabledName     = "client-allmulti-enabled"
	multicastServerIPv6           = "2001:100::20/64"
	multicastServerIPv4           = "192.168.100.20/24"
	clientAllmultiEnabledIPv6     = "2001:100::1/64"
	clientAllmultiEnabledIPv4     = "192.168.100.1/24"
	clientAllmultiDisabledIPv6    = "2001:100::2/64"
	clientAllmultiDisabledIPv4    = "192.168.100.2/24"
	multicastServerIPv6Mac        = "60:00:00:00:10:10"
	multicastServerIPv4Mac        = "20:04:0f:f1:88:20"
	clientAllmultiEnabledIPv6Mac  = "60:00:00:00:00:11"
	clientAllmultiEnabledIPv4Mac  = "20:04:0f:f1:88:11"
	clientAllmultiDisabledIPv6Mac = "60:00:00:00:00:12"
	clientAllmultiDisabledIPv4Mac = "20:04:0f:f1:88:12"
	multicastIPv6GroupIP          = "ff05:5::5:"
	multicastIPv4GroupIP          = "239.100.100.250:"
	bondNadNameDefault            = "bondnaddefault"
	bondNadNameAllMulti           = "bondnadallmulti"
)

var (
	workerNodes               []*nodes.Builder
	multicastPingIPv6CMD      = []string{"bash", "-c", "sleep 5; ping -I net1 ff05:5::05"}
	multicastPingIPv4CMD      = []string{"bash", "-c", "sleep 5; ping -I net1 239.100.100.250"}
	multicastPingDualStackCMD = []string{"bash", "-c", "sleep 5; ping -I net1 239.100.100.250 & " +
		"ping -I net1 ff05:5::05"}
	multicastPingDualNet1Net2StackCMD = []string{"bash", "-c", "sleep 5; ping -I net1 239.100.100.250 & " +
		"ping -I net1 ff05:5::05 & ping -I net2 239.100.100.250 & ping -I net2 ff05:5::05"}
	tcpDumpCMD           = []string{"bash", "-c", "timeout 2m tcpdump -i net1 -c 10"}
	tcpDumpCMDNet2       = []string{"bash", "-c", "timeout 2m tcpdump -i net2 -c 10"}
	addIPv6MCGroupMacCMD = []string{"bash", "-c", "ip maddr add 33:33:0:0:0:5 dev net1"}
	addIPv4MCGroupMacCMD = []string{"bash", "-c", "ip maddr add 01:00:5e:64:64:fa dev net1"}
)

var _ = Describe(
	"allmulti", Ordered, Label(tsparams.LabelSuite, tsparams.LabelSriovHWEnabled), ContinueOnFailure, func() {

		BeforeAll(func() {
			By("Discover worker nodes")
			var err error

			workerNodes, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Fail to discover nodes")

			By("Collecting SR-IOV interfaces for allmulti testing")
			srIovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By(fmt.Sprintf("Define and create sriov network policy on %s", workerNodes[0].Definition.Name))
			nodeSelectorWorker0 := map[string]string{
				"kubernetes.io/hostname": workerNodes[0].Definition.Name,
			}

			_, err = sriov.NewPolicyBuilder(
				APIClient,
				srIovPolicyNode1Name,
				NetConfig.SriovOperatorNamespace,
				srIovPolicyNode0ResName,
				6,
				[]string{fmt.Sprintf("%s#0-5", srIovInterfacesUnderTest[0])},
				nodeSelectorWorker0).WithMTU(9000).WithVhostNet(true).Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create an sriov policy on %s",
				workerNodes[0].Definition.Name))

			By(fmt.Sprintf("Define and create sriov network policy on %s", workerNodes[1].Definition.Name))
			nodeSelectorWorker1 := map[string]string{
				"kubernetes.io/hostname": workerNodes[1].Definition.Name,
			}

			_, err = sriov.NewPolicyBuilder(
				APIClient,
				srIovPolicyNode2Name,
				NetConfig.SriovOperatorNamespace,
				srIovPolicyNode1ResName,
				6,
				[]string{fmt.Sprintf("%s#0-5", srIovInterfacesUnderTest[0])},
				nodeSelectorWorker1).WithMTU(9000).WithVhostNet(true).Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create an sriov policy on %s",
				workerNodes[1].Definition.Name))

			By("Define and create sriov network with allmuti enabled")
			defineAndCreateSrIovNetwork(srIovNetworkAllMultiNode1, srIovPolicyNode0ResName, true)

			By("Define and create sriov network with allmuti disabled")
			defineAndCreateSrIovNetwork(srIovNetworkDefaultNode1, srIovPolicyNode0ResName, false)

			By("Define and create sriov network with allmuti disabled on a different node")
			defineAndCreateSrIovNetwork(srIovNetworkDefaultNode2, srIovPolicyNode1ResName, false)

			By("Define and create sriov network with Bonded Net1 allmuti disabled")
			defineAndCreateSrIovNetworkWithOutIPAM(srIovNetworkDefaultBondNet1, false)

			By("Define and create sriov network with Bonded Net2 allmuti disabled")
			defineAndCreateSrIovNetworkWithOutIPAM(srIovNetworkDefaultBondNet2, false)

			By("Define and create sriov network with Bonded Net1 allmuti enabled")
			defineAndCreateSrIovNetworkWithOutIPAM(srIovNetworkAllMultiBondNet1, true)

			By("Define and create sriov network with Bonded Net2 allmuti enabled")
			defineAndCreateSrIovNetworkWithOutIPAM(srIovNetworkAllMultiBondNet2, true)

			By("Define and create Bonded network attachment for allmulti client")
			nadBondAllmultiConfig := defineBondNAD(bondNadNameAllMulti, "active-backup")

			_, err = nadBondAllmultiConfig.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create Bond Nad %s",
				nadBondAllmultiConfig.Definition.Name)

			By("Define and create Bonded network attachment for default client")
			nadBondDefaultConfig := defineBondNAD(bondNadNameDefault, "active-backup")
			_, err = nadBondDefaultConfig.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create Bond Nad %s",
				nadBondDefaultConfig.Definition.Name)

			By("Waiting until cluster MCP and SR-IOV are stable")
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")
		})

		It("Validate a pod can receive non-member multicast IPv6 traffic over a secondary SRIOV interface"+
			" when allmulti mode is enabled from a multicast source in the same PF", reportxml.ID("67813"), func() {
			multicastServer := createMulticastServer(srIovNetworkDefaultNode1, multicastServerIPv6Mac,
				[]string{multicastServerIPv6}, multicastPingIPv6CMD, workerNodes[0].Definition.Name)

			defaultClient := createTestClient(clientDefaultName, srIovNetworkDefaultNode1,
				clientAllmultiDisabledIPv6Mac, workerNodes[0].Definition.Name, []string{clientAllmultiDisabledIPv6})

			allMultiEnabledClient := createTestClient(clientAllmultiEnabledName, srIovNetworkAllMultiNode1,
				clientAllmultiEnabledIPv6Mac, workerNodes[0].Definition.Name, []string{clientAllmultiEnabledIPv6})

			runAllMultiTestCases(multicastServer, defaultClient, allMultiEnabledClient, multicastServerIPv6,
				clientAllmultiDisabledIPv6, multicastIPv6GroupIP, addIPv6MCGroupMacCMD)
		})

		It("Validate a pod can receive non-member multicast IPv4 traffic over a secondary SRIOV interface"+
			" when allmulti mode is enabled from a multicast source is on a different node",
			reportxml.ID("67815"), func() {
				multicastServer := createMulticastServer(srIovNetworkDefaultNode2,
					multicastServerIPv4Mac, []string{multicastServerIPv4}, multicastPingIPv4CMD,
					workerNodes[1].Definition.Name)

				defaultClient := createTestClient(clientDefaultName, srIovNetworkDefaultNode1,
					clientAllmultiDisabledIPv4Mac, workerNodes[0].Definition.Name, []string{clientAllmultiDisabledIPv4})

				allMultiEnabledClient := createTestClient(clientAllmultiEnabledName, srIovNetworkAllMultiNode1,
					clientAllmultiEnabledIPv4Mac, workerNodes[0].Definition.Name, []string{clientAllmultiEnabledIPv4})

				runAllMultiTestCases(multicastServer, defaultClient, allMultiEnabledClient, clientAllmultiEnabledIPv4,
					clientAllmultiDisabledIPv4, multicastIPv4GroupIP, addIPv4MCGroupMacCMD)
			})

		It("Validate a pod can receive non-member multicast IPv6 traffic over a secondary SRIOV interface"+
			" when allmulti mode is enabled from a multicast source on a different node", reportxml.ID("67816"), func() {
			multicastServer := createMulticastServer(srIovNetworkDefaultNode2, multicastServerIPv6Mac,
				[]string{multicastServerIPv6}, multicastPingIPv6CMD, workerNodes[1].Definition.Name)

			defaultClient := createTestClient(clientDefaultName, srIovNetworkDefaultNode1,
				clientAllmultiDisabledIPv6Mac, workerNodes[0].Definition.Name, []string{clientAllmultiDisabledIPv6})

			allMultiEnabledClient := createTestClient(clientAllmultiEnabledName, srIovNetworkAllMultiNode1,
				clientAllmultiEnabledIPv6Mac, workerNodes[0].Definition.Name, []string{clientAllmultiEnabledIPv6})

			runAllMultiTestCases(multicastServer, defaultClient, allMultiEnabledClient, multicastServerIPv6,
				clientAllmultiDisabledIPv6, multicastIPv6GroupIP, addIPv6MCGroupMacCMD)
		})

		It("Validate a pod can receive non-member multicast traffic over a secondary dual stack SRIOV interface "+
			"when allmulti mode is enabled from a multicast source in the same PF",
			reportxml.ID("67817"), func() {
				multicastServer := createMulticastServer(srIovNetworkDefaultNode1, multicastServerIPv4Mac,
					[]string{multicastServerIPv4, multicastServerIPv6}, multicastPingDualStackCMD, workerNodes[0].Definition.Name)

				defaultClient := createTestClient(clientDefaultName, srIovNetworkDefaultNode1,
					clientAllmultiDisabledIPv4Mac, workerNodes[0].Definition.Name, []string{clientAllmultiDisabledIPv4,
						clientAllmultiDisabledIPv6})

				allMultiEnabledClient := createTestClient(clientAllmultiEnabledName, srIovNetworkAllMultiNode1,
					clientAllmultiEnabledIPv4Mac, workerNodes[0].Definition.Name, []string{clientAllmultiEnabledIPv4,
						clientAllmultiEnabledIPv6})

				By("Run Dual Stack IPv4 tests")
				runAllMultiTestCases(multicastServer, defaultClient, allMultiEnabledClient, clientAllmultiEnabledIPv4,
					clientAllmultiDisabledIPv4, multicastIPv4GroupIP, addIPv4MCGroupMacCMD)

				By("Run Dual Stack IPv6 tests")
				runAllMultiTestCases(multicastServer, defaultClient, allMultiEnabledClient, clientAllmultiEnabledIPv6,
					clientAllmultiDisabledIPv6, multicastIPv6GroupIP, addIPv6MCGroupMacCMD)

			})

		It("Validate a pod can receive non-member multicast IPv4 traffic over a secondary bonded SRIOV interface when "+
			"allmulti mode is enabled from a multicast source in the same PF", reportxml.ID("67818"), func() {
			multicastServer := createMulticastServer(srIovNetworkDefaultNode1, multicastServerIPv4Mac,
				[]string{multicastServerIPv4}, multicastPingIPv4CMD, workerNodes[0].Definition.Name)

			By("Define and run a client pod with allmulti disabled with bonded interfaces")
			defaultClient := createBondedTestClient(clientDefaultName, srIovNetworkDefaultBondNet1,
				srIovNetworkDefaultBondNet2, bondNadNameDefault, workerNodes[0].Definition.Name,
				[]string{clientAllmultiDisabledIPv4})

			By("Define and run a client pod with allmulti enabled")
			allMultiEnabledClient := createBondedTestClient(clientAllmultiEnabledName, srIovNetworkAllMultiBondNet1,
				srIovNetworkAllMultiBondNet2, bondNadNameAllMulti, workerNodes[0].Definition.Name,
				[]string{clientAllmultiEnabledIPv4})

			runAllMultiTestCases(multicastServer, defaultClient, allMultiEnabledClient, clientAllmultiEnabledIPv4,
				clientAllmultiDisabledIPv4, multicastIPv4GroupIP, addIPv4MCGroupMacCMD)
		})

		It("Validate a pod can receive non-member multicast IPv6 traffic over a secondary bonded SRIOV interface when "+
			"allmulti mode is enabled from a multicast source in the same PF", reportxml.ID("67819"), func() {
			multicastServer := createMulticastServer(srIovNetworkDefaultNode1, multicastServerIPv6Mac,
				[]string{multicastServerIPv6}, multicastPingIPv6CMD, workerNodes[0].Definition.Name)

			By("Define and run a client pod with allmulti disabled with bonded interfaces")
			defaultClient := createBondedTestClient(clientDefaultName, srIovNetworkDefaultBondNet1,
				srIovNetworkDefaultBondNet2, bondNadNameDefault, workerNodes[0].Definition.Name,
				[]string{clientAllmultiDisabledIPv6})

			By("Define and run a client pod with allmulti enabled")
			allMultiEnabledClient := createBondedTestClient(clientAllmultiEnabledName, srIovNetworkAllMultiBondNet1,
				srIovNetworkAllMultiBondNet2, bondNadNameAllMulti, workerNodes[0].Definition.Name,
				[]string{clientAllmultiEnabledIPv6})

			runAllMultiTestCases(multicastServer, defaultClient, allMultiEnabledClient, clientAllmultiEnabledIPv6,
				clientAllmultiDisabledIPv6, multicastIPv6GroupIP, addIPv6MCGroupMacCMD)
		})

		It("Validate a pod does not receive non-member multicast traffic over a third SRIOV dual stack interface "+
			"when allmulti mode is enabled on the second SRIOV interface from a multicast source in the same PF",
			reportxml.ID("67820"), func() {
				multicastServer := createMulticastServerWithMultiNets(srIovNetworkDefaultNode1, srIovNetworkDefaultNode1,
					[]string{multicastServerIPv4, multicastServerIPv6, "192.168.101.20/24", "2001:101::20/64"},
					multicastPingDualNet1Net2StackCMD, workerNodes[0].Definition.Name)

				defaultClient := createTestClientWithMultiInterfaces(clientDefaultName,
					srIovNetworkDefaultNode1, srIovNetworkDefaultNode1, workerNodes[0].Definition.Name,
					[]string{clientAllmultiDisabledIPv4, clientAllmultiDisabledIPv6, "192.168.101.2/24", "2001:101::2/64"})

				allMultiEnabledClient := createTestClientWithMultiInterfaces(clientAllmultiEnabledName,
					srIovNetworkDefaultNode1, srIovNetworkAllMultiNode1, workerNodes[0].Definition.Name,
					[]string{clientAllmultiEnabledIPv4, clientAllmultiEnabledIPv6, "192.168.101.1/24", "2001:101::1/64"})

				By("Verify IPv4 and IPv6 on net1 connectivity between the clients and multicast source")
				err := cmd.ICMPConnectivityCheck(multicastServer, []string{clientAllmultiEnabledIPv4,
					clientAllmultiEnabledIPv6, clientAllmultiDisabledIPv4,
					clientAllmultiDisabledIPv6})
				Expect(err).ToNot(HaveOccurred(),
					"Failed to ping between the multicast source and the clients")

				By("Verify IPv4 and IPv6 on net2 connectivity between the clients and multicast source")
				err = cmd.ICMPConnectivityCheck(multicastServer, []string{"192.168.101.1/24", "2001:101::1/64",
					"192.168.101.2/24", "2001:101::2/64"})
				Expect(err).ToNot(HaveOccurred(),
					"Failed to ping between the multicast source and the clients")

				By("Run IPv4 and IPv6 multicast tests on Net1 and Net2")
				runAllMultiDualInterfaceTestCase(defaultClient, allMultiEnabledClient,
					multicastIPv4GroupIP, multicastIPv6GroupIP)
			})

		AfterEach(func() {
			By("Removing all pods from test namespace")
			runningNamespace, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace")
			Expect(runningNamespace.CleanObjects(
				tsparams.WaitTimeout, pod.GetGVR())).ToNot(HaveOccurred())

		})

		AfterAll(func() {
			By("Removing all SR-IOV Policy")
			err := sriov.CleanAllNetworkNodePolicies(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to clean srIovPolicy")

			By("Removing all srIovNetworks")
			err = sriov.CleanAllNetworksByTargetNamespace(
				APIClient, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to clean sriov networks")
		})
	})

func defineAndCreateSrIovNetwork(srIovNetwork, resName string, allMulti bool) {
	srIovNetworkObject := sriov.NewNetworkBuilder(
		APIClient, srIovNetwork, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName, resName).
		WithStaticIpam().WithIPAddressSupport().WithMacAddressSupport().WithLogLevel(netparam.LogLevelDebug)

	if allMulti {
		srIovNetworkObject.WithTrustFlag(true).WithMetaPluginAllMultiFlag(true)
	}

	srIovNetworkObject, err := srIovNetworkObject.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create sriov network")

	Eventually(func() bool {
		_, err := nad.Pull(APIClient, srIovNetworkObject.Object.Name, tsparams.TestNamespaceName)

		return err == nil
	}, tsparams.WaitTimeout, tsparams.RetryInterval).Should(BeTrue(), "Failed to pull "+
		"NetworkAttachmentDefinition")
}

func createMulticastServer(
	sriovNetwork string,
	macAddress string,
	ipAddress []string,
	multicastCmd []string,
	nodeName string) *pod.Builder {
	By("Define and run a multicast server")

	sriovNetworkMC := pod.StaticIPAnnotationWithMacAddress(sriovNetwork, ipAddress, macAddress)
	multicastSourceClient, err := pod.NewBuilder(APIClient, "mc-source-server", tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).DefineOnNode(nodeName).WithPrivilegedFlag().RedefineDefaultCMD(multicastCmd).
		WithSecondaryNetwork(sriovNetworkMC).CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run multicast source server")

	return multicastSourceClient
}

func createTestClient(
	name string,
	sriovNetwork string,
	macAddress string,
	nodeName string,
	ipAddress []string) *pod.Builder {
	By(fmt.Sprintf("Define and run client pod  %s", name))

	sriovNetworkDefault := pod.StaticIPAnnotationWithMacAddress(sriovNetwork, ipAddress, macAddress)

	clientDefault, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).DefineOnNode(nodeName).WithPrivilegedFlag().
		WithSecondaryNetwork(sriovNetworkDefault).CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run default client")

	return clientDefault
}

func createTestClientWithMultiInterfaces(
	name string,
	sriovNetworkNet1 string,
	sriovNetworkNet2 string,
	nodeName string,
	ipAddresses []string) *pod.Builder {
	By(fmt.Sprintf("Define and run container %s", name))

	annotation, err := pod.StaticIPMultiNetDualStackAnnotation([]string{sriovNetworkNet1, sriovNetworkNet2}, ipAddresses)
	Expect(err).ToNot(HaveOccurred(), "Failed to define a 2 net dual stack nad annotation")

	clientDefault, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).DefineOnNode(nodeName).WithPrivilegedFlag().
		WithSecondaryNetwork(annotation).CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run default client")

	return clientDefault
}

func createBondedTestClient(
	name string,
	sriovNetworkNet1 string,
	sriovNetworkNet2 string,
	bondNadName string,
	nodeName string,
	ipAddress []string) *pod.Builder {
	By(fmt.Sprintf("Define and run client pod %s", name))

	annotation := pod.StaticIPBondAnnotationWithInterface(bondNadName, "bond0", []string{sriovNetworkNet1,
		sriovNetworkNet2}, ipAddress)

	bondedTestContainer, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).DefineOnNode(nodeName).WithPrivilegedFlag().
		WithSecondaryNetwork(annotation).CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run bonded container")

	return bondedTestContainer
}

func createMulticastServerWithMultiNets(
	sriovNetworkNet1 string,
	sriovNetworkNet2 string,
	ipAddresses []string,
	multicastCmd []string,
	nodeName string) *pod.Builder {
	By("Define and run a multicast server")

	annotation, err := pod.StaticIPMultiNetDualStackAnnotation([]string{sriovNetworkNet1, sriovNetworkNet2}, ipAddresses)
	Expect(err).ToNot(HaveOccurred(), "Failed to define a 2 net dual stack nad annotation")

	multicastSourceClient, err := pod.NewBuilder(APIClient, "mc-source-server", tsparams.TestNamespaceName,
		NetConfig.CnfNetTestContainer).DefineOnNode(nodeName).WithPrivilegedFlag().RedefineDefaultCMD(multicastCmd).
		WithSecondaryNetwork(annotation).CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run multicast source server")

	return multicastSourceClient
}

func runAllMultiTestCases(
	multicastSourcePod *pod.Builder,
	defaultClientPod *pod.Builder,
	allMultiEnabledPod *pod.Builder,
	allMulticastEnabledIP string,
	defaultClientIP string,
	multicastGroupIP string,
	addIPMCGroupMacCMD []string) {
	By("Verify connectivity between the clients and multicast source")

	clientIPAddresses := []string{allMulticastEnabledIP, defaultClientIP}
	err := cmd.ICMPConnectivityCheck(multicastSourcePod, clientIPAddresses)
	Expect(err).ToNot(HaveOccurred(), "Failed to ping between the multicast source and the clients")

	By("Verify multicast group is not accessible from container without allmulti enabled")
	assertMulticastTrafficIsNotReceived(defaultClientPod, tcpDumpCMD, multicastGroupIP)

	By("Verify multicast group is accessible from container with allmulti enabled")
	assertMulticastTrafficIsReceived(allMultiEnabledPod, tcpDumpCMD, multicastGroupIP)

	By("Add client without allmulti enabled to the multicast group")

	_, err = defaultClientPod.ExecCommand(addIPMCGroupMacCMD)
	Expect(err).ToNot(HaveOccurred(), "Failed to add the multicast group mac address")

	By("Verify the client receives traffic from the multicast group after being added to the group")
	assertMulticastTrafficIsReceived(defaultClientPod, tcpDumpCMD, multicastGroupIP)
}

// defineBondNAD returns network attachment definition for a Bond interface.
func defineBondNAD(nadname, mode string) *nad.Builder {
	bondNad, err := nad.NewMasterBondPlugin(nadname, mode).WithFailOverMac(1).
		WithLinksInContainer(true).WithMiimon(100).
		WithLinks([]nad.Link{{Name: "net1"}, {Name: "net2"}}).WithCapabilities(&nad.Capability{IPs: true}).
		WithIPAM(&nad.IPAM{Type: "static"}).GetMasterPluginConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to define Bond NAD for %s", nadname)

	createdNad, err := nad.NewBuilder(APIClient, nadname, tsparams.TestNamespaceName).WithMasterPlugin(bondNad).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create Bond NAD for %s", nadname)

	return createdNad
}

// defineAndCreateSrIovNetworkWithOutIPAM is used to create sriovnetworks with IPAM for a bonded interface.
func defineAndCreateSrIovNetworkWithOutIPAM(srIovNetwork string, allMulti bool) {
	srIovNetworkObject := sriov.NewNetworkBuilder(
		APIClient, srIovNetwork, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName,
		srIovPolicyNode0ResName).WithMacAddressSupport().WithLogLevel(netparam.LogLevelDebug)

	if allMulti {
		srIovNetworkObject.WithTrustFlag(true).WithMetaPluginAllMultiFlag(true)
	}

	srIovNetworkObject, err := srIovNetworkObject.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create sriov network")

	Eventually(func() bool {
		_, err := nad.Pull(APIClient, srIovNetworkObject.Object.Name, tsparams.TestNamespaceName)

		return err == nil
	}, tsparams.WaitTimeout, tsparams.RetryInterval).Should(BeTrue(), "Failed to pull "+
		"NetworkAttachmentDefinition")
}

func runAllMultiDualInterfaceTestCase(
	defaultClientPod *pod.Builder,
	allMultiEnabledPod *pod.Builder,
	multicastGroupIPv4 string,
	multicastGroupIPv6 string) {
	By("Verify IPv4 multicast group is not accessible from default container without allmulti enabled")
	assertMulticastTrafficIsNotReceived(defaultClientPod, tcpDumpCMD, multicastGroupIPv4)

	By("Verify IPv6 multicast group is not accessible from default container without allmulti enabled")
	assertMulticastTrafficIsNotReceived(defaultClientPod, tcpDumpCMD, multicastGroupIPv6)

	By("Verify IPv4 multicast group is accessible from allmulti enabled container net1 with allmulti enabled")
	assertMulticastTrafficIsReceived(allMultiEnabledPod, tcpDumpCMDNet2, multicastGroupIPv4)

	By("Verify IPv6 multicast group is accessible from allmulti enabled container net1 with allmulti enabled")
	assertMulticastTrafficIsReceived(allMultiEnabledPod, tcpDumpCMDNet2, multicastGroupIPv6)

	By("Verify IPv4 multicast group is not accessible from allmulti enabled container net2 without allmulti enabled")
	assertMulticastTrafficIsNotReceived(allMultiEnabledPod, tcpDumpCMD, multicastGroupIPv4)

	By("Verify IPv6 multicast group is not accessible from allmulti enabled container net2 without allmulti enabled")
	assertMulticastTrafficIsNotReceived(allMultiEnabledPod, tcpDumpCMD, multicastGroupIPv6)

	By("Add client without allmulti enabled to the IPv4 multicast group")

	_, err := allMultiEnabledPod.ExecCommand(addIPv4MCGroupMacCMD)
	Expect(err).ToNot(HaveOccurred(), "Failed to add the multicast group mac address")

	By("Verify the client receives IPv4 multicast traffic after being added to the group")
	assertMulticastTrafficIsReceived(allMultiEnabledPod, tcpDumpCMD, multicastGroupIPv4)

	By("Add client without allmulti enabled to the IPv6 multicast group")

	_, err = allMultiEnabledPod.ExecCommand(addIPv6MCGroupMacCMD)
	Expect(err).ToNot(HaveOccurred(), "Failed to add the multicast group mac address")

	By("Verify the client receives IPv6 multicast traffic after being added to the group")
	assertMulticastTrafficIsReceived(allMultiEnabledPod, tcpDumpCMD, multicastGroupIPv6)
}

// assertMulticastTrafficIsNotReceived uses Consistently waiting a specific amount to time to verify that the
// multicastGroupIP is not received.
func assertMulticastTrafficIsNotReceived(clientPod *pod.Builder, testCmd []string, multicastGroupIP string) {
	execCommand := func() string {
		output, err := clientPod.ExecCommand(testCmd)
		if err != nil {
			glog.V(100).Infof("Error executing command: %s", err)
		}

		return output.String()
	}

	Consistently(execCommand, 5*time.Second, 1*time.Second).ShouldNot(MatchRegexp(multicastGroupIP))
}

// assertMulticastTrafficIsReceived uses Eventually expecting to see the multicastGroupIP.
func assertMulticastTrafficIsReceived(clientPod *pod.Builder, testCmd []string, multicastGroupIP string) {
	execCommand := func() string {
		output, err := clientPod.ExecCommand(testCmd)
		if err != nil {
			glog.V(100).Infof("Error executing command: %s", err)
		}

		return output.String()
	}

	Eventually(execCommand, 5*time.Second, 1*time.Second).Should(MatchRegexp(multicastGroupIP))
}
