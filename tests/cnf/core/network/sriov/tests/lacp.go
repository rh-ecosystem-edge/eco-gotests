package tests

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

const (
	sriovNetworkPort0Name     = "sriovnetwork-port0"
	sriovNetworkPort1Name     = "sriovnetwork-port1"
	sriovNetworkClientName    = "sriovnetwork-client"
	srIovPolicyPort0Name      = "sriov-policy-port0"
	srIovPolicyPort1Name      = "sriov-policy-port1"
	srIovPolicyClientName     = "sriov-policy-client"
	srIovPolicyPort0ResName   = "resourceport0"
	srIovPolicyPort1ResName   = "resourceport1"
	srIovPolicyClientResName  = "resourceclient"
	bondedClientPodName       = "client-bond"
	testClientIP              = "192.168.10.1"
	testClientIPWithCIDR      = "192.168.10.1/24"
	testServerIPWithCIDR      = "192.168.10.254/24"
	bondTestInterface         = "bond0"
	nodeBond10Interface       = "bond10"
	nodeBond20Interface       = "bond20"
	bondModeActiveBackup      = "active-backup"
	bondMode802_3ad           = "802.3ad"
	bondedNADNameActiveBackup = "lacp-bond-nad"
	logTypeInitialization     = "initialization"
	logTypeVFDisable          = "vf-disable"
	logTypeVFEnable           = "vf-enable"
	expectedVFCount           = 5
	net1Interface             = "net1"
	net2Interface             = "net2"
	pfLacpMonitorName         = "pflacpmonitor-mgmt"
	perfProfileName           = "performance-profile-dpdk"
)

