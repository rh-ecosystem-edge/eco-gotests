package tests

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
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
	bondedClientPodName      = "client-bond"
	testClientIP             = "192.168.10.1"
	bondTestInterface        = "bond0"
	nodeBond10Interface      = "bond10"
	nodeBond20Interface      = "bond20"
	bondModeActiveBackup     = "active-backup"
	bondModeBalanceTlb       = "balance-tlb"
	bondModeBalanceAlb       = "balance-alb"
	bondMode802_3ad          = "802.3ad"
	logTypeInitialization    = "initialization"
	logTypeVFDisable         = "vf-disable"
	logTypeVFEnable          = "vf-enable"
	net1Interface            = "net1"
	net2Interface            = "net2"
	pfLacpMonitorName        = "pflacpmonitor-mgmt"
)

var _ = Describe("LACP Status Relay ", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
	var (
		workerNodeList           []*nodes.Builder
		switchInterfaces         []string
		firstTwoSwitchInterfaces []string
		switchCredentials        *sriovenv.SwitchCredentials
		bondedNADName            string
		srIovInterfacesUnderTest []string
		worker0NodeName          string
		worker1NodeName          string
		secondaryInterface0      string
		secondaryInterface1      string
	)

	BeforeAll(func() {

		By("Verifying SR-IOV operator is running")
		err := netenv.IsSriovDeployed(APIClient, NetConfig)
		Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")

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
		Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		srIovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		// Initialize interface variables for reuse
		secondaryInterface0 = srIovInterfacesUnderTest[0]
		secondaryInterface1 = srIovInterfacesUnderTest[1]

		By("Configure lab switch interface to support LACP")
		switchCredentials, err = sriovenv.NewSwitchCredentials()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

		By("Collecting switch interfaces")
		switchInterfaces, err = NetConfig.GetPrimarySwitchInterfaces()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch interfaces")
		Expect(len(switchInterfaces)).To(BeNumerically(">=", 2),
			"At least 2 switch interfaces are required for LACP tests")

		By("Configure LACP on switch interfaces")
		lacpInterfaces, err := NetConfig.GetSwitchLagNames()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch LAG names")
		err = enableLACPOnSwitchInterfaces(switchCredentials, lacpInterfaces)
		Expect(err).ToNot(HaveOccurred(), "Failed to enable LACP on the switch")

		By("Configure physical interfaces to join aggregated ethernet interfaces")
		// Only use the first two switch interfaces for LACP
		firstTwoSwitchInterfaces = switchInterfaces[:2]
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
		nodeErr := checkBondingStatusOnNode(worker0NodeName)
		Expect(nodeErr).ToNot(HaveOccurred(),
			fmt.Sprintf("LACP should be functioning properly on node %s before tests", nodeBond10Interface))
	})

	AfterAll(func() {
		By(fmt.Sprintf("Removing LACP bond interfaces (%s, %s)", nodeBond10Interface, nodeBond20Interface))
		err := removeLACPBondInterfaces(worker0NodeName)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove LACP bond interfaces")

		By("Removing NMState policies")
		err = nmstate.CleanAllNMStatePolicies(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")

		By("Restoring switch configuration to pre-test state")
		if switchCredentials != nil && firstTwoSwitchInterfaces != nil {
			lacpInterfaces, err := NetConfig.GetSwitchLagNames()
			Expect(err).ToNot(HaveOccurred(), "Failed to get switch LAG names")
			// Reuse switch credentials and interfaces from BeforeAll
			err = disableLACPOnSwitch(switchCredentials, lacpInterfaces, firstTwoSwitchInterfaces)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore switch configuration")
		} else {
			By("Switch credentials or interfaces are nil, skipping switch configuration restore")
		}
	})

	Context("linux pod", func() {
		BeforeAll(func() {

			// Create node selectors
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			nodeSelectorWorker1 := createNodeSelector(worker1NodeName)

			// Create SR-IOV policies for port0 and port1 on worker node
			err := createLACPSriovPolicy(srIovPolicyPort0Name, srIovPolicyPort0ResName,
				secondaryInterface0, nodeSelectorWorker0, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for port0")

			err = createLACPSriovPolicy(srIovPolicyPort1Name, srIovPolicyPort1ResName,
				secondaryInterface1, nodeSelectorWorker0, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for port1")

			// Create SR-IOV policy for client on worker node
			err = createLACPSriovPolicy(srIovPolicyClientName, srIovPolicyClientResName,
				secondaryInterface0, nodeSelectorWorker1, worker1NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for client")

			By("Waiting for SR-IOV and MCP to be stable after policy creation")
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV and MCP to be stable")

			By("Creating SriovNetworks for LACP testing")
			createLACPSriovNetwork(sriovNetworkPort0Name, srIovPolicyPort0ResName,
				fmt.Sprintf("port0 on %s", worker0NodeName), false)
			createLACPSriovNetwork(sriovNetworkPort1Name, srIovPolicyPort1ResName,
				fmt.Sprintf("port1 on %s", worker0NodeName), false)
			createLACPSriovNetwork(sriovNetworkClientName, srIovPolicyClientResName,
				fmt.Sprintf("client on %s", worker1NodeName), true)

			By(fmt.Sprintf("Creating test client pod on %s", worker1NodeName))
			err = createLACPTestClient("client-pod", sriovNetworkClientName, worker1NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create test client pod")
		})

		AfterAll(func() {
			By("Cleaning all pods from test namespace")
			err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean all pods from test namespace")

			By("Removing SR-IOV configuration")
			err = netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Removing bonded Network Attachment Definition")
			bondedNAD, err := nad.Pull(APIClient, bondedNADName, tsparams.TestNamespaceName)
			if err == nil {
				err = bondedNAD.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete bonded NAD")
			}
		})

		AfterEach(func() {
			By("Removing LACP block filter from switch interface")
			if switchCredentials != nil {
				setLACPBlockFilterOnInterface(switchCredentials, false)
			}

			By("Cleaning PFLACPMonitor from pf-status-relay-operator namespace")
			err := namespace.NewBuilder(APIClient, NetConfig.PFStatusRelayOperatorNamespace).CleanObjects(
				netparam.DefaultTimeout, pfstatus.GetPfStatusConfigurationGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean PFLACPMonitor")

			By("Deleting client-bond pod")
			bondedClientPod, err := pod.Pull(APIClient, bondedClientPodName, tsparams.TestNamespaceName)
			if err == nil {
				_, err = bondedClientPod.DeleteAndWait(netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete client-bond pod")
			}
		})

		It("Verify bond active-backup recovery when PF LACP failure disables VF", reportxml.ID("83319"), func() {

			By("Creating bonded Network Attachment Definition")
			bondedNADName := "lacp-bond-nad"
			err := createBondedNAD(bondedNADName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD")

			By(fmt.Sprintf("Deploying PFLACPMonitor on %s", worker0NodeName))
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			err = createPFLACPMonitor("pflacpmonitor", srIovInterfacesUnderTest, nodeSelectorWorker0)
			Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")

			By(fmt.Sprintf("Deploying bonded client pod on %s using port0 and port1 VFs", worker0NodeName))
			bondedClientPod, err := createBondedClient(bondedClientPodName, worker0NodeName, bondedNADName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded client pod")

			By("Verify LACP bonding status in bonded client pod")
			podErr := checkBondingStatusInPod(bondedClientPod, bondTestInterface)
			Expect(podErr).ToNot(HaveOccurred(),
				fmt.Sprintf("LACP should be functioning properly in bonded client pod %s", bondTestInterface))

			// Execute the complete LACP failure and recovery test flow
			performLACPFailureAndRecoveryTest(bondedClientPod, worker0NodeName, secondaryInterface0,
				srIovInterfacesUnderTest, switchCredentials)
		})

		It("Verify bond balance-tlb recovery when PF LACP failure disables VF", reportxml.ID("83321"), func() {

			By(fmt.Sprintf("Deploying PFLACPMonitor on %s", worker0NodeName))
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			err := createPFLACPMonitor("pflacpmonitor-tlb", srIovInterfacesUnderTest, nodeSelectorWorker0)
			Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor for balance-tlb test")

			By("Creating bonded Network Attachment Definition for balance-tlb mode")
			bondedTlbNADName := "lacp-bond-nad-tlb"
			err = createBondedNADWithMode(bondedTlbNADName, bondModeBalanceTlb)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD with balance-tlb mode")

			By(fmt.Sprintf("Deploying bonded client pod on %s using port0 and port1 VFs with balance-tlb mode", worker0NodeName))

			bondedTlbClientPod, err := createBondedClient(bondedClientPodName+"-tlb", worker0NodeName, bondedTlbNADName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded client pod with balance-tlb mode")

			By("Verify LACP bonding status in bonded client pod with balance-tlb mode")
			podErr := checkBondingStatusInPodTlb(bondedTlbClientPod, bondTestInterface)
			Expect(podErr).ToNot(HaveOccurred(),
				fmt.Sprintf("LACP should be functioning properly in bonded client pod %s with balance-tlb mode", bondTestInterface))

			// Execute the complete LACP failure and recovery test flow for balance-tlb
			performLACPFailureAndRecoveryTestTlb(bondedTlbClientPod, worker0NodeName, secondaryInterface0,
				srIovInterfacesUnderTest, switchCredentials)
		})

		It("Verify bond balance-alb recovery when PF LACP failure disables VF", reportxml.ID("83322"), func() {

			By(fmt.Sprintf("Deploying PFLACPMonitor on %s", worker0NodeName))
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			err := createPFLACPMonitor("pflacpmonitor-alb", srIovInterfacesUnderTest, nodeSelectorWorker0)
			Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor for balance-alb test")

			By("Creating bonded Network Attachment Definition for balance-alb mode")
			bondedAlbNADName := "lacp-bond-nad-alb"
			err = createBondedNADWithMode(bondedAlbNADName, bondModeBalanceAlb)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD with balance-alb mode")

			By(fmt.Sprintf("Deploying bonded client pod on %s using port0 and port1 VFs with balance-alb mode", worker0NodeName))

			bondedAlbClientPod, err := createBondedClient(bondedClientPodName+"-alb", worker0NodeName, bondedAlbNADName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded client pod with balance-alb mode")

			By("Verify LACP bonding status in bonded client pod with balance-alb mode")
			podErr := checkBondingStatusInPodAlb(bondedAlbClientPod, bondTestInterface)
			Expect(podErr).ToNot(HaveOccurred(),
				fmt.Sprintf("LACP should be functioning properly in bonded client pod %s with balance-alb mode", bondTestInterface))

			// Execute the complete LACP failure and recovery test flow for balance-alb
			performLACPFailureAndRecoveryTestAlb(bondedAlbClientPod, worker0NodeName, secondaryInterface0,
				srIovInterfacesUnderTest, switchCredentials)
		})

		It("Verify that an interface can be added and removed from the PFLACPMonitor interface monitoring",
			reportxml.ID("83323"), func() {

				By(fmt.Sprintf("Deploying PFLACPMonitor on %s with single PF interface", worker0NodeName))
				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
				initialInterfaces := []string{srIovInterfacesUnderTest[0]}
				err := createPFLACPMonitor(pfLacpMonitorName, initialInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to create initial PFLACPMonitor")

				By("Verifying PFLACPMonitor logs show monitored interface and status")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, "", initialInterfaces, 0)
				}, time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show proper initialization and interface monitoring within timeout")

				By("Redeploying PFLACPMonitor to add second PF interface")
				expandedInterfaces := []string{srIovInterfacesUnderTest[0], srIovInterfacesUnderTest[1]}
				err = updatePFLACPMonitor(pfLacpMonitorName, expandedInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to update PFLACPMonitor with additional interface")

				By("Verifying both interfaces are actively monitored in updated PFLACPMonitor")
				bothInterfaces := []string{srIovInterfacesUnderTest[0], srIovInterfacesUnderTest[1]}
				Eventually(func() error {
					return verifyPFLACPMonitorMultiInterfaceLogsEventually(worker0NodeName, bothInterfaces)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show both interfaces being monitored within timeout")

				By("Removing one interface from PFLACPMonitor configuration")
				reducedInterfaces := []string{srIovInterfacesUnderTest[0]} // Keep only first interface
				err = updatePFLACPMonitor(pfLacpMonitorName, reducedInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove interface from PFLACPMonitor")

				By("Verifying removed interface monitoring stops with corresponding log")
				removedInterface := srIovInterfacesUnderTest[1]
				Eventually(func() error {
					return verifyPFLACPMonitorInterfaceRemovalEventually(worker0NodeName, removedInterface)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should stop monitoring removed interface within timeout")

			})

		It("Verify that deployment of a bonded pod succeeds even when the VFs are initially disabled by "+
			"the pf-status-relay operator", reportxml.ID("83324"), func() {

			By(fmt.Sprintf("Setting up PFLACPMonitor on %s to initially disable VFs", worker0NodeName))
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			initialInterfaces := []string{srIovInterfacesUnderTest[0]}

			err := createPFLACPMonitor(pfLacpMonitorName, initialInterfaces, nodeSelectorWorker0)
			Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")

			By("Simulating LACP failure to trigger VF disable by pf-status-relay")
			setLACPBlockFilterOnInterface(switchCredentials, true)

			By("Waiting for pf-status-relay to detect LACP failure and disable VFs")
			Eventually(func() error {
				return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeVFDisable, srIovInterfacesUnderTest[0],
					initialInterfaces, 0)
			}, 2*time.Minute, 10*time.Second).Should(Succeed(),
				"PFLACPMonitor should detect LACP failure and disable VFs within timeout")

			By("Creating bonded Network Attachment Definition while VFs are disabled")
			bondedNADName := "lacp-bond-nad-disabled-vfs"
			err = createBondedNAD(bondedNADName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD")

			By("Deploying bonded client pod while VFs are in disabled state")
			bondedClientPod, err := createBondedClient("bonded-client-disabled-vfs", worker0NodeName,
				bondedNADName)
			Expect(err).ToNot(HaveOccurred(), "Bonded pod deployment should succeed even with "+
				"initially disabled VFs")

			By("Verifying bonded pod is running and functional despite initially disabled VFs")
			// Check that the bond has adapted to the VF disable situation
			podErr := checkBondingStatusInPod(bondedClientPod, bondTestInterface)
			if podErr != nil {
				// This is expected - bond may show issues when VFs are disabled
				By(fmt.Sprintf("Bond status shows expected issues with VFs disabled: %v", podErr))
			}

			By("Verifying pod bonding interface status despite VF disable")
			// Use the same approach as other test cases - check bonding status
			podErr = checkBondingStatusInPod(bondedClientPod, bondTestInterface)
			if podErr != nil {
				// This is expected - bond may show issues when VFs are disabled
				By(fmt.Sprintf("Bond interface shows expected issues with VFs disabled: %v", podErr))
			}

			By("Re-enabling LACP on switch interface to allow VF recovery")
			setLACPBlockFilterOnInterface(switchCredentials, false)

			By("Verifying VFs are re-enabled after LACP recovery")
			Eventually(func() error {
				return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeVFEnable,
					srIovInterfacesUnderTest[0], initialInterfaces, 0)
			}, 2*time.Minute, 10*time.Second).Should(Succeed(),
				"PFLACPMonitor should detect LACP recovery and re-enable VFs within timeout")

			By("Verifying bonded pod network functionality after VF recovery")
			podErr = checkBondingStatusInPod(bondedClientPod, bondTestInterface)
			Expect(podErr).ToNot(HaveOccurred(), "Bonded pod should have functional "+
				"	bonding after VF recovery")

			By("Validating network connectivity through recovered bonded interface")
			validateBondedTCPTraffic(bondedClientPod)
		})

		It("Verify that an interface can be added to PfLACPMonitoring without LACP configured on the interface",
			reportxml.ID("83325"), func() {

				By(fmt.Sprintf("Deploying PFLACPMonitor on %s with two interfaces - first with LACP, second without LACP", worker0NodeName))
				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)

				By("Removing second bond interface to simulate interface without LACP configuration")
				err := removeSecondaryBondInterface(worker0NodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove secondary bond interface")

				// Deploy PFLACPMonitor with both interfaces - one with LACP, one without
				bothInterfaces := []string{secondaryInterface0, secondaryInterface1}
				err = createPFLACPMonitor(pfLacpMonitorName, bothInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor with both interfaces")

				By("Verify PFLACPMonitor logs show first PF with LACP and second PF without LACP")
				Eventually(func() error {
					pflacpPod, err := getPFLACPMonitorPod(worker0NodeName)
					if err != nil {
						return fmt.Errorf("failed to get PFLACPMonitor pod: %w", err)
					}

					podLogs, err := pflacpPod.GetFullLog("")
					if err != nil {
						return fmt.Errorf("failed to get PFLACPMonitor logs: %w", err)
					}

					// Verify first interface shows LACP up
					if !strings.Contains(podLogs, fmt.Sprintf(`"lacp is up","interface":"%s"`, secondaryInterface0)) {
						return fmt.Errorf("first interface %s should show 'lacp is up'", secondaryInterface0)
					}

					// Verify second interface shows "pf is not ready" with "link has no master interface"
					if !strings.Contains(podLogs, fmt.Sprintf(`"pf is not ready","interface":"%s"`, secondaryInterface1)) {
						return fmt.Errorf("second interface %s should show 'pf is not ready'", secondaryInterface1)
					}

					if !strings.Contains(podLogs, "link has no master interface") {
						return fmt.Errorf("logs should show 'link has no master interface' error for unconfigured interface")
					}

					By(fmt.Sprintf("✅ Verified: %s shows 'lacp is up', %s shows 'pf is not ready' with 'link has no master interface'",
						secondaryInterface0, secondaryInterface1))
					return nil
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show first interface with LACP and second interface without LACP")

				By("Configure LACP on the second bond interface (node and switch)")
				// Configure LACP bond interfaces for the second interface that was not configured initially
				err = configureLACPBondInterfaceSecondary(worker0NodeName, secondaryInterface1)
				Expect(err).ToNot(HaveOccurred(), "Failed to configure LACP bond interface for second interface")

				By("Verify LACP is up on the new bond interface with port state 63")
				Eventually(func() error {
					return checkBondingStatusOnNodeSecondary(worker0NodeName, secondaryInterface1)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"Second bond interface should show LACP up with port state 63")

				By("Verify PFLACPMonitor logs show the second PF now has LACP configured")
				Eventually(func() error {
					pflacpPod, err := getPFLACPMonitorPod(worker0NodeName)
					if err != nil {
						return fmt.Errorf("failed to get PFLACPMonitor pod: %w", err)
					}

					podLogs, err := pflacpPod.GetFullLog("")
					if err != nil {
						return fmt.Errorf("failed to get PFLACPMonitor logs: %w", err)
					}

					// Verify second interface now shows LACP up
					if !strings.Contains(podLogs, fmt.Sprintf(`"lacp is up","interface":"%s"`, secondaryInterface1)) {
						return fmt.Errorf("second interface %s should now show 'lacp is up'", secondaryInterface1)
					}

					By(fmt.Sprintf("✅ Verified: %s now shows 'lacp is up' after LACP configuration", secondaryInterface1))
					return nil
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should detect LACP configuration on second interface")

				By("Validating both interfaces now show proper LACP functionality")
				nodeErr := checkBondingStatusOnNode(worker0NodeName)
				Expect(nodeErr).ToNot(HaveOccurred(), "Node bond interfaces should be functional after full LACP configuration")
			})

		It("Verify that a PfLACPMonitoring pod does not update VFs that are set to state Enabled",
			reportxml.ID("83326"), func() {

				By(fmt.Sprintf("Deploy PFLACPMonitor CRD monitoring the PF configured with LACP on %s", worker0NodeName))
				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
				initialInterfaces := []string{secondaryInterface0}

				err := createPFLACPMonitor(pfLacpMonitorName, initialInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")

				By("Verify that the VFs are in auto state with ip link show")
				Eventually(func() error {
					return verifyVFsStateOnInterface(worker0NodeName, secondaryInterface0, "auto")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs should initially be in auto state")

				By("Manually set three of the VFs to enabled mode")
				err = setVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "enable")
				Expect(err).ToNot(HaveOccurred(), "Failed to set VFs to enabled state")

				By("Verifying VFs 1,2,3 are now in enabled state")
				Eventually(func() error {
					return verifyVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "enable")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs 1,2,3 should be in enabled state after manual configuration")

				By("Simulate LACP failure by blocking LACP traffic on the active interface")
				setLACPBlockFilterOnInterface(switchCredentials, true)

				By("Verify on the node /proc/net/bond that LACP on the interface is down (port state not 63)")
				Eventually(func() error {
					return verifyLACPPortStateDown(worker0NodeName, nodeBond10Interface)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"LACP should be down with port state not equal to 63")

				By("Verify interface is up and only non-enabled VFs are disabled")
				Eventually(func() error {
					// Verify interface is still up
					err := verifyInterfaceIsUp(worker0NodeName, secondaryInterface0)
					if err != nil {
						return err
					}

					// Verify enabled VFs (1,2,3) remain enabled
					err = verifyVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "enable")
					if err != nil {
						return err
					}

					// Verify non-enabled VFs (0,4) are now disabled
					err = verifyVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{0, 4}, "disable")
					if err != nil {
						return err
					}

					return nil
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"Interface should be up, enabled VFs remain enabled, non-enabled VFs disabled")

				By("Manually reset the VFs from enabled to disabled")
				err = setVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "disable")
				Expect(err).ToNot(HaveOccurred(), "Failed to reset VFs from enabled to disabled")

				By("Unblock LACP traffic on the switch ports to restore the link")
				setLACPBlockFilterOnInterface(switchCredentials, false)

				By("Verify all VFs are now in auto mode")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeVFEnable, secondaryInterface0, initialInterfaces, 5)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should detect LACP recovery and re-enable VFs to auto state")

				By("Verify in PFLACPMonitor logs that VFs are marked as auto")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeVFEnable, secondaryInterface0, initialInterfaces, 5)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor logs should show VFs marked as auto after recovery")

				By("Validating node bond interface functionality after full recovery")
				nodeErr := checkBondingStatusOnNode(worker0NodeName)
				Expect(nodeErr).ToNot(HaveOccurred(), "Node bond interface should be functional after recovery")
			})

		It("Verify that VFs remain in Disabled state after LACP is blocked and the PFLACPMonitor CRD is deleted and redeployed",
			reportxml.ID("83327"), func() {

				By(fmt.Sprintf("Deploy PFLACPMonitor CRD with interface configured with LACP on %s", worker0NodeName))
				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
				initialInterfaces := []string{secondaryInterface0}

				err := createPFLACPMonitor(pfLacpMonitorName, initialInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")

				By("Simulate LACP failure by blocking LACP traffic on the active interface")
				setLACPBlockFilterOnInterface(switchCredentials, true)

				By("Verify on the node /proc/net/bond that LACP on the interface is down (port state not 63)")
				Eventually(func() error {
					return verifyLACPPortStateDown(worker0NodeName, nodeBond10Interface)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"LACP should be down with port state not equal to 63")

				By("Verify that the VFs are in disabled state with ip link show")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeVFDisable, secondaryInterface0, initialInterfaces, 5)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should detect LACP failure and disable VFs")

				By("Delete the PFLACPMonitor CRD")
				err = deletePFLACPMonitor(pfLacpMonitorName)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete PFLACPMonitor CRD")

				By("Verify that the VFs interfaces remain in disabled state after CRD deletion")
				Eventually(func() error {
					return verifyVFsStateOnInterface(worker0NodeName, secondaryInterface0, "disable")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs should remain in disabled state after PFLACPMonitor deletion")

				By("Unblock LACP traffic on the switch ports to restore the link")
				setLACPBlockFilterOnInterface(switchCredentials, false)

				By("Verify that the VFs are still in disabled state with ip link show")
				Eventually(func() error {
					return verifyVFsStateOnInterface(worker0NodeName, secondaryInterface0, "disable")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs should still be in disabled state even after LACP recovery without PFLACPMonitor")

				By("Redeploy the PFLACPMonitor CR")
				err = createPFLACPMonitor(pfLacpMonitorName, initialInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to redeploy PFLACPMonitor")

				By("Verify that the VFs change to auto state after PFLACPMonitor redeployment")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeVFEnable, secondaryInterface0, initialInterfaces, 5)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should detect LACP up and re-enable VFs to auto state")

				By("Validating final node bond interface functionality")
				nodeErr := checkBondingStatusOnNode(worker0NodeName)
				Expect(nodeErr).ToNot(HaveOccurred(), "Node bond interface should be functional after complete test")
			})
	})
})

func defineBondNad(nadName,
	bondType,
	ipam string,
	numberSlaveInterfaces int) (*nad.Builder, error) {
	// Create bond links for the specified number of slave interfaces
	var bondLinks []nad.Link
	for i := 1; i <= numberSlaveInterfaces; i++ {
		bondLinks = append(bondLinks, nad.Link{Name: fmt.Sprintf("net%d", i)})
	}

	// Add IPAM configuration (following allmulti.go pattern)
	ipamConfig := &nad.IPAM{Type: ipam}

	// Create bond plugin with base configuration (following allmulti.go example)
	bondPlugin := nad.NewMasterBondPlugin(nadName, bondType).
		WithFailOverMac(1).
		WithLinksInContainer(true).
		WithMiimon(100).
		WithLinks(bondLinks).
		WithCapabilities(&nad.Capability{IPs: true}).
		WithIPAM(ipamConfig)

	// Get the master plugin configuration
	masterPluginConfig, err := bondPlugin.GetMasterPluginConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get master plugin config: %w", err)
	}

	// For balance-tlb and balance-alb modes, create NAD manually with proven working config
	if bondType == "balance-tlb" || bondType == "balance-alb" {
		err := createBondedNADManually(nadName, bondType)
		if err != nil {
			return nil, err
		}
		// Return a minimal NAD builder for consistency
		return nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName), nil
	}

	// Create and return NAD using eco-goinfra builder for supported modes
	return nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName).
		WithMasterPlugin(masterPluginConfig), nil
}

// disableLACPOnSwitch removes LACP configuration from switch interfaces.
func disableLACPOnSwitch(credentials *sriovenv.SwitchCredentials, lacpInterfaces, physicalInterfaces []string) error {
	// Safety checks for nil parameters
	if credentials == nil {
		glog.V(90).Infof("Switch credentials are nil, skipping LACP disable")

		return nil
	}

	if lacpInterfaces == nil || physicalInterfaces == nil {
		glog.V(90).Infof("Interface slices are nil, skipping LACP disable")

		return nil
	}

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

// createLACPSriovNetwork creates a single SriovNetwork resource for LACP testing.
func createLACPSriovNetwork(networkName, resourceName, description string, withStaticIP bool) {
	By(fmt.Sprintf("Creating SriovNetwork %s (%s)", networkName, description))

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, networkName, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithMacAddressSupport().
		WithLogLevel(netparam.LogLevelDebug)

	if withStaticIP {
		networkBuilder = networkBuilder.WithStaticIpam()
	}

	err := sriovenv.CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.WaitTimeout)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SriovNetwork %s", networkName))
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

	// Create first bond interface (port 0 of SR-IOV card)
	bond10Policy := nmstate.NewPolicyBuilder(APIClient, nodeBond10Interface, nodeSelector).
		WithBondInterface([]string{sriovInterfacesUnderTest[0]}, nodeBond10Interface, bondMode802_3ad, bondInterfaceOptions)

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bond10Policy)
	if err != nil {
		return fmt.Errorf("failed to create %s NMState policy: %w", nodeBond10Interface, err)
	}

	// Create second bond interface (port 1 of SR-IOV card) if we have a second interface
	if len(sriovInterfacesUnderTest) > 1 {
		bond20Policy := nmstate.NewPolicyBuilder(APIClient, nodeBond20Interface, nodeSelector).
			WithBondInterface([]string{sriovInterfacesUnderTest[1]}, nodeBond20Interface, bondMode802_3ad, bondInterfaceOptions)

		err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bond20Policy)
		if err != nil {
			return fmt.Errorf("failed to create %s NMState policy: %w", nodeBond20Interface, err)
		}
	}

	return nil
}

// createBondedNAD creates a Network Attachment Definition for bonded interfaces.
func createBondedNAD(nadName string) error {
	By(fmt.Sprintf("Creating bonded NAD %s", nadName))

	bondNadBuilder, err := defineBondNad(nadName, bondModeActiveBackup, "static", 2)
	if err != nil {
		return fmt.Errorf("failed to define bonded NAD %s: %w", nadName, err)
	}

	_, err = bondNadBuilder.Create()
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
	bondedClientPod *pod.Builder, workerNodeName, primaryIntf string, srIovInterfacesUnderTest []string,
	switchCredentials *sriovenv.SwitchCredentials) {
	By("Verify initial PFLACPMonitor logs")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeInitialization, "", srIovInterfacesUnderTest, 0)

	By("Test tcp traffic from the bond interface to the client pod")
	validateBondedTCPTraffic(bondedClientPod)

	By("Activate LACP block filter to simulate LACP failure")

	setLACPBlockFilterOnInterface(switchCredentials, true)

	By("Waiting for LACP failure to be detected on node bonding")
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 30*time.Second, 5*time.Second).Should(HaveOccurred(),
		fmt.Sprintf("LACP should fail on node %s after block filter is applied", nodeBond10Interface))

	By("Test bonded interface connectivity after LACP failure")
	validateBondedTCPTraffic(bondedClientPod)

	By("Verify VF disable logs after LACP failure")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeVFDisable, primaryIntf, srIovInterfacesUnderTest, 5)

	By("Check bonding status after LACP failure - expect failures")

	podErr, nodeErr := checkBondingStatus(bondedClientPod, workerNodeName)
	Expect(nodeErr).To(HaveOccurred(),
		fmt.Sprintf("LACP should be failing on node %s after LACP block filter is applied", nodeBond10Interface))

	By("Check pod bonding status after LACP failure - should still work via net2")
	Expect(podErr).ToNot(HaveOccurred(),
		fmt.Sprintf("Pod %s should still be functional via net2 after LACP failure on net1", bondTestInterface))

	By("Test bonded interface connectivity after LACP failure - should still work via backup path")
	validateBondedTCPTraffic(bondedClientPod)

	By("Remove LACP block filter to restore LACP functionality")

	setLACPBlockFilterOnInterface(switchCredentials, false)

	By(fmt.Sprintf("Verify LACP is back up on node %s using /proc/net/bonding", nodeBond10Interface))
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 2*time.Minute, 10*time.Second).Should(BeNil(),
		fmt.Sprintf("LACP should recover on node %s after removing block filter", nodeBond10Interface))

	By("Check PFLACPMonitor logs for LACP recovery - VFs should be set to auto")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeInitialization, primaryIntf, srIovInterfacesUnderTest, 5)

	By("Check /proc/net/bonding on pod - all interfaces should be up")
	Eventually(func() error {
		return checkBondingStatusInPod(bondedClientPod, bondTestInterface)
	}, 2*time.Minute, 10*time.Second).Should(BeNil(),
		fmt.Sprintf("Pod %s should have all interfaces functioning after LACP recovery", bondTestInterface))

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

// updatePFLACPMonitor updates an existing PFLACPMonitor with new interface configuration.
func updatePFLACPMonitor(monitorName string, interfaces []string, nodeSelector map[string]string) error {
	By(fmt.Sprintf("Updating PFLACPMonitor %s with interfaces: %v", monitorName, interfaces))

	// Delete existing PFLACPMonitor
	existingMonitor, err := pfstatus.PullPfStatusConfiguration(
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull existing PFLACPMonitor %s: %w", monitorName, err)
	}

	err = existingMonitor.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete existing PFLACPMonitor %s: %w", monitorName, err)
	}

	// Wait for deletion to complete
	time.Sleep(10 * time.Second)

	// Create new PFLACPMonitor with updated interface configuration
	err = createPFLACPMonitor(monitorName, interfaces, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to recreate PFLACPMonitor %s with new interfaces: %w", monitorName, err)
	}

	By(fmt.Sprintf("Successfully updated PFLACPMonitor %s with %d interfaces", monitorName, len(interfaces)))

	return nil
}

// verifyPFLACPMonitorMultiInterfaceLogs verifies that multiple interfaces are being monitored.
func verifyPFLACPMonitorMultiInterfaceLogs(nodeName string, interfaces []string) {
	By(fmt.Sprintf("Verifying PFLACPMonitor logs show monitoring for %d interfaces: %v", len(interfaces), interfaces))

	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PFLACPMonitor pod")

	podLogs, err := pflacpPod.GetFullLog("")
	Expect(err).ToNot(HaveOccurred(), "Failed to get PFLACPMonitor pod logs")

	// Verify each interface appears in the logs
	for _, interfaceName := range interfaces {
		Expect(podLogs).Should(ContainSubstring(fmt.Sprintf("interface\":\"%s\"", interfaceName)),
			fmt.Sprintf("PFLACPMonitor logs should show monitoring for interface %s", interfaceName))

		By(fmt.Sprintf("Confirmed interface %s is being monitored", interfaceName))
	}

	By(fmt.Sprintf("Successfully verified %d interfaces are being monitored", len(interfaces)))
}

// verifyPFLACPMonitorInterfaceRemoval verifies that monitoring for a removed interface stops.
func verifyPFLACPMonitorInterfaceRemoval(nodeName, removedInterface string) {
	By(fmt.Sprintf("Verifying interface %s is no longer monitored after removal", removedInterface))

	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PFLACPMonitor pod")

	// Get recent logs (after the removal)
	podLogs, err := pflacpPod.GetFullLog("")
	Expect(err).ToNot(HaveOccurred(), "Failed to get PFLACPMonitor pod logs")

	// Look for indication that interface monitoring has stopped
	// This could be absence of new logs for the interface, or explicit removal messages
	logLines := strings.Split(podLogs, "\n")
	recentLines := []string{}

	// Get last 50 lines to check recent activity
	startIndex := len(logLines) - 50
	if startIndex < 0 {
		startIndex = 0
	}
	recentLines = logLines[startIndex:]

	// Verify the removed interface is no longer appearing in recent logs
	removedInterfaceFound := false
	for _, line := range recentLines {
		if strings.Contains(line, fmt.Sprintf("interface\":\"%s\"", removedInterface)) {
			removedInterfaceFound = true

			break
		}
	}

	Expect(removedInterfaceFound).To(BeFalse(),
		fmt.Sprintf("Interface %s should no longer appear in recent PFLACPMonitor logs after removal", removedInterface))

	By(fmt.Sprintf("Confirmed interface %s monitoring has stopped", removedInterface))
}

// addInterfaceToPFLACPMonitor adds a new interface to an existing PFLACPMonitor.
func addInterfaceToPFLACPMonitor(monitorName, newInterface string) error {
	By(fmt.Sprintf("Adding interface %s to PFLACPMonitor %s", newInterface, monitorName))

	// Get existing PFLACPMonitor
	existingMonitor, err := pfstatus.PullPfStatusConfiguration(
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull existing PFLACPMonitor %s: %w", monitorName, err)
	}

	// Add the new interface to the existing configuration
	currentInterfaces := existingMonitor.Definition.Spec.Interfaces
	currentInterfaces = append(currentInterfaces, newInterface)

	// Delete and recreate with updated interface list
	err = existingMonitor.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete PFLACPMonitor for update: %w", err)
	}

	// Wait for deletion to complete
	time.Sleep(10 * time.Second)

	// Recreate with the updated interface list
	nodeSelector := existingMonitor.Definition.Spec.NodeSelector
	err = createPFLACPMonitor(monitorName, currentInterfaces, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to recreate PFLACPMonitor with added interface: %w", err)
	}

	By(fmt.Sprintf("Successfully added interface %s to PFLACPMonitor %s", newInterface, monitorName))

	return nil
}

// removeInterfaceFromPFLACPMonitor removes an interface from an existing PFLACPMonitor.
func removeInterfaceFromPFLACPMonitor(monitorName, interfaceToRemove string) error {
	By(fmt.Sprintf("Removing interface %s from PFLACPMonitor %s", interfaceToRemove, monitorName))

	// Get existing PFLACPMonitor
	existingMonitor, err := pfstatus.PullPfStatusConfiguration(
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull existing PFLACPMonitor %s: %w", monitorName, err)
	}

	// Remove the specified interface from the configuration
	currentInterfaces := existingMonitor.Definition.Spec.Interfaces
	var filteredInterfaces []string
	for _, intf := range currentInterfaces {
		if intf != interfaceToRemove {
			filteredInterfaces = append(filteredInterfaces, intf)
		}
	}

	// Delete and recreate with filtered interface list
	err = existingMonitor.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete PFLACPMonitor for interface removal: %w", err)
	}

	// Wait for deletion to complete
	time.Sleep(10 * time.Second)

	// Recreate with the filtered interface list
	nodeSelector := existingMonitor.Definition.Spec.NodeSelector
	err = createPFLACPMonitor(monitorName, filteredInterfaces, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to recreate PFLACPMonitor without removed interface: %w", err)
	}

	By(fmt.Sprintf("Successfully removed interface %s from PFLACPMonitor %s", interfaceToRemove, monitorName))

	return nil
}

// verifyPFLACPMonitorMultiInterfaceLogsEventually verifies multiple interfaces for Eventually() usage.
func verifyPFLACPMonitorMultiInterfaceLogsEventually(nodeName string, interfaces []string) error {
	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod: %w", err)
	}

	podLogs, err := pflacpPod.GetFullLog("")
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod logs: %w", err)
	}

	// Verify each interface appears in the logs
	for _, interfaceName := range interfaces {
		if !strings.Contains(podLogs, fmt.Sprintf("interface\":\"%s\"", interfaceName)) {
			return fmt.Errorf("interface %s not found in PFLACPMonitor logs", interfaceName)
		}
	}

	By(fmt.Sprintf("Verified %d interfaces are being monitored", len(interfaces)))

	return nil
}

// verifyPFLACPMonitorInterfaceRemovalEventually verifies interface removal for Eventually() usage.
func verifyPFLACPMonitorInterfaceRemovalEventually(nodeName, removedInterface string) error {
	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod: %w", err)
	}

	podLogs, err := pflacpPod.GetFullLog("")
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod logs: %w", err)
	}

	// Get recent log lines to check if removed interface is still being monitored
	logLines := strings.Split(podLogs, "\n")

	// Get last 50 lines to check recent activity
	startIndex := len(logLines) - 50
	if startIndex < 0 {
		startIndex = 0
	}
	recentLines := logLines[startIndex:]

	// Check if removed interface still appears in recent logs
	for _, line := range recentLines {
		if strings.Contains(line, fmt.Sprintf("interface\":\"%s\"", removedInterface)) {
			return fmt.Errorf("interface %s still appears in recent logs, removal not complete", removedInterface)
		}
	}

	By(fmt.Sprintf("Confirmed interface %s is no longer being monitored", removedInterface))

	return nil
}

// verifyPFLACPMonitorLogsEventually is Eventually()-compatible version of verifyPFLACPMonitorLogs.
func verifyPFLACPMonitorLogsEventually(
	nodeName, logType, targetInterface string, srIovInterfacesUnderTest []string, expectedVFs int) error {

	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod: %w", err)
	}

	podLogs, err := pflacpPod.GetFullLog("")
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod logs: %w", err)
	}

	switch logType {
	case logTypeInitialization:
		return verifyInitializationLogsEventually(podLogs, srIovInterfacesUnderTest)
	case logTypeVFDisable:
		return verifyVFDisableLogsEventually(podLogs, targetInterface, expectedVFs)
	case logTypeVFEnable:
		return verifyVFEnableLogsEventually(podLogs, targetInterface, expectedVFs)
	default:
		return fmt.Errorf("invalid logType '%s'. Use '%s', '%s', or '%s'",
			logType, logTypeInitialization, logTypeVFDisable, logTypeVFEnable)
	}
}

// verifyInitializationLogsEventually verifies initialization logs for Eventually() usage.
func verifyInitializationLogsEventually(podLogs string, srIovInterfacesUnderTest []string) error {
	// Verify that configured SR-IOV interfaces are being monitored
	for _, sriovInterface := range srIovInterfacesUnderTest {
		if !strings.Contains(podLogs, fmt.Sprintf(`"interface":"%s"`, sriovInterface)) {
			return fmt.Errorf("PFLACPMonitor should be monitoring interface %s", sriovInterface)
		}
	}

	// Verify LACP is up on configured interfaces
	for _, sriovInterface := range srIovInterfacesUnderTest {
		if !strings.Contains(podLogs, fmt.Sprintf(`"lacp is up","interface":"%s"`, sriovInterface)) {
			return fmt.Errorf("LACP should be up on interface %s", sriovInterface)
		}
	}

	// Verify PFLACPMonitor initialization
	if !strings.Contains(podLogs, "interfaces to monitor") {
		return fmt.Errorf("PFLACPMonitor should show 'interfaces to monitor' in logs")
	}
	if !strings.Contains(podLogs, "Starting application") {
		return fmt.Errorf("PFLACPMonitor should show 'Starting application' in logs")
	}

	By("Verified PFLACPMonitor initialization and interface monitoring")
	return nil
}

// verifyVFDisableLogsEventually verifies VF disable logs for Eventually() usage.
func verifyVFDisableLogsEventually(podLogs, targetInterface string, expectedVFs int) error {
	// Check for VF disable logs
	if !strings.Contains(podLogs, fmt.Sprintf("vf link state was set")) {
		return fmt.Errorf("should show VF link state changes in logs")
	}

	return nil
}

// verifyVFEnableLogsEventually verifies VF enable logs for Eventually() usage.
func verifyVFEnableLogsEventually(podLogs, targetInterface string, expectedVFs int) error {
	// Check for VF enable logs
	if !strings.Contains(podLogs, fmt.Sprintf("vf link state was set")) {
		return fmt.Errorf("should show VF link state changes in logs")
	}

	return nil
}

// createBondedClient creates a bonded client pod using port0 and port1 VFs through the bonded NAD.
func createBondedClient(podName, nodeName, nadName string) (*pod.Builder, error) {
	By(fmt.Sprintf("Creating bonded client pod %s on node %s", podName, nodeName))

	// Create network annotation for bonded interface with the two SR-IOV networks and bonded NAD
	annotation := pod.StaticIPBondAnnotationWithInterface(
		nadName,
		bondTestInterface,
		[]string{sriovNetworkPort0Name, sriovNetworkPort1Name},
		[]string{"192.168.10.254/24"})

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
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace).
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

// deletePFLACPMonitor deletes a PFLACPMonitor resource.
func deletePFLACPMonitor(monitorName string) error {
	By(fmt.Sprintf("Deleting PFLACPMonitor %s", monitorName))

	// Pull the existing PFLACPMonitor
	pflacpMonitor, err := pfstatus.PullPfStatusConfiguration(
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull PFLACPMonitor %s: %w", monitorName, err)
	}

	// Delete the PFLACPMonitor resource
	err = pflacpMonitor.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete PFLACPMonitor %s: %w", monitorName, err)
	}

	By(fmt.Sprintf("Successfully deleted PFLACPMonitor %s", monitorName))
	return nil
}

// removeLACPBondInterfaces removes LACP bond interfaces using NMState.
func removeLACPBondInterfaces(workerNodeName string) error {
	By("Setting bond interfaces to absent state via NMState")

	// Create node selector for specific worker node
	nodeSelector := createNodeSelector(workerNodeName)

	// Create NMState policy to remove bond interfaces
	bondRemovalPolicy := nmstate.NewPolicyBuilder(APIClient, "remove-lacp-bonds", nodeSelector).
		WithAbsentInterface(nodeBond10Interface).
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

	// Get LAG names from environment
	lacpInterfaces, err := NetConfig.GetSwitchLagNames()
	if err != nil {
		return err
	}

	// Then, add physical interfaces to aggregated ethernet interfaces
	// Map first interface to first LAG, second interface to second LAG
	if len(physicalInterfaces) >= 2 && len(lacpInterfaces) >= 2 {
		commands = append(commands,
			fmt.Sprintf("set interfaces %s ether-options 802.3ad %s", physicalInterfaces[0], lacpInterfaces[0]),
			fmt.Sprintf("set interfaces %s ether-options 802.3ad %s", physicalInterfaces[1], lacpInterfaces[1]),
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

// setLACPBlockFilterOnInterface applies or removes the LACP block firewall filter on the first LAG interface.
func setLACPBlockFilterOnInterface(credentials *sriovenv.SwitchCredentials, enable bool) {
	// Check for nil credentials (can happen if BeforeAll failed)
	if credentials == nil {
		glog.V(90).Infof("Switch credentials are nil, skipping LACP filter operation")

		return
	}

	// Get LAG names from environment
	lacpInterfaces, err := NetConfig.GetSwitchLagNames()
	if err != nil {
		glog.Errorf("Failed to get switch LAG names: %v", err)

		return
	}

	var (
		command           string
		actionDescription string
	)

	firstLagInterface := lacpInterfaces[0]

	if enable {
		command = fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching filter input BLOCK-LACP", firstLagInterface)
		actionDescription = "Applying"
	} else {
		command = fmt.Sprintf("delete interfaces %s unit 0 family ethernet-switching filter input BLOCK-LACP",
			firstLagInterface)
		actionDescription = "Removing"
	}

	By(fmt.Sprintf("%s LACP block filter on interface %s", actionDescription, firstLagInterface))

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	Expect(err).ToNot(HaveOccurred(), "Failed to create switch session")
	defer jnpr.Close()

	commands := []string{command}

	err = jnpr.Config(commands)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to %s LACP block filter on interface", strings.ToLower(actionDescription)))
}

// verifyPFStatusRelayOperatorRunning verifies that the PF Status Relay operator is running and ready.
func verifyPFStatusRelayOperatorRunning() error {
	By("Checking PF Status Relay operator deployment status")

	pfStatusOperatorDeployment, err := deployment.Pull(APIClient,
		"pf-status-relay-operator-controller-manager", NetConfig.PFStatusRelayOperatorNamespace)
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
		clientPod.Definition.Name, testClientIP, bondTestInterface))

	command := []string{
		"testcmd",
		fmt.Sprintf("-interface=%s", bondTestInterface),
		"-protocol=tcp",
		"-port=4444",
		fmt.Sprintf("-server=%s", testClientIP),
	}

	output, err := clientPod.ExecCommand(command, clientPod.Definition.Spec.Containers[0].Name)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run testcmd on %s, command output: %s",
		clientPod.Definition.Name, output.String()))

	By("Verify bonded interface connectivity has no packet loss")
	Expect(output.String()).Should(ContainSubstring("0 packet loss"),
		fmt.Sprintf("Bonded interface %s should have 0 packet loss", bondTestInterface))
	Expect(output.String()).Should(ContainSubstring("TCP test passed as expected"),
		"TCP test should pass successfully")
}

// getPFLACPMonitorPod retrieves the PF status relay daemon set pod created by PFLACPMonitor.
func getPFLACPMonitorPod(nodeName string) (*pod.Builder, error) {
	By(fmt.Sprintf("Getting PF status relay daemon set pod on node %s", nodeName))

	// The PF status relay daemon set creates pods - try different patterns
	monitorNS := NetConfig.PFStatusRelayOperatorNamespace
	var podList []*pod.Builder
	var err error

	// Try different possible naming patterns
	possiblePatterns := []string{
		"pf-status-relay-ds-pflacpmonitor",
		"pf-status-relay-ds",
		"pf-status-relay",
		"pflacpmonitor",
	}

	for _, pattern := range possiblePatterns {
		podList, err = pod.ListByNamePattern(APIClient, pattern, monitorNS)
		if err == nil && len(podList) > 0 {
			By(fmt.Sprintf("Found PF status relay pods with pattern: %s", pattern))
			break
		}
	}

	if len(podList) == 0 {
		return nil, fmt.Errorf("no PF status relay daemon set pods found with any pattern in namespace %s. Tried patterns: %v",
			monitorNS, possiblePatterns)
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
	nodeName, logType, targetInterface string, srIovInterfacesUnderTest []string, expectedVFs int) {
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
	case logTypeVFEnable:
		verifyVFEnableLogs(podLogs, targetInterface, expectedVFs)
	default:
		Expect(false).To(BeTrue(),
			fmt.Sprintf("Invalid logType '%s'. Use '%s', '%s', or '%s'",
				logType, logTypeInitialization, logTypeVFDisable, logTypeVFEnable))
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

// verifyVFEnableLogs verifies PFLACPMonitor VF enable logs after LACP recovery.
func verifyVFEnableLogs(podLogs, targetInterface string, expectedVFs int) {
	By(fmt.Sprintf("Verifying VF enable logs for interface %s with %d expected VFs", targetInterface, expectedVFs))

	// Look for VF link state being set to "auto" (re-enabling VFs)
	vfEnableCount := 0
	lines := strings.Split(podLogs, "\n")

	for _, line := range lines {
		if strings.Contains(line, "vf link state was set") &&
			strings.Contains(line, "state\":\"auto\"") &&
			strings.Contains(line, fmt.Sprintf("interface\":\"%s\"", targetInterface)) {
			vfEnableCount++
			By(fmt.Sprintf("Found VF enable log: %s", line))
		}
	}

	Expect(vfEnableCount).To(BeNumerically(">=", expectedVFs),
		fmt.Sprintf("Expected at least %d VF enable logs for interface %s, found %d",
			expectedVFs, targetInterface, vfEnableCount))

	By(fmt.Sprintf("Successfully verified %d VF enable logs for interface %s", expectedVFs, targetInterface))
}

// checkBondingStatus checks LACP bonding status on both pod and node.
func checkBondingStatus(bondedPod *pod.Builder, nodeName string) (podErr, nodeErr error) {
	podErr = checkBondingStatusInPod(bondedPod, bondTestInterface)
	nodeErr = checkBondingStatusOnNode(nodeName)

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
func checkBondingStatusOnNode(nodeName string) error {
	By(fmt.Sprintf("Checking bonding status for %s on node %s", nodeBond10Interface, nodeName))

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", nodeBond10Interface)
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

	return analyzeLACPPortStates(output, nodeBond10Interface, "node")
}

// configureLACPBondInterfaceSecondary configures LACP bond interface for a secondary interface.
func configureLACPBondInterfaceSecondary(nodeName, interfaceName string) error {
	By(fmt.Sprintf("Configuring LACP bond interface for secondary interface %s on node %s", interfaceName, nodeName))

	// Create node selector for the specific node
	nodeSelector := createNodeSelector(nodeName)

	// Use the same bond interface options as the main configuration
	bondInterfaceOptions := nmstate.OptionsLinkAggregation{
		Miimon:   100,
		LacpRate: "fast",
		MinLinks: 1,
	}

	// Create bond policy for the second interface using bond20
	nmstatePolicyName := fmt.Sprintf("lacp-bond-secondary-%s", nodeName)
	secondaryBondPolicy := nmstate.NewPolicyBuilder(APIClient, nmstatePolicyName, nodeSelector).
		WithBondInterface([]string{interfaceName}, nodeBond20Interface, bondMode802_3ad, bondInterfaceOptions)

	// Create the policy and wait for it to be available
	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, secondaryBondPolicy)
	if err != nil {
		return fmt.Errorf("failed to create secondary bond policy for %s: %w", nodeBond20Interface, err)
	}

	By(fmt.Sprintf("Successfully configured secondary LACP bond interface %s for interface %s", nodeBond20Interface, interfaceName))
	return nil
}

// checkBondingStatusOnNodeSecondary checks bonding status on a secondary bond interface.
func checkBondingStatusOnNodeSecondary(nodeName, interfaceName string) error {
	By(fmt.Sprintf("Checking secondary bonding status for %s on node %s", nodeBond20Interface, nodeName))

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", nodeBond20Interface)
	command := fmt.Sprintf("cat %s", bondingPath)

	// Create node selector for the specific node
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to read secondary bonding status on node %s: %w", nodeName, err)
	}

	// Get output for the specific node
	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for secondary bond", nodeName)
	}

	// Verify LACP port states (expecting port state 63)
	err = analyzeLACPPortStates(output, nodeBond20Interface, "node")
	if err != nil {
		return fmt.Errorf("secondary bond interface %s LACP verification failed: %w", nodeBond20Interface, err)
	}

	By(fmt.Sprintf("Secondary bond interface %s shows proper LACP functionality with port state 63", nodeBond20Interface))
	return nil
}

// removeSecondaryBondInterface removes the secondary bond interface to simulate interface without LACP.
func removeSecondaryBondInterface(nodeName string) error {
	By(fmt.Sprintf("Removing secondary bond interface %s from node %s", nodeBond20Interface, nodeName))

	// Create node selector for the specific node
	nodeSelector := createNodeSelector(nodeName)

	// Create NMState policy to remove bond20 interface
	bondRemovalPolicy := nmstate.NewPolicyBuilder(APIClient, fmt.Sprintf("remove-bond20-%s", nodeName), nodeSelector).
		WithAbsentInterface(nodeBond20Interface)

	// Create the policy and wait for it to be applied
	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bondRemovalPolicy)
	if err != nil {
		return fmt.Errorf("failed to remove secondary bond interface %s: %w", nodeBond20Interface, err)
	}

	By(fmt.Sprintf("Successfully removed secondary bond interface %s", nodeBond20Interface))
	return nil
}

// setVFsStateOnNode sets VF states on a specific node using ip link command.
func setVFsStateOnNode(nodeName, interfaceName string, vfIDs []int, state string) error {
	By(fmt.Sprintf("Setting VF states to %s on interface %s for node %s", state, interfaceName, nodeName))

	// Create node selector for the specific node
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	// Set each VF to the specified state
	for _, vfID := range vfIDs {
		command := fmt.Sprintf("ip link set dev %s vf %d state %s", interfaceName, vfID, state)

		outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
		if err != nil {
			return fmt.Errorf("failed to set VF %d state to %s on interface %s: %w", vfID, state, interfaceName, err)
		}

		// Check if command was executed successfully on the target node
		if output, exists := outputs[nodeName]; exists && strings.Contains(output, "error") {
			return fmt.Errorf("error setting VF %d state: %s", vfID, output)
		}

		By(fmt.Sprintf("Successfully set VF %d to state %s on interface %s", vfID, state, interfaceName))
	}

	return nil
}

// verifyVFsStateOnNode verifies that VFs are in the expected state on a specific node.
func verifyVFsStateOnNode(nodeName, interfaceName string, vfIDs []int, expectedState string) error {
	By(fmt.Sprintf("Verifying VF states are %s on interface %s for node %s", expectedState, interfaceName, nodeName))

	// Create node selector for the specific node
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	// Get VF information for the interface
	command := fmt.Sprintf("ip link show %s", interfaceName)
	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get interface %s information: %w", interfaceName, err)
	}

	// Get output for the specific node
	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for interface %s", nodeName, interfaceName)
	}

	// Verify each VF is in the expected state
	for _, vfID := range vfIDs {
		// Look for VF state in the output - format: "vf 1 link/ether ... state enable"
		vfPattern := fmt.Sprintf("vf %d", vfID)
		statePattern := fmt.Sprintf("state %s", expectedState)

		lines := strings.Split(output, "\n")
		found := false
		for _, line := range lines {
			if strings.Contains(line, vfPattern) && strings.Contains(line, statePattern) {
				found = true
				By(fmt.Sprintf("✅ VF %d is in %s state", vfID, expectedState))
				break
			}
		}

		if !found {
			return fmt.Errorf("VF %d is not in %s state on interface %s", vfID, expectedState, interfaceName)
		}
	}

	return nil
}

// verifyVFsStateOnInterface verifies that all VFs on an interface are in the expected state.
func verifyVFsStateOnInterface(nodeName, interfaceName, expectedState string) error {
	By(fmt.Sprintf("Verifying all VFs are in %s state on interface %s for node %s", expectedState, interfaceName, nodeName))

	// Create node selector for the specific node
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	// Get VF information for the interface
	command := fmt.Sprintf("ip link show %s", interfaceName)
	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get interface %s information: %w", interfaceName, err)
	}

	// Get output for the specific node
	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for interface %s", nodeName, interfaceName)
	}

	// Parse output to verify VF states
	lines := strings.Split(output, "\n")
	statePattern := fmt.Sprintf("state %s", expectedState)
	vfCount := 0

	for _, line := range lines {
		if strings.Contains(line, "vf ") && strings.Contains(line, statePattern) {
			vfCount++
		}
	}

	// Expect at least 5 VFs (standard SR-IOV setup)
	if vfCount < 5 {
		return fmt.Errorf("expected at least 5 VFs in %s state, found %d on interface %s", expectedState, vfCount, interfaceName)
	}

	By(fmt.Sprintf("✅ Verified %d VFs are in %s state on interface %s", vfCount, expectedState, interfaceName))
	return nil
}

