package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/scc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/dpdk/internal/link"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/dpdk/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

const (
	srIovNetworkTwoResName       = "dpdkpolicytwo"
	srIovPolicyOneResName        = "dpdkpolicyone"
	setSEBool                    = "setsebool container_use_devices "
	srIovNetworkOneName          = "sriov-net-one"
	srIovNetworkTwoName          = "sriov-net-two"
	tapNetworkOne                = "tap-one"
	tapNetworkTwo                = "tap-two"
	vlanNetworkOne               = "vlan-one"
	vlanNetworkTwo               = "vlan-two"
	ipVlanNetworkOne             = "ip-vlan-one"
	macVlanNetworkOne            = "mac-vlan-one"
	macVlanNetworkTwo            = "mac-vlan-two"
	tapOneInterfaceName          = "ext0"
	tapTwoInterfaceName          = "ext1"
	defaultWhereAboutNetwork     = "1.1.1.0/24"
	defaultWhereAboutGW          = "1.1.1.254"
	networkForRouteTest          = "10.10.10.0/24"
	dpdkServerMac                = "60:00:00:00:00:02"
	dpdkClientMac                = "60:00:00:00:00:01"
	dpdkServerMacTwo             = "60:00:00:00:00:04"
	dpdkClientMacTwo             = "60:00:00:00:00:03"
	firstInterfaceBasedOnTapOne  = "ext0.1"
	secondInterfaceBasedOnTapOne = "ext0.2"
	firstInterfaceBasedOnTapTwo  = "ext1.1"
	mlxVendorID                  = "15b3"
	intelVendorID                = "8086"
	maxMulticastNoiseRate        = 5000
	minimumExpectedDPDKRate      = 1000000
	customUserID                 = 2005
	customGroupID                = 2005
	dummyVlanID                  = 200
)

var (
	sleepCMD         = []string{"/bin/bash", "-c", "sleep INF"}
	rootUser         = int64(0)
	customSCCGroupID = int64(customGroupID)
	customSCCUserID  = int64(customUserID)
	hugePagesGroup   = int64(1001)
	falseFlag        = false
	trueFlag         = true
	workerNodes      []*nodes.Builder

	serverSC = corev1.SecurityContext{
		RunAsUser: &rootUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW"},
		},
	}

	clientPodSC = corev1.PodSecurityContext{
		FSGroup:    &hugePagesGroup,
		RunAsGroup: &customSCCGroupID,
		SeccompProfile: &corev1.SeccompProfile{
			Type: "RuntimeDefault",
		},
	}

	clientSC = corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
			Add:  []corev1.Capability{"IPC_LOCK", "NET_ADMIN", "NET_RAW"},
		},
		RunAsUser:                &customSCCUserID,
		Privileged:               &falseFlag,
		RunAsNonRoot:             &trueFlag,
		AllowPrivilegeEscalation: &trueFlag,
	}

	// enabledSysctlFlags represents sysctl configuration with few enable flags for sysctl plugin.
	enabledSysctlFlags = map[string]string{
		"net.ipv6.conf.IFNAME.accept_ra":  "1",
		"net.ipv4.conf.IFNAME.arp_accept": "1",
	}
	// disabledSysctlFlags represents sysctl configuration with few disabled flags for sysctl plugin.
	disabledSysctlFlags = map[string]string{
		"net.ipv6.conf.IFNAME.accept_ra":  "0",
		"net.ipv4.conf.IFNAME.arp_accept": "0",
	}

	defaultWhereaboutIPAM = nad.IPAMWhereAbouts(defaultWhereAboutNetwork, defaultWhereAboutGW)
	vlanID                uint16
	nicVendor             string
)

type podNetworkAnnotation struct {
	Name      string   `json:"name"`
	Interface string   `json:"interface"`
	Ips       []string `json:"ips,omitempty"`
	Mac       string   `json:"mac"`
	Default   bool     `json:"default,omitempty"`
	DNS       struct {
	} `json:"dns"`
	DeviceInfo struct {
		Type    string `json:"type"`
		Version string `json:"version"`
		Pci     struct {
			PciAddress string `json:"pci-address"`
		} `json:"pci"`
	} `json:"device-info,omitempty"`
}