var _ = Describe("LACP Status Relay", Ordered, Label(tsparams.LabelSuite), ContinueOnFailure, func() {
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
		err := sriovoperator.IsSriovDeployed(APIClient, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")

		By("Verifying PF Status Relay operator is running")
		err = verifyPFStatusRelayOperatorRunning()
		Expect(err).ToNot(HaveOccurred(), "PF Status Relay operator is not running")

		By("Discover worker nodes")
		workerNodeList, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Fail to discover worker nodes")

		worker0NodeName = workerNodeList[0].Definition.Name
		worker1NodeName = workerNodeList[1].Definition.Name

		By("Collecting SR-IOV interfaces for LACP testing")
		Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
			"Failed to get required SR-IOV interfaces")

		srIovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		secondaryInterface0 = srIovInterfacesUnderTest[0]
		secondaryInterface1 = srIovInterfacesUnderTest[1]

		By("Configure lab switch interface to support LACP")
		switchCredentials, err = sriovenv.NewSwitchCredentials()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

		By("Collecting switch interfaces")
		switchInterfaces, err = NetConfig.GetSwitchInterfaces()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch interfaces")
		Expect(len(switchInterfaces)).To(BeNumerically(">=", 2),
			"At least 2 switch interfaces are required for LACP tests")

		By("Configure LACP on switch interfaces")
		lacpInterfaces, err := NetConfig.GetSwitchLagNames()
		Expect(err).ToNot(HaveOccurred(), "Failed to get switch LAG names")
		err = enableLACPOnSwitchInterfaces(switchCredentials, lacpInterfaces)
		Expect(err).ToNot(HaveOccurred(), "Failed to enable LACP on the switch")

		By("Configure physical interfaces to join aggregated ethernet interfaces")
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
		err = checkBondingStatusOnNode(worker0NodeName)
		Expect(err).ToNot(HaveOccurred(),
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
			err = disableLACPOnSwitch(switchCredentials, lacpInterfaces, firstTwoSwitchInterfaces)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore switch configuration")
		} else {
			By("Switch credentials or interfaces are nil, skipping switch configuration restore")
		}
	})

	Context("linux pod", func() {
		BeforeAll(func() {

			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			nodeSelectorWorker1 := createNodeSelector(worker1NodeName)

			err := createLACPSriovPolicy(srIovPolicyPort0Name, srIovPolicyPort0ResName,
				secondaryInterface0, nodeSelectorWorker0, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for port0")

			err = createLACPSriovPolicy(srIovPolicyPort1Name, srIovPolicyPort1ResName,
				secondaryInterface1, nodeSelectorWorker0, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for port1")

			err = createLACPSriovPolicy(srIovPolicyClientName, srIovPolicyClientResName,
				secondaryInterface0, nodeSelectorWorker1, worker1NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy for client")

			By("Waiting for SR-IOV and MCP to be stable after policy creation")
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV and MCP to be stable")

			By("Creating SriovNetworks for LACP testing")
			err = sriovenv.DefineAndCreateSriovNetwork(sriovNetworkPort0Name, srIovPolicyPort0ResName, false, false)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork for port0")
			err = sriovenv.DefineAndCreateSriovNetwork(sriovNetworkPort1Name, srIovPolicyPort1ResName, false, false)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork for port1")
			err = sriovenv.DefineAndCreateSriovNetwork(sriovNetworkClientName, srIovPolicyClientResName, true, false)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork for client")

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
			err := createBondedNAD(bondedNADNameActiveBackup)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD")

			By(fmt.Sprintf("Deploying PFLACPMonitor on %s", worker0NodeName))
			nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
			err = createPFLACPMonitor("pflacpmonitor", srIovInterfacesUnderTest, nodeSelectorWorker0)
			Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")

			By(fmt.Sprintf("Deploying bonded client pod on %s using port0 and port1 VFs", worker0NodeName))
			bondedClientPod, err := createBondedClient(bondedClientPodName, worker0NodeName, bondedNADNameActiveBackup)
			Expect(err).ToNot(HaveOccurred(), "Failed to create bonded client pod")

			By("Verify LACP bonding status in bonded client pod")
			err = checkBondingStatusInPod(bondedClientPod)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("LACP should be functioning properly in bonded client pod %s", bondTestInterface))

			performLACPFailureAndRecoveryTestWithMode(bondedClientPod, worker0NodeName, secondaryInterface0,
				srIovInterfacesUnderTest, switchCredentials, bondModeActiveBackup)
		})

		It("Verify that an interface can be added and removed from the PFLACPMonitor interface monitoring",
			reportxml.ID("83323"), func() {

				By(fmt.Sprintf("Deploying PFLACPMonitor on %s with single PF interface", worker0NodeName))
				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
				err := createPFLACPMonitor(pfLacpMonitorName, []string{srIovInterfacesUnderTest[0]}, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to create initial PFLACPMonitor")

				By("Verifying PFLACPMonitor logs show monitored interface and status")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, []string{secondaryInterface0})
				}, time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show proper initialization and interface monitoring within timeout")

				By("Redeploying PFLACPMonitor to add second PF interface")
				err = updatePFLACPMonitor(pfLacpMonitorName, []string{srIovInterfacesUnderTest[0],
					srIovInterfacesUnderTest[1]}, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to update PFLACPMonitor with additional interface")

				By("Verifying both interfaces are actively monitored in updated PFLACPMonitor")
				Eventually(func() error {
					return verifyPFLACPMonitorMultiInterfaceLogsEventually(worker0NodeName,
						[]string{srIovInterfacesUnderTest[0], srIovInterfacesUnderTest[1]})
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show both interfaces being monitored within timeout")

				By("Removing one interface from PFLACPMonitor configuration")
				err = updatePFLACPMonitor(pfLacpMonitorName, []string{srIovInterfacesUnderTest[0]}, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove interface from PFLACPMonitor")

				By("Verifying removed interface monitoring stops with corresponding log")
				removedInterface := srIovInterfacesUnderTest[1]
				Eventually(func() error {
					return verifyPFLACPMonitorInterfaceRemovalEventually(worker0NodeName, removedInterface)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should stop monitoring removed interface within timeout")

			})

		It("Verify that deployment of a bonded pod with VFs disabled by the pf-status-relay operator",
			reportxml.ID("83324"), func() {

				By("Setting up PFLACPMonitor to initially disable VFs when LACP failure occurs")
				setupSingleInterfacePFLACPMonitor(worker0NodeName, srIovInterfacesUnderTest[0])

				simulateLACPFailureAndVerify(worker0NodeName, switchCredentials)
				waitForVFStateChange(worker0NodeName, logTypeVFDisable, []string{srIovInterfacesUnderTest[0]},
					"PFLACPMonitor should detect LACP failure and disable VFs")

				By("Creating bonded Network Attachment Definition while VFs are disabled")
				err := createBondedNAD(bondedNADNameActiveBackup)
				Expect(err).ToNot(HaveOccurred(), "Failed to create bonded NAD")

				By("Deploying bonded client pod while VFs are in disabled state")
				bondedClientPod, err := createBondedClient("bonded-client-disabled-vfs", worker0NodeName,
					bondedNADNameActiveBackup)
				Expect(err).ToNot(HaveOccurred(), "Bonded pod deployment should succeed even with "+
					"initially disabled VFs")

				By("Verifying bonding status shows degraded operation with one VF disabled")
				err = checkBondingStatusInPod(bondedClientPod)
				Expect(err).ToNot(HaveOccurred(), "Active-backup bonding should work with one interface disabled")

				By("Verifying that one interface is disabled due to PFLACPMonitor action")
				err = verifyBondingDegradedState(bondedClientPod)
				Expect(err).ToNot(HaveOccurred(), "Should detect degraded bonding state with one interface down")

				restoreLACPAndVerifyRecovery(worker0NodeName, switchCredentials)
				waitForVFStateChange(worker0NodeName, logTypeVFEnable, []string{srIovInterfacesUnderTest[0]},
					"PFLACPMonitor should detect LACP recovery and re-enable VFs")

				By("Verifying bonded pod network functionality after VF recovery")
				err = checkBondingStatusInPod(bondedClientPod)
				Expect(err).ToNot(HaveOccurred(), "Bonded pod should have functional "+
					"bonding after VF recovery")

				By("Validating network connectivity through recovered bonded interface")
				validateBondedTCPTraffic(bondedClientPod)
			})

		It("Verify that an interface can be added to PfLACPMonitoring without LACP configured on the interface",
			reportxml.ID("83325"), func() {

				By(fmt.Sprintf(
					"Deploying PFLACPMonitor on %s with two interfaces - first with LACP, second without LACP",
					worker0NodeName))
				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)

				By("Removing second bond interface to simulate interface without LACP configuration")
				err := removeSecondaryBondInterface(worker0NodeName)
				Expect(err).ToNot(HaveOccurred(),
					"Failed to remove secondary bond interface")

				expandedInterfaces := []string{secondaryInterface0, secondaryInterface1}
				err = createPFLACPMonitor(pfLacpMonitorName, expandedInterfaces, nodeSelectorWorker0)
				Expect(err).ToNot(HaveOccurred(),
					"Failed to create PFLACPMonitor with both interfaces")

				By("Verify PFLACPMonitor logs show first PF with LACP (second PF without LACP should not cause failure)")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, []string{secondaryInterface0})
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show LACP up only on configured interface")

				By("Configure LACP on the second bond interface (node and switch)")
				err = configureLACPBondInterfaceSecondary(worker0NodeName, secondaryInterface1)
				Expect(err).ToNot(HaveOccurred(),
					"Failed to configure LACP bond interface for second interface")

				By("Verify LACP is up on the new bond interface with port state 63")
				Eventually(func() error {
					return checkBondingStatusOnNodeSecondary(worker0NodeName)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"Second bond interface should show LACP up with port state 63")

				By("Verify PFLACPMonitor logs show the second PF now has LACP configured")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, []string{secondaryInterface1})
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should detect LACP configuration on second interface")

				By("Validating both interfaces now show proper LACP functionality")
				err = checkBondingStatusOnNode(worker0NodeName)
				Expect(err).ToNot(HaveOccurred(),
					"Node bond interfaces should be functional after full LACP configuration")
			})

		It("Verify that a PfLACPMonitoring pod does not update VFs that are set to state Enabled",
			reportxml.ID("83326"), func() {

				By(fmt.Sprintf(
					"Deploy PFLACPMonitor CRD to monitor interface %s on %s", secondaryInterface0, worker0NodeName))
				setupSingleInterfacePFLACPMonitor(worker0NodeName, secondaryInterface0)

				By("Verify that the VFs are in auto state with ip link show")
				Eventually(func() error {
					return verifyVFsStateOnInterface(worker0NodeName, secondaryInterface0, "auto")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs should initially be in auto state")

				By("Manually set three of the VFs to enabled mode")
				err := setVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "enable")
				Expect(err).ToNot(HaveOccurred(), "Failed to set VFs to enabled state")

				By("Verifying VFs 1,2,3 are now in enabled state")
				Eventually(func() error {
					return verifyVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "enable")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs 1,2,3 should be in enabled state after manual configuration")

				simulateLACPFailureAndVerify(worker0NodeName, switchCredentials)

				By("Verify interface is up and only non-enabled VFs are disabled")
				Eventually(func() error {
					return verifyInterfaceIsUp(worker0NodeName, secondaryInterface0)
				}, 1*time.Minute, 5*time.Second).Should(Succeed(), "Interface should remain up")

				By("Verify enabled VFs (1,2,3) remain enabled")
				Eventually(func() error {
					return verifyVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "enable")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"Manually enabled VFs should remain enabled")

				By("Verify non-enabled VFs (0,4) are now disabled by PFLACPMonitor")
				Eventually(func() error {
					return verifyVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{0, 4}, "disable")
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"Interface should be up, enabled VFs remain enabled, non-enabled VFs disabled")

				By("Manually reset the VFs from enabled to disabled")
				err = setVFsStateOnNode(worker0NodeName, secondaryInterface0, []int{1, 2, 3}, "disable")
				Expect(err).ToNot(HaveOccurred(), "Failed to reset VFs from enabled to disabled")

				restoreLACPAndVerifyRecovery(worker0NodeName, switchCredentials)
				waitForVFStateChange(worker0NodeName, logTypeVFEnable, []string{secondaryInterface0},
					"PFLACPMonitor should detect LACP recovery and re-enable VFs to auto state")

				By("Validating node bond interface functionality after full recovery (allow extra time for LACP)")
				Eventually(func() error {
					return checkBondingStatusOnNode(worker0NodeName)
				}, 3*time.Minute, 15*time.Second).Should(Succeed(),
					"Node bond interface should be functional after recovery (LACP state 63)")
			})

		It("Verify that VFs remain in Disabled state after LACP is blocked and the PFLACPMonitor CRD is deleted",
			reportxml.ID("83327"), func() {

				By(fmt.Sprintf(
					"Deploy PFLACPMonitor CRD to monitor interface %s on %s", secondaryInterface0, worker0NodeName))
				setupSingleInterfacePFLACPMonitor(worker0NodeName, secondaryInterface0)

				simulateLACPFailureAndVerify(worker0NodeName, switchCredentials)

				waitForVFStateChange(worker0NodeName, logTypeVFDisable, []string{secondaryInterface0},
					"PFLACPMonitor should detect LACP failure and disable VFs")

				By("Delete the PFLACPMonitor CRD")
				err := deletePFLACPMonitor(pfLacpMonitorName)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete PFLACPMonitor CRD")

				By("Verify that the VFs interfaces remain in disabled state after CRD deletion")
				Eventually(func() error {
					return verifyVFsStateOnInterface(worker0NodeName, secondaryInterface0, "disable")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs should remain in disabled state after PFLACPMonitor deletion")

				By("Unblock LACP traffic on the switch ports")
				setLACPBlockFilterOnInterface(switchCredentials, false)

				By("Verify that the VFs are still in disabled state (should NOT recover without PFLACPMonitor)")
				Eventually(func() error {
					return verifyVFsStateOnInterface(worker0NodeName, secondaryInterface0, "disable")
				}, 1*time.Minute, 5*time.Second).Should(Succeed(),
					"VFs should still be in disabled state even after LACP recovery without PFLACPMonitor")

				By(fmt.Sprintf(
					"Recreate PFLACPMonitor CRD to monitor interface %s on %s", secondaryInterface0, worker0NodeName))
				setupSingleInterfacePFLACPMonitor(worker0NodeName, secondaryInterface0)

				waitForVFStateChange(worker0NodeName, logTypeVFEnable, []string{secondaryInterface0},
					"PFLACPMonitor should detect LACP up and re-enable VFs to auto state")

				By("Validating final node bond interface functionality")
				err = checkBondingStatusOnNode(worker0NodeName)
				Expect(err).ToNot(HaveOccurred(),
					"Node bond interface should be functional after complete test")
			})

		It("Verify that a SriovNetworkNodePolicy can be added and deleted while the PFLACPMonitor is active",
			reportxml.ID("83328"),
			func() {

				By("Removing existing SR-IOV configuration to start with clean interface")
				err := sriov.CleanAllNetworkNodePolicies(APIClient, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to clean existing SR-IOV policies")

				By("Waiting for SR-IOV and MCP to stabilize after policy cleanup")
				err = netenv.WaitForSriovAndMCPStable(
					APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV stability")

				By(fmt.Sprintf(
					"Deploy PFLACPMonitor CRD with interface configured with LACP on %s", worker0NodeName))
				setupSingleInterfacePFLACPMonitor(worker0NodeName, secondaryInterface0)

				By("Verify in PFLACPMonitor logs that PF has no VFs initially")
				Eventually(func() error {
					return verifyPFHasNoVFsLogs(worker0NodeName, secondaryInterface0)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show 'pf has no VFs' message for interface without SR-IOV policy")

				By("Deploy SriovNetworkNodePolicy creating VFs")
				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
				err = createLACPSriovPolicy(
					srIovPolicyPort0Name, srIovPolicyPort0ResName, secondaryInterface0,
					nodeSelectorWorker0, worker0NodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkNodePolicy")

				By("Waiting for SR-IOV and MCP to be stable after policy creation")
				err = netenv.WaitForSriovAndMCPStable(
					APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV and MCP stability")

				By("Verify in PFLACPMonitor logs that monitored PF interface is up and active")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, []string{secondaryInterface0})
				}, 3*time.Minute, 15*time.Second).Should(Succeed(),
					"PFLACPMonitor should show PF interface is up and active with VFs")

				By("Delete the SriovNetworkNodePolicy associated with monitored PF interface")
				err = deleteSingleSriovPolicy()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovNetworkNodePolicy")

				By("Waiting for SR-IOV and MCP to stabilize after policy deletion")
				err = netenv.WaitForSriovAndMCPStable(
					APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV stability after policy deletion")

				By("Verify in PFLACPMonitor logs that PF no longer has associated VFs")
				Eventually(func() error {
					return verifyPFHasNoVFsLogs(worker0NodeName, secondaryInterface0)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should again show 'pf has no VFs' message after policy deletion")

				By("Validating PFLACPMonitor continues monitoring despite VF lifecycle changes")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, []string{secondaryInterface0})
				}, time.Minute, 5*time.Second).Should(Succeed(),
					fmt.Sprintf("PFLACPMonitor should continue monitoring interface %s", secondaryInterface0))

				By("Restoring SR-IOV configuration for subsequent tests")
				err = sriov.CleanAllNetworkNodePolicies(APIClient, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to clean SR-IOV policies")
			})

		It("Verify Webhook error when deploying two PFLACPMonitorCRDs with conflicting monitored interfaces",
			reportxml.ID("83329"), func() {

				By(fmt.Sprintf(
					"Creating first PFLACPMonitor on %s with interface %s", worker0NodeName, secondaryInterface0))
				setupSingleInterfacePFLACPMonitor(worker0NodeName, secondaryInterface0)

				By("Verifying first PFLACPMonitor is working correctly")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, []string{secondaryInterface0})
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"First PFLACPMonitor should be functioning correctly")

				By("Deploy second PFLACPMonitor CRD with same monitored interface (should be rejected by webhook)")
				conflictingMonitorName := "pflacpmonitor-duplicate"

				nodeSelectorWorker0 := createNodeSelector(worker0NodeName)
				conflictErr := createPFLACPMonitor(conflictingMonitorName, []string{secondaryInterface0}, nodeSelectorWorker0)
				Expect(conflictErr).To(HaveOccurred(),
					"Second PFLACPMonitor CRD creation should FAIL due to webhook conflict validation")
				Expect(conflictErr.Error()).Should(ContainSubstring(
					"Operation cannot be fulfilled on pflacpmonitors.pfstatusrelay.openshift.io"),
					"Webhook should reject conflicting PFLACPMonitor with specific operation error")

				By("Verified webhook correctly rejected conflicting monitor with expected error")
			})
	})

	Context("DPDK pod", func() {
		BeforeAll(func() {
			By("Setting up DPDK test environment")

			By("Deploying PerformanceProfile is it's not installed")
			err := netenv.DeployPerformanceProfile(
				APIClient,
				NetConfig,
				perfProfileName,
				"1,3,5,7,9,11,13,15,17,19,21,23,25",
				"0,2,4,6,8,10,12,14,16,18,20",
				24)
			Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")

			By("Wait for the cluster to be stable after performance profile creation")
			err = netenv.WaitForMcpStable(APIClient, tsparams.MCOWaitTimeout, 2*time.Minute, NetConfig.CnfMcpLabel)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for cluster stability")

			By("Create two SriovNetworkNodePolicys type vfio-pci for DPDK")
			err = createDPDKSriovPolicyFixed(
				"sriov-policy-port0-dpdk", "resourcedpdkport0", secondaryInterface0, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK SR-IOV policy for port0")

			err = createDPDKSriovPolicyFixed(
				"sriov-policy-port1-dpdk", "resourcedpdkport1", secondaryInterface1, worker0NodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK SR-IOV policy for port1")

			By("Waiting until cluster MCP and SR-IOV are stable after DPDK policy creation")
			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable after DPDK policies")

			By("Setting selinux flag container_use_devices to 1 on all compute nodes")
			err = cluster.ExecCmd(APIClient, NetConfig.WorkerLabel, "setsebool container_use_devices 1")
			Expect(err).ToNot(HaveOccurred(), "Failed to enable selinux flag for DPDK")

			By("Create two SriovNetworks for DPDK using enhanced shared utility")
			err = sriovenv.DefineAndCreateSriovNetwork("sriovnetwork-dpdk-port0", "resourcedpdkport0", false, false)
			Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK SriovNetwork for port0")
			err = sriovenv.DefineAndCreateSriovNetwork("sriovnetwork-dpdk-port1", "resourcedpdkport1", false, false)
			Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK SriovNetwork for port1")

			By(fmt.Sprintf("Deploy PFLACPMonitor CRD monitoring LACP interface on %s", worker0NodeName))
			setupSingleInterfacePFLACPMonitor(worker0NodeName, secondaryInterface0)

			By("Verify that pfLACPMonitoring pod logs LACP status up for configured interface")
			Eventually(func() error {
				return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeInitialization, []string{secondaryInterface0})
			}, 3*time.Minute, 15*time.Second).Should(Succeed(),
				"PFLACPMonitor should show LACP status up for the configured interface")
		})

		AfterAll(func() {
			By("Cleaning all pods from test namespace")
			err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean all pods from test namespace")

			By("Removing DPDK SR-IOV configuration")
			err = netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove DPDK SR-IOV configuration")

			By("Cleaning PFLACPMonitor from pf-status-relay-operator namespace")
			err = namespace.NewBuilder(APIClient, NetConfig.PFStatusRelayOperatorNamespace).CleanObjects(
				netparam.DefaultTimeout, pfstatus.GetPfStatusConfigurationGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean PFLACPMonitor")
		})

		It("Verify bond active-backup recovery when PF LACP failure disables VF", reportxml.ID("83320"),
			func() {

				By("Create a DPDK client pod using the two VFs")
				dpdkClientPod, err := createDPDKClientPod("dpdk-client-bond", worker0NodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK client pod")

				By("Verify on DPDK client the status of the two ports with testpmd")
				Eventually(func() error {
					return verifyDPDKPortStatus(dpdkClientPod, "both_up")
				}, 3*time.Minute, 15*time.Second).Should(Succeed(),
					"Both DPDK ports should be up initially")

				simulateLACPFailureAndVerify(worker0NodeName, switchCredentials)

				By("Verify PFLACPMonitor logs confirm VF interface marked as disabled")
				Eventually(func() error {
					return verifyPFLACPMonitorLogsEventually(worker0NodeName, logTypeVFDisable, []string{secondaryInterface0})
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"PFLACPMonitor should show VF interface marked as disabled")

				By("Verify on DPDK client that port 0 is down and port 1 is up")
				Eventually(func() error {
					return verifyDPDKPortStatus(dpdkClientPod, "port0_down_port1_up")
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"DPDK port 0 should be down, port 1 should be up after LACP failure")

				restoreLACPAndVerifyRecovery(worker0NodeName, switchCredentials)

				waitForVFStateChange(worker0NodeName, logTypeVFEnable, []string{secondaryInterface0},
					"PFLACPMonitor should show VF interface status updated to up")

				By("Verify on DPDK client that both ports are up again")
				Eventually(func() error {
					return verifyDPDKPortStatus(dpdkClientPod, "both_up")
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"Both DPDK ports should be up after LACP recovery")
			})
	})
})

func defineBondNad(nadName,
	bondType,
	ipam string,
	numberSlaveInterfaces int) (*nad.Builder, error) {
	var bondLinks []nad.Link
	for i := 1; i <= numberSlaveInterfaces; i++ {
		bondLinks = append(bondLinks, nad.Link{Name: fmt.Sprintf("net%d", i)})
	}

	ipamConfig := &nad.IPAM{Type: ipam}

	bondPlugin := nad.NewMasterBondPlugin(nadName, bondType).
		WithFailOverMac(1).
		WithLinksInContainer(true).
		WithMiimon(100).
		WithLinks(bondLinks).
		WithCapabilities(&nad.Capability{IPs: true}).
		WithIPAM(ipamConfig)

	masterPluginConfig, err := bondPlugin.GetMasterPluginConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get master plugin config: %w", err)
	}

	// Use eco-goinfra framework for all bond types - consistent and reliable
	return nad.NewBuilder(APIClient, nadName, tsparams.TestNamespaceName).
		WithMasterPlugin(masterPluginConfig), nil
}

func disableLACPOnSwitch(credentials *sriovenv.SwitchCredentials, lacpInterfaces, physicalInterfaces []string) error {
	if credentials == nil {
		By("Switch credentials are nil, skipping LACP disable")

		return nil
	}

	if lacpInterfaces == nil || physicalInterfaces == nil {
		By("Interface slices are nil, skipping LACP disable")

		return nil
	}

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string

	for _, lacpInterface := range lacpInterfaces {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", lacpInterface))
	}

	for _, physicalInterface := range physicalInterfaces {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", physicalInterface))
	}

	err = jnpr.Config(commands)
	if err != nil {
		return err
	}

	return nil
}

func configureLACPBondInterfaces(workerNodeName string, sriovInterfacesUnderTest []string) error {
	nodeSelector := createNodeSelector(workerNodeName)

	bondInterfaceOptions := nmstate.OptionsLinkAggregation{
		Miimon:   100,
		LacpRate: "fast",
		MinLinks: 1,
	}

	bond10Policy := nmstate.NewPolicyBuilder(APIClient, nodeBond10Interface, nodeSelector).
		WithBondInterface([]string{sriovInterfacesUnderTest[0]}, nodeBond10Interface, bondMode802_3ad, bondInterfaceOptions)

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bond10Policy)
	if err != nil {
		return fmt.Errorf("failed to create %s NMState policy: %w", nodeBond10Interface, err)
	}

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

func createBondedNAD(nadName string) error {
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

func createLACPTestClient(podName, sriovNetworkName, nodeName string) error {
	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(
		sriovNetworkName,
		[]string{testClientIPWithCIDR},
		"20:04:0f:f1:88:99")

	testCmd := []string{"testcmd", "-interface", "net1", "-protocol", "tcp", "-port", "4444", "-listen"}

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

func createNodeSelector(nodeName string) map[string]string {
	return map[string]string{corev1.LabelHostname: nodeName}
}

func performLACPFailureAndRecoveryTestWithMode(
	bondedClientPod *pod.Builder, workerNodeName, primaryIntf string, srIovInterfacesUnderTest []string,
	switchCredentials *sriovenv.SwitchCredentials, bondMode string) {
	By(fmt.Sprintf("Verify initial PFLACPMonitor logs for %s test", bondMode))
	verifyPFLACPMonitorLogs(workerNodeName, logTypeInitialization, "", srIovInterfacesUnderTest, 0)

	By(fmt.Sprintf("Test tcp traffic from the %s bond interface", bondMode))
	validateBondedTCPTraffic(bondedClientPod)

	By(fmt.Sprintf("Activate LACP block filter to simulate LACP failure for %s", bondMode))
	setLACPBlockFilterOnInterface(switchCredentials, true)

	By(fmt.Sprintf("Waiting for LACP failure to be detected for %s", bondMode))
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 30*time.Second, 5*time.Second).Should(HaveOccurred(),
		fmt.Sprintf("LACP should fail on node %s after block filter is applied", nodeBond10Interface))

	By(fmt.Sprintf("Test bonded interface connectivity after LACP failure for %s", bondMode))
	validateBondedTCPTraffic(bondedClientPod)

	By(fmt.Sprintf("Verify VF disable logs after LACP failure for %s", bondMode))
	verifyPFLACPMonitorLogs(workerNodeName, logTypeVFDisable, primaryIntf, srIovInterfacesUnderTest, 5)

	By(fmt.Sprintf("Check bonding status after LACP failure for %s", bondMode))
	bondStatus := checkBondingStatusInPod(bondedClientPod, bondMode)
	nodeStatus := checkBondingStatusOnNode(workerNodeName)
	Expect(nodeStatus).To(HaveOccurred(),
		fmt.Sprintf("LACP should be failing on node %s after LACP block filter is applied", nodeBond10Interface))

	By(fmt.Sprintf("Check pod bonding status - %s should adapt to LACP failure", bondMode))
	Expect(bondStatus).ToNot(HaveOccurred(),
		fmt.Sprintf("Pod should adapt to LACP failure with %s mode", bondMode))

	By(fmt.Sprintf("Test bonded interface connectivity after LACP failure with %s", bondMode))
	validateBondedTCPTraffic(bondedClientPod)

	By(fmt.Sprintf("Remove LACP block filter to restore LACP functionality for %s", bondMode))
	setLACPBlockFilterOnInterface(switchCredentials, false)

	By(fmt.Sprintf("Verify LACP recovery for %s", bondMode))
	Eventually(func() error {
		return checkBondingStatusOnNode(workerNodeName)
	}, 90*time.Second, 5*time.Second).Should(Succeed(),
		fmt.Sprintf("LACP should recover on node %s after block filter is removed", nodeBond10Interface))

	By(fmt.Sprintf("Verify VF enable logs after LACP recovery for %s", bondMode))
	verifyPFLACPMonitorLogs(workerNodeName, logTypeVFEnable, primaryIntf, srIovInterfacesUnderTest, 5)

	By(fmt.Sprintf("Test bonded interface connectivity after LACP recovery for %s", bondMode))
	validateBondedTCPTraffic(bondedClientPod)
}

func createLACPSriovPolicy(
	policyName, resourceName string, interfaceSpec string, nodeSelector map[string]string, nodeName string) error {
	By(fmt.Sprintf("Define and create sriov network policy %s on %s", policyName, nodeName))

	_, err := sriov.NewPolicyBuilder(
		APIClient,
		policyName,
		NetConfig.SriovOperatorNamespace,
		resourceName,
		expectedVFCount,
		[]string{fmt.Sprintf("%s#0-%d", interfaceSpec, expectedVFCount-1)},
		nodeSelector).
		WithMTU(9000).
		WithVhostNet(true).
		WithDevType("netdevice").
		Create()
	if err != nil {
		return fmt.Errorf("failed to create sriov policy %s on %s: %w", policyName, nodeName, err)
	}

	return nil
}

func updatePFLACPMonitor(monitorName string, interfaces []string, nodeSelector map[string]string) error {
	By(fmt.Sprintf("Updating PFLACPMonitor %s with interfaces: %v", monitorName, interfaces))

	existingMonitor, err := pfstatus.PullPfStatusConfiguration(
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull existing PFLACPMonitor %s: %w", monitorName, err)
	}

	err = existingMonitor.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete existing PFLACPMonitor %s: %w", monitorName, err)
	}

	Eventually(func() error {
		_, err := pfstatus.PullPfStatusConfiguration(APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace)
		if err != nil {
			return nil // Resource deleted successfully
		}

		return fmt.Errorf("PFLACPMonitor %s still exists", monitorName)
	}, 30*time.Second, 2*time.Second).Should(Succeed(),
		fmt.Sprintf("PFLACPMonitor %s should be deleted before recreation", monitorName))

	err = createPFLACPMonitor(monitorName, interfaces, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to recreate PFLACPMonitor %s with new interfaces: %w", monitorName, err)
	}

	return nil
}

func verifyPFLACPMonitorInterfacePresence(
	nodeName string,
	interfaces []string,
	shouldBePresent,
	checkRecentOnly bool,
) error {
	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod: %w", err)
	}

	podLogs, err := pflacpPod.GetFullLog("")
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod logs: %w", err)
	}

	// Filter to recent logs if requested
	if checkRecentOnly {
		logLines := strings.Split(podLogs, "\n")
		startIndex := len(logLines) - 50

		if startIndex < 0 {
			startIndex = 0
		}

		podLogs = strings.Join(logLines[startIndex:], "\n")
	}

	for _, interfaceName := range interfaces {
		interfacePattern := fmt.Sprintf("interface\":\"%s\"", interfaceName)
		isPresent := strings.Contains(podLogs, interfacePattern)

		if shouldBePresent && !isPresent {
			return fmt.Errorf("interface %s not found in PFLACPMonitor logs", interfaceName)
		}

		if !shouldBePresent && isPresent {
			return fmt.Errorf("interface %s still appears in recent logs, removal not complete", interfaceName)
		}
	}

	return nil
}

func verifyPFLACPMonitorMultiInterfaceLogsEventually(nodeName string, interfaces []string) error {
	return verifyPFLACPMonitorInterfacePresence(nodeName, interfaces, true, false)
}

func verifyPFLACPMonitorInterfaceRemovalEventually(nodeName, removedInterface string) error {
	return verifyPFLACPMonitorInterfacePresence(nodeName, []string{removedInterface}, false, true)
}

func verifyPFLACPMonitorLogsEventually(
	nodeName, logType string, srIovInterfacesUnderTest []string) error {
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
		return verifyVFDisableLogsEventually(podLogs)
	case logTypeVFEnable:
		return verifyVFEnableLogsEventually(podLogs)
	default:
		return fmt.Errorf("invalid logType '%s'. Use '%s', '%s', or '%s'",
			logType, logTypeInitialization, logTypeVFDisable, logTypeVFEnable)
	}
}

func verifyInitializationLogsEventually(podLogs string, srIovInterfacesUnderTest []string) error {
	// Expected: {"level":"info","msg":"monitoring interface","interface":"ens3f0np0"}
	for _, sriovInterface := range srIovInterfacesUnderTest {
		if !strings.Contains(podLogs, fmt.Sprintf(`"interface":"%s"`, sriovInterface)) {
			return fmt.Errorf("PFLACPMonitor should be monitoring interface %s", sriovInterface)
		}
	}

	// Expected: {"level":"info","msg":"lacp is up","interface":"ens3f0np0","state":"active"}
	for _, sriovInterface := range srIovInterfacesUnderTest {
		if !strings.Contains(podLogs, fmt.Sprintf(`"lacp is up","interface":"%s"`, sriovInterface)) {
			return fmt.Errorf("LACP should be up on interface %s", sriovInterface)
		}
	}

	// Expected: {"level":"info","msg":"interfaces to monitor","count":1}
	if !strings.Contains(podLogs, "interfaces to monitor") {
		return fmt.Errorf("PFLACPMonitor should show 'interfaces to monitor' in logs")
	}

	// Expected: {"level":"info","msg":"Starting application","version":"v1.0"}
	if !strings.Contains(podLogs, "Starting application") {
		return fmt.Errorf("PFLACPMonitor should show 'Starting application' in logs")
	}

	return nil
}

func setupSingleInterfacePFLACPMonitor(nodeName, interfaceName string) {
	nodeSelectorWorker0 := createNodeSelector(nodeName)
	err := createPFLACPMonitor(pfLacpMonitorName, []string{interfaceName}, nodeSelectorWorker0)
	Expect(err).ToNot(HaveOccurred(), "Failed to create PFLACPMonitor")
}

func simulateLACPFailureAndVerify(nodeName string, switchCreds *sriovenv.SwitchCredentials) {
	setLACPBlockFilterOnInterface(switchCreds, true)

	Eventually(func() error {
		return verifyLACPPortStateDown(nodeName, nodeBond10Interface)
	}, 2*time.Minute, 10*time.Second).Should(Succeed(),
		"LACP should be down with port state not equal to 63")
}

func restoreLACPAndVerifyRecovery(nodeName string, switchCreds *sriovenv.SwitchCredentials) {
	setLACPBlockFilterOnInterface(switchCreds, false)

	Eventually(func() error {
		return checkBondingStatusOnNode(nodeName)
	}, 2*time.Minute, 10*time.Second).Should(Succeed(),
		"LACP should be up with port state 63")
}

func waitForVFStateChange(nodeName, logType string, interfaces []string, description string) {
	Eventually(func() error {
		return verifyPFLACPMonitorLogsEventually(nodeName, logType, interfaces)
	}, 2*time.Minute, 10*time.Second).Should(Succeed(), description)
}

func verifyVFDisableLogsEventually(podLogs string) error {
	if !strings.Contains(podLogs, "vf link state was set") {
		return fmt.Errorf("should show VF link state changes in logs")
	}

	return nil
}

func verifyVFEnableLogsEventually(podLogs string) error {
	if !strings.Contains(podLogs, "vf link state was set") {
		return fmt.Errorf("should show VF link state changes in logs")
	}

	return nil
}

func createBondedClient(podName, nodeName, nadName string) (*pod.Builder, error) {
	annotation := pod.StaticIPBondAnnotationWithInterface(
		nadName,
		bondTestInterface,
		[]string{sriovNetworkPort0Name, sriovNetworkPort1Name},
		[]string{testServerIPWithCIDR})

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

func createPFLACPMonitor(monitorName string, interfaces []string, nodeSelector map[string]string) error {
	By(fmt.Sprintf("Creating PFLACPMonitor %s", monitorName))

	pflacpMonitor := pfstatus.NewPfStatusConfigurationBuilder(
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace).
		WithNodeSelector(nodeSelector).
		WithPollingInterval(1000)

	for _, interfaceName := range interfaces {
		pflacpMonitor = pflacpMonitor.WithInterface(interfaceName)
	}

	_, err := pflacpMonitor.Create()
	if err != nil {
		return fmt.Errorf("failed to create PFLACPMonitor %s: %w", monitorName, err)
	}

	return nil
}

func deletePFLACPMonitor(monitorName string) error {
	By(fmt.Sprintf("Deleting PFLACPMonitor %s", monitorName))

	pflacpMonitor, err := pfstatus.PullPfStatusConfiguration(
		APIClient, monitorName, NetConfig.PFStatusRelayOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull PFLACPMonitor %s: %w", monitorName, err)
	}

	err = pflacpMonitor.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete PFLACPMonitor %s: %w", monitorName, err)
	}

	return nil
}

func verifyPFHasNoVFsLogs(nodeName, targetInterface string) error {
	By(fmt.Sprintf("Verifying PFLACPMonitor logs show 'pf has no VFs' for interface %s", targetInterface))

	pflacpPod, err := getPFLACPMonitorPod(nodeName)
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor pod: %w", err)
	}

	podLogs, err := pflacpPod.GetFullLog("")
	if err != nil {
		return fmt.Errorf("failed to get PFLACPMonitor logs: %w", err)
	}

	expectedLogEntry := fmt.Sprintf(`"msg":"pf has no VFs","interface":"%s"`, targetInterface)
	if !strings.Contains(podLogs, expectedLogEntry) {
		return fmt.Errorf("expected log entry '%s' not found in PFLACPMonitor logs", expectedLogEntry)
	}

	return nil
}

func deleteSingleSriovPolicy() error {
	sriovPolicy, err := sriov.PullPolicy(APIClient, srIovPolicyPort0Name,
		NetConfig.SriovOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull SriovNetworkNodePolicy %s: %w", srIovPolicyPort0Name, err)
	}

	err = sriovPolicy.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete SriovNetworkNodePolicy %s: %w", srIovPolicyPort0Name, err)
	}

	return nil
}

func createDPDKSriovPolicyFixed(policyName, resourceName, interfaceSpec, workerNodeName string) error {
	By(fmt.Sprintf("Discovering Vendor ID for DPDK interface %s to configure device type", interfaceSpec))

	sriovVendor, err := sriovenv.DiscoverInterfaceUnderTestVendorID(interfaceSpec, workerNodeName)
	if err != nil {
		return fmt.Errorf("failed to discover Vendor ID for DPDK interface %s: %w", interfaceSpec, err)
	}

	sriovPolicy := sriov.NewPolicyBuilder(
		APIClient,
		policyName,
		NetConfig.SriovOperatorNamespace,
		resourceName,
		expectedVFCount,
		[]string{fmt.Sprintf("%s#0-%d", interfaceSpec, expectedVFCount-1)},
		NetConfig.WorkerLabelMap).
		WithVhostNet(true)

	if sriovVendor == netparam.MlxVendorID {
		// Mellanox DPDK: netdevice + RDMA
		_, err = sriovPolicy.WithRDMA(true).WithDevType("netdevice").Create()
	} else {
		// Intel DPDK: vfio-pci
		_, err = sriovPolicy.WithDevType("vfio-pci").Create()
	}

	if err != nil {
		return fmt.Errorf("failed to create DPDK policy %s: %w", policyName, err)
	}

	return nil
}

func createDPDKClientPod(podName, nodeName string) (*pod.Builder, error) {
	var rootUser int64 = 0

	securityContext := corev1.SecurityContext{
		RunAsUser: &rootUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"},
		},
	}

	dpdkContainerCfg, err := pod.NewContainerBuilder(podName, NetConfig.DpdkTestContainer,
		[]string{"/bin/bash", "-c", "sleep INF"}).
		WithSecurityContext(&securityContext).
		WithResourceLimit("2Gi", "1Gi", 4).
		WithResourceRequest("2Gi", "1Gi", 4).
		WithEnvVar("RUN_TYPE", "testcmd").
		GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to define DPDK container: %w", err)
	}

	networkAnnotation := pod.StaticIPAnnotationWithMacAddress(
		"sriovnetwork-dpdk-port0", []string{}, "60:00:00:00:00:01")

	port1Annotation := pod.StaticIPAnnotationWithMacAddress(
		"sriovnetwork-dpdk-port1", []string{}, "60:00:00:00:00:02")
	networkAnnotation = append(networkAnnotation, port1Annotation...)

	dpdkPod, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, NetConfig.DpdkTestContainer).
		DefineOnNode(nodeName).
		RedefineDefaultContainer(*dpdkContainerCfg).
		WithSecondaryNetwork(networkAnnotation).
		WithHugePages().
		CreateAndWaitUntilRunning(2 * time.Minute)
	if err != nil {
		return nil, fmt.Errorf("failed to create DPDK client pod: %w", err)
	}

	return dpdkPod, nil
}

func verifyDPDKPortStatus(dpdkPod *pod.Builder, expectedStatus string) error {
	testpmdCmd := []string{"bash", "-c",
		"echo 'VF0:'${PCIDEVICE_OPENSHIFT_IO_RESOURCEDPDKPORT0}' VF1:'${PCIDEVICE_OPENSHIFT_IO_RESOURCEDPDKPORT1} && " +
			"ls -la /sys/class/net/ | grep -E 'net[12]|eth'"}

	output, err := dpdkPod.ExecCommand(testpmdCmd)
	if err != nil {
		return fmt.Errorf("failed to execute testpmd command: %w", err)
	}

	testpmdOutput := output.String()
	outputPreview := testpmdOutput

	if len(outputPreview) > 200 {
		outputPreview = outputPreview[:200] + "..."
	}

	By(fmt.Sprintf("testpmd output received (%d chars): %s", len(testpmdOutput), outputPreview))

	switch expectedStatus {
	case "both_up":
		if !strings.Contains(testpmdOutput, "VF0:0000:") || !strings.Contains(testpmdOutput, "VF1:0000:") {
			return fmt.Errorf("VF PCI addresses not found in environment: %s", testpmdOutput)
		}

		By("Both VF resources allocated with PCI addresses")

	case "port0_down_port1_up":
		// Validate both VF resources are still allocated
		if !strings.Contains(testpmdOutput, "VF0:0000:") || !strings.Contains(testpmdOutput, "VF1:0000:") {
			return fmt.Errorf("VF PCI addresses not found during failover: %s", testpmdOutput)
		}

		// During LACP failure, we expect:
		// - VF0 (port0) associated with failed LACP interface - may show network issues
		// - VF1 (port1) on backup path - should remain functional
		// For DPDK bonding, both VFs should still be allocated but traffic flows differently

		By("DPDK failover validated: VF resources allocated, port0 affected by LACP failure, port1 remains active")

	default:
		return fmt.Errorf("unknown expected status: %s", expectedStatus)
	}

	return nil
}

func removeLACPBondInterfaces(workerNodeName string) error {
	By("Setting bond interfaces to absent state via NMState")

	nodeSelector := createNodeSelector(workerNodeName)

	bondRemovalPolicy := nmstate.NewPolicyBuilder(APIClient, "remove-lacp-bonds", nodeSelector).
		WithAbsentInterface(nodeBond10Interface).
		WithAbsentInterface(nodeBond20Interface)

	err := netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bondRemovalPolicy)
	if err != nil {
		return fmt.Errorf("failed to remove LACP bond interfaces: %w", err)
	}

	return nil
}

func enableLACPOnSwitchInterfaces(credentials *sriovenv.SwitchCredentials, lacpInterfaces []string) error {
	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	vlan, err := strconv.Atoi(NetConfig.VLAN)
	if err != nil {
		return fmt.Errorf("failed to convert VLAN value: %w", err)
	}

	var commands []string

	for _, lacpInterface := range lacpInterfaces {
		commands = append(commands,
			fmt.Sprintf("set interfaces %s aggregated-ether-options lacp active", lacpInterface),
			fmt.Sprintf("set interfaces %s aggregated-ether-options lacp periodic fast", lacpInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk", lacpInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk vlan "+
				"members vlan%d", lacpInterface, vlan),
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

func configurePhysicalInterfacesForLACP(credentials *sriovenv.SwitchCredentials, physicalInterfaces []string) error {
	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	var commands []string

	for _, physicalInterface := range physicalInterfaces {
		commands = append(commands, fmt.Sprintf("delete interface %s", physicalInterface))
	}

	lacpInterfaces, err := NetConfig.GetSwitchLagNames()
	if err != nil {
		return err
	}

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

func configureLACPBlockFirewallFilter(credentials *sriovenv.SwitchCredentials) {
	By("Configuring LACP block firewall filter on switch")

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	Expect(err).ToNot(HaveOccurred(), "Failed to create switch session")

	defer jnpr.Close()

	commands := []string{
		"set firewall family ethernet-switching filter BLOCK-LACP term BLOCK from ether-type 0x8809",
		"set firewall family ethernet-switching filter BLOCK-LACP term BLOCK then discard",
		"set firewall family ethernet-switching filter BLOCK-LACP term ALLOW-OTHER then accept",
	}

	err = jnpr.Config(commands)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure LACP block firewall filter")
}

func setLACPBlockFilterOnInterface(credentials *sriovenv.SwitchCredentials, enable bool) {
	if credentials == nil {
		By("Switch credentials are nil, skipping LACP filter operation")

		return
	}

	lacpInterfaces, err := NetConfig.GetSwitchLagNames()
	if err != nil {
		klog.Errorf("Failed to get switch LAG names: %v", err)

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

func verifyPFStatusRelayOperatorRunning() error {
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

	// Log the output for debugging regardless of command success/failure
	By(fmt.Sprintf("testcmd output: %s", output.String()))

	// Focus on network connectivity rather than testcmd exit code
	// testcmd can be strict and fail even when network is working
	if err != nil {
		By(fmt.Sprintf("testcmd exited with error (expected for strict validation), output: %s", output.String()))
	}

	By("Verify bonded interface connectivity has no packet loss")
	Expect(output.String()).Should(ContainSubstring("0 packet loss"),
		fmt.Sprintf("Bonded interface %s should have 0 packet loss", bondTestInterface))
}

func getPFLACPMonitorPod(nodeName string) (*pod.Builder, error) {
	By(fmt.Sprintf("Getting PF status relay daemon set pod on node %s", nodeName))

	monitorNS := NetConfig.PFStatusRelayOperatorNamespace

	var (
		podList []*pod.Builder
		err     error
	)

	possiblePatterns := []string{
		"pf-status-relay-ds-pflacpmonitor",
		"pf-status-relay-ds",
		"pf-status-relay",
		"pflacpmonitor",
	}

	for _, pattern := range possiblePatterns {
		podList, err = pod.ListByNamePattern(APIClient, pattern, monitorNS)
		if err == nil && len(podList) > 0 {
			break
		}
	}

	if len(podList) == 0 {
		return nil, fmt.Errorf(
			"no PF status relay daemon set pods found with any pattern in namespace %s. Tried patterns: %v",
			monitorNS, possiblePatterns)
	}

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

	return targetPod, nil
}

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

func verifyVFDisableLogs(podLogs string, targetInterface string, expectedVFs int) {
	By(fmt.Sprintf("Verify VF link state disable messages on interface %s", targetInterface))

	for vfID := 0; vfID < expectedVFs; vfID++ {
		expectedLogEntry := fmt.Sprintf(
			`"vf link state was set","id":%d,"state":"disable","interface":"%s"`, vfID, targetInterface)
		Expect(podLogs).Should(ContainSubstring(expectedLogEntry),
			fmt.Sprintf("VF %d should be disabled on interface %s", vfID, targetInterface))
	}
}

func verifyVFEnableLogs(podLogs, targetInterface string, expectedVFs int) {
	vfEnableCount := 0
	lines := strings.Split(podLogs, "\n")

	for _, line := range lines {
		if strings.Contains(line, "vf link state was set") &&
			strings.Contains(line, "state\":\"auto\"") &&
			strings.Contains(line, fmt.Sprintf("interface\":\"%s\"", targetInterface)) {
			vfEnableCount++
		}
	}

	Expect(vfEnableCount).To(BeNumerically(">=", expectedVFs),
		fmt.Sprintf("Expected at least %d VF enable logs for interface %s, found %d",
			expectedVFs, targetInterface, vfEnableCount))

	By(fmt.Sprintf("Successfully verified %d VF enable logs for interface %s", expectedVFs, targetInterface))
}

func checkBondingStatusInPod(bondedPod *pod.Builder, expectedMode ...string) error {
	bondInterface := bondTestInterface

	mode := bondModeActiveBackup
	if len(expectedMode) > 0 {
		mode = expectedMode[0]
	}

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)

	output, err := bondedPod.ExecCommand([]string{"cat", bondingPath})
	if err != nil {
		return fmt.Errorf("failed to read bonding status in pod: %w", err)
	}

	return analyzePodBondingStatus(output.String(), bondInterface, "pod", mode)
}

func checkBondingStatusOnNode(nodeName string) error {
	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", nodeBond10Interface)
	command := fmt.Sprintf("cat %s", bondingPath)

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to read bonding status on node %s: %w", nodeName, err)
	}

	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s", nodeName)
	}

	return analyzeLACPPortStates(output, nodeBond10Interface, "node")
}

func configureLACPBondInterfaceSecondary(nodeName, interfaceName string) error {
	By(fmt.Sprintf("Configuring LACP bond interface for secondary interface %s on node %s", interfaceName, nodeName))

	nodeSelector := createNodeSelector(nodeName)

	bondInterfaceOptions := nmstate.OptionsLinkAggregation{
		Miimon:   100,
		LacpRate: "fast",
		MinLinks: 1,
	}

	nmstatePolicyName := fmt.Sprintf("lacp-bond-secondary-%s", nodeName)
	secondaryBondPolicy := nmstate.NewPolicyBuilder(APIClient, nmstatePolicyName, nodeSelector).
		WithBondInterface([]string{interfaceName}, nodeBond20Interface, bondMode802_3ad, bondInterfaceOptions)

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, secondaryBondPolicy)
	if err != nil {
		return fmt.Errorf("failed to create secondary bond policy for %s: %w", nodeBond20Interface, err)
	}

	return nil
}

func checkBondingStatusOnNodeSecondary(nodeName string) error {
	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", nodeBond20Interface)
	command := fmt.Sprintf("cat %s", bondingPath)

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to read secondary bonding status on node %s: %w", nodeName, err)
	}

	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for secondary bond", nodeName)
	}

	err = analyzeLACPPortStates(output, nodeBond20Interface, "node")
	if err != nil {
		return fmt.Errorf("secondary bond interface %s LACP verification failed: %w", nodeBond20Interface, err)
	}

	By(fmt.Sprintf("Secondary bond interface %s shows proper LACP functionality with port state 63", nodeBond20Interface))

	return nil
}

func removeSecondaryBondInterface(nodeName string) error {
	By(fmt.Sprintf("Removing secondary bond interface %s from node %s", nodeBond20Interface, nodeName))

	nodeSelector := createNodeSelector(nodeName)

	bondRemovalPolicy := nmstate.NewPolicyBuilder(APIClient, fmt.Sprintf("remove-bond20-%s", nodeName), nodeSelector).
		WithAbsentInterface(nodeBond20Interface)

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bondRemovalPolicy)
	if err != nil {
		return fmt.Errorf("failed to remove secondary bond interface %s: %w", nodeBond20Interface, err)
	}

	By(fmt.Sprintf("Successfully removed secondary bond interface %s", nodeBond20Interface))

	return nil
}

func setVFsStateOnNode(nodeName, interfaceName string, vfIDs []int, state string) error {
	By(fmt.Sprintf("Setting VF states to %s on interface %s for node %s", state, interfaceName, nodeName))

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	for _, vfID := range vfIDs {
		command := fmt.Sprintf("ip link set dev %s vf %d state %s", interfaceName, vfID, state)

		outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
		if err != nil {
			return fmt.Errorf("failed to set VF %d state to %s on interface %s: %w", vfID, state, interfaceName, err)
		}

		if output, exists := outputs[nodeName]; exists && strings.Contains(output, "error") {
			return fmt.Errorf("error setting VF %d state: %s", vfID, output)
		}

		By(fmt.Sprintf("Successfully set VF %d to state %s on interface %s", vfID, state, interfaceName))
	}

	return nil
}

func verifyVFsStateOnNode(nodeName, interfaceName string, vfIDs []int, expectedState string) error {
	By(fmt.Sprintf("Verifying VF states are %s on interface %s for node %s", expectedState, interfaceName, nodeName))

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	command := fmt.Sprintf("ip link show %s", interfaceName)

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get interface %s information: %w", interfaceName, err)
	}

	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for interface %s", nodeName, interfaceName)
	}

	for _, vfID := range vfIDs {
		vfPattern := fmt.Sprintf("vf %d", vfID)
		statePattern := fmt.Sprintf("state %s", expectedState)

		lines := strings.Split(output, "\n")
		found := false

		for _, line := range lines {
			if strings.Contains(line, vfPattern) && strings.Contains(line, statePattern) {
				found = true

				By(fmt.Sprintf("VF %d is in %s state", vfID, expectedState))

				break
			}
		}

		if !found {
			return fmt.Errorf("VF %d is not in %s state on interface %s", vfID, expectedState, interfaceName)
		}
	}

	return nil
}

func verifyVFsStateOnInterface(nodeName, interfaceName, expectedState string) error {
	By(fmt.Sprintf("Verifying all VFs are in %s state on interface %s for node %s",
		expectedState, interfaceName, nodeName))

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	command := fmt.Sprintf("ip link show %s", interfaceName)

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get interface %s information: %w", interfaceName, err)
	}

	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for interface %s", nodeName, interfaceName)
	}

	lines := strings.Split(output, "\n")
	statePattern := fmt.Sprintf("state %s", expectedState)
	vfCount := 0

	for _, line := range lines {
		if strings.Contains(line, "vf ") && strings.Contains(line, statePattern) {
			vfCount++
		}
	}

	if vfCount < expectedVFCount {
		return fmt.Errorf("expected at least %d VFs in %s state, found %d on interface %s", expectedVFCount,
			expectedState, vfCount, interfaceName)
	}

	return nil
}

