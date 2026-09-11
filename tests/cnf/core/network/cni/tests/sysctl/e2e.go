package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("CNF Sysctl", Ordered, Label(tsparams.LabelSysctlTestCases), ContinueOnFailure, func() {
	var (
		workerNodeName                   string
		validMacVlanInterfaces           []nodeInterface
		sriovInterfaceName               string
		dutInterfaceOriginalIPForwarding bool
	)

	BeforeAll(func() {
		var err error

		workerNodeName, validMacVlanInterfaces = discoverSysctlMacVlanSetup()

		By("Clean SR-IOV configuration from operator namespace")

		err = sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			netparam.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to clean SR-IOV configuration from operator namespace")

		By("Validate SR-IOV interfaces required for sysctl tests")

		workerNodeList, err := nodes.List(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")
		Expect(validateSysctlSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
			"Failed to validate SR-IOV interfaces for sysctl tests")

		sriovInterfaces, err := NetConfig.GetSriovInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to read SR-IOV interfaces from environment")

		sriovInterfaceName = sriovInterfaces[0]

		By("Define sysctl SR-IOV policy")

		err = sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			sriov.NewPolicyBuilder(
				APIClient,
				tsparams.SysctlSriovPolicyName,
				NetConfig.SriovOperatorNamespace,
				tsparams.ResourceNameSysctl,
				tsparams.SysctlPolicyNumVFs,
				[]string{sriovInterfaceName},
				NetConfig.WorkerLabelMap).
				WithMTU(tsparams.SysctlPolicyMTU).
				WithVFRange(tsparams.SysctlPolicyVFStart, tsparams.SysctlPolicyVFEnd),
			netparam.MCOWaitTimeout,
			sriovoperator.SriovStabilizationDelay)
		Expect(err).ToNot(HaveOccurred(), "Failed to create sysctl SR-IOV policy")

		if len(validMacVlanInterfaces) > 0 {
			By("Enabling IPForwarding on a DUT interface")

			dutInterfaceOriginalIPForwarding, err = getHostIPForwardingQuiet(
				workerNodeName, validMacVlanInterfaces[0].Name)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to read IP forwarding on interface %s", validMacVlanInterfaces[0].Name))
			setHostIPForwarding(workerNodeName, validMacVlanInterfaces[0].Name, true)
		}
	})

	AfterAll(func() {
		if workerNodeName != "" && len(validMacVlanInterfaces) > 0 {
			By("Restoring IPForwarding on a DUT interface")

			Eventually(func() error {
				return setHostIPForwardingQuiet(
					workerNodeName, validMacVlanInterfaces[0].Name, dutInterfaceOriginalIPForwarding)
			}, tsparams.DefaultTimeout, time.Second).Should(Succeed(),
				"Failed to restore IP forwarding on DUT interface")
		}

		By("Clean per-test sysctl resources from test namespace")

		cleanSysctlTestNamespaceForE2E()

		By("Clean SR-IOV configuration from operator namespace")

		err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			netparam.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to clean SR-IOV configuration from operator namespace")
	})

	// Per-test cleanup only; sysctl-sriov-policy from BeforeAll is kept until AfterAll.
	BeforeEach(func() {
		cleanSysctlTestNamespaceForE2E()
	})

	Context("pod one secondary interface,", func() {
		It("apply full allowed interface sysctl set on one of two NAD macvlans, verify sysctl -n, "+
			"reject sysctl -w, and validate ICMP redirect behavior",
			reportxml.ID("50437"), func() {
				macVlanIf := requireSysctlMacVlanInterfaces()[0].Name

				By("Define and create NAD with full allowed interface sysctl set")
				createSysctlTuningNad(
					tsparams.NetworkWithSysctlMutation,
					tsparams.AllFlagsSysctlPluginConfig,
					macVlanIf)

				By("Define and create NAD without sysctl config")
				createStaticIpamNad(
					tsparams.NetworkWithoutSysctlMutation, macVlanIf)

				By("Create server and redirect pod")
				createServerPod()
				createRedirectPod()

				By("Create client pod connected to NAD without sysctl mutation")

				clientNetCfg := defineClientNetCfg(tsparams.NetworkWithoutSysctlMutation)
				runningClientPod := createClientPod(clientNetCfg)

				By("Verifying Multus network-status JSON after CNI ADD")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithoutSysctlMutation,
					tsparams.MultusFirstInterfaceName,
					tsparams.ClientIPv4)

				By("Test ping, route and sysctl flag")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, false)

				By("Recreate client pod connected to NAD with sysctl mutation")

				clientNetCfg = defineClientNetCfg(tsparams.NetworkWithSysctlMutation)
				runningClientPod = recreateClientPod(runningClientPod, clientNetCfg)

				By("Verifying Multus network-status JSON after CNI ADD with sysctl mutation")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithSysctlMutation,
					tsparams.MultusFirstInterfaceName,
					tsparams.ClientIPv4)

				By("Verify all allowed interface sysctls are applied on secondary interface")
				assertSysctlConfiguredOnPodInterface(
					runningClientPod,
					tsparams.AllFlagsSysctlPluginConfig,
					tsparams.MultusFirstInterfaceName)

				By("Verify interface sysctl cannot be changed manually in the running pod")
				assertSysctlWriteRejectedOnPodInterface(
					runningClientPod,
					tsparams.SingleSysctlFlag,
					tsparams.MultusFirstInterfaceName,
					"1")

				By("Test ping, route and sysctl flag negative")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, true)
			})

		It("set accept_redirects=0 on one of two SriovNetworks, verify network-status, "+
			"and validate ICMP redirect behavior",
			reportxml.ID("50439"), func() {
				By("Define sr-iov network without sysctl mutation flags")
				createSysctlTuningSriovNetwork(tsparams.NetworkWithoutSysctlMutation, nil, true)

				By("Define and create sr-iov network with sysctl mutation flag accept_redirects=0")
				createSysctlTuningSriovNetwork(
					tsparams.NetworkWithSysctlMutation, tsparams.SingleSysctlFlag, true)

				By("Create server and redirect pod")
				createServerPod()
				createRedirectPod()

				By("Create client pod connected to sr-iov network without sysctl mutation")

				clientNetCfg := defineClientNetCfg(tsparams.NetworkWithoutSysctlMutation)
				runningClientPod := createClientPod(clientNetCfg)

				By("Verifying Multus network-status JSON after CNI ADD")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithoutSysctlMutation,
					tsparams.MultusFirstInterfaceName,
					tsparams.ClientIPv4)

				By("Test ping, route and sysctl flag")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, false)

				By("Recreate client pod connected to sr-iov network with sysctl mutation")

				clientNetCfg = defineClientNetCfg(tsparams.NetworkWithSysctlMutation)
				runningClientPod = recreateClientPod(runningClientPod, clientNetCfg)

				By("Verifying Multus network-status JSON after CNI ADD with sysctl mutation")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithSysctlMutation,
					tsparams.MultusFirstInterfaceName,
					tsparams.ClientIPv4)

				By("Test ping, route and sysctl flag negative")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, true)
			})
	})

	Context("pod multiple interfaces,", func() {
		It("apply plain macvlan on net1 and full allowed interface sysctl set on net2, "+
			"verify sysctl -n on net2, reject sysctl -w, and validate dual-interface ICMP redirect",
			reportxml.ID("50438"), func() {
				macVlanIf := requireSysctlMacVlanInterfaces()[0].Name

				By("Define and create NAD with full allowed interface sysctl set")
				createSysctlTuningNad(
					tsparams.NetworkWithSysctlMutation,
					tsparams.AllFlagsSysctlPluginConfig,
					macVlanIf)

				By("Define and create NAD without sysctl config")
				createStaticIpamNad(
					tsparams.NetworkWithoutSysctlMutation, macVlanIf)

				By("Create server and redirect pod")
				createServerPodWithInit(defineDualServerNetCfg(), tsparams.SrvDualInitCMD)
				createRedirectPodWithInit(defineDualRedirectNetCfg(), tsparams.RdrDualInitCMD)

				By("Create client pod connected to both NADs")

				runningClientPod := createClientPodWithInit(defineDualClientNetCfg(), tsparams.ClientDualInitCMD)

				By("Verifying Multus network-status JSON after CNI ADD on both secondary interfaces")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithoutSysctlMutation,
					tsparams.MultusFirstInterfaceName,
					tsparams.ClientIPv4)
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithSysctlMutation,
					tsparams.MultusSecondInterfaceName,
					tsparams.ClientSecondIPv4)

				By("Verify all allowed interface sysctls are applied on sysctl-mutated interface")
				assertSysctlConfiguredOnPodInterface(
					runningClientPod,
					tsparams.AllFlagsSysctlPluginConfig,
					tsparams.MultusSecondInterfaceName)

				By("Verify interface sysctl cannot be changed manually in the running pod")
				assertSysctlWriteRejectedOnPodInterface(
					runningClientPod,
					tsparams.SingleSysctlFlag,
					tsparams.MultusSecondInterfaceName,
					"1")

				By("Test ping, route and sysctl flag on interface connected to NAD without mutation")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, false)

				By("Test ping, route and sysctl flag on interface connected to NAD with mutation, negative")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopSecondIPAddr, tsparams.MultusSecondInterfaceName, true)
			})

		It("set accept_redirects=0 on first SriovNetwork and accept_redirects=1 on the second SriovNetwork, "+
			"verify network-status on both interfaces, and validate dual-interface ICMP redirect",
			reportxml.ID("50501"), func() {
				By("Define sr-iov network without sysctl mutation flags")
				createSysctlTuningSriovNetwork(tsparams.NetworkWithoutSysctlMutation, nil, true)

				By("Define and create sr-iov network with sysctl mutation flag accept_redirects=0")
				createSysctlTuningSriovNetwork(
					tsparams.NetworkWithSysctlMutation, tsparams.SingleSysctlFlag, true)

				By("Create server and redirect pod")
				createServerPodWithInit(defineDualServerNetCfg(), tsparams.SrvDualInitCMD)
				createRedirectPodWithInit(defineDualRedirectNetCfg(), tsparams.RdrDualInitCMD)

				By("Create client pod connected to both sr-iov networks")

				runningClientPod := createClientPodWithInit(defineDualClientNetCfg(), tsparams.ClientDualInitCMD)

				By("Verifying Multus network-status JSON after CNI ADD on both secondary interfaces")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithoutSysctlMutation,
					tsparams.MultusFirstInterfaceName,
					tsparams.ClientIPv4)
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithSysctlMutation,
					tsparams.MultusSecondInterfaceName,
					tsparams.ClientSecondIPv4)

				By("Test ping, route and sysctl flag on interface connected to sr-iov network without mutation")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, false)

				By("Test ping, route and sysctl flag on interface connected to sr-iov network with mutation, negative")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopSecondIPAddr, tsparams.MultusSecondInterfaceName, true)
			})
	})

	Context("bond,", func() {
		It("sriov-bond interface with one SriovNetwork and two VF slaves on one bond NAD, "+
			"set accept_redirects=0 on bond0, recreate pod with sysctl mutation, and validate ICMP redirect",
			reportxml.ID("50502"), func() {
				By("Define sr-iov network without sysctl flags for bond slaves")
				createSysctlTuningSriovNetwork(tsparams.SysctlBondSlaveSriovNetworkName, nil, false)

				By("Define bond NAD without sysctl plugin")
				createSysctlBondNad(
					tsparams.NetworkWithoutSysctlMutation,
					[]string{tsparams.MultusFirstInterfaceName, tsparams.MultusSecondInterfaceName},
					nil)

				By("Define bond NAD with sysctl mutation flag accept_redirects=0")
				createSysctlBondNad(
					tsparams.NetworkWithSysctlMutation,
					[]string{tsparams.MultusFirstInterfaceName, tsparams.MultusSecondInterfaceName},
					tsparams.SingleSysctlFlag)

				By("Create server and redirect pod")
				createServerPodWithInit(defineBondServerNetCfg(false), tsparams.SrvInitCMD)
				createRedirectPodWithInit(defineBondRedirectNetCfg(false), tsparams.RdrInitCMD)

				By("Create client pod connected to bond NAD without sysctl mutation")

				clientNetCfg := defineBondClientNetCfg(false)
				runningClientPod := createClientPodWithInit(clientNetCfg, tsparams.ClientInitCMD)

				By("Verifying Multus network-status JSON after CNI ADD on bond0")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithoutSysctlMutation,
					tsparams.BondInterfaceName,
					tsparams.ClientIPv4)

				By("Test ping, route and sysctl flag on bond0")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.BondInterfaceName, false)

				By("Recreate client pod connected to bond NAD with sysctl mutation accept_redirects=0")

				clientNetCfg[2].Name = tsparams.NetworkWithSysctlMutation
				runningClientPod = recreateClientPod(runningClientPod, clientNetCfg)

				By("Verifying Multus network-status JSON after CNI ADD with sysctl mutation on bond0")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithSysctlMutation,
					tsparams.BondInterfaceName,
					tsparams.ClientIPv4)

				By("Test ping, route and sysctl flag negative on bond0")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.BondInterfaceName, true)
			})

		It("sriov-bond with two bond NADs, set accept_redirects=0 on bond1 only, "+
			"and validate dual-interface ICMP redirect on bond0 and bond1",
			reportxml.ID("50503"), func() {
				By("Define sr-iov network without sysctl flags for bond slaves")
				createSysctlTuningSriovNetwork(tsparams.SysctlBondSlaveSriovNetworkName, nil, false)

				By("Define bond NAD without sysctl plugin")
				createSysctlBondNad(
					tsparams.NetworkWithoutSysctlMutation,
					[]string{tsparams.MultusFirstInterfaceName, tsparams.MultusSecondInterfaceName},
					nil)

				By("Define bond NAD with sysctl mutation flag accept_redirects=0")
				createSysctlBondNad(
					tsparams.NetworkWithSysctlMutation,
					[]string{"net4", "net5"},
					tsparams.SingleSysctlFlag)

				By("Create server and redirect pod")
				createServerPodWithInit(defineBondServerNetCfg(true), tsparams.SrvDualInitCMD)
				createRedirectPodWithInit(defineBondRedirectNetCfg(true), tsparams.RdrDualInitCMD)

				By("Create client pod connected to both bond NAD networks")

				runningClientPod := createClientPodWithInit(defineBondClientNetCfg(true), tsparams.ClientDualInitCMD)

				By("Verifying Multus network-status JSON after CNI ADD on both bond interfaces")
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithoutSysctlMutation,
					tsparams.BondInterfaceName,
					tsparams.ClientIPv4)
				verifySysctlNetworkStatus(
					runningClientPod,
					tsparams.NetworkWithSysctlMutation,
					tsparams.BondInterfaceNameSecond,
					tsparams.ClientSecondIPv4)

				By("Test ping, route and sysctl flag on bond network without mutation")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.BondInterfaceName, false)

				By("Test ping, route and sysctl flag on bond network with mutation, negative")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopSecondIPAddr, tsparams.BondInterfaceNameSecond, true)
			})
	})
})
