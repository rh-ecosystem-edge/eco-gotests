package tests

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	nmstateShared "github.com/nmstate/kubernetes-nmstate/api/shared"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/perfprofile"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	sriovAndResourceNameExManagedTrue = "extmanaged"
	logLevelDebug                     = "debug"
	mlxVendorID                       = "15b3"
)

var _ = Describe(
	"QinQ", Ordered, Label(tsparams.LabelQinQTestCases, tsparams.LabelSriovHWEnabled), ContinueOnFailure, func() {
		var (
			err                         error
			dot1ad                      = "802.1ad"
			dot1q                       = "802.1q"
			srIovPolicyNetDevice        = "sriovnetpolicy-netdevice"
			srIovPolicyResNameNetDevice = "sriovpolicynetdevice"
			srIovPolicyVfioPci          = "sriovpolicy-vfiopci"
			srIovPolicyResNameVfioPci   = "sriovpolicyvfiopci"
			srIovNetworkDot1AD          = "sriovnetwork-dot1ad"
			srIovNetworkDot1Q           = "sriovnetwork-dot1q"
			srIovNetworkDPDKDot1AD      = "sriovnetwork-dpdk-dot1ad"
			srIovNetworkDPDKDot1Q       = "sriovnetwork-dpdk-dot1q"
			srIovNetworkDPDKClient      = "sriovnetwork-dpdk-client"
			srIovNetworkPromiscuous     = "sriovnetwork-promiscuous"
			serverNameDPDKDot1ad        = "server-dpdk-1ad"
			serverNameDPDKDot1q         = "server-dpdk-1q"
			serverNameDot1ad            = "server-1ad"
			serverNameDot1q             = "server-1q"
			clientNameDot1ad            = "client-1ad"
			clientNameDPDKDot1ad        = "client-dpdk-1ad"
			clientNameDot1q             = "client-1q"
			clientNameDPDKDot1q         = "client-dpdk-1q"
			nadCVLAN100                 = "nadcvlan100"
			nadCVLAN101                 = "nadcvlan101"
			nadCVLANDpdk                = "nadcvlandpdk"
			nadMasterBond0              = "nadmasterbond0"
			perfProfileName             = "performance-profile-dpdk"
			intNet1                     = "net1"
			intNet2                     = "net2"
			intNet3                     = "net3"
			intBond0                    = "bond0.100"
			intelDeviceIDE810QSFP       = "1593"
			intelDeviceIDE810SFP        = "1592"
			intelDeviceIDE710           = "158b"
			testCmdNet2                 = []string{"bash", "-c", "sleep 5; testcmd -interface net2 -protocol tcp " +
				"-port 4444 -listen"}
			testCmdNet2Net3 = []string{"bash", "-c", "sleep 5; testcmd -interface net2 -protocol tcp " +
				"-port 4444 -listen & testcmd -interface net3 -protocol tcp -port 4444 -listen"}
			testCmdBond0 = []string{"bash", "-c", "sleep 5; testcmd -interface bond0.100 -protocol tcp " +
				"-port 4444 -listen"}
			tcpDumpNet1CMD              = []string{"bash", "-c", "tcpdump -l -i net1 -e > /tmp/tcpdump"}
			tcpDumpReadFileCMD          = []string{"bash", "-c", "grep -E 'vlan (100|101)[^0-9]' /tmp/tcpdump | tail -20"}
			tcpDumpDot1ADOutput         = "(ethertype 802\\.1Q-QinQ \\(0x88a8\\)).*?(ethertype 802\\.1Q.*?vlan 100)"
			tcpDumpDot1QOutput          = "(ethertype 802\\.1Q \\(0x8100\\)).*?(ethertype 802\\.1Q.*?vlan 100)"
			tcpDumpDot1QDPDKOutput      = "(ethertype 802\\.1Q \\(0x8100\\)).*?(ethertype 802\\.1Q \\(0x8100\\), vlan 100)"
			tcpDumpDot1ADDPDKOutput     = "(ethertype 802\\.1Q-QinQ \\(0x88a8\\)).*?(ethertype 802\\.1Q \\(0x8100\\), vlan 100)"
			tcpDumpDot1ADCVLAN101Output = "(ethertype 802\\.1Q-QinQ \\(0x88a8\\)).*?(ethertype 802\\.1Q.*?vlan 101)"
			tcpDumpDot1QCVLAN101QOutput = "(ethertype 802\\.1Q \\(0x8100\\)).*?(ethertype 802\\.1Q.*?vlan 101)"
			workerNodeList              = []*nodes.Builder{}
			srIovInterfacesUnderTest    []string
			sriovDeviceID               string
			sriovVendor                 string
			switchCredentials           *sriovocpenv.SwitchCredentials
			switchInterfaces            []string
			serverIPV4IP, _, _          = net.ParseCIDR(tsparams.ServerIPv4IPAddress)
			serverIPV6IP, _, _          = net.ParseCIDR(tsparams.ServerIPv6IPAddress)
			serverIPV4IP2, _, _         = net.ParseCIDR(tsparams.ServerIPv4IPAddress2)
			serverIPV6IP2, _, _         = net.ParseCIDR(tsparams.ServerIPv6IPAddress2)
			serverIPAddressesNet2       = []string{serverIPV4IP.String(), serverIPV6IP.String()}
			serverIPAddressesNet3       = []string{serverIPV4IP2.String(), serverIPV6IP2.String()}
			clientIPAddressesNet2       = []string{tsparams.ClientIPv4IPAddress, tsparams.ClientIPv6IPAddress}
			clientIPAddressesNet3       = []string{tsparams.ClientIPv4IPAddress2, tsparams.ClientIPv6IPAddress2}
		)

		BeforeAll(func() {
			By("Checking if cluster is SNO")

			isSNO, err := cluster.IsSNOCluster(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to check if cluster is SNO")

			if isSNO {
				Skip("Skipping test on SNO (Single Node OpenShift) cluster - requires 2+ workers")
			}

			By("Discover worker nodes")

			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Fail to discover worker nodes")
			Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			By("Collecting SR-IOV interfaces for qinq testing")

			srIovInterfacesUnderTest, err = SriovOcpConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Verify SR-IOV Device IDs for interface under test")

			sriovDeviceID = discoverInterfaceUnderTestDeviceID(srIovInterfacesUnderTest[0],
				workerNodeList[0].Definition.Name)
			Expect(sriovDeviceID).ToNot(BeEmpty(), "Expected sriovDeviceID not to be empty")

			By("Fetching SR-IOV Vendor ID for interface under test")

			sriovVendor, err = sriovoperator.DiscoverInterfaceUnderTestVendorID(
				APIClient, SriovOcpConfig.OcpSriovOperatorNamespace,
				srIovInterfacesUnderTest[0], workerNodeList[0].Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch SR-IOV Vendor ID for interface under test")

			By("Fetching switch credentials and interfaces")

			switchCredentials, err = sriovocpenv.NewSwitchCredentials()
			Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

			switchInterfaces, err = SriovOcpConfig.GetSwitchInterfaces()
			Expect(err).ToNot(HaveOccurred(), "Failed to get switch interfaces")

			By("Enable VF promiscuous support on sriov interface under test")
			setVFPromiscMode(workerNodeList[0].Definition.Name, srIovInterfacesUnderTest[0], sriovVendor, "on")
		})

		Context("802.1AD", func() {
			BeforeAll(func() {
				By("Verify SR-IOV Device IDs for interface under test")

				if sriovDeviceID != intelDeviceIDE810QSFP && sriovDeviceID != intelDeviceIDE810SFP {
					Skip(fmt.Sprintf("The NIC %s does not support 802.1AD", sriovDeviceID))
				}

				By("Configure lab switch interface to support VLAN double tagging")

				err = enableDot1ADonSwitchInterfaces(switchCredentials, switchInterfaces)
				Expect(err).ToNot(HaveOccurred(), "Failed to enable 802.1AD on the switch")

				By("Define and create sriovnetwork Polices")
				defineCreateSriovNetPolices(srIovPolicyNetDevice, srIovPolicyResNameNetDevice, srIovInterfacesUnderTest[0],
					sriovVendor, "netdevice")
				By("Define and create sriovnetworks")
				defineAndCreateSriovNetworks(srIovNetworkPromiscuous, srIovNetworkDot1AD, srIovNetworkDot1Q,
					srIovPolicyResNameNetDevice)
				By("Define and create network-attachment-definitions")
				defineAndCreateNADs(nadCVLAN100, nadCVLAN101, nadMasterBond0, intNet1)
			})

			It("Verify network traffic over a 802.1ad QinQ tunnel between two SRIOV pods on the same PF",
				reportxml.ID("71676"), func() {
					By("Define and create a server container")

					serverAnnotation := defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, true)
					serverPod := createServerTestPod(serverNameDot1ad, workerNodeList[0].Definition.Name, testCmdNet2,
						serverAnnotation)

					By("Define and create a 802.1AD client container")

					clientAnnotation := defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, false)
					clientPod := createClientTestPod(clientNameDot1ad, workerNodeList[0].Definition.Name, clientAnnotation)

					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Validate IPv4 and IPv6 connectivity between the containers over the qinq tunnel.")

					err = sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over the 802.1AD connection")

					By("Validate IPv4 and IPv6 tcp traffic and dot1ad encapsulation from the client to server")
					validateTCPTraffic(clientPod, intNet2, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1ADOutput)
				})

			It("Verify network traffic over a 802.1ad QinQ tunnel between two SRIOV containers in different nodes",
				reportxml.ID("71678"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a server container")

					annotation := defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, true)
					serverPod := createServerTestPod(serverNameDot1ad, workerNodeList[1].Definition.Name, testCmdNet2,
						annotation)

					By("Define and create a 802.1AD client container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, false)
					clientPod := createClientTestPod(clientNameDot1ad, workerNodeList[0].Definition.Name, annotation)

					By("Validate IPv4 and IPv6 connectivity between the containers over the qinq tunnel.")

					err := sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over the 802.1ad connection")

					By("Validate IPv4 and IPv6 tcp traffic and dot1q encapsulation from the client to server")
					validateTCPTraffic(clientPod, intNet2, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1ADOutput)
				})

			It("Verify network traffic over an 802.1ad Q-in-Q tunnel with multiple C-VLANs using the same S-VLAN",
				reportxml.ID("71682"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a server container")

					annotation := defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, true, nadCVLAN101)
					serverPod := createServerTestPod(serverNameDot1ad, workerNodeList[0].Definition.Name, testCmdNet2Net3,
						annotation)

					By("Define and create a 802.1AD client container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, false, nadCVLAN101)
					clientPod := createClientTestPod(clientNameDot1ad, workerNodeList[0].Definition.Name, annotation)

					By("Validate IPv4 and IPv6 connectivity between the containers using CVLAN100.")

					err := sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, "net2")
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over cvlan100")

					By("Validate IPv4 and IPv6 connectivity between the containers using CVLAN101.")

					err = sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet3, "net3")
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over CVLAN101")

					By("Validate IPv4 and IPv6 tcp traffic and dot1ad encapsulation from the client to server " +
						"with CVLAN100.")
					validateTCPTraffic(clientPod, intNet2, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged with CVLAN100 ")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1ADOutput)

					By("Validate IPv4 and IPv6 tcp traffic and dot1ad encapsulation from the client to server " +
						"with CVLAN101.")
					validateTCPTraffic(clientPod, intNet3, serverIPAddressesNet3)

					By("Validate that the TCP traffic is double tagged with CVLAN101 ")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1ADCVLAN101Output)
				})

			It("Verify a negative test with an 802.1ad to 802.1q tunnel between two SRIOV containers",
				reportxml.ID("71680"), func() {
					By("Define and create a server container")

					annotation := defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN100, true)
					serverPod := createServerTestPod(serverNameDot1q, workerNodeList[0].Definition.Name, testCmdNet2,
						annotation)

					By("Define and create a 802.1AD client container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, false)
					_ = createClientTestPod(clientNameDot1q, workerNodeList[0].Definition.Name, annotation)

					By("Validate IPv4 and IPv6 connectivity between the containers over the qinq tunnel.")

					err := sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intNet2)
					Expect(err).To(HaveOccurred(),
						"Ping was successful and expected to fail")
				})

			It("Verify simultaneous network traffic over an 802.1ad and 802.1q Q-in-Q tunneling between two clients "+
				"SRIOV containers",
				reportxml.ID("73105"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a 802.1AD server container")

					annotation := defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, true)
					serverDotADPod := createServerTestPod(serverNameDot1ad, workerNodeList[0].Definition.Name, testCmdNet2,
						annotation)

					By("Define and create a 802.1AD  client container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, false)
					clientDotADPod := createClientTestPod(clientNameDot1ad, workerNodeList[0].Definition.Name, annotation)

					By("Define and create a 802.1Q server container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN101, true)
					serverDotQPod := createServerTestPod(serverNameDot1q, workerNodeList[0].Definition.Name, testCmdNet2,
						annotation)

					By("Define and create a 802.1Q client container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN101, false)
					clientDotQPod := createClientTestPod(clientNameDot1q, workerNodeList[0].Definition.Name, annotation)

					By("Validate IPv4 and IPv6 connectivity between the 802.1AD containers using CVLAN100.")

					err := sriovocpenv.ICMPConnectivityCheck(serverDotADPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over cvlan100")

					By("Validate IPv4 and IPv6 connectivity between the 802.1Q containers using CVLAN101.")

					err = sriovocpenv.ICMPConnectivityCheck(serverDotQPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over cvlan101")

					By("Validate IPv4 and IPv6 tcp traffic and dot1ad encapsulation from the client to server")
					validateTCPTraffic(clientDotADPod, intNet2, serverIPAddressesNet2)

					By("Validate that the 802.1AD TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1ADOutput)

					By("Validate IPv4 and IPv6 tcp traffic and dot1q encapsulation from the client to server")
					validateTCPTraffic(clientDotQPod, intNet2, serverIPAddressesNet2)

					By("Validate that the 802.1Q TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1QCVLAN101QOutput)
				})

			It("Verify network traffic over a 802.1ad Q-in-Q tunneling with Bond interfaces between two clients "+
				"one SRIOV containers",
				reportxml.ID("71684"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a server container")

					serverAnnotation := pod.StaticIPBondAnnotationWithInterface(
						"nadcvlan100", "bond0.100",
						[]string{srIovNetworkDot1AD, srIovNetworkDot1AD, nadMasterBond0},
						[]string{tsparams.ServerIPv4IPAddress, tsparams.ServerIPv6IPAddress})

					serverPod := createServerTestPod(serverNameDot1ad, workerNodeList[0].Definition.Name, testCmdBond0,
						serverAnnotation)

					By("Define and create a 802.1AD client container")

					clientAnnotation := pod.StaticIPBondAnnotationWithInterface("nadcvlan100", "bond0.100",
						[]string{srIovNetworkDot1AD, srIovNetworkDot1AD, nadMasterBond0},
						[]string{tsparams.ClientIPv4IPAddress, tsparams.ClientIPv6IPAddress})
					clientPod := createClientTestPod(clientNameDot1ad, workerNodeList[0].Definition.Name, clientAnnotation)

					By("Validate IPv4 and IPv6 connectivity between the containers over the qinq tunnel.")

					err = sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intBond0)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over the 802.1AD connection")

					By("Validate IPv4 and IPv6 tcp traffic and dot1ad encapsulation from the client to server")
					validateTCPTraffic(clientPod, intBond0, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1ADOutput)
				})
			AfterAll(func() {
				By("Remove 802.1AD TPID from switch interfaces")

				err = removeSwitchTPID(switchCredentials, switchInterfaces)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove 802.1AD TPID from switch")

				By("Clean the test env of sriov and pod deployments")
				cleanTestEnvSRIOVConfiguration()
			})
		})

		Context("802.1Q", func() {
			BeforeAll(func() {
				By("Define and create sriov network policy using worker node label with netDevice type netdevice")

				_, err := sriov.NewPolicyBuilder(
					APIClient,
					srIovPolicyNetDevice,
					SriovOcpConfig.OcpSriovOperatorNamespace,
					srIovPolicyResNameNetDevice,
					5,
					[]string{fmt.Sprintf("%s#0-4", srIovInterfacesUnderTest[0])},
					SriovOcpConfig.WorkerLabelMap).Create()
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create sriovnetwork policy %s",
					srIovPolicyNetDevice))

				By("Waiting until cluster MCP and SR-IOV are stable")

				err = sriovoperator.WaitForSriovAndMCPStable(
					APIClient, tsparams.MCOWaitTimeout, time.Minute,
					SriovOcpConfig.MCPLabel, SriovOcpConfig.OcpSriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")

				By("Define and create sriov-network for the promiscuous client")

				sriovNetworkBuilder := sriov.NewNetworkBuilder(APIClient,
					srIovNetworkPromiscuous, SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName,
					srIovPolicyResNameNetDevice).WithTrustFlag(true).WithLogLevel(logLevelDebug)
				err = createSriovNetworkAndWaitForNAD(sriovNetworkBuilder, tsparams.NADWaitTimeout)
				Expect(err).ToNot(HaveOccurred(),
					"Failed to create and wait for NAD creation for Sriov Network %s with error %v",
					srIovNetworkPromiscuous, err)

				By("Define and create sriov-network with 802.1q S-VLAN")
				defineAndCreateSrIovNetworkWithQinQ(srIovNetworkDot1Q, srIovPolicyResNameNetDevice, dot1q)
				Expect(err).ToNot(HaveOccurred(), "Failed to create sriov network srIovNetworkDot1Q")

				By("Define and create network-attachment-definitions")
				defineAndCreateNADs(nadCVLAN100, nadCVLAN101, nadMasterBond0, intNet1)
			})

			It("Verify network traffic over a 802.1q QinQ tunnel between two SRIOV pods on the same PF",
				reportxml.ID("71677"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a server container")

					serverAnnotation := defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN100, true)
					serverPod := createServerTestPod(serverNameDot1q, workerNodeList[0].Definition.Name, testCmdNet2,
						serverAnnotation)

					By("Define and create a 802.1Q client container")

					clientAnnotation := defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN100, false)
					clientPod := createClientTestPod(clientNameDot1q, workerNodeList[0].Definition.Name, clientAnnotation)

					By("Validate IPv4 and IPv6 connectivity between the containers over the qinq tunnel.")

					err := sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over the 802.1q connection")

					By("Validate IPv4 and IPv6 tcp traffic and dot1q encapsulation from the client to server")
					validateTCPTraffic(clientPod, intNet2, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1QOutput)
				})

			It("Verify network traffic over a 802.1Q QinQ tunnel between two SRIOV containers in different nodes",
				reportxml.ID("71679"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a server container")

					annotation := defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN100, true)
					serverPod := createServerTestPod(serverNameDot1q, workerNodeList[1].Definition.Name, testCmdNet2,
						annotation)

					By("Define and create a 802.1Q client container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN100, false)
					clientPod := createClientTestPod(clientNameDot1q, workerNodeList[0].Definition.Name, annotation)

					By("Validate IPv4 and IPv6 connectivity between the containers over the qinq tunnel.")

					err := sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over the 802.1q connection.")

					By("Validate IPv4 and IPv6 tcp traffic and dot1q encapsulation from the client to server")
					validateTCPTraffic(clientPod, intNet2, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1QOutput)
				})

			It("Verify network traffic over a double tagged 802.1Q tunnel with multiple C-VLANs using the same S-VLAN",
				reportxml.ID("71683"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a server container")

					annotation := defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN100, true, nadCVLAN101)
					serverPod := createServerTestPod(serverNameDot1q, workerNodeList[0].Definition.Name, testCmdNet2Net3,
						annotation)

					By("Define and create a 802.1Q client container")

					annotation = defineNetworkAnnotation(srIovNetworkDot1Q, nadCVLAN100, false, nadCVLAN101)
					clientPod := createClientTestPod(clientNameDot1q, workerNodeList[0].Definition.Name, annotation)

					By("Validate IPv4 and IPv6 connectivity between the containers using CVLAN100 over the qinq tunnel.")

					err := sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over cvlan100")

					By("Validate IPv4 and IPv6 connectivity between the containers using CVLAN101 over the qinq tunnel.")

					err = sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet3, intNet3)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over CVLAN101")

					By("Validate IPv4 and IPv6 tcp traffic and dot1q encapsulation from the client to server " +
						"using CVLAN100")
					validateTCPTraffic(clientPod, intNet2, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged with CVLAN100 ")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1QOutput)

					By("Validate IPv4 and IPv6 tcp traffic and dot1q encapsulation from the client to server " +
						"using CVLAN101")
					validateTCPTraffic(clientPod, intNet3, serverIPAddressesNet3)

					By("Validate that the TCP traffic is double tagged with CVLAN101 ")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1QCVLAN101QOutput)
				})
			AfterAll(func() {
				By("Clean the test env of sriov and pod deployments")
				cleanTestEnvSRIOVConfiguration()
			})
		})

		Context("DPDK", func() {
			BeforeAll(func() {
				By("Deploying PerformanceProfile is it's not installed")

				err = perfprofile.DeployPerformanceProfile(
					APIClient,
					SriovOcpConfig.WorkerLabelMap,
					SriovOcpConfig.MCPLabel,
					perfProfileName,
					"1,3,5,7,9,11,13,15,17,19,21,23,25",
					"0,2,4,6,8,10,12,14,16,18,20",
					24,
					tsparams.MCOWaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")

				defineCreateSriovNetPolices(srIovPolicyVfioPci, srIovPolicyResNameVfioPci, srIovInterfacesUnderTest[0],
					sriovVendor, "vfio-pci")

				By("Setting selinux flag container_use_devices to 1 on all compute nodes")

				err = cluster.ExecCmd(APIClient, SriovOcpConfig.WorkerLabel, "setsebool container_use_devices 1")
				Expect(err).ToNot(HaveOccurred(), "Fail to enable selinux flag")

				By("Define and create sriov-network with 802.1ad S-VLAN")
				defineAndCreateSrIovNetworkWithQinQ(srIovNetworkDPDKDot1AD, srIovPolicyResNameVfioPci, dot1ad)
				err = defineAndCreateSriovNetwork(srIovNetworkDPDKClient, srIovPolicyResNameVfioPci, false, false)
				Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK SriovNetwork client")

				By("Define and create sriov-network with 802.1q S-VLAN")
				defineAndCreateSrIovNetworkWithQinQ(srIovNetworkDPDKDot1Q, srIovPolicyResNameVfioPci, dot1q)

				By("Define and create a network attachment definition for dpdk container")

				tapNad, err := createTapNad(nadCVLANDpdk, tsparams.TestNamespaceName, 0, 0, nil)
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to define the Network-Attachment-Definition %s",
					nadCVLANDpdk))
				_, err = tapNad.Create()
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to create Network-Attachment-Definition %s",
					nadCVLANDpdk))
			})
			It("Verify network traffic over a 802.1ad QinQ tunnel between two DPDK pods on the same PF",
				reportxml.ID("72636"), func() {
					By("Verify SR-IOV Device IDs for interface under test")

					if sriovDeviceID != intelDeviceIDE810QSFP && sriovDeviceID != intelDeviceIDE810SFP {
						Skip(fmt.Sprintf("The NIC %s does not support 802.1AD", sriovDeviceID))
					}

					runQinQDpdkTestCases(
						workerNodeList[0].Definition.Name,
						serverNameDPDKDot1ad,
						clientNameDPDKDot1ad,
						srIovNetworkDPDKDot1AD,
						nadCVLANDpdk,
						tcpDumpDot1ADDPDKOutput)
				})

			It("Verify network traffic over a 802.1q QinQ tunnel between two DPDK pods on the same PF",
				reportxml.ID("72638"), func() {
					testOutPutString := tcpDumpDot1QDPDKOutput

					if sriovDeviceID == intelDeviceIDE710 {
						vlan, err := SriovOcpConfig.GetVLAN()
						Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN value")

						testOutPutString = fmt.Sprintf("(ethertype 802\\.1Q \\(0x8100\\)).*?(vlan %d)", vlan)
					}

					runQinQDpdkTestCases(
						workerNodeList[0].Definition.Name,
						serverNameDPDKDot1q,
						clientNameDPDKDot1q,
						srIovNetworkDPDKDot1Q,
						nadCVLANDpdk,
						testOutPutString)
				})
			AfterAll(func() {
				By("Clean the test env of sriov and pod deployments")
				cleanTestEnvSRIOVConfiguration()
			})
		})

		Context("nmstate", func() {
			const configureNMStatePolicyName = "configurevfs"

			BeforeAll(func() {
				By("Verify SR-IOV Device IDs for interface under test")

				if sriovDeviceID != intelDeviceIDE810QSFP && sriovDeviceID != intelDeviceIDE810SFP {
					Skip(fmt.Sprintf("The NIC %s does not support 802.1AD", sriovDeviceID))
				}

				By("Checking if NMState operator is installed")

				_, err = namespace.Pull(APIClient, nmstateOperatorNamespace)
				if err != nil {
					Skip("NMState operator is not installed on this cluster")
				}

				By("Creating a new instance of NMstate instance")

				err = createNewNMStateAndWaitUntilItsRunning(7 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NMState instance")

				var isMellanox bool

				isMellanox, err = isMellanoxDevice(
					srIovInterfacesUnderTest[0], workerNodeList[0].Object.Name,
				)
				Expect(err).ToNot(HaveOccurred(), "Failed to check if interface is a Mellanox device")

				if isMellanox {
					err = configureSriovMlnxFirmwareOnWorkersAndWaitMCP(
						tsparams.MCOWaitTimeout,
						time.Minute,
						workerNodeList,
						srIovInterfacesUnderTest[0],
						true,
						5,
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to configure Mellanox firmware")
				}

				By("Creating SR-IOV VFs via NMState")

				err = configureVFsAndWaitUntilItsConfigured(
					configureNMStatePolicyName,
					srIovInterfacesUnderTest[0],
					SriovOcpConfig.WorkerLabelMap,
					5,
					tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VFs via NMState")

				err = waitUntilVfsCreated(
					workerNodeList,
					srIovInterfacesUnderTest[0], 5, tsparams.DefaultTimeout,
				)
				Expect(err).ToNot(HaveOccurred(), "Expected number of VFs are not created")

				By("Configure SR-IOV with flag ExternallyManaged true")

				err = createSriovPolicyWithExManaged(sriovAndResourceNameExManagedTrue, srIovInterfacesUnderTest[0])
				Expect(err).ToNot(HaveOccurred(),
					"Failed to create sriov configuration with flag ExternallyManaged true")

				By("Define and create a network attachment definition with a C-VLAN 100")

				err = createVlanNad(nadCVLAN100, tsparams.TestNamespaceName, "net1", 100,
					nad.IPAMStatic())
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to create Network-Attachment-Definition %s",
					nadCVLAN100))

				By("Define and create sriov-networks")
				defineAndCreateSriovNetworks(srIovNetworkPromiscuous, srIovNetworkDot1AD, srIovNetworkDot1Q,
					sriovAndResourceNameExManagedTrue)

				By("Enable VF promiscuous support on sriov interface under test")
				setVFPromiscMode(workerNodeList[0].Definition.Name, srIovInterfacesUnderTest[0], sriovVendor, "on")
			})

			It("Verify an 802.1ad QinQ tunneling between two containers with the VFs configured by NMState",
				reportxml.ID("71681"), func() {
					By("Define and create a container in promiscuous mode")

					tcpDumpContainer := createPromiscuousClient(workerNodeList[0].Definition.Name,
						tcpDumpNet1CMD)

					By("Define and create a 802.1AD server container")

					serverAnnotation := defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, true)
					serverPod := createServerTestPod(serverNameDot1ad, workerNodeList[0].Definition.Name, testCmdNet2,
						serverAnnotation)

					By("Define and create a 802.1AD client container")

					clientAnnotation := defineNetworkAnnotation(srIovNetworkDot1AD, nadCVLAN100, false)
					clientPod := createClientTestPod(clientNameDot1ad, workerNodeList[0].Definition.Name, clientAnnotation)

					By("Validate IPv4 and IPv6 connectivity between the containers over the qinq tunnel.")

					err = sriovocpenv.ICMPConnectivityCheck(serverPod, clientIPAddressesNet2, intNet2)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to ping the client container over the 802.1AD connection")

					By("Validate IPv4 and IPv6 tcp traffic and dot1ad encapsulation from the client to server")
					validateTCPTraffic(clientPod, intNet2, serverIPAddressesNet2)

					By("Validate that the TCP traffic is double tagged")
					readAndValidateTCPDump(tcpDumpContainer, tcpDumpReadFileCMD, tcpDumpDot1ADOutput)
				})

			AfterAll(func() {
				By("Removing SR-IOV VFs via NMState")

				nmstatePolicy := nmstate.NewPolicyBuilder(
					APIClient, configureNMStatePolicyName, SriovOcpConfig.WorkerLabelMap).
					WithInterfaceAndVFs(srIovInterfacesUnderTest[0], 0)
				err = updatePolicyAndWaitUntilItsAvailable(tsparams.DefaultTimeout, nmstatePolicy)
				Expect(err).ToNot(HaveOccurred(), "Failed to update NMState network policy")

				By("Verifying that VFs removed")

				err = waitUntilVfsCreated(
					workerNodeList,
					srIovInterfacesUnderTest[0], 0, tsparams.DefaultTimeout,
				)
				Expect(err).ToNot(HaveOccurred(), "Unexpected amount of VF")

				By("Removing NMState policies")

				err = nmstate.CleanAllNMStatePolicies(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")
			})
		})

		AfterEach(func() {
			By("Removing all containers from test namespace")

			runningNamespace, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace")

			Expect(runningNamespace.CleanObjects(
				tsparams.WaitTimeout, pod.GetGVR())).ToNot(HaveOccurred(), "Failed to the test namespace")
		})

		AfterAll(func() {
			By("Restore switch interfaces to default configuration")

			err = disableQinQOnSwitch(switchCredentials, switchInterfaces)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to remove VLAN double tagging configuration from the switch")

			By(fmt.Sprintf("Disable VF promiscuous support on %s", srIovInterfacesUnderTest[0]))
			setVFPromiscMode(workerNodeList[0].Definition.Name, srIovInterfacesUnderTest[0], sriovVendor, "off")

			By("Removing all SR-IOV Policy")

			err = sriov.CleanAllNetworkNodePolicies(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to clean srIovPolicy")

			By("Removing all srIovNetworks")

			err = sriov.CleanAllNetworksByTargetNamespace(
				APIClient, SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to clean sriov networks")

			By("Waiting until cluster MCP and SR-IOV are stable")

			err = sriovoperator.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute,
				SriovOcpConfig.MCPLabel, SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")
		})
	})

func defineAndCreateSrIovNetworkWithQinQ(srIovNetwork, resName, vlanProtocol string) {
	vlan, err := SriovOcpConfig.GetVLAN()
	Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN value")

	sriovNetworkBuilder := sriov.NewNetworkBuilder(
		APIClient, srIovNetwork, SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName, resName).
		WithVlanProto(vlanProtocol).WithVLAN(uint16(vlan)).WithLogLevel(logLevelDebug)
	err = createSriovNetworkAndWaitForNAD(sriovNetworkBuilder, tsparams.NADWaitTimeout)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to create and wait for NAD creation for Sriov Network %s with error %v",
		srIovNetwork, err)
}

