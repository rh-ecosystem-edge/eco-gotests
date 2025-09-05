package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pfstatus"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netnmstate"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	lacpInterface1         = "ae10"
	lacpInterface2         = "ae20"
	sriovNetworkPort0Name  = "sriovnetwork-port0"
	sriovNetworkPort1Name  = "sriovnetwork-port1"
	sriovNetworkClientName = "sriovnetwork-client"

	// SR-IOV Policy Names (RFC 1123 compliant).
	srIovPolicyPort0Name  = "sriov-policy-port0"
	srIovPolicyPort1Name  = "sriov-policy-port1"
	srIovPolicyClientName = "sriov-policy-client"

	srIovPolicyPort0ResName  = "resourceport0"
	srIovPolicyPort1ResName  = "resourceport1"
	srIovPolicyClientResName = "resourceclient"
	bondedNADName            = "nad-bond-1"
	bondedClientPodName      = "client-bond"
	testClientIP             = "192.168.10.1"
	testInterface            = "bond0"
	podBondInterface         = "bond0"
	nodeBondInterface        = "bond10"
	nodeBond20Interface      = "bond20"
	bondMode802_3ad          = "802.3ad"
	logTypeInitialization    = "initialization"
	logTypeVFDisable         = "vf-disable"
)

var _ = Describe("LACP", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		switchInterfaces         []string
		srIovInterfacesUnderTest []string
		worker0NodeName          string
		worker1NodeName          string
		primaryInterface         string // primaryInterface
		secondaryInterface       string // secondaryInterface
	)

	BeforeAll(func() {
		var err error

		By("Verifying SR-IOV operator is running")
		err = verifySriovOperatorRunning()
		Expect(err).ToNot(HaveOccurred(), "SR-IOV operator is not running")

		By("Verifying PF Status Relay operator is running")
		err = verifyPFStatusRelayOperatorRunning()
		Expect(err).ToNot(HaveOccurred(), "PF Status Relay operator is not running")

		By("Discover worker nodes")
		workerNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Fail to discover worker nodes")

		// Initialize worker node name variables for reuse
		worker0NodeName = workerNodeList[0].Definition.Name
		worker1NodeName = workerNodeList[1].Definition.Name

		By("Collecting SR-IOV interfaces for LACP testing")
		srIovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		// Initialize interface variables for reuse
		primaryInterface = srIovInterfacesUnderTest[0]
		secondaryInterface = srIovInterfacesUnderTest[1]

		Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		By("Configure lab switch interface to support LACP")
		switchCredentials, err := sriovenv.NewSwitchCredentials()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

		By("Collecting switch interfaces")
		switchInterfaces, err = NetConfig.GetPrimarySwitchInterfaces()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch interfaces")

		By("Configure LACP on switch interfaces")
		lacpInterfaces := []string{lacpInterface1, lacpInterface2}
		err = enableLACPOnSwitchInterfaces(switchCredentials, lacpInterfaces)
		Expect(err).ToNot(HaveOccurred(), "Failed to enable LACP on the switch")

		By("Configure physical interfaces to join aggregated ethernet interfaces")
		// Only configure the first two switch interfaces for LACP
		firstTwoSwitchInterfaces := switchInterfaces[:2]
		err = configurePhysicalInterfacesForLACP(switchCredentials, firstTwoSwitchInterfaces)
		Expect(err).ToNot(HaveOccurred(), "Failed to configure physical interfaces for LACP")

		By("Configure LACP block firewall filter on switch")
		configureLACPBlockFirewallFilter(switchCredentials)

		By("Creating NMState instance")
		err = netnmstate.CreateNewNMStateAndWaitUntilItsRunning(7 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to create NMState instance")

		By(fmt.Sprintf("Configure LACP bond interfaces on %s node", worker0NodeName))
		err = configureLACPBondInterfaces(worker0NodeName, srIovInterfacesUnderTest)
		Expect(err).ToNot(HaveOccurred(), "Failed to configure LACP bond interfaces")

		By("Verify initial LACP bonding status is working properly on node before tests")
		nodeErr := checkBondingStatusOnNode(worker0NodeName, nodeBondInterface)
		Expect(nodeErr).ToNot(HaveOccurred(), "LACP should be functioning properly on node bond10 before tests")
	})

	AfterAll(func() {
		By("Removing LACP bond interfaces (bond10, bond20)")
		err := removeLACPBondInterfaces(worker0NodeName)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove LACP bond interfaces")

		By("Removing NMState policies")
		err = nmstate.CleanAllNMStatePolicies(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")

		By("Restoring switch configuration to pre-test state")
		switchCredentials, err := sriovenv.NewSwitchCredentials()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

		lacpInterfaces := []string{lacpInterface1, lacpInterface2}
		// Only delete the first two switch interfaces, not all of them
		firstTwoSwitchInterfaces := switchInterfaces[:2]
		err = disableLACPOnSwitch(switchCredentials, lacpInterfaces, firstTwoSwitchInterfaces)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore switch configuration")
	})

	Context("linux client", func() {
		BeforeAll(func() {
			var err error

			// Create node selectors
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			nodeSelectorWorker1 := createNodeSelector(worker1NodeName)

			// Create SR-IOV policies for port0 and port1 on worker node
			err = createLACPSriovPolicy(srIovPolicyPort0Name, srIovPolicyPort0ResName,
				primaryInterface, nodeSelectorWorker0, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for port0")

			err = createLACPSriovPolicy(srIovPolicyPort1Name, srIovPolicyPort1ResName,
				secondaryInterface, nodeSelectorWorker0, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for port1")

			// Create SR-IOV policy for client on worker node
			err = createLACPSriovPolicy(srIovPolicyClientName, srIovPolicyClientResName,
				primaryInterface, nodeSelectorWorker1, worker1NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for client")

			By("Waiting for SR-IOV and MCP to be stable after policy creation")
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV and MCP to be stable")

			By("Creating SriovNetworks for LACP testing")
			defineLACPSriovNetwork(sriovNetworkPort0Name, srIovPolicyPort0ResName,
				fmt.Sprintf("port0 on %s", worker0NodeName), false)
			defineLACPSriovNetwork(sriovNetworkPort1Name, srIovPolicyPort1ResName,
				fmt.Sprintf("port1 on %s", worker0NodeName), false)
			defineLACPSriovNetwork(sriovNetworkClientName, srIovPolicyClientResName,
				fmt.Sprintf("client on %s", worker1NodeName), true)

			By("Creating bonded Network Attachment Definition")
			err = createBondedNAD(bondedNADName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD")

			By(fmt.Sprintf("Creating test client pod on %s", worker1NodeName))
			err = createLACPTestClient("client-pod", sriovNetworkClientName, worker1NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create test client pod")
		})

		AfterAll(func() {
			By("Cleaning all pods from test namespace")
			err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean all pods from test namespace")

			By("Removing SR-IOV Networks")
			sriovNetworkPort0, err := sriov.PullNetwork(APIClient, sriovNetworkPort0Name, NetConfig.SriovOperatorNamespace)
			if err == nil {
				err = sriovNetworkPort0.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV network port0")
			}

			sriovNetworkPort1, err := sriov.PullNetwork(APIClient, sriovNetworkPort1Name, NetConfig.SriovOperatorNamespace)
			if err == nil {
				err = sriovNetworkPort1.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV network port1")
			}

			sriovNetworkClient, err := sriov.PullNetwork(APIClient, sriovNetworkClientName, NetConfig.SriovOperatorNamespace)
			if err == nil {
				err = sriovNetworkClient.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV network client")
			}

			By("Removing bonded Network Attachment Definition")
			bondedNAD, err := nad.Pull(APIClient, bondedNADName, tsparams.TestNamespaceName)
			if err == nil {
				err = bondedNAD.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete bonded NAD")
			}

			By("Removing SR-IOV Policies")
			sriovPolicyPort0, err := sriov.PullPolicy(APIClient, srIovPolicyPort0Name, NetConfig.SriovOperatorNamespace)
			if err == nil {
				err = sriovPolicyPort0.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV policy port0")
			}

			sriovPolicyPort1, err := sriov.PullPolicy(APIClient, srIovPolicyPort1Name, NetConfig.SriovOperatorNamespace)
			if err == nil {
				err = sriovPolicyPort1.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV policy port1")
			}

			sriovPolicyClient, err := sriov.PullPolicy(APIClient, srIovPolicyClientName, NetConfig.SriovOperatorNamespace)
			if err == nil {
				err = sriovPolicyClient.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV policy client")
			}

			By("Waiting for SR-IOV and MCP to be stable after cleanup")
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV and MCP to be stable after cleanup")
		})

		AfterEach(func() {
			By("Removing LACP block filter from switch interface")
			switchCredentials, err := sriovenv.NewSwitchCredentials()
			Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

			setLACPBlockFilterOnInterface(switchCredentials, false)

			By("Cleaning PFLACPMonitor from pf-status-relay-operator namespace")
			err = namespace.NewBuilder(APIClient, "openshift-pf-status-relay-operator").CleanObjects(
				netparam.DefaultTimeout, pfstatus.GetPfStatusConfigurationGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean PFLACPMonitor")

			By("Deleting client-bond pod")
			bondedClientPod, err := pod.Pull(APIClient, bondedClientPodName, tsparams.TestNamespaceName)
			if err == nil {
				_, err = bondedClientPod.DeleteAndWait(netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete client-bond pod")
			}
		})

		It("Verify that a Linux pod with an active-backup bonded interface fails over when the associated VF is "+
			"disabled due to a LACP failure on the node's PF interface", reportxml.ID("83319"), func() {

			By(fmt.Sprintf("Deploying PFLACPMonitor on %s", worker0NodeName))
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			err := createPFLACPMonitor("pflacpmonitor", srIovInterfacesUnderTest, nodeSelectorWorker0)
			Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")

			By(fmt.Sprintf("Deploying bonded client pod on %s using port0 and port1 VFs", worker0NodeName))
			bondedClientPod, err := createBondedClient(bondedClientPodName, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded client pod")

			By("Verify LACP bonding status in bonded client pod")
			podErr := checkBondingStatusInPod(bondedClientPod, podBondInterface)
			Expect(podErr).ToNot(HaveOccurred(), "LACP should be functioning properly in bonded client pod bond0")

			// Execute the complete LACP failure and recovery test flow
			performLACPFailureAndRecoveryTest(bondedClientPod, worker0NodeName, srIovInterfacesUnderTest, primaryInterface)
		})
	})

	Context("dpdk client", func() {
	})
})

// DefineBondNad returns network attachment definition for a Bond interface.
func DefineBondNad(nadName string,
	bondType string,
	mtu int,
	numberSlaveInterfaces int, ipam string) (*netattdefv1.NetworkAttachmentDefinition, error) {
	slaveInterfaces := bondNADSlaveInterfaces(numberSlaveInterfaces)
	bondNad := &netattdefv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nadName,
			Namespace: tsparams.TestNamespaceName,
		},
		Spec: netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(
				`{"type": "bond", "cniVersion": "0.3.1", "name": "%s",
"mode": "%s", "failOverMac": 1, "linksInContainer": true, "miimon": "100", "mtu": %d,
"links": [%s], "capabilities": {"ips": true}, `,
				nadName, bondType, mtu, slaveInterfaces),
		}}

	switch ipam {
	case "static":
		bondNad.Spec.Config += fmt.Sprintf(`"ipam": {"type": "%s"}}`, ipam)
	case "whereabouts":
		bondNad.Spec.Config += fmt.Sprintf(`"ipam": {"type": "%s", "range": "%s"}}`,
			ipam, "2001:1db8:85a3::0/126")
	default:
		return nil, fmt.Errorf("wrong ipam type %s", ipam)
	}

	return bondNad, nil
}

// bondNADSlaveInterfaces returns string with slave interfaces for Bond interface Network Attachment Definition.
func bondNADSlaveInterfaces(numberInterfaces int) string {
	slaveInterfaces := `{"name": "net1"}`

	for i := 2; i <= numberInterfaces; i++ {
		slaveInterfaces += fmt.Sprintf(`,{"name": "net%d"}`, i)
	}

	return slaveInterfaces
}

// disableLACPOnSwitch removes LACP configuration from switch interfaces.
func disableLACPOnSwitch(credentials *sriovenv.SwitchCredentials, lacpInterfaces, physicalInterfaces []string) error {
	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string

	// Remove LACP configuration from aggregated ethernet interfaces
	for _, lacpInterface := range lacpInterfaces {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", lacpInterface))
	}

	// Remove physical interface configuration
	for _, physicalInterface := range physicalInterfaces {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", physicalInterface))
	}

	err = jnpr.Config(commands)
	if err != nil {
		return err
	}

	return nil
}

// defineLACPSriovNetwork creates a single SriovNetwork resource for LACP testing.
func defineLACPSriovNetwork(networkName, resourceName, description string, withStaticIP bool) {
	By(fmt.Sprintf("Creating SriovNetwork %s (%s)", networkName, description))

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, networkName, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithMacAddressSupport().
		WithLogLevel(netparam.LogLevelDebug)

	if withStaticIP {
		networkBuilder = networkBuilder.WithStaticIpam()
	}

	_, err := networkBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SriovNetwork %s", networkName))

	By(fmt.Sprintf("Waiting for NetworkAttachmentDefinition %s to be created", networkName))
	Eventually(func() error {
		_, err := nad.Pull(APIClient, networkName, tsparams.TestNamespaceName)

		return err

	}, tsparams.WaitTimeout, tsparams.RetryInterval).Should(BeNil(),
		fmt.Sprintf("Failed to pull NetworkAttachmentDefinition %s", networkName))
}

// configureLACPBondInterfaces creates LACP bond interfaces on worker nodes using NMState.
func configureLACPBondInterfaces(workerNodeName string, sriovInterfacesUnderTest []string) error {
	// Create node selector for specific worker node
	nodeSelector := createNodeSelector(workerNodeName)

	bondInterfaceOptions := nmstate.OptionsLinkAggregation{
		Miimon:   100,
		LacpRate: "fast",
		MinLinks: 1,
	}

	// Create bond10 interface (port 0 of SR-IOV card)
	bond10Policy := nmstate.NewPolicyBuilder(APIClient, nodeBondInterface, nodeSelector).
		WithBondInterface([]string{sriovInterfacesUnderTest[0]}, nodeBondInterface, bondMode802_3ad, bondInterfaceOptions)

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bond10Policy)
	if err != nil {
		return fmt.Errorf("failed to create bond10 NMState policy: %w", err)
	}

	// Create bond20 interface (port 1 of SR-IOV card) if we have a second interface
	if len(sriovInterfacesUnderTest) > 1 {
		bond20Policy := nmstate.NewPolicyBuilder(APIClient, nodeBond20Interface, nodeSelector).
			WithBondInterface([]string{sriovInterfacesUnderTest[1]}, nodeBond20Interface, bondMode802_3ad, bondInterfaceOptions)

		err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bond20Policy)
		if err != nil {
			return fmt.Errorf("failed to create bond20 NMState policy: %w", err)
		}
	}

	return nil
}

// createBondedNAD creates a Network Attachment Definition for bonded interfaces.
func createBondedNAD(nadName string) error {
	By(fmt.Sprintf("Creating bonded NAD %s", nadName))

	bondNadDef, err := DefineBondNad(nadName, "active-backup", 1500, 2, "static")
	if err != nil {
		return fmt.Errorf("failed to define bonded NAD %s: %w", nadName, err)
	}

	err = APIClient.Create(context.TODO(), bondNadDef)
	if err != nil {
		return fmt.Errorf("failed to create bonded NAD %s: %w", nadName, err)
	}

	By(fmt.Sprintf("Waiting for bonded NAD %s to be available", nadName))
	Eventually(func() error {
		_, err := nad.Pull(APIClient, nadName, tsparams.TestNamespaceName)

		return err

	}, tsparams.WaitTimeout, tsparams.RetryInterval).Should(BeNil(),
		fmt.Sprintf("Failed to pull bonded NAD %s", nadName))

	return nil
}

// createLACPTestClient creates a test client pod with network annotation and custom command.
func createLACPTestClient(podName, sriovNetworkName, nodeName string) error {
	By(fmt.Sprintf("Creating test client pod %s on node %s", podName, nodeName))

	// Create network annotation with static IP
	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(
		sriovNetworkName,
		[]string{"192.168.10.1/24"},
		"20:04:0f:f1:88:99")

	// Define custom command
	testCmd := []string{"testcmd", "-interface", "net1", "-protocol", "tcp", "-port", "4444", "-listen"}

	// Create and start the pod
	_, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		RedefineDefaultCMD(testCmd).
		WithSecondaryNetwork(networkAnnotation).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)

	if err != nil {
		return fmt.Errorf("failed to create and start test client pod %s: %w", podName, err)
	}

	return nil
}

// createNodeSelector creates a node selector map for the given node name using the standard Kubernetes label.
func createNodeSelector(nodeName string) map[string]string {
	return map[string]string{corev1.LabelHostname: nodeName}
}

// performLACPFailureAndRecoveryTest executes the complete LACP failure and recovery test flow.
func performLACPFailureAndRecoveryTest(
	bondedClientPod *pod.Builder, workerNodeName string, srIovInterfacesUnderTest []string, primaryIntf string) {
	By("Verify initial PFLACPMonitor logs")
	verifyPFLACPMonitorLogs(workerNodeName, srIovInterfacesUnderTest, logTypeInitialization, 0, "")

	By("Test tcp traffic from the bond interface to the client pod")
	validateBondedTCPTraffic(bondedClientPod)

	By("Activate LACP block filter to simulate LACP failure")

	switchCredentials, err := sriovenv.NewSwitchCredentials()
	Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

	setLACPBlockFilterOnInterface(switchCredentials, true)

	By("Test bonded interface connectivity after LACP failure")
	validateBondedTCPTraffic(bondedClientPod)

	By("Verify VF disable logs after LACP failure")
	verifyPFLACPMonitorLogs(workerNodeName, srIovInterfacesUnderTest, logTypeVFDisable,
		5, primaryIntf)

	By("Check bonding status after LACP failure - expect failures")

	podErr, nodeErr := checkBondingStatus(bondedClientPod, workerNodeName)
	Expect(nodeErr).To(HaveOccurred(), "LACP should be failing on node bond10 after LACP block filter is applied")

	By("Check pod bonding status after LACP failure - should still work via net2")
	Expect(podErr).ToNot(HaveOccurred(), "Pod bond0 should still be functional via net2 after LACP failure on net1")

	By("Test bonded interface connectivity after LACP failure - should still work via backup path")
	validateBondedTCPTraffic(bondedClientPod)

	By("Remove LACP block filter from ae10 to restore LACP functionality")

	switchCredentials, err = sriovenv.NewSwitchCredentials()
	Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")
	setLACPBlockFilterOnInterface(switchCredentials, false)

	By("Verify LACP is back up on node bond10 using /proc/net/bonding")
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName, nodeBondInterface)
	}, 2*time.Minute, 10*time.Second).Should(BeNil(), "LACP should recover on node bond10 after removing block filter")

	By("Check PFLACPMonitor logs for LACP recovery - VFs should be set to auto")
	verifyPFLACPMonitorLogs(workerNodeName, srIovInterfacesUnderTest, logTypeInitialization,
		5, primaryIntf)

	By("Check /proc/net/bonding on pod - all interfaces should be up")
	Eventually(func() error {
		return checkBondingStatusInPod(bondedClientPod, podBondInterface)
	}, 2*time.Minute, 10*time.Second).Should(BeNil(),
		"Pod bond0 should have all interfaces functioning after LACP recovery")

	By("Test final connectivity - should work with full bonding restored")
	validateBondedTCPTraffic(bondedClientPod)
}

