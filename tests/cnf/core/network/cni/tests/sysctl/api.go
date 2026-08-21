package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
)

var _ = Describe("CNF Sysctl", Ordered, Label(tsparams.LabelSysctlTestCases), ContinueOnFailure, func() {
	var validMacVlanInterfaces []nodeInterface

	BeforeAll(func() {
		_, validMacVlanInterfaces = ensureSysctlMacVlanSetup()
	})

	BeforeEach(func() {
		cleanSysctlTestNamespace()
	})

	Context("pod one secondary interface,", func() {
		It("one NAD, forward all valid interface level flags one global kernel flag", reportxml.ID("50342"), func() {
			By("Define and create NAD with invalid sysctl flag")

			nadWithInvalidSysctlFlag := copySysctlMap(tsparams.AllFlagsSysctlPluginConfig)
			nadWithInvalidSysctlFlag[tsparams.GlobalSysctlFlag] = "1"
			createSysctlTuningNad(
				tsparams.FirstSysctlNetworkName,
				nadWithInvalidSysctlFlag,
				validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			// Static IP is required so macvlan/IPAM succeeds and tuning CNI can reject the global sysctl.
			defineCreatePodWithNetworksAndWaitUntilPending(
				pod.StaticIPAnnotationWithNamespace(
					tsparams.FirstSysctlNetworkName,
					tsparams.TestNamespaceName,
					[]string{tsparams.FirstSysctlNetworkIPv4CIDR}))

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(tsparams.GlobalSysctlFlag)
		})

		It("one NAD, forward interface level duplicated flags", reportxml.ID("50346"), func() {
			Skip("TC skipped due to BZ:2077683")

			By("Define and create NAD with duplicated sysctl flag")

			duplicatedFlag := createSysctlTuningNadWithDuplicatedKernelArg(
				tsparams.FirstSysctlNetworkName,
				validMacVlanInterfaces[0].Name,
				tsparams.AllFlagsSysctlPluginConfig)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending(
				pod.StaticIPAnnotationWithNamespace(
					tsparams.FirstSysctlNetworkName,
					tsparams.TestNamespaceName,
					[]string{tsparams.FirstSysctlNetworkIPv4CIDR}))

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(duplicatedFlag)
		})
	})

	Context("pod multiple secondary interfaces,", func() {
		It("two NADs, Forward all valid interface level flags to one interface and multiple flags to the "+
			"second interface with one general network kernel flag", reportxml.ID("50432"), func() {
			By("Define and create NAD with all valid interface level flags")
			createSysctlTuningNad(
				tsparams.FirstSysctlNetworkName,
				tsparams.AllFlagsSysctlPluginConfig,
				validMacVlanInterfaces[0].Name)

			By("Define and create NAD with multiple valid interface level flags plus single network kernel flag")

			multipleFlagsSysctlOneGlobal := copySysctlMap(tsparams.MultipleFlagsSysctl)
			multipleFlagsSysctlOneGlobal[tsparams.TCPFastopenSysctlFlag] = "0"
			createSysctlTuningNad(
				tsparams.SecondSysctlNetworkName,
				multipleFlagsSysctlOneGlobal,
				validMacVlanInterfaces[0].Name)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending(defineDualSysctlNetPodNetworks())

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(tsparams.TCPFastopenSysctlFlag)
		})

		It("two NADs, try to inject static interface level flag for net1 interface using net2 NAD",
			reportxml.ID("50434"), func() {
				By("Define and create NAD")
				createSysctlTuningNad(
					tsparams.FirstSysctlNetworkName,
					tsparams.SingleSysctlFlag,
					validMacVlanInterfaces[0].Name)

				By("Define and create NAD with static sysctl interface flag")

				staticSysctlInterfaceKernelKey := fmt.Sprintf(
					"net.ipv4.conf.%s.accept_redirects", tsparams.MultusFirstInterfaceName)
				oneStaticInterfaceSysctlFlag := map[string]string{staticSysctlInterfaceKernelKey: "0"}
				createSysctlTuningNad(
					tsparams.SecondSysctlNetworkName,
					oneStaticInterfaceSysctlFlag,
					validMacVlanInterfaces[0].Name)

				By("Define and create pod")
				defineCreatePodWithNetworksAndWaitUntilPending(defineDualSysctlNetPodNetworks())

				By("Wait until sysctl failed event")
				waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(staticSysctlInterfaceKernelKey)
			})

		It("two NADs, forward all valid flags to both interfaces and the second "+
			"interface has static interface level duplicated flag", reportxml.ID("50435"), func() {
			By("Define and create NAD with all sysctl flags")
			createSysctlTuningNadWithDuplicatedKernelArg(
				tsparams.FirstSysctlNetworkName,
				validMacVlanInterfaces[0].Name,
				tsparams.AllFlagsSysctlPluginConfig)

			By("Define and create NAD with static interface duplicated sysctl flags")

			staticSysctlInterfaceKernelKey := fmt.Sprintf(
				"net.ipv4.conf.%s.accept_redirects", tsparams.MultusFirstInterfaceName)
			allFlagsWithOneStaticInterfaceSysctlFlag := copySysctlMap(tsparams.AllFlagsSysctlPluginConfig)
			allFlagsWithOneStaticInterfaceSysctlFlag[staticSysctlInterfaceKernelKey] = "0"
			createSysctlTuningNadWithDuplicatedKernelArg(
				tsparams.SecondSysctlNetworkName,
				validMacVlanInterfaces[0].Name,
				allFlagsWithOneStaticInterfaceSysctlFlag)

			By("Define and create pod")
			defineCreatePodWithNetworksAndWaitUntilPending(defineDualSysctlNetPodNetworks())

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(staticSysctlInterfaceKernelKey)
		})
	})
})