func createPromiscuousClient(nodeName string, tcpDumpCMD []string) *pod.Builder {
	sriovNetworkDefault := pod.StaticIPAnnotation("sriovnetwork-promiscuous", []string{"192.168.100.1/24"})

	clientDefault, err := pod.NewBuilder(APIClient, "client-promiscuous", tsparams.TestNamespaceName,
		SriovOcpConfig.OcpSriovTestContainer).DefineOnNode(nodeName).WithPrivilegedFlag().RedefineDefaultCMD(tcpDumpCMD).
		WithSecondaryNetwork(sriovNetworkDefault).CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run promiscuous pod")

	return clientDefault
}

func createServerTestPod(name, nodeName string, command []string,
	networkAnnotation []*multus.NetworkSelectionElement) *pod.Builder {
	By(fmt.Sprintf("Define and run test pod  %s", name))
	serverBuild, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName,
		SriovOcpConfig.OcpSriovTestContainer).DefineOnNode(nodeName).WithSecondaryNetwork(networkAnnotation).
		RedefineDefaultCMD(command).WithPrivilegedFlag().CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run default client")

	return serverBuild
}

func createClientTestPod(name, nodeName string, networkAnnotation []*multus.NetworkSelectionElement) *pod.Builder {
	By(fmt.Sprintf("Define and run test pod  %s", name))

	clientBuild, err := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName,
		SriovOcpConfig.OcpSriovTestContainer).DefineOnNode(nodeName).WithSecondaryNetwork(networkAnnotation).
		WithPrivilegedFlag().CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to define and run default client")

	return clientBuild
}

