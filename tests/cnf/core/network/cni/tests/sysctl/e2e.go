package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/cni/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/sriovhelper"
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
		By("Verifying SR-IOV operator is deployed")

		err := sriovoperator.IsSriovDeployed(APIClient, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sysctl SR-IOV test cases")

		By("Clean SR-IOV configuration from test namespace")

		err = sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			netparam.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to clean SR-IOV configuration")

		By("Collect node list based on worker label")

		workerNodeList, err := nodes.List(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")
		Expect(workerNodeList).ToNot(BeEmpty(), "Cluster has no worker nodes for sysctl tests")

		workerNodeName = workerNodeList[0].Definition.Name

		By("Collect node valid interface for macvlan configuration")

		validMacVlanInterfaces = getValidMacVlanInterfaces(workerNodeName, 1)
		if len(validMacVlanInterfaces) < 1 {
			Skip("cluster doesn't have secondary interfaces available for sysctl test")
		}

		By("Find available sr-iov interfaces")

		sriovInterfaces, err := NetConfig.GetSriovInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for sysctl tests")

		sriovInterfaceName = sriovInterfaces[0]

		By("Validating SR-IOV interfaces exist on worker nodes")

		Expect(sriovhelper.ValidateSriovInterfaces(workerNodeList, 1)).To(Succeed(),
			"Failed to validate SR-IOV interfaces on worker nodes")

		By("Define sysctl SR-IOV policy")

		sriovPolicy := sriov.NewPolicyBuilder(
			APIClient,
			"sysctl-policy",
			NetConfig.SriovOperatorNamespace,
			tsparams.ResourceNameSysctl,
			tsparams.SysctlPolicyNumVFs,
			[]string{sriovInterfaceName},
			NetConfig.WorkerLabelMap).
			WithDevType("netdevice").
			WithMTU(tsparams.SysctlPolicyMTU).
			WithVFRange(tsparams.SysctlPolicyVFStart, tsparams.SysctlPolicyVFEnd)

		err = sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			sriovPolicy,
			netparam.MCOWaitTimeout,
			netparam.DefaultRetryInterval)
		Expect(err).ToNot(HaveOccurred(), "Failed to create sysctl SR-IOV policy")

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
			}, netparam.MCOWaitTimeout, netparam.DefaultRetryInterval).Should(Succeed(),
				"Failed to restore IP forwarding on DUT interface")
		}

		By("Removing SR-IOV configuration")

		err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
			APIClient,
			NetConfig.WorkerLabelEnvVar,
			NetConfig.SriovOperatorNamespace,
			netparam.MCOWaitTimeout,
			tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
	})

	BeforeEach(func() {
		if len(validMacVlanInterfaces) < 1 {
			Skip("cluster doesn't have secondary interfaces available for sysctl test")
		}

		cleanSysctlTestNamespace()
	})

	Context("pod one secondary interface,", func() {
		It("set accept_redirects=0 on one of two SriovNetworks", reportxml.ID("50439"), func() {
			By("Define sr-iov network without sysctl mutation flags")
			createSysctlTuningSriovNetwork(
				tsparams.NetworkWithoutSysctlMutation, workerNodeName, sriovInterfaceName, nil, true)

			By("Default and create sr-iov network with sysctl mutation flag accept_redirects=0")
			createSysctlTuningSriovNetwork(
				tsparams.NetworkWithSysctlMutation, workerNodeName, sriovInterfaceName, tsparams.SingleSysctlFlag, true)

			By("Create server and redirect pod")
			createServerPod()
			createRedirectPod()

			By("Create client pod connected to sr-iov network without sysctl mutation")

			clientNetCfg := defineClientNetCfg(tsparams.NetworkWithoutSysctlMutation)
			runningClientPod := createClientPod(clientNetCfg)

			By("Test ping, route and sysctl flag")
			testIcmpRouteSysctlFlag(
				runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, false)

			By("Recreate client pod connected to sr-iov network with sysctl mutation")

			clientNetCfg = defineClientNetCfg(tsparams.NetworkWithSysctlMutation)
			runningClientPod = recreateClientPod(runningClientPod, clientNetCfg)

			By("Test ping, route and sysctl flag negative")
			testIcmpRouteSysctlFlag(
				runningClientPod, tsparams.SrvLopIPAddr, tsparams.MultusFirstInterfaceName, true)
		})
	})
})