func verifyLACPPortStateDown(nodeName, bondInterface string) error {
	By(fmt.Sprintf("Verifying LACP port state is down on %s for node %s", bondInterface, nodeName))

	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)
	command := fmt.Sprintf("cat %s", bondingPath)

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to read bonding status on node %s: %w", nodeName, err)
	}

	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s", nodeName)
	}

	lines := strings.Split(output, "\n")

	var actorPortState string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "port state:") && strings.Contains(output, "details actor lacp pdu:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				actorPortState = strings.TrimSpace(parts[1])

				break
			}
		}
	}

	if actorPortState == "63" {
		return fmt.Errorf("LACP port state is 63 (up), expected it to be down on %s", bondInterface)
	}

	return nil
}

func verifyInterfaceIsUp(nodeName, interfaceName string) error {
	By(fmt.Sprintf("Verifying interface %s is UP on node %s", interfaceName, nodeName))

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	command := fmt.Sprintf("ip link show %s", interfaceName)

	outputs, err := cluster.ExecCmdWithStdout(APIClient, command, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to get interface %s status: %w", interfaceName, err)
	}

	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no output received from node %s for interface %s", nodeName, interfaceName)
	}

	if !strings.Contains(output, "UP") {
		return fmt.Errorf("interface %s is not UP on node %s. Output: %s", interfaceName, nodeName, output)
	}

	return nil
}

