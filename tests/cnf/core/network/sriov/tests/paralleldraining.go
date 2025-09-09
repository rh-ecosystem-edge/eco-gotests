package tests

import (
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
)

const sriovAndResourceNameParallelDrain = "paralleldraining"

var _ = Describe("ParallelDraining", Ordered, Label(tsparams.LabelParallelDrainingTestCases),
	ContinueOnFailure, func() {
		var (
			sriovInterfacesUnderTest []string
			workerNodeList           []*nodes.Builder
			err                      error
			poolConfigName           = "pool1"
			poolConfig2Name          = "pool2"
			testKey                  = "test"
			testLabel1               = map[string]string{testKey: "label1"}
			testLabel2               = map[string]string{testKey: "label2"}
		)

		BeforeAll(func() {
			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")
			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Verifying if parallel draining tests can be executed on given cluster")
			err = netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 2)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support parallel draining test cases - doesn't have enough nodes")
		})
		BeforeEach(func() {
			By("Configuring SR-IOV")
			createSriovConfigurationParallelDrain(sriovInterfacesUnderTest[0])

			By("Creating test pods and checking connectivity between the them")
			Eventually(func() error {
				_, err := nad.Pull(APIClient, sriovAndResourceNameParallelDrain, tsparams.TestNamespaceName)

				return err
			}, 10*time.Second, 1*time.Second).Should(BeNil(), fmt.Sprintf(
				"Failed to pull NAD %s", sriovAndResourceNameParallelDrain))

			err := sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
				sriovAndResourceNameParallelDrain, sriovAndResourceNameParallelDrain,
				tsparams.ClientMacAddress, tsparams.ServerMacAddress,
				[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
			Expect(err).ToNot(HaveOccurred(), "Connectivity check between test pods failed")

			By("Adding pods with terminationGracePeriodSeconds on each worker node")
			createPodWithVFOnEachWorker(workerNodeList)
		})

		AfterEach(func() {
			removeLabelFromWorkersIfExists(workerNodeList, testLabel1)

			By("Removing SR-IOV configuration")
			err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			err = sriov.CleanAllPoolConfigs(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SriovNetworkPoolConfigs")

			By("Cleaning test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		It("without SriovNetworkPoolConfig", reportxml.ID("68640"), func() {
			By("Removing test configuration to call draining mechanism")
			removeTestConfiguration()

			By("Validating that nodes are drained one by one")
			Eventually(isDrainingRunningAsExpected, time.Minute, tsparams.RetryInterval).WithArguments(1).
				Should(BeTrue(), "draining runs not as expected")

			err = netenv.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("without maxUnavailable field", reportxml.ID("68661"), func() {
			By("Creating SriovNetworkPoolConfig without maxUnavailable field")
			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName, NetConfig.SriovOperatorNamespace).
				WithNodeSelector(NetConfig.WorkerLabelMap).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig without maxUnavailable field.")

			By("Removing test configuration to call draining mechanism")
			removeTestConfiguration()

			By("Validating that nodes are drained all together")
			Eventually(isDrainingRunningAsExpected, time.Minute, tsparams.RetryInterval).WithArguments(len(workerNodeList)).
				Should(BeTrue(), "draining runs not as expected")

			err = netenv.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("1 SriovNetworkPoolConfig: maxUnavailable value is 2", reportxml.ID("68662"), func() {
			By("Validating that the cluster has more 2 worker nodes")
			if len(workerNodeList) < 3 {
				Skip(fmt.Sprintf("The cluster has less than 3 workers: %d", len(workerNodeList)))
			}

			By("Creating SriovNetworkPoolConfig with maxUnavailable 2")
			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName, NetConfig.SriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromInt32(2)).
				WithNodeSelector(NetConfig.WorkerLabelMap).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with maxUnavailable 2.")

			By("Removing test configuration to call draining mechanism")
			removeTestConfiguration()

			By("Validating that nodes are drained by 2")
			Eventually(isDrainingRunningAsExpected, time.Minute, tsparams.RetryInterval).WithArguments(2).
				Should(BeTrue(), "draining runs not as expected")

			err = netenv.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("2 SriovNetworkPoolConfigs", reportxml.ID("68663"), func() {
			By("Validating that the cluster has more 2 worker nodes")
			if len(workerNodeList) < 3 {
				Skip(fmt.Sprintf("The cluster has less than 3 workers: %d", len(workerNodeList)))
			}

			By("Labeling workers under test with the specified test label")
			_, err = workerNodeList[0].WithNewLabel(netenv.MapFirstKeyValue(testLabel1)).Update()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to label worker %s with the test label %v",
				workerNodeList[0].Object.Name, testLabel1))
			_, err = workerNodeList[1].WithNewLabel(netenv.MapFirstKeyValue(testLabel1)).Update()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to label worker %s with the test label %v",
				workerNodeList[1].Object.Name, testLabel1))
			_, err = workerNodeList[2].WithNewLabel(netenv.MapFirstKeyValue(testLabel2)).Update()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to label worker %s with the test label %v",
				workerNodeList[2].Object.Name, testLabel2))

			By("Creating SriovNetworkPoolConfig with maxUnavailable 2")
			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName, NetConfig.SriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromInt32(2)).
				WithNodeSelector(testLabel1).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with maxUnavailable 2")

			By("Creating SriovNetworkPoolConfig with maxUnavailable 0")
			poolConfig2, err := sriov.NewPoolConfigBuilder(APIClient, poolConfig2Name, NetConfig.SriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromInt32(0)).
				WithNodeSelector(testLabel2).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with maxUnavailable 0")

			By("Removing test configuration to call draining mechanism")
			removeTestConfiguration()

			By("Verifying that two workers are draining, and the third worker remains in an idle state permanently")
			Eventually(isDrainingRunningAsExpected, time.Minute, tsparams.RetryInterval).WithArguments(2).
				Should(BeTrue(), "draining runs not as expected")

			sriovNodeStateList, err := sriov.ListNetworkNodeState(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to collect all SriovNetworkNodeStates")

			Consistently(func() bool {
				err = sriovNodeStateList[2].Discover()
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to discover the worker %s",
					sriovNodeStateList[2].Objects.Name))

				return sriovNodeStateList[2].Objects.Annotations["sriovnetwork.openshift.io/current-state"] == "Idle" &&
					sriovNodeStateList[2].Objects.Status.SyncStatus == "InProgress"
			}, 2*time.Minute, tsparams.RetryInterval).Should(BeTrue(),
				fmt.Sprintf("The third worker is not in an idle and InProgress states forever. His state is: %s,%s",
					sriovNodeStateList[2].Objects.Status.SyncStatus,
					sriovNodeStateList[2].Objects.Annotations["sriovnetwork.openshift.io/current-state"]))

			By("Removing the test labels from the workers")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
			removeLabelFromWorkersIfExists(workerNodeList, testLabel1)

			By("Removing SriovNetworkPoolConfig with maxUnavailable set to 0 and waiting for all workers to drain")
			err = poolConfig2.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SriovNetworkPoolConfig")

			err = netenv.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("Draining does not remove non SR-IOV pod", reportxml.ID("68664"), func() {
			By("Creating non SR-IOV pod on the first worker")
			nonSriovPod, err := pod.NewBuilder(
				APIClient, "nonsriov", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
				DefineOnNode(workerNodeList[0].Object.Name).
				CreateAndWaitUntilRunning(netparam.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to create the non SR-IOV test pod")

			By("Creating SriovNetworkPoolConfig with 100% maxUnavailable field")
			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName, NetConfig.SriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromString("100%")).
				WithNodeSelector(NetConfig.WorkerLabelMap).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with 100% maxUnavailable field")

			By("Removing test configuration to call draining mechanism")
			removeTestConfiguration()

			By("Validating that all workers are drained simultaneously")
			Eventually(isDrainingRunningAsExpected, time.Minute, tsparams.RetryInterval).WithArguments(len(workerNodeList)).
				Should(BeTrue(), "draining runs not as expected")

			err = netenv.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")

			By("Checking that non SR-IOV pod is still on the first worker")
			if !nonSriovPod.Exists() {
				Fail("Non SR-IOV pod has been removed after the draining process")
			}
		})
	})

func createSriovConfigurationParallelDrain(sriovInterfaceName string) {
	By("Creating SR-IOV policy")

	sriovPolicy := sriov.NewPolicyBuilder(
		APIClient,
		sriovAndResourceNameParallelDrain,
		NetConfig.SriovOperatorNamespace,
		sriovAndResourceNameParallelDrain,
		5,
		[]string{sriovInterfaceName}, NetConfig.WorkerLabelMap)

	err := sriovenv.CreateSriovPolicyAndWaitUntilItsApplied(sriovPolicy, tsparams.MCOWaitTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

	By("Creating SR-IOV network")

	_, err = sriov.NewNetworkBuilder(APIClient, sriovAndResourceNameParallelDrain, NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName, sriovAndResourceNameParallelDrain).WithStaticIpam().WithMacAddressSupport().
		WithIPAddressSupport().WithLogLevel(netparam.LogLevelDebug).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")
}

func removeTestConfiguration() {
	err := netenv.RemoveAllSriovNetworks()
	Expect(err).ToNot(HaveOccurred(), "Failed to clean all SR-IOV Networks")
	err = sriov.CleanAllNetworkNodePolicies(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to clean all SR-IOV policies")
}

func isDrainingRunningAsExpected(expectedConcurrentDrains int) bool {
	sriovNodeStateList, err := sriov.ListNetworkNodeState(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to collect all SriovNetworkNodeStates")
	Expect(len(sriovNodeStateList)).ToNot(Equal(0), "SriovNetworkNodeStates list is empty")

	var inProgressWithDrainingCompleteCount int

	for _, sriovNodeState := range sriovNodeStateList {
		// Check if syncStatus is "InProgress" and CurrentSyncState is "DrainComplete"
		if sriovNodeState.Objects.Status.SyncStatus == "InProgress" &&
			sriovNodeState.Objects.Annotations["sriovnetwork.openshift.io/current-state"] == "Draining" {
			inProgressWithDrainingCompleteCount++
		}
	}

	return inProgressWithDrainingCompleteCount == expectedConcurrentDrains
}

func createPodWithVFOnEachWorker(workerList []*nodes.Builder) {
	for i, worker := range workerList {
		// 192.168.0.1 and 192.168.0.2 addresses are busy by client and server pods
		ipaddress := "192.168.0." + strconv.Itoa(i+3) + "/24"
		secNetwork := pod.StaticIPAnnotation(sriovAndResourceNameParallelDrain, []string{ipaddress})
		_, err := pod.NewBuilder(
			APIClient, "testpod"+worker.Object.Name, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
			DefineOnNode(worker.Object.Name).WithSecondaryNetwork(secNetwork).
			WithTerminationGracePeriodSeconds(5).
			CreateAndWaitUntilRunning(netparam.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create a pod %s", "testpod-"+worker.Object.Name))
	}
}

func removeLabelFromWorkersIfExists(workerList []*nodes.Builder, label map[string]string) {
	key, value := netenv.MapFirstKeyValue(label)
	for _, worker := range workerList {
		if _, ok := worker.Object.Labels[key]; ok {
			By(fmt.Sprintf("Removing label with key %s from worker %s", key, worker.Object.Name))
			_, err := worker.RemoveLabel(key, value).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test label")
		}
	}
}