// verifyLACPPortStateDown verifies that LACP port state is down (not 63).
func verifyLACPPortStateDown(nodeName, bondInterface string) error {
	By(fmt.Sprintf("Verifying LACP port state is down on %s for node %s", bondInterface, nodeName))

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

	// Parse output to find port states
	lines := strings.Split(output, "\n")
	var actorPortState string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for actor port state
		if strings.Contains(line, "port state:") && strings.Contains(output, "details actor lacp pdu:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				actorPortState = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	// Verify port state is NOT 63 (which indicates LACP is down)
	if actorPortState == "63" {
		return fmt.Errorf("LACP port state is 63 (up), expected it to be down on %s", bondInterface)
	}

	By(fmt.Sprintf("✅ Verified LACP port state is down (state: %s, not 63) on %s", actorPortState, bondInterface))
	return nil
}

// verifyInterfaceIsUp verifies that a network interface is in UP state.
func verifyInterfaceIsUp(nodeName, interfaceName string) error {
	By(fmt.Sprintf("Verifying interface %s is UP on node %s", interfaceName, nodeName))

	// Create node selector for the specific node
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	// Check interface status
	command := fmt.Sprintf("ip link show %s", interfaceName)
	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get interface %s status: %w", interfaceName, err)
	}

	// Get output for the specific node
	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for interface %s", nodeName, interfaceName)
	}

	// Check if interface is UP
	if !strings.Contains(output, "UP") {
		return fmt.Errorf("interface %s is not UP on node %s. Output: %s", interfaceName, nodeName, output)
	}

	By(fmt.Sprintf("✅ Verified interface %s is UP", interfaceName))
	return nil
}