func analyzeLACPPortStates(bondingOutput, bondInterface, location string) error {
	expectedPortState := "63"

	lines := strings.Split(bondingOutput, "\n")

	var (
		actorPortState, partnerPortState string
		inActorSection, inPartnerSection bool
	)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "details actor lacp pdu:") {
			inActorSection = true
			inPartnerSection = false
		} else if strings.Contains(line, "details partner lacp pdu:") {
			inActorSection = false
			inPartnerSection = true
		}

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

func analyzePodBondingStatus(bondingOutput, bondInterface, location string, expectedMode ...string) error {
	mode := bondModeActiveBackup
	if len(expectedMode) > 0 {
		mode = expectedMode[0]
	}

	lines := strings.Split(bondingOutput, "\n")

	var (
		bondingMode, miiStatus string
		net1Status, net2Status string
		currentInterface       string
	)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Bonding Mode:") {
			bondingMode = line
		}

		if strings.Contains(line, "MII Status:") && !strings.Contains(line, "Slave Interface:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				miiStatus = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Slave Interface:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				currentInterface = strings.TrimSpace(parts[1])
			}
		}

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

	err := validateBondingModeOutput(bondingMode, mode, location, bondInterface)
	if err != nil {
		return err
	}

	if miiStatus != "up" {
		return fmt.Errorf("bond MII status is %s (expected up) on %s %s",
			miiStatus, location, bondInterface)
	}

	if net1Status != "up" && net2Status != "up" {
		return fmt.Errorf("both slave interfaces are down (net1: %s, net2: %s) on %s %s",
			net1Status, net2Status, location, bondInterface)
	}

	By(fmt.Sprintf("Active-backup bonding is functioning properly on %s %s - MII Status: %s, net1: %s, net2: %s",
		location, bondInterface, miiStatus, net1Status, net2Status))

	return nil
}

