package rdscorecommon

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"

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
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list all nodes: %s", err)

			return false
		}

		for _, _node := range allNodes {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing node %q", _node.Definition.Name)

			for _, condition := range _node.Object.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString {
					if condition.Status != rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is notReady", _node.Definition.Name)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

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
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\t*** VerifyUngracefulReboot started ***")

	if len(RDSCoreConfig.NodesCredentialsMap) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("BMC Details not specified")
		Skip("BMC Details not specified. Skipping...")
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(
		fmt.Sprintf("NodesCredentialsMap:\n\t%#v", RDSCoreConfig.NodesCredentialsMap))

	var bmcMap = make(map[string]*bmc.BMC)

	for node, auth := range RDSCoreConfig.NodesCredentialsMap {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			fmt.Sprintf("Creating BMC client for node %s", node))
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
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
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(
				fmt.Sprintf("Starting go routine for %s", nodeName))

			defer GinkgoRecover()
			defer wg.Done()

			klog.V(rdscoreparams.RDSCoreLogLevel).Info(
				fmt.Sprintf("[%s] Setting timeout for context", nodeName))

			By(fmt.Sprintf("Querying power state on %s", nodeName))

			klog.V(rdscoreparams.RDSCoreLogLevel).Info(
				fmt.Sprintf("Checking power state on %s", nodeName))

			state, err := client.SystemPowerState()
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to get power state of %s", nodeName))

			klog.V(rdscoreparams.RDSCoreLogLevel).Info(
				fmt.Sprintf("Power state on %s -> %s", nodeName, state))

			Expect(state).To(Equal("On"),
				fmt.Sprintf("Unexpected power state %s", state))

			err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true,
				func(ctx context.Context) (bool, error) {
					if err := client.SystemForceReset(); err != nil {
						klog.V(rdscoreparams.RDSCoreLogLevel).Info(
							fmt.Sprintf("Failed to power cycle %s -> %v", nodeName, err))

						return false, err
					}

					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("Successfully powered cycle %s", nodeName))

					return true, nil
				})

			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to reboot node %s", nodeName))
		}(&waitGroup, node, client)
	}

	By("Wait for all reboots to finish")

	waitGroup.Wait()
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Finished waiting for go routines to finish")
	time.Sleep(1 * time.Minute)

	WaitAllNodesAreReady(ctx)
}

