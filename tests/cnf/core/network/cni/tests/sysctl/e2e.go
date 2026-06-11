package tests

import (
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
		workerNodeName         string
		validMacVlanInterfaces []nodeInterface
		sriovInterfaceName     string
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

		By("Find available sr-iov interfaces")

		sriovInterfaces, err := getRequestedSriovInterfaceNames(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for sysctl tests")

		sriovInterfaceName = sriovInterfaces[0]

		By("Select worker node exposing the requested SR-IOV interface")

		workerNodeName, err = findWorkerNodeWithSriovInterface(workerNodeList, sriovInterfaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to find worker node with requested SR-IOV interface")

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
			time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to create sysctl SR-IOV policy")

		By("Collect node valid interface for macvlan configuration")

		validMacVlanInterfaces = getValidMacVlanInterfaces(workerNodeName, 1)

		By("Enabling IPForwarding on a DUT interface")
		setHostIPForwarding(workerNodeName, validMacVlanInterfaces[0].Name, true)
	})

	AfterAll(func() {
		if workerNodeName == "" || len(validMacVlanInterfaces) == 0 {
			return
		}

		By("Disabling IPForwarding on a DUT interface")

		Eventually(func() error {
			return setHostIPForwardingQuiet(workerNodeName, validMacVlanInterfaces[0].Name, false)
		}, netparam.MCOWaitTimeout, 30*time.Second).Should(Succeed(),
			"Failed to disable IP forwarding on DUT interface")

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
			createSysctlTuningSriovNetwork(tsparams.NetworkWithoutSysctlMutation, nil, true)

			By("Default and create sr-iov network with sysctl mutation flag accept_redirects=0")
			createSysctlTuningSriovNetwork(
				tsparams.NetworkWithSysctlMutation, tsparams.SingleSysctlFlag, true)

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