func defineNetworkAnnotation(sVlan, cVlan string, server bool, cVlan2 ...string) []*multus.NetworkSelectionElement {
	annotation := []*multus.NetworkSelectionElement{}
	svlanAnnotation := pod.StaticAnnotation(sVlan)

	if server {
		cvlanAnnotation := pod.StaticIPAnnotation(cVlan, []string{tsparams.ServerIPv4IPAddress,
			tsparams.ServerIPv6IPAddress})

		if len(cVlan2) != 0 {
			cvlanAnnotation2 := pod.StaticIPAnnotation(cVlan2[0], []string{tsparams.ServerIPv4IPAddress2,
				tsparams.ServerIPv6IPAddress2})

			return append(annotation, svlanAnnotation, cvlanAnnotation[0], cvlanAnnotation2[0])
		}

		return append(annotation, svlanAnnotation, cvlanAnnotation[0])
	}

	cvlanAnnotation := pod.StaticIPAnnotation(cVlan, []string{tsparams.ClientIPv4IPAddress,
		tsparams.ClientIPv6IPAddress})

	if len(cVlan2) != 0 {
		cvlanAnnotation2 := pod.StaticIPAnnotation(cVlan2[0], []string{tsparams.ClientIPv4IPAddress2,
			tsparams.ClientIPv6IPAddress2})

		return append(annotation, svlanAnnotation, cvlanAnnotation[0], cvlanAnnotation2[0])
	}

	return append(annotation, svlanAnnotation, cvlanAnnotation[0])
}