// analyzeLACPPortStates analyzes the bonding status output for LACP port states.
func analyzeLACPPortStates(bondingOutput, bondInterface, location string) error {
	By(fmt.Sprintf("Analyzing LACP port states for %s on %s", bondInterface, location))

	// When LACP is up, the port state should be 63 for both actor and partner
	expectedPortState := "63"

	// Split output into lines for parsing
	lines := strings.Split(bondingOutput, "\n")

	var (
		actorPortState, partnerPortState string
		inActorSection, inPartnerSection bool
	)

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

	var (
		bondingMode, miiStatus string
		net1Status, net2Status string
		currentInterface       string
	)

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
				case net1Interface:
					net1Status = status
				case net2Interface:
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

// createBondedNADWithMode creates a Network Attachment Definition for bonded interfaces with a specific bonding mode.
func createBondedNADWithMode(nadName, bondMode string) error {
	By(fmt.Sprintf("Creating bonded NAD %s with mode %s", nadName, bondMode))

	// For balance-tlb and balance-alb modes, create NAD manually with proven working config
	if bondMode == "balance-tlb" || bondMode == "balance-alb" {
		return createBondedNADManually(nadName, bondMode)
	}

	// Use eco-goinfra for supported modes (active-backup, balance-rr, balance-xor)
	bondNadBuilder, err := defineBondNad(nadName, bondMode, "static", 2)
	if err != nil {
		return fmt.Errorf("failed to define bonded NAD %s with mode %s: %w", nadName, bondMode, err)
	}

	_, err = bondNadBuilder.Create()
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

// createBondedNADManually creates a NAD using the proven working configuration for balance-tlb/alb modes.
func createBondedNADManually(nadName, bondMode string) error {
	By(fmt.Sprintf("Creating bonded NAD %s manually with proven %s configuration", nadName, bondMode))

	// Use your proven working configuration
	nadYAML := fmt.Sprintf(`apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: %s
  namespace: %s
spec:
  config: |-
    {"type": "bond", "cniVersion": "0.3.1", "name": "bond-net1",
    "mode": "%s", "failOverMac": 0, "linksInContainer": true, "miimon": "100", "mtu": 1450,
    "links": [{"name": "net1"},{"name": "net2"}], "capabilities": {"ips": true}, "ipam": {"type": "static"}}
`, nadName, tsparams.TestNamespaceName, bondMode)

	err := os.WriteFile("/tmp/"+nadName+".yaml", []byte(nadYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write NAD YAML: %w", err)
	}

	// Use kubectl to create it
	cmd := exec.Command("oc", "apply", "-f", "/tmp/"+nadName+".yaml")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to create NAD via oc: %w, output: %s", err, string(output))
	}

	By(fmt.Sprintf("Successfully created bonded NAD %s with %s mode using proven configuration", nadName, bondMode))

	// Wait for NAD to be available (same as eco-goinfra approach)
	Eventually(func() error {
		_, err := nad.Pull(APIClient, nadName, tsparams.TestNamespaceName)
		return err
	}, tsparams.WaitTimeout, tsparams.RetryInterval).Should(BeNil(),
		fmt.Sprintf("Failed to pull bonded NAD %s", nadName))

	return nil
}

// checkBondingStatusInPodTlb checks bonding status in a pod specifically for balance-tlb mode.
func checkBondingStatusInPodTlb(bondedPod *pod.Builder, bondInterface string) error {
	By(fmt.Sprintf("Checking balance-tlb bonding status for %s in pod %s", bondInterface, bondedPod.Definition.Name))

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)

	output, err := bondedPod.ExecCommand([]string{"cat", bondingPath})
	if err != nil {
		return fmt.Errorf("failed to read bonding status in pod: %w", err)
	}

	return analyzePodBondingStatusTlb(output.String(), bondInterface, "pod")
}

// checkBondingStatusInPodAlb checks bonding status in a pod specifically for balance-alb mode.
func checkBondingStatusInPodAlb(bondedPod *pod.Builder, bondInterface string) error {
	By(fmt.Sprintf("Checking balance-alb bonding status for %s in pod %s", bondInterface, bondedPod.Definition.Name))

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)

	output, err := bondedPod.ExecCommand([]string{"cat", bondingPath})
	if err != nil {
		return fmt.Errorf("failed to read bonding status in pod: %w", err)
	}

	return analyzePodBondingStatusAlb(output.String(), bondInterface, "pod")
}