// WaitAllDeploymentsAreAvailable wait for all deployments in all namespaces to be Available.
func WaitAllDeploymentsAreAvailable(ctx SpecContext) {
	By("Checking all deployments")

	Eventually(func() bool {
		allDeployments, err := deployment.ListInAllNamespaces(APIClient, metav1.ListOptions{})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list all deployments: %s", err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			fmt.Sprintf("Found %d deployments", len(allDeployments)))

		var nonAvailableDeployments []*deployment.Builder

		for _, deploy := range allDeployments {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Processing deployment %q in %q namespace", deploy.Definition.Name, deploy.Definition.Namespace)

			for _, condition := range deploy.Object.Status.Conditions {
				if condition.Type == "Available" {
					if condition.Status != rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
							"Deployment %q in %q namespace is NotAvailable", deploy.Definition.Name, deploy.Definition.Namespace)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\tReason: %s", condition.Reason)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\tMessage: %s", condition.Message)

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
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\t*** Starting Soft Reboot Test Suite ***")

	if len(RDSCoreConfig.NodesCredentialsMap) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("BMC Details not specified")
		Skip("BMC Details not specified. Skipping...")
	}

	By("Getting list of all nodes")

	allNodes, err := nodes.List(APIClient, metav1.ListOptions{})

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error listing pods in the cluster: %v", err))
	Expect(len(allNodes)).ToNot(Equal(0), "0 nodes found in the cluster")

	for _, _node := range allNodes {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing node %q", _node.Definition.Name)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cordoning node %q", _node.Definition.Name)
		err := _node.Cordon()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to cordon %q due to %v", _node.Definition.Name, err))

		defer UncordonNode(_node, uncordonNodeInterval, uncordonNodeTimeout)

		time.Sleep(5 * time.Second)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Draining node %q", _node.Definition.Name)
		err = _node.Drain()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to drain %q due to %v", _node.Definition.Name, err))

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			fmt.Sprintf("NodesCredentialsMap:\n\t%#v", RDSCoreConfig.NodesCredentialsMap))

		var bmcClient *bmc.BMC

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			fmt.Sprintf("Creating BMC client for node %s", _node.Definition.Name))

		if auth, ok := RDSCoreConfig.NodesCredentialsMap[_node.Definition.Name]; !ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(
				fmt.Sprintf("BMC Details for %q not found", _node.Definition.Name))
			Fail(fmt.Sprintf("BMC Details for %q not found", _node.Definition.Name))
		} else {
			bmcClient = bmc.New(auth.BMCAddress).
				WithRedfishUser(auth.Username, auth.Password).
				WithRedfishTimeout(6 * time.Minute)
		}

		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if err := bmcClient.SystemForceReset(); err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("Failed to power cycle %s -> %v", _node.Definition.Name, err))

					return false, err
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Info(
					fmt.Sprintf("Successfully powered cycle %s", _node.Definition.Name))

				return true, nil
			})

		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to reboot node %s", _node.Definition.Name))

		By(fmt.Sprintf("Checking node %q got into NotReady", _node.Definition.Name))

		Eventually(func(ctx SpecContext) bool {
			currentNode, err := nodes.Pull(APIClient, _node.Definition.Name)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull node: %v", err)

				return false
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString {
					if condition.Status != rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is notReady", currentNode.Definition.Name)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

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
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error pulling in node: %v", err)

				return false
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString {
					if condition.Status == rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

						return true
					}
				}
			}

			return false
		}).WithTimeout(25*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(BeTrue(),
			"Node hasn't reached Ready state")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Uncordoning node %q", _node.Definition.Name)
		err = _node.Uncordon()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to uncordon %q due to %v", _node.Definition.Name, err))

		time.Sleep(15 * time.Second)
	}
}