// createLACPSriovPolicy creates an SR-IOV policy for LACP testing with common settings.
func createLACPSriovPolicy(
	policyName, resourceName string, interfaceSpec string, nodeSelector map[string]string, nodeName string) error {
	By(fmt.Sprintf("Define and create sriov network policy %s on %s", policyName, nodeName))

	_, err := sriov.NewPolicyBuilder(
		APIClient,
		policyName,
		NetConfig.SriovOperatorNamespace,
		resourceName,
		5,
		[]string{fmt.Sprintf("%s#0-4", interfaceSpec)},
		nodeSelector).WithMTU(9000).WithVhostNet(true).Create()

	if err != nil {
		return fmt.Errorf("failed to create sriov policy %s on %s: %w", policyName, nodeName, err)
	}

	return nil
}

// createBondedClient creates a bonded client pod using port0 and port1 VFs through the bonded NAD.
func createBondedClient(podName, nodeName string) (*pod.Builder, error) {
	By(fmt.Sprintf("Creating bonded client pod %s on node %s", podName, nodeName))

	// Create network annotation for bonded interface with the two SR-IOV networks and bonded NAD
	annotation := pod.StaticIPBondAnnotationWithInterface(
		bondedNADName,    // bonded NAD name (nad-bond-1)
		podBondInterface, // bond interface name
		[]string{sriovNetworkPort0Name, sriovNetworkPort1Name}, // SR-IOV networks (port0, port1)
		[]string{"192.168.10.254/24"})                          // IP address for bonded interface

	// Create and start the bonded client pod
	bondedClient, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotation).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)

	if err != nil {
		return nil, fmt.Errorf("failed to create and start bonded client pod %s: %w", podName, err)
	}

	return bondedClient, nil
}