// performLACPFailureAndRecoveryTestTlb executes the complete LACP failure and recovery test flow for balance-tlb mode.
func performLACPFailureAndRecoveryTestTlb(
	bondedClientPod *pod.Builder, workerNodeName, primaryIntf string, srIovInterfacesUnderTest []string,
	switchCredentials *sriovenv.SwitchCredentials) {
	By("Verify initial PFLACPMonitor logs for balance-tlb test")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeInitialization, "", srIovInterfacesUnderTest, 0)

	By("Test tcp traffic from the balance-tlb bond interface to the client pod")
	validateBondedTCPTraffic(bondedClientPod)

	By("Activate LACP block filter to simulate LACP failure for balance-tlb test")
	setLACPBlockFilterOnInterface(switchCredentials, true)

	By("Waiting for LACP failure to be detected on node bonding for balance-tlb")
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 30*time.Second, 5*time.Second).Should(HaveOccurred(),
		fmt.Sprintf("LACP should fail on node %s after block filter is applied", nodeBond10Interface))

	By("Test bonded interface connectivity after LACP failure for balance-tlb")
	validateBondedTCPTraffic(bondedClientPod)

	By("Verify VF disable logs after LACP failure for balance-tlb test")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeVFDisable, primaryIntf, srIovInterfacesUnderTest, 5)

	By("Check bonding status after LACP failure - expect failures for balance-tlb")
	podErr := checkBondingStatusInPodTlb(bondedClientPod, bondTestInterface)
	nodeErr := checkBondingStatusOnNode(workerNodeName)
	Expect(nodeErr).To(HaveOccurred(),
		fmt.Sprintf("LACP should be failing on node %s after LACP block filter is applied", nodeBond10Interface))

	By("Check pod bonding status after LACP failure - balance-tlb should adapt")
	Expect(podErr).ToNot(HaveOccurred(),
		fmt.Sprintf("Pod %s should adapt to LACP failure with balance-tlb mode", bondTestInterface))

	By("Test bonded interface connectivity after LACP failure - should work with balance-tlb")
	validateBondedTCPTraffic(bondedClientPod)

	By("Remove LACP block filter to restore LACP functionality for balance-tlb test")
	setLACPBlockFilterOnInterface(switchCredentials, false)

	By(fmt.Sprintf("Verify LACP is back up on node %s using /proc/net/bonding for balance-tlb",
		workerNodeName))
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 90*time.Second, 5*time.Second).Should(Succeed(),
		fmt.Sprintf("LACP should recover on node %s after block filter is removed", nodeBond10Interface))

	By("Verify VF enable logs after LACP recovery for balance-tlb test")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeVFEnable, primaryIntf, srIovInterfacesUnderTest, 5)

	By("Test bonded interface connectivity after LACP recovery for balance-tlb")
	validateBondedTCPTraffic(bondedClientPod)
}