// VerifyHardRebootSuite container that contains tests for ungraceful cluster reboot verification.
//
//nolint:funlen
func VerifyHardRebootSuite() {
	Describe(
		"Ungraceful reboot validation",
		Label("rds-core-ungraceful-reboot"), func() {
			BeforeAll(func(ctx SpecContext) {
				By("Creating a workload with CephFS PVC")
				VerifyCephFSPVC(ctx)

				By("Creating a workload with CephFS PVC")
				VerifyCephRBDPVC(ctx)

				By("Creating SR-IOV workloads on the same node")
				VerifySRIOVWorkloadsOnSameNode(ctx)

				By("Creating SR-IOV workloads on different nodes")
				VerifySRIOVWorkloadsOnDifferentNodes(ctx)

				By("Creating MACVLAN workloads on the same node")
				VerifyMacVlanOnSameNode()

				By("Creating MACVLAN workloads on different nodes")
				VerifyMacVlanOnDifferentNodes()

				By("Creating IPVLAN workloads on the same node")
				VerifyIPVlanOnSameNode()

				By("Creating IPVLAN workloads on different nodes")
				VerifyIPVlanOnDifferentNodes()
			})

			It("Verifies ungraceful cluster reboot",
				Label("rds-core-hard-reboot"), reportxml.ID("30020"), VerifyUngracefulReboot)

			It("Verifies all ClusterOperators are Available after ungraceful reboot",
				Label("rds-core-hard-reboot"), reportxml.ID("71868"), func() {
					By("Checking all cluster operators")

					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Waiting for all ClusterOperators to be Available")
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					ok, err := clusteroperator.WaitForAllClusteroperatorsAvailable(
						APIClient, 15*time.Minute, metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "Failed to get cluster operator status")
					Expect(ok).To(BeTrue(), "Some cluster operators not Available")
				})

			It("Removes all pods with UnexpectedAdmissionError", Label("sriov-unexpected-pods"),
				MustPassRepeatedly(3), func(ctx SpecContext) {
					By("Remove any pods in UnexpectedAdmissionError state")

					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Remove pods with UnexpectedAdmissionError status")

					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					listOptions := metav1.ListOptions{
						FieldSelector: "status.phase=Failed",
					}

					var podsList []*pod.Builder

					var err error

					Eventually(func() bool {
						podsList, err = pod.ListInAllNamespaces(APIClient, listOptions)
						if err != nil {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods: %v", err)

							return false
						}

						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pods matching search criteria",
							len(podsList))

						for _, failedPod := range podsList {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q ns matches search criteria",
								failedPod.Definition.Name, failedPod.Definition.Namespace)
						}

						return true
					}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
						"Failed to search for pods with UnexpectedAdmissionError status")

					for _, failedPod := range podsList {
						if failedPod.Definition.Status.Reason == "UnexpectedAdmissionError" {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting pod %q in %q ns",
								failedPod.Definition.Name, failedPod.Definition.Namespace)

							_, err := failedPod.DeleteAndWait(5 * time.Minute)
							Expect(err).ToNot(HaveOccurred(), "could not delete pod in UnexpectedAdmissionError state")
						}
					}
				})

			It("Verifies all deploymentes are available",
				Label("rds-core-hard-reboot"), reportxml.ID("71872"), WaitAllDeploymentsAreAvailable)

			It("Verifies CephFS PVC is still accessible",
				Label("rds-core-hard-reboot-cephfs"), reportxml.ID("71873"), VerifyDataOnCephFSPVC)

			It("Verifies CephRBD PVC is still accessible",
				Label("rds-core-hard-reboot-cephrbd"), reportxml.ID("71990"), VerifyDataOnCephRBDPVC)

			It("Verifies CephFS workload is deployable after hard reboot",
				Label("odf-cephfs-pvc"), reportxml.ID("71851"), MustPassRepeatedly(3), VerifyCephFSPVC)

			It("Verifies CephRBD workload is deployable after hard reboot",
				Label("odf-cephrbd-pvc"), reportxml.ID("71992"), MustPassRepeatedly(3), VerifyCephRBDPVC)

			It("Verifies SR-IOV workloads on different nodes post reboot",
				Label("rds-core-hard-reboot-sriov-different-node"), reportxml.ID("71952"),
				VerifySRIOVConnectivityBetweenDifferentNodes)

			It("Verifies SR-IOV workloads on the same node post reboot",
				Label("rds-core-hard-reboot-sriov-same-node"), reportxml.ID("71951"),
				VerifySRIOVConnectivityOnSameNode)

			It("Verifies MACVLAN workloads on the same node post hard reboot",
				Label("rds-core-post-hard-reboot-macvlan-same-node"), reportxml.ID("72569"),
				VerifyMACVLANConnectivityOnSameNode)

			It("Verifies MACVLAN workloads on different nodes post hard reboot",
				Label("rds-core-post-hard-reboot-macvlan-different-nodes"), reportxml.ID("72568"),
				VerifyMACVLANConnectivityOnSameNode)

			It("Verifies IPVLAN workloads on the same node post hard reboot",
				Label("ipvlan", "verify-ipvlan-same-node"), reportxml.ID("75565"),
				VerifyIPVLANConnectivityOnSameNode)

			It("Verifies IPVLAN workloads on different nodes post hard reboot",
				Label("ipvlan", "verify-ipvlan-different-nodes"), reportxml.ID("75059"),
				VerifyIPVLANConnectivityBetweenDifferentNodes)
		})
}