// createPFLACPMonitor creates a PFLACPMonitor resource for monitoring LACP status on physical interfaces.
func createPFLACPMonitor(monitorName string, interfaces []string, nodeSelector map[string]string) error {
	By(fmt.Sprintf("Creating PFLACPMonitor %s", monitorName))

	// Create PFLACPMonitor using eco-goinfra
	pflacpMonitor := pfstatus.NewPfStatusConfigurationBuilder(
		APIClient, monitorName, "openshift-pf-status-relay-operator").
		WithNodeSelector(nodeSelector).
		WithPollingInterval(1000)

	// Add each interface to the monitor
	for _, interfaceName := range interfaces {
		pflacpMonitor = pflacpMonitor.WithInterface(interfaceName)
	}

	// Create the PFLACPMonitor resource
	_, err := pflacpMonitor.Create()
	if err != nil {
		return fmt.Errorf("failed to create PFLACPMonitor %s: %w", monitorName, err)
	}

	By(fmt.Sprintf("Successfully created PFLACPMonitor %s", monitorName))

	return nil
}

// removeLACPBondInterfaces removes LACP bond interfaces (bond10, bond20) using NMState.
func removeLACPBondInterfaces(workerNodeName string) error {
	By("Setting bond interfaces to absent state via NMState")

	// Create node selector for specific worker node
	nodeSelector := createNodeSelector(workerNodeName)

	// Create NMState policy to remove bond interfaces
	bondRemovalPolicy := nmstate.NewPolicyBuilder(APIClient, "remove-lacp-bonds", nodeSelector).
		WithAbsentInterface(nodeBondInterface).
		WithAbsentInterface(nodeBond20Interface)

	// Update the policy and wait for it to be applied
	err := netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bondRemovalPolicy)
	if err != nil {
		return fmt.Errorf("failed to remove LACP bond interfaces: %w", err)
	}

	return nil
}