// performLACPFailureAndRecoveryTestAlb executes the complete LACP failure and recovery test flow for balance-alb mode.
func performLACPFailureAndRecoveryTestAlb(
	bondedClientPod *pod.Builder, workerNodeName, primaryIntf string, srIovInterfacesUnderTest []string,
	switchCredentials *sriovenv.SwitchCredentials) {
	By("Verify initial PFLACPMonitor logs for balance-alb test")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeInitialization, "", srIovInterfacesUnderTest, 0)

	By("Test tcp traffic from the balance-alb bond interface to the client pod")
	validateBondedTCPTraffic(bondedClientPod)

	By("Activate LACP block filter to simulate LACP failure for balance-alb test")
	setLACPBlockFilterOnInterface(switchCredentials, true)

	By("Waiting for LACP failure to be detected on node bonding for balance-alb")
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 30*time.Second, 5*time.Second).Should(HaveOccurred(),
		fmt.Sprintf("LACP should fail on node %s after block filter is applied", nodeBond10Interface))

	By("Test bonded interface connectivity after LACP failure for balance-alb")
	validateBondedTCPTraffic(bondedClientPod)

	By("Verify VF disable logs after LACP failure for balance-alb test")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeVFDisable, primaryIntf, srIovInterfacesUnderTest, 5)

	By("Check bonding status after LACP failure - expect failures for balance-alb")
	podErr := checkBondingStatusInPodAlb(bondedClientPod, bondTestInterface)
	nodeErr := checkBondingStatusOnNode(workerNodeName)
	Expect(nodeErr).To(HaveOccurred(),
		fmt.Sprintf("LACP should be failing on node %s after LACP block filter is applied", nodeBond10Interface))

	By("Check pod bonding status after LACP failure - balance-alb should adapt")
	Expect(podErr).ToNot(HaveOccurred(),
		fmt.Sprintf("Pod %s should adapt to LACP failure with balance-alb mode", bondTestInterface))

	By("Test bonded interface connectivity after LACP failure - should work with balance-alb")
	validateBondedTCPTraffic(bondedClientPod)

	By("Remove LACP block filter to restore LACP functionality for balance-alb test")
	setLACPBlockFilterOnInterface(switchCredentials, false)

	By(fmt.Sprintf("Verify LACP is back up on node %s using /proc/net/bonding for balance-alb",
		workerNodeName))
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 90*time.Second, 5*time.Second).Should(Succeed(),
		fmt.Sprintf("LACP should recover on node %s after block filter is removed", nodeBond10Interface))

	By("Verify VF enable logs after LACP recovery for balance-alb test")
	verifyPFLACPMonitorLogs(workerNodeName, logTypeVFEnable, primaryIntf, srIovInterfacesUnderTest, 5)

	By("Test bonded interface connectivity after LACP recovery for balance-alb")
	validateBondedTCPTraffic(bondedClientPod)
}

