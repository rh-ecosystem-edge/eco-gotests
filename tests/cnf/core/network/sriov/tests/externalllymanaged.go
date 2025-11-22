package tests

import (
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/webhook"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netnmstate"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
)

const sriovAndResourceNameExManagedTrue = "extmanaged"

var _ = Describe("ExternallyManaged", Ordered, Label(tsparams.LabelExternallyManagedTestCases),
	ContinueOnFailure, func() {
		Context("General", Label("generalexcreated"), func() {
			const configureNMStatePolicyName = "configurevfs"
			var (
				sriovInterfacesUnderTest []string
				workerNodeList           []*nodes.Builder
			)
			BeforeAll(func() {
				By("Verifying if SR-IOV tests can be executed on given cluster")
				err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 2)
				if err != nil {
					Skip(fmt.Sprintf(
						"given cluster is not suitable for SR-IOV tests because it doesn't have enought nodes: %s", err.Error()))
				}

				By("Creating a new instance of NMstate instance")
				err = netnmstate.CreateNewNMStateAndWaitUntilItsRunning(7 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NMState instance")

				By("Validating SR-IOV interfaces")
				workerNodeList, err = nodes.List(APIClient,
					metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
				Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

				Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces")
				sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
				Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

				if sriovenv.IsMellanoxDevice(sriovInterfacesUnderTest[0], workerNodeList[0].Object.Name) {
					err = sriovenv.ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP(workerNodeList, sriovInterfacesUnderTest[0], true, 5)
					Expect(err).ToNot(HaveOccurred(), "Failed to configure Mellanox firmware")
				}

				By("Creating SR-IOV VFs via NMState")
				err = netnmstate.ConfigureVFsAndWaitUntilItsConfigured(
					configureNMStatePolicyName,
					sriovInterfacesUnderTest[0],
					NetConfig.WorkerLabelMap,
					5,
					netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VFs via NMState")

				err = sriovenv.WaitUntilVfsCreated(workerNodeList, sriovInterfacesUnderTest[0], 5, netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Expected number of VFs are not created")

				By("Configure SR-IOV with flag ExternallyManaged true")
				createSriovConfiguration(sriovAndResourceNameExManagedTrue, sriovInterfacesUnderTest[0], true)
			})

			AfterAll(func() {
				By("Removing SR-IOV configuration")
				err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

				By("Verifying that VFs still exist")
				err = sriovenv.WaitUntilVfsCreated(workerNodeList, sriovInterfacesUnderTest[0], 5, netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Unexpected amount of VF")

				err = netnmstate.AreVFsCreated(workerNodeList[0].Object.Name, sriovInterfacesUnderTest[0], 5)
				Expect(err).ToNot(HaveOccurred(), "VFs were removed during the test")

				By("Removing SR-IOV VFs via NMState")
				nmstatePolicy := nmstate.NewPolicyBuilder(
					APIClient, configureNMStatePolicyName, NetConfig.WorkerLabelMap).
					WithInterfaceAndVFs(sriovInterfacesUnderTest[0], 0)
				err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nmstatePolicy)
				Expect(err).ToNot(HaveOccurred(), "Failed to update NMState network policy")

				By("Verifying that VFs removed")
				err = sriovenv.WaitUntilVfsCreated(workerNodeList, sriovInterfacesUnderTest[0], 0, netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Unexpected amount of VF")

				By("Removing NMState policies")
				err = nmstate.CleanAllNMStatePolicies(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")
			})

			AfterEach(func() {
				By("Cleaning test namespace")
				err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())

				Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
			})

			DescribeTable("Verifying connectivity with different IP protocols", reportxml.ID("63527"),
				func(ipStack string) {
					By("Defining test parameters")
					clientIPs, serverIPs, err := defineIterationParams(ipStack)
					Expect(err).ToNot(HaveOccurred(), "Failed to define test parameters")

					By("Creating test pods and checking connectivity")
					err = sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[1].Object.Name,
						sriovAndResourceNameExManagedTrue, sriovAndResourceNameExManagedTrue, "", "",
						clientIPs, serverIPs)
					Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
				},

				Entry("", netparam.IPV4Family, reportxml.SetProperty("IPStack", netparam.IPV4Family)),
				Entry("", netparam.IPV6Family, reportxml.SetProperty("IPStack", netparam.IPV6Family)),
				Entry("", netparam.DualIPFamily, reportxml.SetProperty("IPStack", netparam.DualIPFamily)),
			)

			It("Recreate VFs when SR-IOV policy is applied", reportxml.ID("63533"), func() {
				By("Creating test pods and checking connectivity")
				err := sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovAndResourceNameExManagedTrue, sriovAndResourceNameExManagedTrue,
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")

				By("Removing created SR-IOV VFs via NMState")
				nmstatePolicy := nmstate.NewPolicyBuilder(
					APIClient, configureNMStatePolicyName, NetConfig.WorkerLabelMap).
					WithInterfaceAndVFs(sriovInterfacesUnderTest[0], 0)
				err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nmstatePolicy)
				Expect(err).ToNot(HaveOccurred(), "Failed to update NMState network policy")

				By("Verifying that VFs removed")
				err = sriovenv.WaitUntilVfsCreated(workerNodeList, sriovInterfacesUnderTest[0], 0, netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Unexpected amount of VF")

				By("Removing NMState policies")
				err = nmstate.CleanAllNMStatePolicies(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")

				By("Removing all test pods")
				err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean all test pods")

				By("Creating SR-IOV VFs again via NMState")
				err = netnmstate.ConfigureVFsAndWaitUntilItsConfigured(configureNMStatePolicyName,
					sriovInterfacesUnderTest[0], NetConfig.WorkerLabelMap, 5, netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate VFs via NMState")

				err = sriovenv.WaitUntilVfsCreated(workerNodeList, sriovInterfacesUnderTest[0], 5, netparam.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Expected number of VFs are not created")

				By("Re-create test pods and verify connectivity after recreating the VFs")
				err = sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovAndResourceNameExManagedTrue, sriovAndResourceNameExManagedTrue,
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})

			It("SR-IOV network with options", reportxml.ID("63534"), func() {
				By("Collecting default MaxTxRate and Vlan values")
				defaultMaxTxRate, defaultVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
					sriovInterfacesUnderTest[0])

				By("Updating Vlan and MaxTxRate configurations in the SriovNetwork")
				newMaxTxRate := defaultMaxTxRate + 1
				newVlanID := defaultVlanID + 1
				sriovNetwork, err := sriov.PullNetwork(APIClient, sriovAndResourceNameExManagedTrue,
					NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV network object")
				_, err = sriovNetwork.WithMaxTxRate(uint16(newMaxTxRate)).WithVLAN(uint16(newVlanID)).Update(false)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SR-IOV network with new configuration")

				By("Creating test pods and checking connectivity")
				err = sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovAndResourceNameExManagedTrue, sriovAndResourceNameExManagedTrue,
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")

				By("Checking that VF configured with new VLAN and MaxTxRate values")
				Eventually(func() []int {
					currentmaxTxRate, currentVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
						sriovInterfacesUnderTest[0])

					return []int{currentmaxTxRate, currentVlanID}
				}, time.Minute, tsparams.RetryInterval).Should(Equal([]int{newMaxTxRate, newVlanID}),
					"MaxTxRate and VlanId have been not configured properly")

				By("Removing all test pods")
				err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean all test pods")

				By("Checking that VF has initial configuration")

				Eventually(func() []int {
					currentmaxTxRate, currentVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
						sriovInterfacesUnderTest[0])

					return []int{currentmaxTxRate, currentVlanID}
				}, netparam.DefaultTimeout, tsparams.RetryInterval).
					Should(Equal([]int{defaultMaxTxRate, defaultVlanID}),
						"MaxTxRate and VlanId configuration have not been reverted to the initial one")

				By("Removing SR-IOV configuration")
				err = netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

				By("Checking that VF has initial configuration")
				Eventually(func() []int {
					currentmaxTxRate, currentVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
						sriovInterfacesUnderTest[0])

					return []int{currentmaxTxRate, currentVlanID}
				}, time.Minute, tsparams.RetryInterval).Should(And(Equal([]int{defaultMaxTxRate, defaultVlanID})),
					"MaxTxRate and VlanId configurations have not been reverted to the initial one")

				By("Configure SR-IOV with flag ExternallyManaged true")
				createSriovConfiguration(sriovAndResourceNameExManagedTrue, sriovInterfacesUnderTest[0], true)
			})

			It("SR-IOV operator removal", reportxml.ID("63537"), func() {
				By("Creating test pods and checking connectivity")
				err := sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovAndResourceNameExManagedTrue, sriovAndResourceNameExManagedTrue, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")

				By("Collecting info about installed SR-IOV operator")
				sriovNamespace, sriovOperatorgroup, sriovSubscription := collectingInfoSriovOperator()

				By("Removing SR-IOV operator")
				removeSriovOperator(sriovNamespace)
				Expect(sriovenv.IsSriovDeployed()).To(HaveOccurred(), "SR-IOV operator is not removed")

				By("Installing SR-IOV operator")
				installSriovOperator(sriovNamespace, sriovOperatorgroup, sriovSubscription)
				Eventually(sriovenv.IsSriovDeployed, time.Minute, tsparams.RetryInterval).
					ShouldNot(HaveOccurred(), "SR-IOV operator is not installed")

				By("Verifying that VFs still exist after SR-IOV operator reinstallation")
				err = netnmstate.AreVFsCreated(workerNodeList[0].Object.Name, sriovInterfacesUnderTest[0], 5)
				Expect(err).ToNot(HaveOccurred(), "VFs were removed after SR-IOV operator reinstallation")

				By("Configure SR-IOV with flag ExternallyManage true")
				createSriovConfiguration(sriovAndResourceNameExManagedTrue, sriovInterfacesUnderTest[0], true)

				By("Recreating test pods and checking connectivity")
				err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to remove test pods")

				err = sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovAndResourceNameExManagedTrue, sriovAndResourceNameExManagedTrue, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})
		})

		Context("Bond deployment", Label("bonddeployment"), func() {
			var (
				vfsUnderTest            []string
				workerNodeList          []*nodes.Builder
				err                     error
				testVlan                uint64
				secondBondInterfaceName = "bond2"
			)

			BeforeAll(func() {
				By("Verifying that the cluster deployed via bond interface")
				workerNodeList, err = nodes.List(APIClient,
					metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
				Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

				_, bondSlaves, err := netnmstate.
					CheckThatWorkersDeployedWithBondVfs(workerNodeList, tsparams.TestNamespaceName)
				if err != nil {
					Skip(fmt.Sprintf("The cluster is not suitable for testing: %s", err.Error()))
				}

				Expect(len(bondSlaves)).To(BeNumerically(">", 1),
					"Base VF interfaces should be more than 1")

				By("Getting VFs for the test")
				vfsUnderTest = getVfsUnderTest(bondSlaves)

				By("Getting cluster vlan for the test")
				testVlanString, err := NetConfig.GetClusterVlan()
				Expect(err).ToNot(HaveOccurred(), "Failed to get test Vlan")
				testVlan, err = strconv.ParseUint(testVlanString, 10, 16)
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to convert testVlanString %s to integer",
					testVlanString))
			})

			AfterAll(func() {
				By("Removing created bond interface via NMState")
				nmstatePolicy := nmstate.NewPolicyBuilder(APIClient, secondBondInterfaceName, NetConfig.WorkerLabelMap).
					WithAbsentInterface(fmt.Sprintf("%s.%d", secondBondInterfaceName, testVlan)).
					WithAbsentInterface(secondBondInterfaceName)
				err = netnmstate.UpdatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nmstatePolicy)
				Expect(err).ToNot(HaveOccurred(), "Failed to update NMState network policy")

				By("Removing SR-IOV configuration")
				err = netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

				By("Cleaning test namespace")
				testNamespace, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to pull test namespace %s", tsparams.TestNamespaceName))

				err = testNamespace.CleanObjects(
					tsparams.DefaultTimeout,
					pod.GetGVR(),
					nad.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")

				By("Removing NMState policies")
				err = nmstate.CleanAllNMStatePolicies(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")
			})

			It("Combination between SR-IOV and MACVLAN CNIs", reportxml.ID("63536"), func() {
				By("Creating a new bond interface with the VFs and vlan interface for this bond via nmstate operator")
				bondPolicy := nmstate.NewPolicyBuilder(APIClient, secondBondInterfaceName, NetConfig.WorkerLabelMap).
					WithBondInterface(vfsUnderTest, secondBondInterfaceName, "active-backup").
					WithVlanInterface(secondBondInterfaceName, uint16(testVlan))

				err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, bondPolicy)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NMState Policy")

				By("Creating mac-vlan networkAttachmentDefinition for the new bond interface")
				macVlanPlugin, err := define.MasterNadPlugin(secondBondInterfaceName, "bridge", nad.IPAMStatic(),
					fmt.Sprintf("%s.%d", secondBondInterfaceName, testVlan))
				Expect(err).ToNot(HaveOccurred(), "Failed to define master nad plugin")
				bondNad, err := nad.NewBuilder(APIClient, "nadbond", tsparams.TestNamespaceName).
					WithMasterPlugin(macVlanPlugin).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create nadbond NetworkAttachmentDefinition")

				By("Creating SR-IOV policy with flag ExternallyManage true")
				pfInterface, err := cmd.GetSrIovPf(vfsUnderTest[0], tsparams.TestNamespaceName, workerNodeList[0].Object.Name)
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get PF for VF interface %s", vfsUnderTest[0]))

				sriovPolicy := sriov.NewPolicyBuilder(
					APIClient,
					sriovAndResourceNameExManagedTrue,
					NetConfig.SriovOperatorNamespace,
					sriovAndResourceNameExManagedTrue,
					6, []string{fmt.Sprintf("%s#%d-%d", pfInterface, 2, 2)}, NetConfig.WorkerLabelMap).
					WithExternallyManaged(true)

				err = sriovenv.CreateSriovPolicyAndWaitUntilItsApplied(sriovPolicy, tsparams.MCOWaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

				By("Creating SR-IOV network")
				_, err = sriov.NewNetworkBuilder(
					APIClient, sriovAndResourceNameExManagedTrue, NetConfig.SriovOperatorNamespace,
					tsparams.TestNamespaceName, sriovAndResourceNameExManagedTrue).
					WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().WithVLAN(uint16(testVlan)).
					WithLogLevel(netparam.LogLevelDebug).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")

				By("Creating test pods and checking connectivity between test pods")
				err = sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[1].Object.Name,
					bondNad.Definition.Name, sriovAndResourceNameExManagedTrue,
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})
		})
	})

func createSriovConfiguration(sriovAndResName, sriovInterfaceName string, externallyManaged bool) {
	By("Creating SR-IOV policy with flag ExternallyManaged true")

	sriovPolicy := sriov.NewPolicyBuilder(APIClient, sriovAndResName, NetConfig.SriovOperatorNamespace, sriovAndResName,
		5, []string{sriovInterfaceName + "#0-1"}, NetConfig.WorkerLabelMap).WithExternallyManaged(externallyManaged)

	err := sriovenv.CreateSriovPolicyAndWaitUntilItsApplied(sriovPolicy, tsparams.MCOWaitTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

	By("Creating SR-IOV network")

	_, err = sriov.NewNetworkBuilder(APIClient, sriovAndResName, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, sriovAndResName).WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().
		WithLogLevel(netparam.LogLevelDebug).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")
}

// defineIterationParams defines ip settings for iteration based on ipFamily parameter.
func defineIterationParams(ipFamily string) (clientIPs, serverIPs []string, err error) {
	switch ipFamily {
	case netparam.IPV4Family:
		return []string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress}, nil
	case netparam.IPV6Family:
		return []string{tsparams.ClientIPv6IPAddress}, []string{tsparams.ServerIPv6IPAddress}, nil
	case netparam.DualIPFamily:
		return []string{tsparams.ClientIPv4IPAddress, tsparams.ClientIPv6IPAddress},
			[]string{tsparams.ServerIPv4IPAddress, tsparams.ServerIPv6IPAddress}, nil
	}

	return nil, nil, fmt.Errorf(
		"ipStack parameter %s is invalid; allowed values are %s, %s, %s ",
		ipFamily, netparam.IPV4Family, netparam.IPV6Family, netparam.DualIPFamily)
}

func getVlanIDAndMaxTxRateForVf(nodeName, sriovInterfaceName string) (maxTxRate, vlanID int) {
	nmstateState, err := nmstate.PullNodeNetworkState(APIClient, nodeName)
	Expect(err).ToNot(HaveOccurred(), "Failed to discover NMState network state")
	sriovVfs, err := nmstateState.GetSriovVfs(sriovInterfaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to get all SR-IOV VFs")

	return *sriovVfs[0].MaxTxRate, *sriovVfs[0].VlanID
}

func collectingInfoSriovOperator() (
	sriovNamespace *namespace.Builder,
	sriovOperatorGroup *olm.OperatorGroupBuilder,
	sriovSubscription *olm.SubscriptionBuilder) {
	sriovNs, err := namespace.Pull(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV operator namespace")
	sriovOg, err := olm.PullOperatorGroup(APIClient, "sriov-network-operators", NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV OperatorGroup")
	sriovSub, err := olm.PullSubscription(
		APIClient,
		"sriov-network-operator-subscription",
		NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull sriov-network-operator-subscription")

	return sriovNs, sriovOg, sriovSub
}

func removeSriovOperator(sriovNamespace *namespace.Builder) {
	By("Clean all SR-IOV policies and networks")

	err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
	Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

	By("Remove SR-IOV operator config")

	sriovOperatorConfig, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovOperatorConfig")

	_, err = sriovOperatorConfig.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to remove default SR-IOV operator config")

	By("Validation that SR-IOV webhooks are not available")

	for _, webhookname := range []string{"network-resources-injector-config", "sriov-operator-webhook-config"} {
		Eventually(func() error {
			_, err := webhook.PullMutatingConfiguration(APIClient, webhookname)

			return err
		}, time.Minute, tsparams.RetryInterval).Should(HaveOccurred(),
			fmt.Sprintf("MutatingWebhook %s was not removed", webhookname))
	}

	Eventually(func() error {
		_, err := webhook.PullValidatingConfiguration(APIClient, "sriov-operator-webhook-config")

		return err
	}, time.Minute, tsparams.RetryInterval).Should(HaveOccurred(),
		"ValidatingWebhook sriov-operator-webhook-config was not removed")

	By("Removing SR-IOV namespace")

	err = sriovNamespace.DeleteAndWait(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to delete SR-IOV namespace %s", NetConfig.SriovOperatorNamespace))
}

func installSriovOperator(sriovNamespace *namespace.Builder,
	sriovOperatorGroup *olm.OperatorGroupBuilder,
	sriovSubscription *olm.SubscriptionBuilder) {
	By("Creating SR-IOV operator namespace")

	sriovNs := namespace.NewBuilder(APIClient, sriovNamespace.Definition.Name)
	_, err := sriovNs.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SR-IOV namespace %s", sriovNs.Definition.Name))

	By("Creating SR-IOV OperatorGroup")

	sriovOg := olm.NewOperatorGroupBuilder(
		APIClient,
		sriovOperatorGroup.Definition.Name,
		sriovOperatorGroup.Definition.Namespace)
	_, err = sriovOg.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SR-IOV OperatorGroup %s", sriovOg.Definition.Name))

	By("Creating SR-IOV operator Subscription")

	sriovSub := olm.NewSubscriptionBuilder(
		APIClient, sriovSubscription.Definition.Name,
		sriovSubscription.Definition.Namespace,
		sriovSubscription.Definition.Spec.CatalogSource,
		sriovSubscription.Definition.Spec.CatalogSourceNamespace,
		sriovSubscription.Definition.Spec.Package)
	_, err = sriovSub.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SR-IOV Subscription %s", sriovSub.Definition.Name))

	By("Creating SR-IOV operator default configuration")

	_, err = sriov.NewOperatorConfigBuilder(APIClient, sriovNamespace.Definition.Name).
		WithOperatorWebhook(true).
		WithInjector(true).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV operator config")
}

func getVfsUnderTest(busyVfs []string) []string {
	var vfsUnderTest []string

	for _, busyVf := range busyVfs {
		runes := []rune(busyVf)
		if len(runes) > 0 {
			runes[len(runes)-1] = '1'
		}

		vfsUnderTest = append(vfsUnderTest, string(runes))
	}

	return vfsUnderTest
}
