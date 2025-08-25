package tests

import (
	"context"
	"fmt"
	"strconv"
	"time"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	lacpInterface1           = "ae10"
	lacpInterface2           = "ae20"
	sriovNetworkPort0Name    = "sriovnetwork-port0"
	sriovNetworkPort1Name    = "sriovnetwork-port1"
	sriovNetworkClientName   = "sriovnetwork-client"
	srIovPolicyClientResName = "resourceclient"
	bondedNADName            = "nad-bond-1"
)

var _ = Describe("Day1Day2", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		switchInterfaces         []string
		srIovInterfacesUnderTest []string
	)

	BeforeAll(func() {
		var err error

		By("Discover worker nodes")
		workerNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Fail to discover worker nodes")

		By("Collecting SR-IOV interfaces for LACP testing")
		srIovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

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
		err = configurePhysicalInterfacesForLACP(switchCredentials, switchInterfaces)
		Expect(err).ToNot(HaveOccurred(), "Failed to configure physical interfaces for LACP")

		By("Creating NMState instance")
		err = netnmstate.CreateNewNMStateAndWaitUntilItsRunning(7 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to create NMState instance")

		By("Configure LACP bond interfaces on worker-0 node")
		err = configureLACPBondInterfaces(workerNodeList[0].Definition.Name, srIovInterfacesUnderTest)
		Expect(err).ToNot(HaveOccurred(), "Failed to configure LACP bond interfaces")
	})

	AfterAll(func() {
		By("Removing LACP bond interfaces (bond10, bond20)")
		err := removeLACPBondInterfaces(workerNodeList[0].Definition.Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove LACP bond interfaces")

		By("Removing NMState policies")
		err = nmstate.CleanAllNMStatePolicies(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")

		By("Restoring switch configuration to pre-test state")
		switchCredentials, err := sriovenv.NewSwitchCredentials()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

		lacpInterfaces := []string{lacpInterface1, lacpInterface2}
		err = disableLACPOnSwitch(switchCredentials, lacpInterfaces, switchInterfaces)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore switch configuration")
	})

	Context("linux client", func() {
		BeforeAll(func() {
			var err error

			By(fmt.Sprintf("Define and create sriov network policy on %s", workerNodeList[0].Definition.Name))
			nodeSelectorWorker0 := map[string]string{
				"kubernetes.io/hostname": workerNodeList[0].Definition.Name,
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
				workerNodeList[0].Definition.Name))

			By(fmt.Sprintf("Define and create sriov network policy for port1 on %s", workerNodeList[0].Definition.Name))
			_, err = sriov.NewPolicyBuilder(
				APIClient,
				srIovPolicyNode2Name+"port1",
				NetConfig.SriovOperatorNamespace,
				srIovPolicyNode1ResName,
				6,
				[]string{fmt.Sprintf("%s#0-5", srIovInterfacesUnderTest[1])},
				nodeSelectorWorker0).WithMTU(9000).WithVhostNet(true).Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create an sriov policy for port1 on %s",
				workerNodeList[0].Definition.Name))

			By(fmt.Sprintf("Define and create sriov network policy on %s", workerNodeList[1].Definition.Name))
			nodeSelectorWorker1 := map[string]string{
				"kubernetes.io/hostname": workerNodeList[1].Definition.Name,
			}

			_, err = sriov.NewPolicyBuilder(
				APIClient,
				srIovPolicyNode2Name,
				NetConfig.SriovOperatorNamespace,
				srIovPolicyClientResName,
				6,
				[]string{fmt.Sprintf("%s#0-5", srIovInterfacesUnderTest[0])},
				nodeSelectorWorker1).WithMTU(9000).WithVhostNet(true).Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create an sriov policy on %s",
				workerNodeList[1].Definition.Name))

			By("Waiting for SR-IOV and MCP to be stable after policy creation")
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV and MCP to be stable")

			By("Creating SriovNetworks for LACP testing")
			defineLACPSriovNetwork(sriovNetworkPort0Name, srIovPolicyNode0ResName, "port0 on worker-0", false)
			defineLACPSriovNetwork(sriovNetworkPort1Name, srIovPolicyNode1ResName, "port1 on worker-0", false)
			defineLACPSriovNetwork(sriovNetworkClientName, srIovPolicyClientResName, "client on worker-1", true)

			By("Creating bonded Network Attachment Definition")
			err = createBondedNAD(bondedNADName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD")

			By("Creating test client pod on worker-1")
			err = createLACPTestClient("client-pod", sriovNetworkClientName, workerNodeList[1].Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to create test client pod")
		})

		AfterAll(func() {
			// Context-specific cleanup can go here if needed
		})

		AfterEach(func() {
			By("Cleaning test namespace")
			err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		It("Verify that a Linux pod with an active-backup bonded interface fails over when the associated VF is "+
			"disabled due to a LACP failure on the node's PF interface", reportxml.ID("83319"), func() {

			By("Deploying PFLACPMonitor on worker-0")
			nodeSelectorWorker0 := map[string]string{
				"kubernetes.io/hostname": workerNodeList[0].Definition.Name,
			}
			err := createPFLACPMonitor("pflacpmonitor", srIovInterfacesUnderTest, nodeSelectorWorker0)
			Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")

			By("Deploying bonded client pod on worker-0 using port0 and port1 VFs")
			_, err = createBondedClient("client-bond", workerNodeList[0].Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded client pod")
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
	nodeSelector := map[string]string{
		"kubernetes.io/hostname": workerNodeName,
	}

	// Create bond10 interface (port 0 of SR-IOV card)
	bond10Policy := nmstate.NewPolicyBuilder(APIClient, "bond10", nodeSelector).
		WithBondInterface([]string{sriovInterfacesUnderTest[0]}, "bond10", "802.3ad")

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bond10Policy)
	if err != nil {
		return fmt.Errorf("failed to create bond10 NMState policy: %w", err)
	}

	// Create bond20 interface (port 1 of SR-IOV card) if we have a second interface
	if len(sriovInterfacesUnderTest) > 1 {
		bond20Policy := nmstate.NewPolicyBuilder(APIClient, "bond20", nodeSelector).
			WithBondInterface([]string{sriovInterfacesUnderTest[1]}, "bond20", "802.3ad")

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

// createBondedClient creates a bonded client pod using port0 and port1 VFs through the bonded NAD.
func createBondedClient(podName, nodeName string) (*pod.Builder, error) {
	By(fmt.Sprintf("Creating bonded client pod %s on node %s", podName, nodeName))

	// Create network annotation for bonded interface with the two SR-IOV networks and bonded NAD
	annotation := pod.StaticIPBondAnnotationWithInterface(
		bondedNADName, // bonded NAD name (nad-bond-1)
		"bond0",       // bond interface name
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

// createPFLACPMonitor creates a PFLACPMonitor resource for monitoring LACP status on physical interfaces
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
	nodeSelector := map[string]string{
		"kubernetes.io/hostname": workerNodeName,
	}

	// Create NMState policy to remove bond interfaces
	bondRemovalPolicy := nmstate.NewPolicyBuilder(APIClient, "remove-lacp-bonds", nodeSelector).
		WithAbsentInterface("bond10").
		WithAbsentInterface("bond20")

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