// analyzePodBondingStatusTlb analyzes the bonding status output for balance-tlb mode (used in pods).
func analyzePodBondingStatusTlb(bondingOutput, bondInterface, location string) error {
	By(fmt.Sprintf("Analyzing balance-tlb bonding status for %s on %s", bondInterface, location))

	lines := strings.Split(bondingOutput, "\n")

	var (
		bondingMode, miiStatus string
		net1Status, net2Status string
		currentInterface       string
	)

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
				case net1Interface:
					net1Status = status
				case net2Interface:
					net2Status = status
				}
			}
		}
	}

	// Validate bonding mode - balance-tlb typically shows as "transmit load balancing"
	if !strings.Contains(bondingMode, "transmit load balancing") && !strings.Contains(bondingMode, "balance-tlb") {
		return fmt.Errorf("expected transmit load balancing (balance-tlb) bonding mode, got: %s on %s %s",
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

	By(fmt.Sprintf("Balance-tlb bonding is functioning properly on %s %s - MII Status: %s, net1: %s, net2: %s",
		location, bondInterface, miiStatus, net1Status, net2Status))

	return nil
}

// analyzePodBondingStatusAlb analyzes the bonding status output for balance-alb mode (used in pods).
func analyzePodBondingStatusAlb(bondingOutput, bondInterface, location string) error {
	By(fmt.Sprintf("Analyzing balance-alb bonding status for %s on %s", bondInterface, location))

	lines := strings.Split(bondingOutput, "\n")

	var (
		bondingMode, miiStatus string
		net1Status, net2Status string
		currentInterface       string
	)

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
				case net1Interface:
					net1Status = status
				case net2Interface:
					net2Status = status
				}
			}
		}
	}

	// Validate bonding mode - balance-alb typically shows as "adaptive load balancing"
	if !strings.Contains(bondingMode, "adaptive load balancing") && !strings.Contains(bondingMode, "balance-alb") {
		return fmt.Errorf("expected adaptive load balancing (balance-alb) bonding mode, got: %s on %s %s",
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

	By(fmt.Sprintf("Balance-alb bonding is functioning properly on %s %s - MII Status: %s, net1: %s, net2: %s",
		location, bondInterface, miiStatus, net1Status, net2Status))

	return nil
}
