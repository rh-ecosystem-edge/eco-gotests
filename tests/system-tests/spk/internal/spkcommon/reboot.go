package spkcommon

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkparams"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// WaitAllNodesAreReady waits for all the nodes in the cluster to report Ready state.
func WaitAllNodesAreReady(ctx SpecContext) {
	By("Checking all nodes are Ready")

	Eventually(func(ctx SpecContext) bool {
		allNodes, err := nodes.List(APIClient, metav1.ListOptions{})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list all nodes: %s", err)

			return false
		}

		for _, _node := range allNodes {
			klog.V(spkparams.SPKLogLevel).Infof("Processing node %q", _node.Definition.Name)

			for _, condition := range _node.Object.Status.Conditions {
				if condition.Type == spkparams.ConditionTypeReadyString {
					if condition.Status != spkparams.ConstantTrueString {
						klog.V(spkparams.SPKLogLevel).Infof("Node %q is notReady", _node.Definition.Name)
						klog.V(spkparams.SPKLogLevel).Infof("  Reason: %s", condition.Reason)

						return false
					}
				}
			}
		}

		return true
	}).WithTimeout(25*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(BeTrue(),
		"Some nodes are notReady")
}

// VerifyUngracefulReboot performs ungraceful reboot of the cluster.
func VerifyUngracefulReboot(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("\t*** VerifyUngracefulReboot started ***")

	if len(SPKConfig.NodesCredentialsMap) == 0 {
		klog.V(spkparams.SPKLogLevel).Infof("BMC Details not specified")
		Skip("BMC Details not specified. Skipping...")
	}

	klog.V(spkparams.SPKLogLevel).Info(
		fmt.Sprintf("NodesCredentialsMap:\n\t%#v", SPKConfig.NodesCredentialsMap))

	var bmcMap = make(map[string]*bmc.BMC)

	for node, auth := range SPKConfig.NodesCredentialsMap {
		klog.V(spkparams.SPKLogLevel).Info(
			fmt.Sprintf("Creating BMC client for node %s", node))
		klog.V(spkparams.SPKLogLevel).Info(
			fmt.Sprintf("BMC Auth %#v", auth))

		bmcClient := bmc.New(auth.BMCAddress).
			WithRedfishUser(auth.Username, auth.Password).
			WithRedfishTimeout(6 * time.Minute)

		bmcMap[node] = bmcClient
	}

	var waitGroup sync.WaitGroup

	for node, client := range bmcMap {
		waitGroup.Add(1)

		go func(wg *sync.WaitGroup, nodeName string, client *bmc.BMC) {
			klog.V(spkparams.SPKLogLevel).Info(
				fmt.Sprintf("Starting go routine for %s", nodeName))

			defer GinkgoRecover()
			defer wg.Done()

			By(fmt.Sprintf("Querying power state on %s", nodeName))

			klog.V(spkparams.SPKLogLevel).Info(
				fmt.Sprintf("Checking power state on %s", nodeName))

			state, err := client.SystemPowerState()
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to login to %s", nodeName))

			klog.V(spkparams.SPKLogLevel).Info(
				fmt.Sprintf("Power state on %s -> %s", nodeName, state))

			Expect(state).To(Equal("On"),
				fmt.Sprintf("Unexpected power state %s", state))

			err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true,
				func(ctx context.Context) (bool, error) {
					if err := client.SystemForceReset(); err != nil {
						klog.V(spkparams.SPKLogLevel).Info(
							fmt.Sprintf("Failed to power cycle %s -> %v", nodeName, err))

						return false, err
					}

					klog.V(spkparams.SPKLogLevel).Info(
						fmt.Sprintf("Successfully powered cycle %s", nodeName))

					return true, nil
				})

			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to reboot node %s", nodeName))
		}(&waitGroup, node, client)
	}

	By("Wait for all reboots to finish")

	waitGroup.Wait()
	klog.V(spkparams.SPKLogLevel).Infof("Finished waiting for go routines to finish")
	time.Sleep(1 * time.Minute)

	WaitAllNodesAreReady(ctx)
}