func discoverInterfaceUnderTestDeviceID(srIovInterfaceUnderTest, workerNodeName string) string {
	sriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(
		APIClient, workerNodeName, SriovOcpConfig.OcpSriovOperatorNamespace).GetUpNICs()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("fail to discover device ID for network interface %s",
		srIovInterfaceUnderTest))

	for _, srIovInterface := range sriovInterfaces {
		if srIovInterface.Name == srIovInterfaceUnderTest {
			return srIovInterface.DeviceID
		}
	}

	return ""
}

func validateTCPTraffic(clientPod *pod.Builder, interfaceName string, destIPAddrs []string) {
	for _, destIPAddr := range destIPAddrs {
		command := []string{
			"testcmd",
			fmt.Sprintf("--interface=%s", interfaceName),
			fmt.Sprintf("--server=%s", destIPAddr),
			"--protocol=tcp",
			"--mtu=100",
			"--port=4444",
		}

		outPut, err := clientPod.ExecCommand(
			command)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to run testcmd on %s command output: %s",
			clientPod.Definition.Name, outPut.String()))
	}
}

func readAndValidateTCPDump(clientPod *pod.Builder, testCmd []string, pattern string) {
	By("Start to capture traffic on the promiscuous client")

	output, err := clientPod.ExecCommand(testCmd)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error executing command: %s", output.String()))

	err = validateDot1Encapsulation(output.String(), pattern)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to validate qinq encapsulation %s", output.String()))
}

