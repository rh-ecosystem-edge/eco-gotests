package tests

import (
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
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
				metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
			Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = SriovOcpConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Verifying if parallel draining tests can be executed on given cluster")

			err = sriovocpenv.DoesClusterHaveEnoughNodes(1, 2)
			if err != nil {
				Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
			}
		})
		BeforeEach(func() {
			By("Configuring SR-IOV")
			createSriovConfigurationParallelDrainOcp(sriovInterfacesUnderTest[0])

			By("Creating test pods and checking connectivity between them")
			Eventually(func() error {
				_, err := nad.Pull(APIClient, sriovAndResourceNameParallelDrain, tsparams.TestNamespaceName)

				return err
			}, tsparams.NADTimeout, time.Second).Should(BeNil(), fmt.Sprintf(
				"Failed to pull NAD %s", sriovAndResourceNameParallelDrain))

			err := sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
				sriovAndResourceNameParallelDrain, sriovAndResourceNameParallelDrain,
				tsparams.TestPodClientMAC, tsparams.TestPodServerMAC,
				[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
			Expect(err).ToNot(HaveOccurred(), "Connectivity check between test pods failed")

			By("Adding pods with terminationGracePeriodSeconds on each worker node")
			createPodWithVFOnEachWorkerOcp(workerNodeList)
		})

		AfterEach(func() {
			removeLabelFromWorkersIfExistsOcp(workerNodeList, testLabel1)
			removeLabelFromWorkersIfExistsOcp(workerNodeList, testLabel2)

			By("Removing SriovNetworkPoolConfigs before SR-IOV config to unblock draining")

			err := sriov.CleanAllPoolConfigs(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SriovNetworkPoolConfigs")

			By("Removing SR-IOV configuration")

			err = sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				SriovOcpConfig.WorkerLabelEnvVar,
				SriovOcpConfig.OcpSriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Cleaning test namespace")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				tsparams.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		It("without SriovNetworkPoolConfig", reportxml.ID("68640"), func() {
			By("Removing test configuration to call draining mechanism")
			removeTestConfigurationOcp()

			By("Validating that nodes are drained one by one")
			Eventually(isDrainingRunningAsExpectedOcp, time.Minute, tsparams.RetryInterval).WithArguments(1).
				Should(BeTrue(), "draining runs not as expected")

			err = sriovoperator.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout,
				SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("without maxUnavailable field", reportxml.ID("68661"), func() {
			By("Creating SriovNetworkPoolConfig without maxUnavailable field")

			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName,
				SriovOcpConfig.OcpSriovOperatorNamespace).
				WithNodeSelector(SriovOcpConfig.WorkerLabelMap).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig without maxUnavailable field.")

			By("Removing test configuration to call draining mechanism")
			removeTestConfigurationOcp()

			By("Validating that nodes are drained all together")
			Eventually(isDrainingRunningAsExpectedOcp, time.Minute, tsparams.RetryInterval).
				WithArguments(len(workerNodeList)).
				Should(BeTrue(), "draining runs not as expected")

			err = sriovoperator.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout,
				SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("1 SriovNetworkPoolConfig: maxUnavailable value is 2", reportxml.ID("68662"), func() {
			By("Validating that the cluster has more than 2 worker nodes")

			if len(workerNodeList) < 3 {
				Skip(fmt.Sprintf("The cluster has less than 3 workers: %d", len(workerNodeList)))
			}

			By("Creating SriovNetworkPoolConfig with maxUnavailable 2")

			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName,
				SriovOcpConfig.OcpSriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromInt32(2)).
				WithNodeSelector(SriovOcpConfig.WorkerLabelMap).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with maxUnavailable 2.")

			By("Removing test configuration to call draining mechanism")
			removeTestConfigurationOcp()

			By("Validating that nodes are drained by 2")
			Eventually(isDrainingRunningAsExpectedOcp, time.Minute, tsparams.RetryInterval).WithArguments(2).
				Should(BeTrue(), "draining runs not as expected")

			err = sriovoperator.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout,
				SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("2 SriovNetworkPoolConfigs", reportxml.ID("68663"), func() {
			By("Validating that the cluster has more than 2 worker nodes")

			if len(workerNodeList) < 3 {
				Skip(fmt.Sprintf("The cluster has less than 3 workers: %d", len(workerNodeList)))
			}

			By("Labeling workers under test with the specified test label")

			_, err = workerNodeList[0].WithNewLabel(sriovocpenv.MapFirstKeyValue(testLabel1)).Update()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to label worker %s with the test label %v",
				workerNodeList[0].Object.Name, testLabel1))
			_, err = workerNodeList[1].WithNewLabel(sriovocpenv.MapFirstKeyValue(testLabel1)).Update()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to label worker %s with the test label %v",
				workerNodeList[1].Object.Name, testLabel1))
			_, err = workerNodeList[2].WithNewLabel(sriovocpenv.MapFirstKeyValue(testLabel2)).Update()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to label worker %s with the test label %v",
				workerNodeList[2].Object.Name, testLabel2))

			By("Creating SriovNetworkPoolConfig with maxUnavailable 2")

			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName,
				SriovOcpConfig.OcpSriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromInt32(2)).
				WithNodeSelector(testLabel1).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with maxUnavailable 2")

			By("Creating SriovNetworkPoolConfig with maxUnavailable 0")

			poolConfig2, err := sriov.NewPoolConfigBuilder(APIClient, poolConfig2Name,
				SriovOcpConfig.OcpSriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromInt32(0)).
				WithNodeSelector(testLabel2).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with maxUnavailable 0")

			By("Removing test configuration to call draining mechanism")
			removeTestConfigurationOcp()

			By("Verifying that two workers are draining, and the third worker remains in an idle state permanently")
			Eventually(isDrainingRunningAsExpectedOcp, time.Minute, tsparams.RetryInterval).WithArguments(2).
				Should(BeTrue(), "draining runs not as expected")

			sriovNodeStateList, err := sriov.ListNetworkNodeState(APIClient,
				SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to collect all SriovNetworkNodeStates")

			thirdWorkerName := workerNodeList[2].Object.Name

			var thirdWorkerStateIdx int

			thirdWorkerFound := false

			for idx, state := range sriovNodeStateList {
				if state.Objects.Name == thirdWorkerName {
					thirdWorkerStateIdx = idx
					thirdWorkerFound = true

					break
				}
			}

			Expect(thirdWorkerFound).To(BeTrue(),
				fmt.Sprintf("No SriovNetworkNodeState found for worker %s", thirdWorkerName))

			Consistently(func() bool {
				err = sriovNodeStateList[thirdWorkerStateIdx].Discover()
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to discover the worker %s",
					sriovNodeStateList[thirdWorkerStateIdx].Objects.Name))

				stateAnnotation := "sriovnetwork.openshift.io/current-state"
				currentState := sriovNodeStateList[thirdWorkerStateIdx].Objects.Annotations[stateAnnotation]
				syncStatus := sriovNodeStateList[thirdWorkerStateIdx].Objects.Status.SyncStatus

				return currentState == "Idle" && syncStatus == "InProgress"
			}, 2*time.Minute, tsparams.RetryInterval).Should(BeTrue(),
				fmt.Sprintf("The third worker is not in an idle and InProgress states forever. His state is: %s,%s",
					sriovNodeStateList[thirdWorkerStateIdx].Objects.Status.SyncStatus,
					sriovNodeStateList[thirdWorkerStateIdx].Objects.Annotations["sriovnetwork.openshift.io/current-state"]))

			By("Removing the test labels from the workers")

			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
			removeLabelFromWorkersIfExistsOcp(workerNodeList, testLabel1)
			removeLabelFromWorkersIfExistsOcp(workerNodeList, testLabel2)

			By("Removing SriovNetworkPoolConfig with maxUnavailable set to 0 and waiting for all workers to drain")

			err = poolConfig2.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SriovNetworkPoolConfig")

			err = sriovoperator.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout,
				SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")
		})

		It("Draining does not remove non SR-IOV pod", reportxml.ID("68664"), func() {
			By("Creating non SR-IOV pod on the first worker")

			nonSriovPod, err := pod.NewBuilder(
				APIClient, "nonsriov", tsparams.TestNamespaceName, SriovOcpConfig.OcpSriovTestContainer).
				DefineOnNode(workerNodeList[0].Object.Name).
				CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to create the non SR-IOV test pod")

			By("Creating SriovNetworkPoolConfig with 100% maxUnavailable field")

			_, err = sriov.NewPoolConfigBuilder(APIClient, poolConfigName,
				SriovOcpConfig.OcpSriovOperatorNamespace).
				WithMaxUnavailable(intstr.FromString("100%")).
				WithNodeSelector(SriovOcpConfig.WorkerLabelMap).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkPoolConfig with 100% maxUnavailable field")

			By("Removing test configuration to call draining mechanism")
			removeTestConfigurationOcp()

			By("Validating that all workers are drained simultaneously")
			Eventually(isDrainingRunningAsExpectedOcp, time.Minute, tsparams.RetryInterval).
				WithArguments(len(workerNodeList)).
				Should(BeTrue(), "draining runs not as expected")

			err = sriovoperator.WaitForSriovStable(APIClient, tsparams.MCOWaitTimeout,
				SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for stable cluster.")

			By("Checking that non SR-IOV pod is still on the first worker")

			if !nonSriovPod.Exists() {
				Fail("Non SR-IOV pod has been removed after the draining process")
			}
		})
	})