// VerifyGracefulRebootSuite container that contains tests for graceful reboot verification.
//
//nolint:funlen
func VerifyGracefulRebootSuite() {
	Describe(
		"Graceful reboot validation",
		Label("rds-core-graceful-reboot"), func() {
			BeforeAll(func(ctx SpecContext) {
				By("Creating a workload with CephFS PVC")
				VerifyCephFSPVC(ctx)

				By("Creating a workload with CephFS PVC")
				VerifyCephRBDPVC(ctx)

				By("Creating SR-IOV worklods that run on same node")
				VerifySRIOVWorkloadsOnSameNode(ctx)

				By("Verifying SR-IOV workloads on different nodes")
				VerifySRIOVWorkloadsOnDifferentNodes(ctx)

				By("Creating MACVLAN workloads on the same node")
				VerifyMacVlanOnSameNode()

				By("Creating MACVLAN workloads on different nodes")
				VerifyMacVlanOnDifferentNodes()

				By("Creating IPVLAN workloads on the same node")
				VerifyIPVlanOnSameNode()

				By("Creating IPVLAN workloads on different nodes")
				VerifyIPVlanOnDifferentNodes()
			})

			It("Verifies graceful cluster reboot",
				Label("rds-core-soft-reboot"), reportxml.ID("30021"), VerifySoftReboot)

			It("Verifies all ClusterOperators are Available after ungraceful reboot",
				Label("rds-core-soft-reboot"), reportxml.ID("72040"), func() {
					By("Checking all cluster operators")

					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Waiting for all ClusterOperators to be Available")
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					ok, err := clusteroperator.WaitForAllClusteroperatorsAvailable(
						APIClient, 15*time.Minute, metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "Failed to get cluster operator status")
					Expect(ok).To(BeTrue(), "Some cluster operators not Available")
				})

			It("Verifies all deploymentes are available",
				Label("rds-core-soft-reboot"), reportxml.ID("72041"), WaitAllDeploymentsAreAvailable)

			It("Verifies CephFS PVC is still accessible",
				Label("rds-core-soft-reboot-cephfs"), reportxml.ID("72042"), VerifyDataOnCephFSPVC)

			It("Verifies CephRBD PVC is still accessible",
				Label("rds-core-soft-reboot-cephrbd"), reportxml.ID("72044"), VerifyDataOnCephRBDPVC)

			It("Verifies CephFS workload is deployable after graceful reboot",
				Label("rds-core-sofrt-reboot-odf-cephfs-pvc"), reportxml.ID("72045"), MustPassRepeatedly(3), VerifyCephFSPVC)

			It("Verifies CephRBD workload is deployable after graceful reboot",
				Label("rds-core-soft-reboot-odf-cephrbd-pvc"), reportxml.ID("72046"), MustPassRepeatedly(3), VerifyCephRBDPVC)

			It("Verifies SR-IOV workloads on different nodes post graceful reboot",
				Label("rds-core-soft-reboot-sriov-different-node"), reportxml.ID("72039"),
				VerifySRIOVConnectivityBetweenDifferentNodes)

			It("Verifies SR-IOV workloads on the same node post graceful reboot",
				Label("rds-core-soft-reboot-sriov-same-node"), reportxml.ID("72038"),
				VerifySRIOVConnectivityOnSameNode)

			It("Verifies SR-IOV workloads deployable on the same node after graceful reboot",
				Label("rds-core-soft-reboot-sriov-same-node"), reportxml.ID("72048"), MustPassRepeatedly(3),
				VerifySRIOVWorkloadsOnSameNode)

			It("Verifies SR-IOV workloads deployable on different nodes after graceful reboot",
				Label("rds-core-soft-reboot-sriov-different-node"), reportxml.ID("72049"), MustPassRepeatedly(3),
				VerifySRIOVWorkloadsOnDifferentNodes)

			It("Verifies MACVLAN workloads on the same node post soft reboot",
				Label("rds-core-post-soft-reboot-macvlan-same-node"), reportxml.ID("72571"),
				VerifyMACVLANConnectivityOnSameNode)

			It("Verifies MACVLAN workloads on different nodes post soft reboot",
				Label("rds-core-post-soft-reboot-macvlan-different-nodes"), reportxml.ID("72570"),
				VerifyMACVLANConnectivityOnSameNode)

			It("Verifies IPVLAN workloads on the same node post soft reboot",
				Label("ipvlan", "verify-ipvlan-same-node"), reportxml.ID("75564"),
				VerifyIPVLANConnectivityOnSameNode)

			It("Verifies IPVLAN workloads on different nodes post soft reboot",
				Label("ipvlan", "verify-ipvlan-different-nodes"), reportxml.ID("75058"),
				VerifyIPVLANConnectivityBetweenDifferentNodes)
		})
}