// WaitAllDeploymentsAreAvailable wait for all deployments in all namespaces to be Available.
func WaitAllDeploymentsAreAvailable(ctx SpecContext) {
	By("Checking all deployments")

	Eventually(func() bool {
		allDeployments, err := deployment.ListInAllNamespaces(APIClient, metav1.ListOptions{})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list all deployments: %s", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Info(
			fmt.Sprintf("Found %d deployments", len(allDeployments)))

		var nonAvailableDeployments []*deployment.Builder

		for _, deploy := range allDeployments {
			klog.V(spkparams.SPKLogLevel).Infof(
				"Processing deployment %q in %q namespace", deploy.Definition.Name, deploy.Definition.Namespace)

			for _, condition := range deploy.Object.Status.Conditions {
				if condition.Type == "Available" {
					if condition.Status != spkparams.ConstantTrueString {
						klog.V(spkparams.SPKLogLevel).Infof(
							"Deployment %q in %q namespace is NotAvailable", deploy.Definition.Name, deploy.Definition.Namespace)
						klog.V(spkparams.SPKLogLevel).Infof("\tReason: %s", condition.Reason)
						klog.V(spkparams.SPKLogLevel).Infof("\tMessage: %s", condition.Message)

						nonAvailableDeployments = append(nonAvailableDeployments, deploy)
					}
				}
			}
		}

		return len(nonAvailableDeployments) == 0
	}).WithTimeout(25*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(BeTrue(),
		"There are non-available deployments") // end Eventually
}