// enableLACPOnSwitchInterfaces configures LACP on the specified switch interfaces.
func enableLACPOnSwitchInterfaces(credentials *sriovenv.SwitchCredentials, lacpInterfaces []string) error {
	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	// Get VLAN from NetConfig (dynamically discovered per cluster)
	vlan, err := strconv.Atoi(NetConfig.VLAN)
	if err != nil {
		return fmt.Errorf("failed to convert VLAN value: %w", err)
	}

	vlanName := fmt.Sprintf("vlan%d", vlan)

	var commands []string

	// Configure LACP for each interface
	for _, lacpInterface := range lacpInterfaces {
		commands = append(commands,
			fmt.Sprintf("set interfaces %s aggregated-ether-options lacp active", lacpInterface),
			fmt.Sprintf("set interfaces %s aggregated-ether-options lacp periodic fast", lacpInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk", lacpInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk vlan "+
				"members %s", lacpInterface, vlanName),
			fmt.Sprintf("set interfaces %s native-vlan-id %d", lacpInterface, vlan),
			fmt.Sprintf("set interfaces %s mtu 9216", lacpInterface),
		)
	}

	err = jnpr.Config(commands)
	if err != nil {
		return err
	}

	return nil
}

// configurePhysicalInterfacesForLACP configures physical interfaces to join aggregated ethernet interfaces.
func configurePhysicalInterfacesForLACP(credentials *sriovenv.SwitchCredentials, physicalInterfaces []string) error {
	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string

	// First, delete existing configuration on physical interfaces
	for _, physicalInterface := range physicalInterfaces {
		commands = append(commands, fmt.Sprintf("delete interface %s", physicalInterface))
	}

	// Then, add physical interfaces to aggregated ethernet interfaces
	// Assuming first interface goes to ae10, second to ae20
	if len(physicalInterfaces) >= 2 {
		commands = append(commands,
			fmt.Sprintf("set interfaces %s ether-options 802.3ad %s", physicalInterfaces[0], lacpInterface1),
			fmt.Sprintf("set interfaces %s ether-options 802.3ad %s", physicalInterfaces[1], lacpInterface2),
		)
	}

	err = jnpr.Config(commands)
	if err != nil {
		return err
	}

	return nil
}