func createSriovConfigurationParallelDrainOcp(sriovInterfaceName string) {
	By("Creating SR-IOV policy")

	sriovPolicy := sriov.NewPolicyBuilder(
		APIClient,
		sriovAndResourceNameParallelDrain,
		SriovOcpConfig.OcpSriovOperatorNamespace,
		sriovAndResourceNameParallelDrain,
		5,
		[]string{sriovInterfaceName}, SriovOcpConfig.WorkerLabelMap)

	err := sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
		APIClient,
		SriovOcpConfig.WorkerLabelEnvVar,
		SriovOcpConfig.OcpSriovOperatorNamespace,
		sriovPolicy,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

	By("Creating SR-IOV network")

	sriovNetworkBuilder := sriov.NewNetworkBuilder(APIClient, sriovAndResourceNameParallelDrain,
		SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName, sriovAndResourceNameParallelDrain).
		WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().WithLogLevel("debug")

	_, err = sriovNetworkBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %s", sriovAndResourceNameParallelDrain)
}

func removeTestConfigurationOcp() {
	err := sriovoperator.RemoveAllSriovNetworks(APIClient,
		SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to clean all SR-IOV Networks")
	err = sriov.CleanAllNetworkNodePolicies(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to clean all SR-IOV policies")
}

func isDrainingRunningAsExpectedOcp(expectedConcurrentDrains int) bool {
	sriovNodeStateList, err := sriov.ListNetworkNodeState(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to collect all SriovNetworkNodeStates")
	Expect(len(sriovNodeStateList)).ToNot(Equal(0), "SriovNetworkNodeStates list is empty")

	var inProgressDrainingCount int

	for _, sriovNodeState := range sriovNodeStateList {
		if sriovNodeState.Objects.Status.SyncStatus == "InProgress" &&
			sriovNodeState.Objects.Annotations["sriovnetwork.openshift.io/current-state"] == "Draining" {
			inProgressDrainingCount++
		}
	}

	return inProgressDrainingCount == expectedConcurrentDrains
}

func createPodWithVFOnEachWorkerOcp(workerList []*nodes.Builder) {
	for i, worker := range workerList {
		ipaddress := "192.168.0." + strconv.Itoa(i+3) + "/24"
		secNetwork := pod.StaticIPAnnotation(sriovAndResourceNameParallelDrain, []string{ipaddress})
		_, err := pod.NewBuilder(
			APIClient, "testpod"+worker.Object.Name, tsparams.TestNamespaceName,
			SriovOcpConfig.OcpSriovTestContainer).
			DefineOnNode(worker.Object.Name).WithSecondaryNetwork(secNetwork).
			WithTerminationGracePeriodSeconds(5).
			CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create a pod %s", "testpod"+worker.Object.Name))
	}
}

func removeLabelFromWorkersIfExistsOcp(workerList []*nodes.Builder, label map[string]string) {
	key, value := sriovocpenv.MapFirstKeyValue(label)
	for _, worker := range workerList {
		if _, ok := worker.Object.Labels[key]; ok {
			By(fmt.Sprintf("Removing label with key %s from worker %s", key, worker.Object.Name))
			_, err := worker.RemoveLabel(key, value).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test label")
		}
	}
}
