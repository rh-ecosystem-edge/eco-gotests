package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("Mellanox Secure Boot", Ordered, Label(tsparams.LabelMlxSecureBoot),
	ContinueOnFailure, func() {
		var (
			sriovInterfacesUnderTest []string
			totalVFs                 = 127
			workerNodeList           []*nodes.Builder
			bmcClient                *bmc.BMC
		)
		BeforeAll(func() {
			By("Verifying if tests can be executed on given cluster")
			err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support Mellanox Secure Boot test cases as it doesn't have enough nodes")

			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
			// Restricting to the first worker node for further operations
			workerNodeList = workerNodeList[:1]

			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interface for testing")

			By("Skipping test cases if the SR-IOV device is not Mellanox")
			if !sriovenv.IsMellanoxDevice(sriovInterfacesUnderTest[0], workerNodeList[0].Object.Name) {
				Skip("Mellanox Secure Boot test cases are supported only on Mellanox devices")
			}

			By("Collecting information to create a BMC client")
			bmcClient, err = sriovenv.CreateBMCClient()
			Expect(err).ToNot(HaveOccurred(), "Failed to create BMC client")

			By("Enabling Mellanox firmware and wait for the cluster becomes stable")
			err = sriovenv.ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP(
				workerNodeList, sriovInterfacesUnderTest[0], true, totalVFs)
			Expect(err).ToNot(HaveOccurred(), "Failed to configure Mellanox firmware")

			By("Disabling Mellanox plugin in SriovOperatorConfig")
			sriovOperatorConfig, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV operator config")
			_, err = sriovOperatorConfig.WithDisablePlugins([]string{"mellanox"}).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to configure disablePlugins to include mellanox")

			By("Enabling secure boot on the worker and reboot the node")
			isSecureBootEnabled, err := bmcClient.IsSecureBootEnabled()
			Expect(err).ToNot(HaveOccurred(), "Failed to validate if SecureBoot is enabled")

			if !isSecureBootEnabled {
				err = sriovenv.ConfigureSecureBoot(bmcClient, "enable")
				Expect(err).ToNot(HaveOccurred(), "Failed to enable a secure boot")
			}
		})

		AfterAll(func() {
			By("Removing SR-IOV configuration")
			sriovOperatorConfig, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV operator config")
			_, err = sriovOperatorConfig.RemoveDisablePlugins().Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete disablePlugins")

			err = netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Disabling secure boot on the worker and reboot the node")
			isSecureBootEnabled, err := bmcClient.IsSecureBootEnabled()
			Expect(err).ToNot(HaveOccurred(), "Failed to validate if SecureBoot is enabled")

			if isSecureBootEnabled {
				err = sriovenv.ConfigureSecureBoot(bmcClient, "disable")
				Expect(err).ToNot(HaveOccurred(), "Failed to disable a secure boot")
			}
		})

		It("End-to-End SR-IOV Configuration and Validation", reportxml.ID("77014"), func() {
			By("Creating SR-IOV policy")
			const sriovAndResourceNameSecureBoot = "securebootpolicy"

			configDaemonPods, err := pod.List(APIClient, NetConfig.SriovOperatorNamespace, metav1.ListOptions{
				LabelSelector: "app=sriov-network-config-daemon",
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", workerNodeList[0].Definition.Name)})
			Expect(err).ToNot(HaveOccurred(), "Failed to pull sriov-network-config-daemon pod")
			Expect(len(configDaemonPods)).To(Equal(1),
				"Should be one sriov-network-config-daemon pod per worker node")

			initialRestartCount := podRestartCount(configDaemonPods[0].Object.Name)

			sriovPolicy := sriov.NewPolicyBuilder(
				APIClient,
				sriovAndResourceNameSecureBoot,
				NetConfig.SriovOperatorNamespace,
				sriovAndResourceNameSecureBoot,
				6,
				sriovInterfacesUnderTest[:1],
				map[string]string{"kubernetes.io/hostname": workerNodeList[0].Definition.Name})

			err = sriovenv.CreateSriovPolicyAndWaitUntilItsApplied(sriovPolicy, tsparams.MCOWaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

			currentRestartCount := podRestartCount(configDaemonPods[0].Object.Name)
			Expect(currentRestartCount).To(Equal(initialRestartCount),
				"The sriov-network-config-daemon pod restarted unexpectedly after applying the SR-IOV configuration")

			By("Creating SR-IOV network")
			_, err = sriov.NewNetworkBuilder(
				APIClient, sriovAndResourceNameSecureBoot, NetConfig.SriovOperatorNamespace,
				tsparams.TestNamespaceName, sriovAndResourceNameSecureBoot).WithStaticIpam().
				WithIPAddressSupport().WithLogLevel(netparam.LogLevelDebug).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")

			By("Creating test pods and checking connectivity")
			err = sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Definition.Name, workerNodeList[0].Definition.Name,
				sriovAndResourceNameSecureBoot, sriovAndResourceNameSecureBoot, "", "",
				[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
			Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")

			By("Removing SR-IOV node policy")
			err = sriovPolicy.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV policy")

			err = netenv.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
				NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for MCP and SR-IOV update")

			By("Validation that totalvfs is still the same after the policy removal")
			sriovNodeState := sriov.NewNetworkNodeStateBuilder(
				APIClient, workerNodeList[0].Object.Name, NetConfig.SriovOperatorNamespace)
			err = sriovNodeState.Discover()
			Expect(err).ToNot(HaveOccurred(), "Failed to discover SR-IOV node state")

			currentTotalVFs, err := sriovNodeState.GetTotalVFs(sriovInterfacesUnderTest[0])
			Expect(err).ToNot(HaveOccurred(), "Failed to get totalvfs on SR-IOV node")

			Expect(currentTotalVFs).To(BeNumerically("==", totalVFs))
		})
	})

func podRestartCount(podName string) int32 {
	podWorker, err := pod.Pull(APIClient, podName, NetConfig.SriovOperatorNamespace)

	Expect(err).ToNot(HaveOccurred(), "Failed to pull the pod")

	return podWorker.Object.Status.ContainerStatuses[0].RestartCount
}