// configureLACPBlockFirewallFilter configures a firewall filter on the switch to block LACP traffic.
func configureLACPBlockFirewallFilter(credentials *sriovenv.SwitchCredentials) {
	By("Configuring LACP block firewall filter on switch")

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	Expect(err).ToNot(HaveOccurred(), "Failed to create switch session")
	defer jnpr.Close()

	commands := []string{
		// Create firewall filter to block LACP traffic (ether-type 0x8809)
		"set firewall family ethernet-switching filter BLOCK-LACP term BLOCK from ether-type 0x8809",
		"set firewall family ethernet-switching filter BLOCK-LACP term BLOCK then discard",
		"set firewall family ethernet-switching filter BLOCK-LACP term ALLOW-OTHER then accept",
	}

	err = jnpr.Config(commands)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure LACP block firewall filter")

	By("Successfully configured LACP block firewall filter")
}

// setLACPBlockFilterOnInterface applies or removes the LACP block firewall filter on ae10 interface.
func setLACPBlockFilterOnInterface(credentials *sriovenv.SwitchCredentials, enable bool) {
	var command string

	var actionDescription string

	if enable {
		command = fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching filter input BLOCK-LACP", lacpInterface1)
		actionDescription = "Applying"
	} else {
		command = fmt.Sprintf("delete interfaces %s unit 0 family ethernet-switching filter input BLOCK-LACP", lacpInterface1)
		actionDescription = "Removing"
	}

	By(fmt.Sprintf("%s LACP block filter on interface %s", actionDescription, lacpInterface1))

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	Expect(err).ToNot(HaveOccurred(), "Failed to create switch session")
	defer jnpr.Close()

	commands := []string{command}

	err = jnpr.Config(commands)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to %s LACP block filter on interface", strings.ToLower(actionDescription)))

	By(fmt.Sprintf("Successfully %s LACP block filter on interface %s",
		strings.ToLower(actionDescription), lacpInterface1))
}