func validateDot1Encapsulation(fileOutput, dot1X string) error {
	regex := regexp.MustCompile(dot1X)
	klog.V(90).Infof("REGEX %s", regex.String())
	match := regex.FindStringSubmatch(fileOutput)

	if len(match) == 0 {
		return fmt.Errorf("regular expression did not match")
	}

	if len(match) != 3 {
		return fmt.Errorf("failed to match double encapsulation")
	}

	klog.V(90).Infof("Matched S-VLAN %s", match[1])
	klog.V(90).Infof("Matched C-VLAN %s", match[2])

	return nil
}

func enableDot1ADonSwitchInterfaces(credentials *sriovocpenv.SwitchCredentials, switchInterfaces []string) error {
	jnpr, err := sriovocpenv.NewJunosSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	for _, switchInterface := range switchInterfaces {
		commands := []string{
			fmt.Sprintf("delete interfaces %s unit 0", switchInterface),
			fmt.Sprintf("set interfaces %s ether-options ethernet-switch-profile tag-protocol-id 0x88a8",
				switchInterface),
			fmt.Sprintf("set interfaces %s flexible-vlan-tagging encapsulation extended-vlan-bridge", switchInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk",
				switchInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching vlan members all",
				switchInterface),
		}

		err = jnpr.Config(commands)
		if err != nil {
			return err
		}
	}

	return nil
}