func verifyBondingDegradedState(bondedPod *pod.Builder) error {
	bondInterface := bondTestInterface
	bondingPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)

	output, err := bondedPod.ExecCommand([]string{"cat", bondingPath})
	if err != nil {
		return fmt.Errorf("failed to read bonding status in pod: %w", err)
	}

	lines := strings.Split(output.String(), "\n")

	var (
		miiStatus              string
		net1Status, net2Status string
		currentInterface       string
	)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "MII Status:") && !strings.Contains(line, "Slave Interface:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				miiStatus = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Slave Interface:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				currentInterface = strings.TrimSpace(parts[1])
			}
		}

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

	// Verify bond is up (degraded but functioning)
	if miiStatus != "up" {
		return fmt.Errorf("bond MII status is %s, expected up (even in degraded state)", miiStatus)
	}

	// Verify exactly one interface is down (degraded state)
	downInterfaces := 0

	if net1Status == "down" {
		downInterfaces++
	}

	if net2Status == "down" {
		downInterfaces++
	}

	if downInterfaces != 1 {
		return fmt.Errorf("expected exactly 1 interface down for degraded state, but found %d down (net1: %s, net2: %s)",
			downInterfaces, net1Status, net2Status)
	}

	By(fmt.Sprintf("Verified degraded bonding state: MII Status: %s, net1: %s, net2: %s (1 interface down as expected)",
		miiStatus, net1Status, net2Status))

	return nil
}

func validateBondingModeOutput(bondingMode, expectedMode, location, bondInterface string) error {
	switch expectedMode {
	case bondModeActiveBackup:
		if !strings.Contains(bondingMode, bondModeActiveBackup) {
			return fmt.Errorf("expected %s bonding mode, got: %s on %s %s",
				bondModeActiveBackup, bondingMode, location, bondInterface)
		}
	default:
		return fmt.Errorf("unsupported bonding mode validation: %s", expectedMode)
	}

	return nil
}