// verifySriovOperatorRunning verifies that the SR-IOV operator is running and ready.
func verifySriovOperatorRunning() error {
	By("Checking SR-IOV operator deployment status")

	sriovOperatorDeployment, err := deployment.Pull(APIClient, "sriov-network-operator", NetConfig.SriovOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to find SR-IOV operator deployment: %w", err)
	}

	if !sriovOperatorDeployment.IsReady(netparam.DefaultTimeout) {
		return fmt.Errorf("SR-IOV operator deployment is not ready")
	}

	By("SR-IOV operator is running and ready")

	return nil
}

// verifyPFStatusRelayOperatorRunning verifies that the PF Status Relay operator is running and ready.
func verifyPFStatusRelayOperatorRunning() error {
	By("Checking PF Status Relay operator deployment status")

	pfStatusOperatorDeployment, err := deployment.Pull(APIClient,
		"pf-status-relay-operator-controller-manager", "openshift-pf-status-relay-operator")
	if err != nil {
		return fmt.Errorf("failed to find PF Status Relay operator deployment: %w", err)
	}

	if !pfStatusOperatorDeployment.IsReady(netparam.DefaultTimeout) {
		return fmt.Errorf("PF Status Relay operator deployment is not ready")
	}

	By("PF Status Relay operator is running and ready")

	return nil
}

// validateBondedTCPTraffic validates TCP traffic over bonded interface with packet loss verification.
func validateBondedTCPTraffic(clientPod *pod.Builder) {
	By(fmt.Sprintf("Validating TCP traffic from %s to %s via interface %s",
		clientPod.Definition.Name, testClientIP, testInterface))

	command := []string{
		"testcmd",
		fmt.Sprintf("-interface=%s", testInterface),
		"-protocol=tcp",
		"-port=4444",
		fmt.Sprintf("-server=%s", testClientIP),
	}

	output, err := clientPod.ExecCommand(command, clientPod.Definition.Spec.Containers[0].Name)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run testcmd on %s, command output: %s",
		clientPod.Definition.Name, output.String()))

	By("Verify bonded interface connectivity has no packet loss")
	Expect(output.String()).Should(ContainSubstring("0 packet loss"),
		fmt.Sprintf("Bonded interface %s should have 0 packet loss", testInterface))
	Expect(output.String()).Should(ContainSubstring("TCP test passed as expected"),
		"TCP test should pass successfully")
}