func removeSwitchTPID(credentials *sriovocpenv.SwitchCredentials, switchInterfaces []string) error {
	jnpr, err := sriovocpenv.NewJunosSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	for _, switchInterface := range switchInterfaces {
		commands := []string{
			fmt.Sprintf("delete interfaces %s ether-options ethernet-switch-profile tag-protocol-id",
				switchInterface),
		}

		err = jnpr.Config(commands)
		if err != nil {
			return err
		}
	}

	return nil
}

func disableQinQOnSwitch(switchCredentials *sriovocpenv.SwitchCredentials, switchInterfaces []string) error {
	jnpr, err := sriovocpenv.NewJunosSession(switchCredentials.SwitchIP, switchCredentials.User,
		switchCredentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	vlan, err := SriovOcpConfig.GetVLAN()
	if err != nil {
		return err
	}

	vlanName := strconv.Itoa(vlan)

	for _, switchInterface := range switchInterfaces {
		commands := []string{
			fmt.Sprintf("delete interfaces %s ether-options ethernet-switch-profile tag-protocol-id",
				switchInterface),
			fmt.Sprintf("delete interfaces %s flexible-vlan-tagging", switchInterface),
			fmt.Sprintf("delete interfaces %s encapsulation extended-vlan-bridge", switchInterface),
			fmt.Sprintf("delete interfaces %s unit 0", switchInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode access", switchInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching vlan members %s", switchInterface, vlanName),
		}

		err = jnpr.Config(commands)
		if err != nil {
			return err
		}
	}

	return nil
}

func defineTestServerPmdCmd(ethPeer, pciAddress string) []string {
	baseCmd := fmt.Sprintf("dpdk-testpmd -a %s -- --forward-mode txonly --eth-peer=0,%s "+
		"--cmdline-file=/etc/cmd/cmd_file --stats-period 5", pciAddress, ethPeer)

	return []string{"/bin/bash", "-c", baseCmd}
}

func defineTestClientPmdCmd(pciAddress string) []string {
	baseCmd := fmt.Sprintf(
		"timeout -s SIGKILL 20 dpdk-testpmd "+
			"--vdev=virtio_user0,path=/dev/vhost-net,queues=2,queue_size=1024,iface=net2 -a %s "+
			"-- --stats-period 5", pciAddress)

	return []string{baseCmd}
}

func defineAndCreateServerDPDKPod(
	podName,
	nodeName string,
	serverPodNetConfig []*multus.NetworkSelectionElement,
	podCmd []string) *pod.Builder {
	var rootUser int64

	securityContext := corev1.SecurityContext{
		RunAsUser: &rootUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"},
		},
	}

	dpdkContainerCfg, err := pod.NewContainerBuilder(podName, SriovOcpConfig.DpdkTestContainer, podCmd).
		WithSecurityContext(&securityContext).WithResourceLimit("2Gi", "1Gi", 4).
		WithResourceRequest("2Gi", "1Gi", 4).WithEnvVar("RUN_TYPE", "testcmd").
		GetContainerCfg()

	Expect(err).ToNot(HaveOccurred(), "Fail to define server dpdk container")

	dpdkPort0Cmd := `port stop 0
tx_vlan set 0 100
port start 0
start
`
	configMapData := map[string]string{"cmd_file": dpdkPort0Cmd}
	configMap, err := configmap.NewBuilder(APIClient, "dpdk-port-cmd", tsparams.TestNamespaceName).
		WithData(configMapData).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create config map")
	dpdkPod, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, SriovOcpConfig.DpdkTestContainer).
		WithSecondaryNetwork(serverPodNetConfig).
		DefineOnNode(nodeName).
		RedefineDefaultContainer(*dpdkContainerCfg).
		WithHugePages().WithLocalVolume(configMap.Definition.Name, "/etc/cmd").
		CreateAndWaitUntilRunning(4 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Fail to create a dpdk server pod")

	return dpdkPod
}

func defineAndCreateClientDPDKPod(
	podName,
	nodeName string,
	serverPodNetConfig []*multus.NetworkSelectionElement) *pod.Builder {
	var rootUser = int64(0)

	securityContext := corev1.SecurityContext{
		RunAsUser: &rootUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"},
		},
	}
	testCommand := []string{"bash", "-c", "tcpdump -l -i net2 -e > /tmp/tcpdump"}

	dpdkContainerCfg, err := pod.NewContainerBuilder(podName, SriovOcpConfig.DpdkTestContainer,
		[]string{"/bin/bash", "-c", "sleep INF"}).WithSecurityContext(&securityContext).
		WithResourceLimit("2Gi", "1Gi", 4).
		WithResourceRequest("2Gi", "1Gi", 4).WithEnvVar("RUN_TYPE", "testcmd").
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Fail to define client dpdk container")

	dpdkPod, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName,
		SriovOcpConfig.DpdkTestContainer).WithSecondaryNetwork(serverPodNetConfig).DefineOnNode(nodeName).
		RedefineDefaultContainer(*dpdkContainerCfg).WithHugePages().RedefineDefaultCMD(testCommand).
		CreateAndWaitUntilRunning(4 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Fail to create a dpdk client pod")

	return dpdkPod
}

func runQinQDpdkTestCases(nodeName, serverName, clientName, sriovNetworkName, nadCVLANDpdk, outPutSubString string) {
	By("Define and create a 802.1AD dpdk server container")

	annotation := pod.StaticIPAnnotationWithMacAddress(sriovNetworkName, []string{}, tsparams.ServerMacAddress)
	testCmdServer := defineTestServerPmdCmd(tsparams.ClientMacAddress,
		"${PCIDEVICE_OPENSHIFT_IO_SRIOVPOLICYVFIOPCI}")
	_ = defineAndCreateServerDPDKPod(serverName, nodeName, annotation, testCmdServer)

	By("Define and create a dpdk client container")

	var annotationDpdk []*multus.NetworkSelectionElement

	sVlan := pod.StaticIPAnnotationWithMacAddress("sriovnetwork-dpdk-client", []string{}, tsparams.ClientMacAddress)
	cVlan := pod.StaticAnnotation(nadCVLANDpdk)
	annotationDpdk = append(annotationDpdk, sVlan[0], cVlan)
	clientDpdk := defineAndCreateClientDPDKPod(clientName, nodeName, annotationDpdk)
	Expect(clientDpdk.WaitUntilRunning(time.Minute)).ToNot(HaveOccurred(),
		"Fail to wait until pod is running")

	By("Validate dpdk_testpmd traffic from the server to the client using CVLAN100.")

	clientRxCmd := defineTestClientPmdCmd("${PCIDEVICE_OPENSHIFT_IO_SRIOVPOLICYVFIOPCI}")

	err := rxTrafficOnClientPod(clientDpdk, clientRxCmd[0])
	Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")

	By("Validate that the TCP traffic is double tagged")
	readAndValidateTCPDump(clientDpdk, []string{"bash", "-c", "tail -20 /tmp/tcpdump"}, outPutSubString)
}

