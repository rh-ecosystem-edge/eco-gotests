package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
)

var _ = Describe("CNF Sysctl", Ordered, Label(tsparams.LabelSysctlTestCases), ContinueOnFailure, func() {
	var (
		workerNodeName                   string
		validMacVlanInterfaces           []nodeInterface
		dutInterfaceOriginalIPForwarding bool
	)

	BeforeAll(func() {
		var err error

		workerNodeName, validMacVlanInterfaces = ensureSysctlMacVlanSetup()

		By("Enabling IPForwarding on a DUT interface")

		dutInterfaceOriginalIPForwarding, err = getHostIPForwardingQuiet(
			workerNodeName, validMacVlanInterfaces[0].Name)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to read IP forwarding on interface %s", validMacVlanInterfaces[0].Name))
		setHostIPForwarding(workerNodeName, validMacVlanInterfaces[0].Name, true)
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
	})

	BeforeEach(func() {
		cleanSysctlTestNamespace()
	})

	Context("pod one secondary interface,", func() {
		It("set accept_redirects=0 on one of two NAD macvlans",
			reportxml.ID("50437"), func() {
				By("Define and create NAD with sysctl mutation flag accept_redirects=0")
				createSysctlTuningNad(
					tsparams.NetworkWithSysctlMutation,
					tsparams.SingleSysctlFlag,
					validMacVlanInterfaces[0].Name)

				By("Define and create NAD without sysctl config")
				createStaticIpamNad(
					tsparams.NetworkWithoutSysctlMutation, validMacVlanInterfaces[0].Name)

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

				By("Test ping, route and sysctl flag negative")
				testIcmpRouteSysctlFlag(
					runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, true)
			})
	})
})