// getPFLACPMonitorPod retrieves the PF status relay daemon set pod created by PFLACPMonitor.
func getPFLACPMonitorPod(nodeName string) (*pod.Builder, error) {
	By(fmt.Sprintf("Getting PF status relay daemon set pod on node %s", nodeName))

	// The PF status relay daemon set creates pods with names like: pf-status-relay-ds-pflacpmonitor-xxxxx
	podNamePattern := "pf-status-relay-ds-pflacpmonitor"
	namespace := "openshift-pf-status-relay-operator"

	// Find the pod by name pattern
	podList, err := pod.ListByNamePattern(APIClient, podNamePattern, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list PF status relay pods: %w", err)
	}

	if len(podList) == 0 {
		return nil, fmt.Errorf("no PF status relay daemon set pods found with pattern %s in namespace %s",
			podNamePattern, namespace)
	}

	// Find the pod running on the specified node
	var targetPod *pod.Builder

	for _, podObj := range podList {
		if podObj.Definition.Spec.NodeName == nodeName {
			targetPod = podObj

			break
		}
	}

	if targetPod == nil {
		return nil, fmt.Errorf("no PF status relay daemon set pod found on node %s", nodeName)
	}

	By(fmt.Sprintf("Found PF status relay pod %s on node %s", targetPod.Definition.Name, nodeName))

	return targetPod, nil
}

// verifyPFLACPMonitorLogs verifies PFLACPMonitor logs for different scenarios.
func verifyPFLACPMonitorLogs(
	nodeName string, srIovInterfacesUnderTest []string, logType string, expectedVFs int, targetInterface string) {
	By("Verify PFLACPMonitor pod logs")

	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PFLACPMonitor pod")

	podLogs, err := pflacpPod.GetFullLog("")
	Expect(err).ToNot(HaveOccurred(), "Failed to get PFLACPMonitor pod logs")

	By(fmt.Sprintf("PFLACPMonitor logs:\n%s", podLogs))

	switch logType {
	case logTypeInitialization:
		verifyInitializationLogs(podLogs, srIovInterfacesUnderTest)
	case logTypeVFDisable:
		verifyVFDisableLogs(podLogs, targetInterface, expectedVFs)
	default:
		Expect(false).To(BeTrue(),
			fmt.Sprintf("Invalid logType '%s'. Use '%s' or '%s'", logType, logTypeInitialization, logTypeVFDisable))
	}
}

// verifyInitializationLogs verifies PFLACPMonitor initialization and LACP up status.
func verifyInitializationLogs(podLogs string, srIovInterfacesUnderTest []string) {
	By("Verify that configured SR-IOV interfaces are being monitored")

	for _, sriovInterface := range srIovInterfacesUnderTest {
		Expect(podLogs).Should(ContainSubstring(fmt.Sprintf(`"interface":"%s"`, sriovInterface)),
			fmt.Sprintf("PFLACPMonitor should be monitoring interface %s", sriovInterface))
	}

	By("Verify LACP is up on configured interfaces")

	for _, sriovInterface := range srIovInterfacesUnderTest {
		Expect(podLogs).Should(ContainSubstring(fmt.Sprintf(`"lacp is up","interface":"%s"`, sriovInterface)),
			fmt.Sprintf("LACP should be up on interface %s", sriovInterface))
	}

	By("Verify PFLACPMonitor initialization")
	Expect(podLogs).Should(SatisfyAll(
		ContainSubstring("interfaces to monitor"),
		ContainSubstring("Starting application")),
		"PFLACPMonitor should show proper initialization")
}

// verifyVFDisableLogs verifies that VFs are disabled on a specific interface.
func verifyVFDisableLogs(podLogs string, targetInterface string, expectedVFs int) {
	By(fmt.Sprintf("Verify VF link state disable messages on interface %s", targetInterface))

	for vfID := 0; vfID < expectedVFs; vfID++ {
		expectedLogEntry := fmt.Sprintf(
			`"vf link state was set","id":%d,"state":"disable","interface":"%s"`, vfID, targetInterface)
		Expect(podLogs).Should(ContainSubstring(expectedLogEntry),
			fmt.Sprintf("VF %d should be disabled on interface %s", vfID, targetInterface))
	}

	By(fmt.Sprintf("Verified that %d VFs are disabled on interface %s", expectedVFs, targetInterface))
}