var _ = Describe("rootless", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {

	Context("server-tx, client-rx connectivity test on different nodes", Label("rootless"), func() {
		BeforeAll(func() {
			By("Discover worker nodes")
			var err error
			workerNodes, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Fail to discover nodes")

			By("Collecting SR-IOV interface for rootless dpdk tests")
			srIovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Collection SR-IOV interfaces from Nodes")
			nicVendor, err = discoverNICVendor(srIovInterfacesUnderTest[0], workerNodes[0].Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "failed to discover NIC vendor %s", srIovInterfacesUnderTest[0])

			By("Defining dpdk-policies")
			srIovPolicies := []*sriov.PolicyBuilder{
				sriov.NewPolicyBuilder(
					APIClient,
					"dpdk-policy-one",
					NetConfig.SriovOperatorNamespace,
					srIovPolicyOneResName,
					5,
					[]string{fmt.Sprintf("%s#0-1", srIovInterfacesUnderTest[0])},
					NetConfig.WorkerLabelMap).WithMTU(1500).WithVhostNet(true),
				sriov.NewPolicyBuilder(
					APIClient,
					"dpdk-policy-two",
					NetConfig.SriovOperatorNamespace,
					srIovNetworkTwoResName,
					5,
					[]string{fmt.Sprintf("%s#2-4", srIovInterfacesUnderTest[0])},
					NetConfig.WorkerLabelMap).WithMTU(1500).WithVhostNet(false),
			}

			for index := range srIovPolicies {
				srIovPolicyName := srIovPolicies[index].Definition.Name
				switch nicVendor {
				case mlxVendorID:
					By(fmt.Sprintf("Adding Mlx specific configuration to dpdk-policy %s", srIovPolicyName))
					srIovPolicies[index].WithDevType("netdevice").WithRDMA(true)
				case intelVendorID:
					By(fmt.Sprintf("Adding Intel specific configuration to dpdk-policy %s", srIovPolicyName))
					srIovPolicies[index].WithDevType("vfio-pci")
				}
				By(fmt.Sprintf("Creating dpdk-policy %s", srIovPolicyName))
				_, err = srIovPolicies[index].Create()
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Fail to create %s dpdk policy", srIovPolicies[index].Definition.Name))
			}

			By("Waiting until cluster MCP and SR-IOV are stable")
			// This used to be to check for sriov not to be stable first,
			// then stable. The issue is that if no configuration is applied, then
			// the status will never go to not stable and the test will fail.
			time.Sleep(5 * time.Second)
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "fail cluster is not stable")

			By("Setting selinux flag container_use_devices to 1 on all compute nodes")
			err = cluster.ExecCmd(APIClient, NetConfig.WorkerLabel, setSEBool+"1")
			Expect(err).ToNot(HaveOccurred(), "Fail to enable selinux flag")

			By("Setting vlan ID")
			vlanID, err = NetConfig.GetVLAN()
			Expect(err).ToNot(HaveOccurred(), "Fail to set vlanID")
			Expect(dummyVlanID).ToNot(BeEquivalentTo(vlanID),
				"Both vlans have the same ID. Please select different ID using ECO_CNF_CORE_NET_VLAN env variable")
		})

		It("single VF, multiple tap devices, multiple mac-vlans", reportxml.ID("63806"), func() {
			defineAndCreateSrIovNetworks(vlanID)
			defineAndCreateTapNADs(nil, nil)

			By("Creating first mac-vlan NetworkAttachmentDefinition")
			_, err := define.MacVlanNad(
				APIClient, macVlanNetworkOne, tsparams.TestNamespaceName, tapOneInterfaceName, defaultWhereaboutIPAM)
			Expect(err).ToNot(HaveOccurred(), "Fail to create first mac-vlan NetworkAttachmentDefinition")

			By("Creating second mac-vlan NetworkAttachmentDefinition")
			_, err = define.MacVlanNad(
				APIClient, macVlanNetworkTwo, tsparams.TestNamespaceName, tapTwoInterfaceName, defaultWhereaboutIPAM)
			Expect(err).ToNot(HaveOccurred(), "Fail to create second mac-vlan NetworkAttachmentDefinition")

			By("Creating server pod")

			serverPodNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
				srIovNetworkTwoName, tsparams.TestNamespaceName, dpdkServerMac)
			defineAndCreateDPDKPod(
				"serverpod",
				workerNodes[0].Definition.Name,
				serverSC,
				nil,
				serverPodNetConfig,
				defineTestServerPmdCmd(dpdkClientMac, "${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYTWO}", ""))

			By("Creating client pod")
			clientPodNetConfig := definePodNetwork([]map[string]string{
				{"netName": srIovNetworkOneName, "macAddr": dpdkClientMac},
				{"netName": tapNetworkOne, "intName": tapOneInterfaceName},
				{"netName": tapNetworkTwo, "intName": tapTwoInterfaceName},
				{"netName": macVlanNetworkOne, "intName": firstInterfaceBasedOnTapOne, "macAddr": dpdkClientMac},
				{"netName": macVlanNetworkOne, "intName": secondInterfaceBasedOnTapOne},
				{"netName": macVlanNetworkTwo, "intName": firstInterfaceBasedOnTapTwo}})

			clientPod := defineAndCreateDPDKPod(
				"clientpod",
				workerNodes[1].Definition.Name,
				clientSC,
				&clientPodSC,
				clientPodNetConfig,
				sleepCMD,
			)

			err = cmd.RxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapOneInterfaceName,
				"${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYONE}"))
			Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")

			checkRxOutputRateForInterfaces(
				clientPod,
				map[string]int{
					tapOneInterfaceName:          minimumExpectedDPDKRate,
					tapTwoInterfaceName:          maxMulticastNoiseRate,
					firstInterfaceBasedOnTapOne:  minimumExpectedDPDKRate,
					secondInterfaceBasedOnTapOne: maxMulticastNoiseRate},
			)

			err = cmd.RxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapTwoInterfaceName,
				"${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYONE}"))
			Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")
			checkRxOutputRateForInterfaces(
				clientPod,
				map[string]int{tapTwoInterfaceName: minimumExpectedDPDKRate, firstInterfaceBasedOnTapTwo: maxMulticastNoiseRate})
		})

		It("multiple VFs, one tap plus MAC-VLAN, second tap plus 2 VLANs, filter untagged and tagged traffic",
			reportxml.ID("63818"), func() {
				defineAndCreateSrIovNetworks(vlanID)
				defineAndCreateTapNADs(nil, nil)

				By("Creating mac-vlan one")
				_, err := define.MacVlanNad(
					APIClient, macVlanNetworkOne, tsparams.TestNamespaceName, tapOneInterfaceName, defaultWhereaboutIPAM)
				Expect(err).ToNot(HaveOccurred(), "Fail to create first mac-vlan NetworkAttachmentDefinition")

				By("Creating vlan one NetworkAttachmentDefinition")
				defineAndCreateVlanNad(vlanNetworkOne, tapTwoInterfaceName, vlanID, nad.IPAMStatic())

				By("Creating vlan two NetworkAttachmentDefinition")
				defineAndCreateVlanNad(vlanNetworkTwo, tapTwoInterfaceName, dummyVlanID, nad.IPAMStatic())

				serverPodOneNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
					srIovNetworkOneName, tsparams.TestNamespaceName, dpdkServerMac)

				By("Creating first server pod")
				srvNetOne := defineTestServerPmdCmd(dpdkClientMac, "${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYONE}", "")
				defineAndCreateDPDKPod(
					"serverpod-one",
					workerNodes[0].Definition.Name,
					serverSC,
					nil,
					serverPodOneNetConfig,
					srvNetOne)

				By("Creating second server pod")
				serverPodTwoNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
					srIovNetworkTwoName, tsparams.TestNamespaceName, dpdkServerMacTwo)

				srvNetTwo := defineTestServerPmdCmd(dpdkClientMacTwo, "${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYTWO}", "")
				defineAndCreateDPDKPod(
					"serverpod-two",
					workerNodes[1].Definition.Name,
					serverSC,
					nil,
					serverPodTwoNetConfig,
					srvNetTwo)

				By("Creating client pod")
				firstVlanInterfaceBasedOnTapTwo := fmt.Sprintf("%s.%d", tapTwoInterfaceName, vlanID)
				secondVlanInterfaceBasedOnTapTwo := fmt.Sprintf("%s.%d", tapTwoInterfaceName, dummyVlanID)
				clientPodNetConfig := definePodNetwork([]map[string]string{
					{"netName": srIovNetworkOneName, "macAddr": dpdkClientMac},
					{"netName": srIovNetworkOneName, "macAddr": dpdkClientMacTwo},
					{"netName": tapNetworkOne, "intName": tapOneInterfaceName},
					{"netName": tapNetworkTwo, "intName": tapTwoInterfaceName},
					{"netName": macVlanNetworkOne, "intName": firstInterfaceBasedOnTapOne, "macAddr": dpdkClientMac},
					{"netName": vlanNetworkOne, "intName": firstVlanInterfaceBasedOnTapTwo, "ipAddr": "1.1.1.1/24"},
					{"netName": vlanNetworkTwo, "intName": secondVlanInterfaceBasedOnTapTwo, "ipAddr": "2.2.2.2/24"}})
				clientPod := defineAndCreateDPDKPod(
					"clientpod", workerNodes[1].Definition.Name, clientSC, &clientPodSC, clientPodNetConfig, sleepCMD)

				By("Collecting PCI Address")
				Eventually(
					isPciAddressAvailable, tsparams.WaitTimeout, tsparams.RetryInterval).WithArguments(clientPod).Should(BeTrue())
				pciAddressList, err := getPCIAddressListFromSrIovNetworkName(
					clientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"])
				Expect(err).ToNot(HaveOccurred(), "Fail to collect PCI addresses")

				By("Running client dpdk-testpmd")
				err = cmd.RxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapOneInterfaceName, pciAddressList[0]))
				Expect(err).ToNot(HaveOccurred(),
					"The Receive traffic test on the the client pod failed")

				By("Checking the rx output of tap ext0 device")
				checkRxOutputRateForInterfaces(
					clientPod, map[string]int{
						tapOneInterfaceName:         minimumExpectedDPDKRate,
						firstInterfaceBasedOnTapOne: minimumExpectedDPDKRate})
				err = cmd.RxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapTwoInterfaceName, pciAddressList[1]))
				Expect(err).ToNot(HaveOccurred(),
					"The Receive traffic test on the the client pod failed")

				checkRxOutputRateForInterfaces(clientPod, map[string]int{
					tapTwoInterfaceName:              minimumExpectedDPDKRate,
					firstVlanInterfaceBasedOnTapTwo:  minimumExpectedDPDKRate,
					secondVlanInterfaceBasedOnTapTwo: maxMulticastNoiseRate})
			})

		It("multiple VFs, one tap plus IP-VLANs, second tap plus plus VLAN and sysctl, filter untagged and tagged"+
			" traffic, add and remove routes", reportxml.ID("63878"), func() {
			defineAndCreateSrIovNetworks(vlanID)
			defineAndCreateTapNADs(enabledSysctlFlags, disabledSysctlFlags)

			serverPodOneNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
				srIovNetworkOneName, tsparams.TestNamespaceName, dpdkServerMac)

			By("Creating ip-vlan interface")
			defineAndCreateIPVlanNad(ipVlanNetworkOne, tapOneInterfaceName, nad.IPAMStatic())

			By("Creating vlan-one interface")
			defineAndCreateVlanNad(vlanNetworkOne, tapTwoInterfaceName, vlanID, nad.IPAMWhereAbouts("2.2.2.0/24", "2.2.2.254"))

			By("Creating first server pod")
			srvNetOne := defineTestServerPmdCmd(
				dpdkClientMac, "${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYONE}", "1.1.1.50,1.1.1.100")
			defineAndCreateDPDKPod(
				"serverpod-one", workerNodes[0].Definition.Name, serverSC, nil, serverPodOneNetConfig, srvNetOne)
			serverPodTwoNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
				srIovNetworkTwoName, tsparams.TestNamespaceName, dpdkServerMacTwo)

			By("Creating second server pod")
			srvNetTwo := defineTestServerPmdCmd(dpdkClientMacTwo, "${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYTWO}", "")
			defineAndCreateDPDKPod(
				"serverpod-two", workerNodes[0].Definition.Name, serverSC, nil, serverPodTwoNetConfig, srvNetTwo)

			By("Creating client pod")
			vlanInterfaceName := fmt.Sprintf("%s.%d", tapTwoInterfaceName, vlanID)
			clientPodNetConfig := definePodNetwork([]map[string]string{
				{"netName": srIovNetworkOneName, "macAddr": dpdkClientMac},
				{"netName": srIovNetworkOneName, "macAddr": dpdkClientMacTwo},
				{"netName": tapNetworkOne, "intName": tapOneInterfaceName},
				{"netName": tapNetworkTwo, "intName": tapTwoInterfaceName},
				{"netName": ipVlanNetworkOne, "intName": firstInterfaceBasedOnTapOne, "ipAddr": "1.1.1.100/24"},
				{"netName": ipVlanNetworkOne, "intName": secondInterfaceBasedOnTapOne, "ipAddr": "1.1.1.200/24"},
				{"netName": vlanNetworkOne, "intName": vlanInterfaceName}})
			clientPod := defineAndCreateDPDKPod(
				"clientpod", workerNodes[1].Definition.Name, clientSC, &clientPodSC, clientPodNetConfig, sleepCMD)

			By("Collecting PCI Address")
			Eventually(
				isPciAddressAvailable, tsparams.WaitTimeout, tsparams.RetryInterval).WithArguments(clientPod).Should(BeTrue())
			pciAddressList, err := getPCIAddressListFromSrIovNetworkName(
				clientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"])
			Expect(err).ToNot(HaveOccurred(), "Fail to collect PCI addresses")

			err = cmd.RxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapOneInterfaceName, pciAddressList[0]))
			Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")

			checkRxOutputRateForInterfaces(
				clientPod, map[string]int{
					tapOneInterfaceName:          minimumExpectedDPDKRate,
					firstInterfaceBasedOnTapOne:  minimumExpectedDPDKRate,
					secondInterfaceBasedOnTapOne: maxMulticastNoiseRate,
				})
			err = cmd.RxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapTwoInterfaceName, pciAddressList[1]))
			Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")

			checkRxOutputRateForInterfaces(
				clientPod, map[string]int{
					tapTwoInterfaceName:          minimumExpectedDPDKRate,
					vlanInterfaceName:            minimumExpectedDPDKRate,
					secondInterfaceBasedOnTapOne: maxMulticastNoiseRate,
				})

			testRouteInjection(clientPod, firstInterfaceBasedOnTapOne)
		})

		It("multiple VFs, one tap with VLAN plus sysctl, second tap with two mac-vlans plus sysctl, filter untagged "+
			"and tagged traffic, add and remove routes, deployment restart", reportxml.ID("63846"), func() {

			defineAndCreateSrIovNetworks(vlanID)
			defineAndCreateTapNADs(enabledSysctlFlags, disabledSysctlFlags)

			By("Creating vlan-one NetworkAttachmentDefinition")
			defineAndCreateVlanNad(vlanNetworkOne, tapOneInterfaceName, vlanID, defaultWhereaboutIPAM)

			By("Creating mac-vlan one NetworkAttachmentDefinition")
			_, err := define.MacVlanNad(
				APIClient, macVlanNetworkOne, tsparams.TestNamespaceName, tapTwoInterfaceName, defaultWhereaboutIPAM)
			Expect(err).ToNot(HaveOccurred(), "Fail to create first mac-vlan NetworkAttachmentDefinition")

			By("Creating mac-vlan two NetworkAttachmentDefinition")
			_, err = define.MacVlanNad(
				APIClient, macVlanNetworkOne, tsparams.TestNamespaceName, tapTwoInterfaceName, defaultWhereaboutIPAM)
			Expect(err).ToNot(HaveOccurred(), "Fail to create second mac-vlan NetworkAttachmentDefinition")

			By("Creating first server pod")
			serverPodOneNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
				srIovNetworkTwoName, tsparams.TestNamespaceName, dpdkServerMac)
			srvCmdOne := defineTestServerPmdCmd(dpdkClientMac, "${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYTWO}", "")
			defineAndCreateDPDKPod(
				"serverpod-one", workerNodes[0].Definition.Name, serverSC, nil, serverPodOneNetConfig, srvCmdOne)

			By("Creating second server pod")
			serverPodTwoNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
				srIovNetworkOneName, tsparams.TestNamespaceName, dpdkServerMacTwo)
			srvCmdTwo := defineTestServerPmdCmd(dpdkClientMacTwo, "${PCIDEVICE_OPENSHIFT_IO_DPDKPOLICYONE}", "")
			defineAndCreateDPDKPod(
				"serverpod-two", workerNodes[0].Definition.Name, serverSC, nil, serverPodTwoNetConfig, srvCmdTwo)

			By("Creating SCC")
			_, err = scc.NewBuilder(APIClient, "scc-test-admin", "MustRunAsNonRoot", "RunAsAny").
				WithPrivilegedContainer(false).WithPrivilegedEscalation(true).
				WithDropCapabilities([]corev1.Capability{"ALL"}).
				WithAllowCapabilities([]corev1.Capability{"IPC_LOCK", "NET_ADMIN", "NET_RAW"}).
				WithFSGroup("RunAsAny").
				WithSeccompProfiles([]string{"*"}).
				WithSupplementalGroups("RunAsAny").
				WithUsers([]string{"system:serviceaccount:dpdk-tests:default"}).Create()
			Expect(err).ToNot(HaveOccurred(), "Fail to create SCC")

			By("Creating client deployment")
			secondInterfaceBasedOnTapTwo := "ext1.2"
			firstVlanInterfaceBasedOnTapOne := fmt.Sprintf("%s.%d", tapOneInterfaceName, vlanID)
			clientPodNetConfig := definePodNetwork([]map[string]string{
				{"netName": srIovNetworkOneName, "macAddr": dpdkClientMac},
				{"netName": srIovNetworkOneName, "macAddr": dpdkClientMacTwo},
				{"netName": tapNetworkOne, "intName": tapOneInterfaceName},
				{"netName": tapNetworkTwo, "intName": tapTwoInterfaceName},
				{"netName": vlanNetworkOne, "intName": firstVlanInterfaceBasedOnTapOne},
				{"netName": macVlanNetworkOne, "intName": firstInterfaceBasedOnTapTwo, "macAddr": dpdkClientMacTwo},
				{"netName": macVlanNetworkOne, "intName": secondInterfaceBasedOnTapTwo}})

			deploymentContainer := pod.NewContainerBuilder("dpdk", NetConfig.DpdkTestContainer, sleepCMD)
			deploymentContainerCfg, err := deploymentContainer.WithSecurityContext(&clientSC).
				WithResourceLimit("2Gi", "1Gi", 4).
				WithResourceRequest("2Gi", "1Gi", 4).
				WithEnvVar("RUN_TYPE", "testcmd").
				GetContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "Fail to get deployment container config")

			_, err = deployment.NewBuilder(
				APIClient, "deployment-one", tsparams.TestNamespaceName, map[string]string{"test": "dpdk"},
				*deploymentContainerCfg).
				WithNodeSelector(map[string]string{"kubernetes.io/hostname": workerNodes[1].Definition.Name}).
				WithSecurityContext(&clientPodSC).
				WithLabel("test", "dpdk").
				WithSecondaryNetwork(clientPodNetConfig).
				WithHugePages().
				WithTerminationGracePeriodSeconds(90).
				CreateAndWaitUntilReady(tsparams.WaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Fail to create deployment")
			deploymentPod := fetchNewDeploymentPod("deployment-one")

			By("Collecting PCI Address")
			Eventually(
				isPciAddressAvailable, tsparams.WaitTimeout, tsparams.RetryInterval).WithArguments(deploymentPod).Should(BeTrue())
			pciAddressList, err := getPCIAddressListFromSrIovNetworkName(
				deploymentPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"])
			Expect(err).ToNot(HaveOccurred(), "Fail to collect PCI addresses")

			err = cmd.RxTrafficOnClientPod(deploymentPod, defineTestPmdCmd(tapOneInterfaceName, pciAddressList[0]))
			Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")

			checkRxOutputRateForInterfaces(
				deploymentPod, map[string]int{
					tapOneInterfaceName:             minimumExpectedDPDKRate,
					firstVlanInterfaceBasedOnTapOne: minimumExpectedDPDKRate,
				})

			err = cmd.RxTrafficOnClientPod(deploymentPod, defineTestPmdCmd(tapTwoInterfaceName, pciAddressList[1]))
			Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")

			checkRxOutputRateForInterfaces(
				deploymentPod, map[string]int{
					tapTwoInterfaceName:          minimumExpectedDPDKRate,
					firstInterfaceBasedOnTapTwo:  minimumExpectedDPDKRate,
					secondInterfaceBasedOnTapTwo: maxMulticastNoiseRate,
				})

			testRouteInjection(deploymentPod, firstVlanInterfaceBasedOnTapOne)

			By("Removing previous deployment pod")
			_, err = deploymentPod.DeleteAndWait(tsparams.WaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Fail to remove deployment pod")

			By("Collecting re-started deployment pods")
			deploymentPod = fetchNewDeploymentPod("deployment-one")

			By("Collecting PCI Address")
			Eventually(
				isPciAddressAvailable, tsparams.WaitTimeout, tsparams.RetryInterval).WithArguments(deploymentPod).Should(BeTrue())
			pciAddressList, err = getPCIAddressListFromSrIovNetworkName(
				deploymentPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"])
			Expect(err).ToNot(HaveOccurred(), "Fail to collect PCI addresses")

			err = cmd.RxTrafficOnClientPod(deploymentPod, defineTestPmdCmd(tapOneInterfaceName, pciAddressList[0]))
			Expect(err).ToNot(HaveOccurred(),
				"The Receive traffic test on the the client pod failed %s")

			checkRxOutputRateForInterfaces(
				deploymentPod, map[string]int{
					tapOneInterfaceName:             minimumExpectedDPDKRate,
					firstVlanInterfaceBasedOnTapOne: minimumExpectedDPDKRate,
				})
			err = cmd.RxTrafficOnClientPod(deploymentPod, defineTestPmdCmd(tapTwoInterfaceName, pciAddressList[1]))
			Expect(err).ToNot(HaveOccurred(), "The Receive traffic test on the the client pod failed")

			checkRxOutputRateForInterfaces(
				deploymentPod, map[string]int{
					tapTwoInterfaceName:          minimumExpectedDPDKRate,
					firstInterfaceBasedOnTapTwo:  minimumExpectedDPDKRate,
					secondInterfaceBasedOnTapTwo: maxMulticastNoiseRate,
				})

			testRouteInjection(deploymentPod, firstVlanInterfaceBasedOnTapOne)
		})
	})

	AfterEach(func() {
		By("Removing all srIovNetworks")
		err := sriov.CleanAllNetworksByTargetNamespace(
			APIClient, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Fail to clean srIovNetworks")

		By("Removing all pods from test namespace")
		runningNamespace, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace")
		Expect(runningNamespace.CleanObjects(
			tsparams.WaitTimeout, deployment.GetGVR(), nad.GetGVR(), pod.GetGVR())).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		By("Removing all pods from test namespace")
		runningNamespace, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace")
		Expect(runningNamespace.CleanObjects(tsparams.WaitTimeout, pod.GetGVR())).ToNot(HaveOccurred(),
			"Fail to clean namespace")

		By("Re-setting selinux flag container_use_devices to 0 on all compute nodes")
		err = cluster.ExecCmd(APIClient, NetConfig.WorkerLabel, setSEBool+"0")
		Expect(err).ToNot(HaveOccurred(), "Fail to disable selinux flag")

		By("Removing all SR-IOV Policy")
		err = sriov.CleanAllNetworkNodePolicies(APIClient, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Fail to clean srIovPolicy")

		By("Removing all srIovNetworks")
		err = sriov.CleanAllNetworksByTargetNamespace(
			APIClient, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Fail to clean sriov networks")

		By("Removing SecurityContextConstraints")
		testScc, err := scc.Pull(APIClient, "scc-test-admin")
		if err == nil {
			err = testScc.Delete()
			Expect(err).ToNot(HaveOccurred(), "Fail to remove scc")
		}

		By("Waiting until cluster MCP and SR-IOV are stable")
		// This used to be to check for sriov not to be stable first,
		// then stable. The issue is that if no configuration is applied, then
		// the status will never go to not stable and the test will fail.
		time.Sleep(5 * time.Second)
		err = netenv.WaitForSriovAndMCPStable(
			APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Fail to wait until cluster is stable")
	})
})

func defineAndCreateTapNADs(firstTapSysctlConfig, secondTapSysctlConfig map[string]string) {
	By("Creating first tap interface")

	_, err := define.TapNad(APIClient, tapNetworkOne, tsparams.TestNamespaceName, 0, 0, firstTapSysctlConfig)
	Expect(err).ToNot(HaveOccurred(), "Fail to create first tap NetworkAttachmentDefinition")

	_, err = define.TapNad(
		APIClient, tapNetworkTwo, tsparams.TestNamespaceName, customUserID, customGroupID, secondTapSysctlConfig)
	Expect(err).ToNot(HaveOccurred(), "Fail to create second tap NetworkAttachmentDefinition")
}

func defineAndCreateSrIovNetworks(secondSrIovNetworkVlanID uint16) {
	By("Creating srIovNetwork sriov-net-one")
	defineAndCreateSrIovNetwork(srIovNetworkOneName, srIovPolicyOneResName, 0)

	By("Creating srIovNetwork sriov-net-two")
	defineAndCreateSrIovNetwork(srIovNetworkTwoName, srIovNetworkTwoResName, secondSrIovNetworkVlanID)
}

func defineAndCreateIPVlanNad(name, intName string, ipam *nad.IPAM) *nad.Builder {
	ipVlanNad, err := define.IPVlanNad(APIClient, name, tsparams.TestNamespaceName, intName, ipam)
	Expect(err).ToNot(HaveOccurred(), "Fail to create ipVlan NetworkAttachmentDefinition")

	return ipVlanNad
}

func defineAndCreateVlanNad(name, intName string, vlanID uint16, ipam *nad.IPAM) *nad.Builder {
	vlanNad, err := define.VlanNad(APIClient, name, tsparams.TestNamespaceName, intName, vlanID, ipam)
	Expect(err).ToNot(HaveOccurred(), "Fail to create vlan NetworkAttachmentDefinition")

	return vlanNad
}

func defineAndCreateSrIovNetwork(srIovNetwork, resName string, vlanID uint16) {
	srIovNetworkObject := sriov.NewNetworkBuilder(
		APIClient, srIovNetwork, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName, resName).
		WithMacAddressSupport().WithLogLevel(netparam.LogLevelDebug)

	if vlanID != 0 {
		srIovNetworkObject.WithVLAN(vlanID)
	}

	srIovNetworkObject, err := srIovNetworkObject.Create()
	Expect(err).ToNot(HaveOccurred(), "Fail to create sriov network")

	Eventually(func() bool {
		_, err := nad.Pull(APIClient, srIovNetworkObject.Object.Name, tsparams.TestNamespaceName)

		return err == nil
	}, tsparams.WaitTimeout, tsparams.RetryInterval).Should(BeTrue(), "Fail to pull NetworkAttachmentDefinition")
}

func defineAndCreateDPDKPod(
	podName,
	nodeName string,
	securityContext corev1.SecurityContext,
	podSC *corev1.PodSecurityContext,
	serverPodNetConfig []*types.NetworkSelectionElement,
	podCmd []string) *pod.Builder {
	dpdkContainer := pod.NewContainerBuilder(podName, NetConfig.DpdkTestContainer, podCmd)
	dpdkContainerCfg, err := dpdkContainer.WithSecurityContext(&securityContext).
		WithResourceLimit("2Gi", "1Gi", 4).
		WithResourceRequest("2Gi", "1Gi", 4).WithEnvVar("RUN_TYPE", "testcmd").
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Fail to set dpdk container")

	dpdkPod := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName,
		NetConfig.DpdkTestContainer).WithSecondaryNetwork(serverPodNetConfig).
		DefineOnNode(nodeName).RedefineDefaultContainer(*dpdkContainerCfg).WithHugePages().
		WithTerminationGracePeriodSeconds(90)

	if podSC != nil {
		dpdkPod = dpdkPod.WithSecurityContext(podSC)
	}

	dpdkPod, err = dpdkPod.CreateAndWaitUntilRunning(tsparams.WaitTimeout)
	Expect(err).ToNot(HaveOccurred(), "Fail to create server pod")

	return dpdkPod
}

func defineTestServerPmdCmd(ethPeer, pciAddress, txIPs string) []string {
	baseCmd := fmt.Sprintf("dpdk-testpmd -a %s -- --forward-mode txonly --eth-peer=0,%s", pciAddress, ethPeer)
	if txIPs != "" {
		baseCmd += fmt.Sprintf(" --tx-ip=%s", txIPs)
	}

	baseCmd += " --stats-period 5"

	return []string{"/bin/bash", "-c", baseCmd}
}

func definePodNetwork(podNetMapList []map[string]string) []*types.NetworkSelectionElement {
	var clientPodNetConfig []*types.NetworkSelectionElement

	for _, podNet := range podNetMapList {
		_, isMacAddressSet := podNet["macAddr"]
		_, isIntNameSet := podNet["intName"]
		_, isIPPresent := podNet["ipAddr"]

		if isIntNameSet && isIPPresent {
			clientPodNetConfig = append(clientPodNetConfig, pod.StaticIPAnnotationWithInterfaceAndNamespace(
				podNet["netName"], tsparams.TestNamespaceName, podNet["intName"], []string{podNet["ipAddr"]})...)

			continue
		}

		if isMacAddressSet && isIntNameSet {
			clientPodNetConfig = append(clientPodNetConfig, pod.StaticIPAnnotationWithInterfaceMacAndNamespace(
				podNet["netName"], tsparams.TestNamespaceName, podNet["intName"], podNet["macAddr"])...)

			continue
		}

		if isMacAddressSet {
			clientPodNetConfig = append(clientPodNetConfig, pod.StaticIPAnnotationWithMacAndNamespace(
				podNet["netName"], tsparams.TestNamespaceName, podNet["macAddr"])...)

			continue
		}

		clientPodNetConfig = append(
			clientPodNetConfig, pod.StaticIPAnnotationWithInterfaceAndNamespace(
				podNet["netName"], tsparams.TestNamespaceName, podNet["intName"], nil)...)
	}

	return clientPodNetConfig
}

func defineTestPmdCmd(interfaceName string, pciAddress string) string {
	return fmt.Sprintf("timeout -s SIGKILL 20 dpdk-testpmd "+
		"--vdev=virtio_user0,path=/dev/vhost-net,queues=2,queue_size=1024,iface=%s "+
		"-a %s -- --stats-period 5", interfaceName, pciAddress)
}

func checkRxOutputRateForInterfaces(clientPod *pod.Builder, interfaceTrafficRateMap map[string]int) {
	for interfaceName, TrafficRate := range interfaceTrafficRateMap {
		comparator := ">"
		if TrafficRate == 5000 {
			comparator = "<"
		}

		By(fmt.Sprintf("Checking the rx output of %s device", interfaceName))
		Expect(getLinkRx(clientPod, interfaceName)).To(BeNumerically(comparator, TrafficRate),
			"Fail traffic rate is not in expected range")
	}
}

func getLinkRx(runningPod *pod.Builder, linkName string) int {
	linkRawInfo, err := runningPod.ExecCommand(
		[]string{"/bin/bash", "-c", fmt.Sprintf("ip --json -s link show dev %s", linkName)})
	Expect(err).ToNot(HaveOccurred(), "Failed to collect link info")
	linkInfo, err := link.NewBuilder(linkRawInfo)
	Expect(err).ToNot(HaveOccurred(), "Failed to collect link info")

	return linkInfo.GetRxByte()
}

func isPciAddressAvailable(clientPod *pod.Builder) bool {
	if !clientPod.Exists() {
		return false
	}

	podNetAnnotation := clientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
	if podNetAnnotation == "" {
		return false
	}

	var err error

	pciAddressList, err := getPCIAddressListFromSrIovNetworkName(podNetAnnotation)
	if err != nil {
		return false
	}

	if len(pciAddressList) < 2 {
		return false
	}

	return true
}

func getPCIAddressListFromSrIovNetworkName(podNetworkStatus string) ([]string, error) {
	var podNetworkStatusType []podNetworkAnnotation

	err := json.Unmarshal([]byte(podNetworkStatus), &podNetworkStatusType)
	if err != nil {
		return nil, err
	}

	var pciAddressList []string

	for _, networkAnnotation := range podNetworkStatusType {
		if strings.Contains(networkAnnotation.Name, srIovNetworkOneName) {
			pciAddressList = append(pciAddressList, networkAnnotation.DeviceInfo.Pci.PciAddress)
		}
	}

	return pciAddressList, nil
}

func verifySysctlKernelParametersConfiguredOnPodInterface(
	podUnderTest *pod.Builder, sysctlPluginConfig map[string]string, interfaceName string) {
	for key, value := range sysctlPluginConfig {
		sysctlKernelParam := strings.Replace(key, "IFNAME", interfaceName, 1)

		By(fmt.Sprintf("Validate sysctl flag: %s has the right value in pod's interface: %s",
			sysctlKernelParam, interfaceName))

		cmdBuffer, err := podUnderTest.ExecCommand([]string{"sysctl", "-n", sysctlKernelParam})
		Expect(err).ToNot(HaveOccurred(), "Fail to execute cmd command on the pod")
		Expect(strings.TrimSpace(cmdBuffer.String())).To(BeIdenticalTo(value),
			"sysctl kernel param is not in expected state")
	}
}

func defineRoute(dstNet, nextHop, devName, mode string) []string {
	return []string{"/bin/bash", "-c", fmt.Sprintf("route %s -net %s gw %s dev %s", mode, dstNet, nextHop, devName)}
}

func setRouteOnPod(client *pod.Builder, dstNet, nextHop, devName string) (bytes.Buffer, error) {
	cmd := defineRoute(dstNet, nextHop, devName, "add")

	return client.ExecCommand(cmd)
}

func delRouteOnPod(client *pod.Builder, dstNet, nextHop, devName string) (bytes.Buffer, error) {
	cmd := defineRoute(dstNet, nextHop, devName, "del")

	return client.ExecCommand(cmd)
}

func verifyIfRouteExist(clientPod *pod.Builder, dstNetwork, gateway, nextHostInterface string, expectedState bool) {
	buff, err := clientPod.ExecCommand([]string{"route", "-n"})
	Expect(err).ToNot(HaveOccurred())

	var doesRoutePresent bool

	for _, line := range strings.Split(buff.String(), "\n") {
		if strings.Contains(line, dstNetwork) &&
			strings.Contains(line, gateway) &&
			strings.Contains(line, nextHostInterface) {
			doesRoutePresent = true
		}
	}

	Expect(doesRoutePresent).To(BeIdenticalTo(expectedState), "Fail to find required route")
}

func fetchNewDeploymentPod(deploymentPodPrefix string) *pod.Builder {
	By("Re-Collecting deployment pods")

	var deploymentPod *pod.Builder

	Eventually(func() bool {
		namespacePodList, _ := pod.List(APIClient, tsparams.TestNamespaceName, metav1.ListOptions{})
		for _, namespacePod := range namespacePodList {
			if strings.Contains(namespacePod.Definition.Name, deploymentPodPrefix) {
				deploymentPod = namespacePod

				return true
			}
		}

		return false
	}, time.Minute, 300*time.Second).Should(BeTrue(), "Failed to collect deployment pods")

	err := deploymentPod.WaitUntilRunning(300 * time.Second)
	Expect(err).ToNot(HaveOccurred(), "Fail to wait until deployment pod is running")

	return deploymentPod
}

func testRouteInjection(clientPod *pod.Builder, nextHopInterface string) {
	By("Verifying sysctl plugin configuration")
	verifySysctlKernelParametersConfiguredOnPodInterface(clientPod, enabledSysctlFlags, tapOneInterfaceName)
	verifySysctlKernelParametersConfiguredOnPodInterface(clientPod, disabledSysctlFlags, tapTwoInterfaceName)

	By("Adding route to rootless pod")

	nextHopIPAddr := "1.1.1.10"
	_, err := setRouteOnPod(clientPod, networkForRouteTest, nextHopIPAddr, nextHopInterface)
	Expect(err).ToNot(HaveOccurred())

	By("Verifying if route exist in rootless pod")
	verifyIfRouteExist(clientPod, "10.10.10.0", nextHopIPAddr, nextHopInterface, true)

	By("Removing route from rootless pod")

	_, err = delRouteOnPod(clientPod, networkForRouteTest, nextHopIPAddr, nextHopInterface)
	Expect(err).ToNot(HaveOccurred())

	By("Verifying if route was removed from rootless pod")
	verifyIfRouteExist(clientPod, "10.10.10.0", nextHopIPAddr, nextHopInterface, false)
}

func discoverNICVendor(srIovInterfaceUnderTest, workerNodeName string) (string, error) {
	upSrIovInterfaces, err := sriov.NewNetworkNodeStateBuilder(
		APIClient, workerNodeName, NetConfig.SriovOperatorNamespace).GetUpNICs()
	if err != nil {
		return "", err
	}

	for _, srIovInterface := range upSrIovInterfaces {
		if srIovInterface.Name == srIovInterfaceUnderTest {
			switch srIovInterface.Vendor {
			case mlxVendorID:
				klog.V(90).Infof("Mellanox NIC detected")

				return mlxVendorID, nil
			case intelVendorID:
				klog.V(90).Infof("Intel NIC detected")

				return intelVendorID, nil
			default:
				return "", fmt.Errorf("fail to discover vendor ID for network interface %s", srIovInterfaceUnderTest)
			}
		}
	}

	return "", fmt.Errorf("fail to discover interface: %s on worker node %s", srIovInterfaceUnderTest, workerNodeName)
}