func defineQinQBondNAD(nadname, mode string) *nad.Builder {
	bondNad, err := nad.NewMasterBondPlugin(nadname, mode).WithFailOverMac(1).
		WithLinksInContainer(true).WithVLANInContainer(uint16(100)).WithMiimon(100).
		WithLinks([]nad.Link{{Name: "net1"}, {Name: "net2"}}).WithIPAM(&nad.IPAM{Type: ""}).GetMasterPluginConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to define Bond NAD for %s", nadname)

	createdNad, err := nad.NewBuilder(APIClient, nadname, tsparams.TestNamespaceName).WithMasterPlugin(bondNad).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create Bond NAD for %s", nadname)

	return createdNad
}

func defineCreateSriovNetPolices(vfioPCIName, vfioPCIResName, sriovInterface,

	sriovVendor, reqDriver string) {
	By("Define and create sriov network policy using worker node label with netDevice type vfio-pci")

	sriovPolicy := sriov.NewPolicyBuilder(
		APIClient,
		vfioPCIName,
		SriovOcpConfig.OcpSriovOperatorNamespace,
		vfioPCIResName,
		6,
		[]string{fmt.Sprintf("%s#0-5", sriovInterface)},
		SriovOcpConfig.WorkerLabelMap).WithVhostNet(true)

	switch reqDriver {
	case "vfio-pci":
		if sriovVendor == mlxVendorID {
			_, err := sriovPolicy.WithRDMA(true).WithDevType("netdevice").Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create Mellanox sriovnetwork policy %s",
				vfioPCIName))
		} else {
			_, err := sriovPolicy.WithDevType("vfio-pci").Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create Intel sriovnetwork policy %s",
				vfioPCIName))
		}
	case "netdevice":
		_, err := sriovPolicy.WithDevType("netdevice").Create()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create sriovnetwork policy %s",
			vfioPCIName))
	}

	By("Waiting until cluster MCP and SR-IOV are stable")

	err := sriovoperator.WaitForSriovAndMCPStable(
		APIClient, tsparams.MCOWaitTimeout, time.Minute,
		SriovOcpConfig.MCPLabel, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")
}

func defineAndCreateSriovNetworks(sriovNetworkPromiscName, sriovNetworkDot1ADName, sriovNetworkDot1QName,
	sriovResName string) {
	By("Define and create sriov-network for the promiscuous client")

	sriovNetworkBuilder := sriov.NewNetworkBuilder(APIClient,
		sriovNetworkPromiscName, SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName,
		sriovResName).WithTrustFlag(true).WithLogLevel(logLevelDebug)
	err := createSriovNetworkAndWaitForNAD(sriovNetworkBuilder, tsparams.NADWaitTimeout)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to create and wait for NAD creation for Sriov Network %s with error %v",
		sriovNetworkPromiscName, err)

	By("Define and create sriov-network with 802.1ad S-VLAN")
	defineAndCreateSrIovNetworkWithQinQ(sriovNetworkDot1ADName, sriovResName, "802.1ad")

	By("Define and create sriov-network with 802.1q S-VLAN")
	defineAndCreateSrIovNetworkWithQinQ(sriovNetworkDot1QName, sriovResName, "802.1q")
}

func defineAndCreateNADs(nadCVLAN100, nadCVLAN101, nadMasterBond0, intNet1 string) {
	By("Define and create a network attachment definition with a C-VLAN 100")

	err := createVlanNad(nadCVLAN100, tsparams.TestNamespaceName, "net1", 100,
		nad.IPAMStatic())
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to create Network-Attachment-Definition %s",
		nadCVLAN100))

	By("Define and create a network attachment definition with a C-VLAN 101")

	err = createVlanNad(nadCVLAN101, tsparams.TestNamespaceName, intNet1, 101,
		nad.IPAMStatic())
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to create Network-Attachment-Definition %s",
		nadCVLAN101))

	By("Define and create a Bonded network attachment definition with a C-VLAN 100")

	_ = defineQinQBondNAD(nadMasterBond0, "active-backup")
}

func setVFPromiscMode(nodeName, srIovInterfacesUnderTest, sriovVendor, onOff string) {
	promiscVFCommand := fmt.Sprintf("ethtool --set-priv-flags %s vf-true-promisc-support %s",
		srIovInterfacesUnderTest, onOff)
	if sriovVendor == mlxVendorID {
		promiscVFCommand = fmt.Sprintf("ip link set %s promisc %s",
			srIovInterfacesUnderTest, onOff)
	}

	output, err := runCommandOnHostNetworkPod(nodeName, SriovOcpConfig.OcpSriovOperatorNamespace, promiscVFCommand)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command on node %s", output))
}

func cleanTestEnvSRIOVConfiguration() {
	By("Removing all containers from test namespace")

	runningNamespace, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace")

	Expect(runningNamespace.CleanObjects(
		tsparams.WaitTimeout, pod.GetGVR())).ToNot(HaveOccurred(), "Failed to the test namespace")
	By("Removing all SR-IOV Policy")

	err = sriov.CleanAllNetworkNodePolicies(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to clean srIovPolicy")

	By("Removing all srIovNetworks")

	err = sriov.CleanAllNetworksByTargetNamespace(
		APIClient, SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to clean sriov networks")

	By("Waiting until cluster MCP and SR-IOV are stable")

	err = sriovoperator.WaitForSriovAndMCPStable(
		APIClient, tsparams.MCOWaitTimeout, time.Minute,
		SriovOcpConfig.MCPLabel, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")
}

func createSriovPolicyWithExManaged(sriovAndResName, sriovInterfaceName string) error {
	klog.V(90).Infof("Creating SR-IOV policy with flag ExternallyManaged true")

	sriovPolicy := sriov.NewPolicyBuilder(APIClient, sriovAndResName, SriovOcpConfig.OcpSriovOperatorNamespace,
		sriovAndResName,
		5, []string{sriovInterfaceName}, SriovOcpConfig.WorkerLabelMap).WithExternallyManaged(true)

	err := sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
		APIClient,
		SriovOcpConfig.WorkerLabelEnvVar,
		SriovOcpConfig.OcpSriovOperatorNamespace,
		sriovPolicy,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration)
	if err != nil {
		return fmt.Errorf("failed to sriov policy, %w", err)
	}

	return nil
}

func createSriovNetworkAndWaitForNAD(sNet *sriov.NetworkBuilder, timeout time.Duration) error {
	sriovNetwork, err := sNet.Create()
	if err != nil {
		return err
	}

	targetNs := sriovNetwork.Object.Spec.NetworkNamespace
	if targetNs == "" {
		targetNs = sriovNetwork.Object.Namespace
	}

	return wait.PollUntilContextTimeout(context.TODO(),
		time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, pullErr := nad.Pull(APIClient, sriovNetwork.Object.Name, targetNs)

			return pullErr == nil, nil
		})
}

func defineAndCreateSriovNetwork(networkName, resourceName string, withStaticIP, withTrust bool) error {
	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, networkName, SriovOcpConfig.OcpSriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithMacAddressSupport().
		WithLogLevel(logLevelDebug)

	if withTrust {
		networkBuilder = networkBuilder.WithTrustFlag(true)
	}

	if withStaticIP {
		networkBuilder = networkBuilder.WithStaticIpam()
	}

	return createSriovNetworkAndWaitForNAD(networkBuilder, tsparams.NADWaitTimeout)
}

func runCommandOnHostNetworkPod(nodeName, nsName, command string) (string, error) {
	klog.V(90).Infof("Running command %s on the host network pod on node %s",
		command, nodeName)

	testPod, err := pod.NewBuilder(APIClient, "hostnetworkpod", nsName, SriovOcpConfig.OcpSriovTestContainer).
		DefineOnNode(nodeName).WithPrivilegedFlag().WithHostNetwork().CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		return "", err
	}

	output, err := testPod.ExecCommand([]string{"/bin/bash", "-c", command})
	if err != nil {
		return "", err
	}

	_, err = testPod.DeleteAndWait(tsparams.DefaultTimeout)
	if err != nil {
		return "", err
	}

	return output.String(), nil
}