// checkBondingStatus checks LACP bonding status on both pod and node.
func checkBondingStatus(bondedPod *pod.Builder, nodeName string) (podErr, nodeErr error) {
	podErr = checkBondingStatusInPod(bondedPod, podBondInterface)
	nodeErr = checkBondingStatusOnNode(nodeName, nodeBondInterface)

	return podErr, nodeErr
}

// checkBondingStatusInPod checks bonding status inside a pod.
func checkBondingStatusInPod(bondedPod *pod.Builder, bondInterface string) error {
	By(fmt.Sprintf("Checking bonding status for %s in pod %s", bondInterface, bondedPod.Definition.Name))

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)

	output, err := bondedPod.ExecCommand([]string{"cat", bondingPath})
	if err != nil {
		return fmt.Errorf("failed to read bonding status in pod: %w", err)
	}

	return analyzePodBondingStatus(output.String(), bondInterface, "pod")
}

// checkBondingStatusOnNode checks bonding status on a specific node.
func checkBondingStatusOnNode(nodeName, bondInterface string) error {
	By(fmt.Sprintf("Checking bonding status for %s on node %s", bondInterface, nodeName))

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)
	command := fmt.Sprintf("cat %s", bondingPath)

	// Create node selector for the specific node
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to read bonding status on node %s: %w", nodeName, err)
	}

	// Get output for the specific node
	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s", nodeName)
	}

	return analyzeLACPPortStates(output, bondInterface, "node")
}

// analyzeLACPPortStates analyzes the bonding status output for LACP port states.
func analyzeLACPPortStates(bondingOutput, bondInterface, location string) error {
	By(fmt.Sprintf("Analyzing LACP port states for %s on %s", bondInterface, location))

	// When LACP is up, the port state should be 63 for both actor and partner
	expectedPortState := "63"

	// Split output into lines for parsing
	lines := strings.Split(bondingOutput, "\n")

	var actorPortState, partnerPortState string

	var inActorSection, inPartnerSection bool

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Identify sections
		if strings.Contains(line, "details actor lacp pdu:") {
			inActorSection = true
			inPartnerSection = false
		} else if strings.Contains(line, "details partner lacp pdu:") {
			inActorSection = false
			inPartnerSection = true
		}

		// Look for port state in the appropriate section
		if strings.Contains(line, "port state:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				portState := strings.TrimSpace(parts[1])
				if inActorSection {
					actorPortState = portState
				} else if inPartnerSection {
					partnerPortState = portState
				}
			}
		}
	}

	// Check if both port states are 63
	if actorPortState != expectedPortState {
		return fmt.Errorf("LACP actor port state is %s (expected %s) on %s %s",
			actorPortState, expectedPortState, location, bondInterface)
	}

	if partnerPortState != expectedPortState {
		return fmt.Errorf("LACP partner port state is %s (expected %s) on %s %s",
			partnerPortState, expectedPortState, location, bondInterface)
	}

	By(fmt.Sprintf("LACP is functioning properly on %s %s - actor port state: %s, partner port state: %s",
		location, bondInterface, actorPortState, partnerPortState))

	return nil
}

// analyzePodBondingStatus analyzes the bonding status output for active-backup mode (used in pods).
func analyzePodBondingStatus(bondingOutput, bondInterface, location string) error {
	By(fmt.Sprintf("Analyzing active-backup bonding status for %s on %s", bondInterface, location))

	lines := strings.Split(bondingOutput, "\n")

	var bondingMode, miiStatus string

	var net1Status, net2Status string

	var currentInterface string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check bonding mode
		if strings.Contains(line, "Bonding Mode:") {
			bondingMode = line
		}

		// Check overall MII Status
		if strings.Contains(line, "MII Status:") && !strings.Contains(line, "Slave Interface:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				miiStatus = strings.TrimSpace(parts[1])
			}
		}

		// Identify slave interfaces
		if strings.Contains(line, "Slave Interface:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				currentInterface = strings.TrimSpace(parts[1])
			}
		}

		// Check slave interface MII status
		if strings.Contains(line, "MII Status:") && currentInterface != "" {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				status := strings.TrimSpace(parts[1])

				switch currentInterface {
				case "net1":
					net1Status = status
				case "net2":
					net2Status = status
				}
			}
		}
	}

	// Validate bonding mode
	if !strings.Contains(bondingMode, "active-backup") {
		return fmt.Errorf("expected active-backup bonding mode, got: %s on %s %s",
			bondingMode, location, bondInterface)
	}

	// Validate overall MII status
	if miiStatus != "up" {
		return fmt.Errorf("bond MII status is %s (expected up) on %s %s",
			miiStatus, location, bondInterface)
	}

	// Validate that at least one slave interface is up
	if net1Status != "up" && net2Status != "up" {
		return fmt.Errorf("both slave interfaces are down (net1: %s, net2: %s) on %s %s",
			net1Status, net2Status, location, bondInterface)
	}

	By(fmt.Sprintf("Active-backup bonding is functioning properly on %s %s - MII Status: %s, net1: %s, net2: %s",
		location, bondInterface, miiStatus, net1Status, net2Status))

	return nil
}