// VerifySoftReboot performs graceful reboot of a cluster with cordoning and draining of individual nodes.
//
//nolint:gocognit,funlen
func VerifySoftReboot(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("\t*** Starting Soft Reboot Test Suite ***")

	if len(SPKConfig.NodesCredentialsMap) == 0 {
		klog.V(spkparams.SPKLogLevel).Infof("BMC Details not specified")
		Skip("BMC Details not specified. Skipping...")
	}

	By("Getting list of all nodes")

	allNodes, err := nodes.List(APIClient, metav1.ListOptions{})

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error listing pods in the cluster: %v", err))
	Expect(len(allNodes)).ToNot(Equal(0), "0 nodes found in the cluster")

	for _, _node := range allNodes {
		klog.V(spkparams.SPKLogLevel).Infof("Processing node %q", _node.Definition.Name)

		klog.V(spkparams.SPKLogLevel).Infof("Cordoning node %q", _node.Definition.Name)
		err := _node.Cordon()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to cordon %q due to %v", _node.Definition.Name, err))
		time.Sleep(5 * time.Second)

		klog.V(spkparams.SPKLogLevel).Infof("Draining node %q", _node.Definition.Name)
		err = _node.Drain()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to drain %q due to %v", _node.Definition.Name, err))

		klog.V(spkparams.SPKLogLevel).Info(
			fmt.Sprintf("NodesCredentialsMap:\n\t%#v", SPKConfig.NodesCredentialsMap))

		var bmcClient *bmc.BMC

		klog.V(spkparams.SPKLogLevel).Info(
			fmt.Sprintf("Creating BMC client for node %s", _node.Definition.Name))

		if auth, ok := SPKConfig.NodesCredentialsMap[_node.Definition.Name]; !ok {
			klog.V(spkparams.SPKLogLevel).Info(
				fmt.Sprintf("BMC Details for %q not found", _node.Definition.Name))
			Fail(fmt.Sprintf("BMC Details for %q not found", _node.Definition.Name))
		} else {
			bmcClient = bmc.New(auth.BMCAddress).
				WithRedfishUser(auth.Username, auth.Password).
				WithRedfishTimeout(6 * time.Minute)
		}

		klog.V(spkparams.SPKLogLevel).Info(
			fmt.Sprintf("[%s] Setting timeout for context", _node.Definition.Name))

		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if err := bmcClient.SystemForceReset(); err != nil {
					klog.V(spkparams.SPKLogLevel).Info(
						fmt.Sprintf("Failed to power cycle %s -> %v", _node.Definition.Name, err))

					return false, err
				}

				klog.V(spkparams.SPKLogLevel).Info(
					fmt.Sprintf("Successfully powered cycle %s", _node.Definition.Name))

				return true, nil
			})

		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to reboot node %s", _node.Definition.Name))

		By(fmt.Sprintf("Checking node %q got into NotReady", _node.Definition.Name))

		Eventually(func(ctx SpecContext) bool {
			currentNode, err := nodes.Pull(APIClient, _node.Definition.Name)
			if err != nil {
				klog.V(spkparams.SPKLogLevel).Infof("Failed to pull node: %v", err)

				return false
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == spkparams.ConditionTypeReadyString {
					if condition.Status != spkparams.ConstantTrueString {
						klog.V(spkparams.SPKLogLevel).Infof("Node %q is notReady", currentNode.Definition.Name)
						klog.V(spkparams.SPKLogLevel).Infof("  Reason: %s", condition.Reason)

						return true
					}
				}
			}

			return false
		}).WithTimeout(25*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(BeTrue(),
			"Node hasn't reached notReady state")

		By(fmt.Sprintf("Checking node %q got into Ready", _node.Definition.Name))

		Eventually(func(ctx SpecContext) bool {
			currentNode, err := nodes.Pull(APIClient, _node.Definition.Name)
			if err != nil {
				klog.V(spkparams.SPKLogLevel).Infof("Error pulling in node: %v", err)

				return false
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == spkparams.ConditionTypeReadyString {
					if condition.Status == spkparams.ConstantTrueString {
						klog.V(spkparams.SPKLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)
						klog.V(spkparams.SPKLogLevel).Infof("  Reason: %s", condition.Reason)

						return true
					}
				}
			}

			return false
		}).WithTimeout(25*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(BeTrue(),
			"Node hasn't reached Ready state")

		klog.V(spkparams.SPKLogLevel).Infof("Uncordoning node %q", _node.Definition.Name)
		err = _node.Uncordon()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to uncordon %q due to %v", _node.Definition.Name, err))

		time.Sleep(15 * time.Second)
	}
}