func rxTrafficOnClientPod(clientPod *pod.Builder, clientRxCmd string) error {
	timeoutError := "command terminated with exit code 137"

	klog.V(90).Infof("Checking dpdk-pmd traffic command %s from the client pod %s",
		clientRxCmd, clientPod.Definition.Name)

	output, err := clientPod.ExecCommand([]string{"/bin/bash", "-c", clientRxCmd})
	if err != nil && err.Error() != timeoutError {
		return fmt.Errorf("failed to execute dpdk-testpmd on pod %s: error %w, output: %s",
			clientPod.Definition.Name, err, output.String())
	}

	return nil
}

func createVlanNad(name, nsName, intName string, vlanID uint16, ipam *nad.IPAM) error {
	masterPlugin, err := nad.NewMasterVlanPlugin(name, vlanID).WithMasterInterface(intName).
		WithIPAM(ipam).WithLinkInContainer().GetMasterPluginConfig()
	if err != nil {
		return err
	}

	_, err = nad.NewBuilder(APIClient, name, nsName).WithMasterPlugin(masterPlugin).Create()

	return err
}

func createTapNad(name string, nsname string, user int, group int,
	sysctlConfig map[string]string) (*nad.Builder, error) {
	plugins := []nad.Plugin{*nad.TapPlugin(user, group, true)}

	if sysctlConfig != nil {
		plugins = append(plugins, *nad.TuningSysctlPlugin(true, sysctlConfig))
	}

	return nad.NewBuilder(APIClient, name, nsname).WithPlugins(name, &plugins).Create()
}

func isMellanoxDevice(intName, nodeName string) (bool, error) {
	sriovNetworkState := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName,
		SriovOcpConfig.OcpSriovOperatorNamespace)

	driverName, err := sriovNetworkState.GetDriverName(intName)
	if err != nil {
		return false, fmt.Errorf("failed to get driver name for interface %s on node %s: %w", intName, nodeName, err)
	}

	return driverName == "mlx5_core", nil
}

func waitUntilVfsCreated(
	nodeList []*nodes.Builder,
	sriovInterfaceName string,
	numberOfVfs int,
	timeout time.Duration,
) error {
	for _, node := range nodeList {
		err := wait.PollUntilContextTimeout(
			context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
				sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
					APIClient, node.Object.Name, SriovOcpConfig.OcpSriovOperatorNamespace)

				if discoverErr := sriovNetworkState.Discover(); discoverErr != nil {
					return false, nil
				}

				sriovNumVfs, numErr := sriovNetworkState.GetNumVFs(sriovInterfaceName)
				if numErr != nil {
					return false, nil
				}

				return sriovNumVfs == numberOfVfs, nil
			})
		if err != nil {
			return err
		}
	}

	return nil
}

func configureSriovMlnxFirmwareOnWorkersAndWaitMCP(
	mcpTimeout time.Duration,
	stableDuration time.Duration,
	workerNodes []*nodes.Builder,
	sriovInterfaceName string,
	enableSriov bool,
	numVfs int,
) error {
	for _, workerNode := range workerNodes {
		sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
			APIClient, workerNode.Object.Name, SriovOcpConfig.OcpSriovOperatorNamespace)

		pciAddress, err := sriovNetworkState.GetPciAddress(sriovInterfaceName)
		if err != nil {
			return fmt.Errorf("failed to get PCI address: %w", err)
		}

		mstconfigCmd := fmt.Sprintf("mstconfig -y -d %s set SRIOV_EN=%t NUM_OF_VFS=%d",
			pciAddress, enableSriov, numVfs)

		pods, err := pod.List(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace, metav1.ListOptions{
			LabelSelector: "app=sriov-network-config-daemon",
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", workerNode.Object.Name),
		})
		if err != nil || len(pods) == 0 {
			return fmt.Errorf("failed to find config daemon pod on node %s", workerNode.Object.Name)
		}

		output, err := pods[0].ExecCommand([]string{"bash", "-c", mstconfigCmd})
		if err != nil {
			return fmt.Errorf("failed to configure Mellanox firmware on node %s: %s %w",
				workerNode.Object.Name, output.String(), err)
		}

		_, _ = pods[0].ExecCommand([]string{"bash", "-c", "chroot /host reboot"})
	}

	time.Sleep(10 * time.Second)

	return cluster.WaitForMcpStable(APIClient, mcpTimeout, stableDuration, SriovOcpConfig.MCPLabel)
}

const (
	nmstateName                  = "nmstate"
	nmstateHandlerDsName         = "nmstate-handler"
	nmstateWebhookDeploymentName = "nmstate-webhook"
	nmstateOperatorNamespace     = "openshift-nmstate"
)

func createNewNMStateAndWaitUntilItsRunning(timeout time.Duration) error {
	nmstateInstance, err := nmstate.PullNMstate(APIClient, nmstateName)
	if err == nil {
		_, err = nmstateInstance.Delete()
		if err != nil {
			return err
		}
	}

	_, err = nmstate.NewBuilder(APIClient, nmstateName).Create()
	if err != nil {
		return err
	}

	return isNMStateDeployedAndReady(timeout)
}

func isNMStateDeployedAndReady(timeout time.Duration) error {
	var (
		nmstateHandlerDs         *daemonset.Builder
		nmstateWebhookDeployment *deployment.Builder
		err                      error
	)

	err = wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			nmstateHandlerDs, err = daemonset.Pull(
				APIClient, nmstateHandlerDsName, nmstateOperatorNamespace)
			if err != nil {
				return false, nil
			}

			nmstateWebhookDeployment, err = deployment.Pull(
				APIClient, nmstateWebhookDeploymentName, nmstateOperatorNamespace)
			if err != nil {
				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return err
	}

	time.Sleep(10 * time.Second)

	if !nmstateHandlerDs.IsReady(timeout) {
		return fmt.Errorf("nmstate handler daemonset is not ready")
	}

	if !nmstateWebhookDeployment.IsReady(timeout) {
		return fmt.Errorf("nmstate webhook deployment is not ready")
	}

	return nil
}

func configureVFsAndWaitUntilItsConfigured(
	policyName string,
	sriovInterfaceName string,
	nodeLabel map[string]string,
	numberOfVFs uint8,
	timeout time.Duration) error {
	nmstatePolicy := nmstate.NewPolicyBuilder(
		APIClient, policyName, nodeLabel).WithInterfaceAndVFs(sriovInterfaceName, numberOfVFs)

	nmstatePolicy, err := nmstatePolicy.Create()
	if err != nil {
		return err
	}

	return nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionAvailable, timeout)
}

func updatePolicyAndWaitUntilItsAvailable(timeout time.Duration, nmstatePolicy *nmstate.PolicyBuilder) error {
	nmstatePolicy, err := nmstatePolicy.Update(true)
	if err != nil {
		return err
	}

	err = nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionProgressing, timeout)
	if err != nil {
		return err
	}

	return nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionAvailable, timeout)
}
