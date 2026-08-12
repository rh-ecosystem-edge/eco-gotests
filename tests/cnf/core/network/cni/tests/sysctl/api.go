package tests

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
)

var _ = Describe("CNF Sysctl API", Ordered, Label(tsparams.LabelSysctlTestCases), ContinueOnFailure, func() {
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
					[]string{tsparams.ClientIPv4CIDR}))

			By("Wait until sysctl failed event")
			waitUntilEventListContainsSysctlFailedCreatePodSandBoxMessage(tsparams.GlobalSysctlFlag)
		})
	})
})