// VerifyHardRebootSuite container that contains tests for ungraceful cluster reboot verification.
func VerifyHardRebootSuite() {
	Describe(
		"Ungraceful reboot validation",
		Label("spk-ungraceful-reboot"), func() {
			It("Verifies ungraceful cluster reboot",
				Label("spk-hard-reboot"), reportxml.ID("30020"), VerifyUngracefulReboot)

			It("Verifies all ClusterOperators are Available after ungraceful reboot",
				Label("spk-hard-reboot"), reportxml.ID("71868"), func() {
					By("Checking all cluster operators")

					klog.V(spkparams.SPKLogLevel).Infof("Waiting for all ClusterOperators to be Available")
					klog.V(spkparams.SPKLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					ok, err := clusteroperator.WaitForAllClusteroperatorsAvailable(
						APIClient, 15*time.Minute, metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "Failed to get cluster operator status")
					Expect(ok).To(BeTrue(), "Some cluster operators not Available")
				})

			It("Removes all pods with UnexpectedAdmissionError", Label("sriov-unexpected-pods"),
				MustPassRepeatedly(3), func(ctx SpecContext) {
					By("Remove any pods in UnexpectedAdmissionError state")

					klog.V(spkparams.SPKLogLevel).Infof("Remove pods with UnexpectedAdmissionError status")

					klog.V(spkparams.SPKLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					listOptions := metav1.ListOptions{
						FieldSelector: "status.phase=Failed",
					}

					var podsList []*pod.Builder

					var err error

					Eventually(func() bool {
						podsList, err = pod.ListInAllNamespaces(APIClient, listOptions)
						if err != nil {
							klog.V(spkparams.SPKLogLevel).Infof("Failed to list pods: %v", err)

							return false
						}

						klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching search criteria",
							len(podsList))

						for _, failedPod := range podsList {
							klog.V(spkparams.SPKLogLevel).Infof("Pod %q in %q ns matches search criteria",
								failedPod.Definition.Name, failedPod.Definition.Namespace)
						}

						return true
					}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
						"Failed to search for pods with UnexpectedAdmissionError status")

					for _, failedPod := range podsList {
						if failedPod.Definition.Status.Reason == "UnexpectedAdmissionError" {
							klog.V(spkparams.SPKLogLevel).Infof("Deleting pod %q in %q ns",
								failedPod.Definition.Name, failedPod.Definition.Namespace)

							_, err := failedPod.DeleteAndWait(5 * time.Minute)
							Expect(err).ToNot(HaveOccurred(), "could not delete pod in UnexpectedAdmissionError state")
						}
					}
				})

			It("Verifies all deploymentes are available",
				Label("spk-hard-reboot"), reportxml.ID("71872"), WaitAllDeploymentsAreAvailable)
		})
}

// VerifyGracefulRebootSuite container that contains tests for graceful reboot verification.
func VerifyGracefulRebootSuite() {
	Describe(
		"Graceful reboot validation",
		Label("spk-graceful-reboot"), func() {
			It("Verifies graceful cluster reboot",
				Label("spk-soft-reboot"), reportxml.ID("30021"), VerifySoftReboot)

			It("Verifies all ClusterOperators are Available after graceful reboot",
				Label("spk-hard-reboot"), reportxml.ID("72040"), func() {
					By("Checking all cluster operators")

					klog.V(spkparams.SPKLogLevel).Infof("Waiting for all ClusterOperators to be Available")
					klog.V(spkparams.SPKLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					ok, err := clusteroperator.WaitForAllClusteroperatorsAvailable(
						APIClient, 15*time.Minute, metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "Failed to get cluster operator status")
					Expect(ok).To(BeTrue(), "Some cluster operators not Available")
				})

			It("Verifies all deploymentes are available after graceful reboot",
				Label("spk-hard-reboot"), reportxml.ID("72041"), WaitAllDeploymentsAreAvailable)
		})
}

func cleanupStuckContainerPods(nsName string) {
	By("Remove any pods in ContainerCreating state")

	klog.V(spkparams.SPKLogLevel).Infof("Remove pods with ContainerCreating status")

	listOptions := metav1.ListOptions{
		FieldSelector: "status.phase=Pending",
	}

	var (
		podsList []*pod.Builder
		err      error
		ctx      SpecContext
	)

	Eventually(func(ns string) bool {
		podsList, err = pod.List(APIClient, ns, listOptions)
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list pods: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching search criteria",
			len(podsList))

		for _, failedPod := range podsList {
			klog.V(spkparams.SPKLogLevel).Infof("Pod %q in %q ns matches search criteria",
				failedPod.Definition.Name, failedPod.Definition.Namespace)
		}

		return true
	}).WithArguments(nsName).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to search for pods with UnexpectedAdmissionError status")

	for _, failedPod := range podsList {
		klog.V(spkparams.SPKLogLevel).Infof("Deleting pod %q in %q ns",
			failedPod.Definition.Name, failedPod.Definition.Namespace)

		_, err := failedPod.DeleteAndWait(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "could not delete pod in ContainerCreating state")
	}
}

// CleanupStuckContainerPods removes stuck pods.
func CleanupStuckContainerPods(ctx SpecContext) {
	cleanupStuckContainerPods(SPKConfig.SPKDataNS)
	cleanupStuckContainerPods(SPKConfig.SPKDnsNS)
}
