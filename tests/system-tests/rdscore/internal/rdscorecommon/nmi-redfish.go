package rdscorecommon

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish/redfish"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

//nolint:funlen
func triggerNMIRedfish(nodeLabel string) {
	var (
		nodeList []*nodes.Builder
		err      error
		ctx      SpecContext
	)

	if nodeLabel == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node Label is empty. Skipping...")

		Skip("Empty node selector label")
	}

	if len(RDSCoreConfig.NodesCredentialsMap) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("BMC Details not specified")
		Skip("BMC Details not specified. Skipping...")
	}

	By("Retrieve nodes list")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Find nodes matching label %q", nodeLabel)

	Eventually(func() bool {
		nodeList, err = nodes.List(
			APIClient,
			metav1.ListOptions{LabelSelector: nodeLabel},
		)

		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list nodes: %w", err)

			return false
		}

		return len(nodeList) > 0
	}).WithContext(ctx).WithTimeout(1*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
		fmt.Sprintf("Failed to find nodes matching label: %q", nodeLabel))

	for _, node := range nodeList {
		By(fmt.Sprintf("Trigger NMI via RedFish on node %q", node.Definition.Name))
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Triggering NMI via RedFish on %q",
			node.Definition.Name)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			fmt.Sprintf("NodesCredentialsMap:\n\t%#v", RDSCoreConfig.NodesCredentialsMap))

		var bmcClient *bmc.BMC

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			fmt.Sprintf("Creating BMC client for node %s", node.Definition.Name))

		if auth, ok := RDSCoreConfig.NodesCredentialsMap[node.Definition.Name]; !ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				fmt.Sprintf("BMC Details for %q not found", node.Definition.Name))
			Fail(fmt.Sprintf("BMC Details for %q not found", node.Definition.Name))
		} else {
			bmcClient = bmc.New(auth.BMCAddress).
				WithRedfishUser(auth.Username, auth.Password).
				WithRedfishTimeout(6 * time.Minute)
		}

		By(fmt.Sprintf("Sending NMI reset action to %q", node.Definition.Name))

		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				if err := bmcClient.SystemResetAction(redfish.NmiResetType); err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
						fmt.Sprintf("Failed to trigger NMI on %s -> %v", node.Definition.Name, err))

					return false, err
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					fmt.Sprintf("Successfully triggered NMI on %s", node.Definition.Name))

				return true, nil
			})

		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to trigger NMI on node %s", node.Definition.Name))

		waitForNodeToBeNotReady(ctx, node.Definition.Name, 15*time.Second, 25*time.Minute)

		By(fmt.Sprintf("Waiting for node %q to return to Ready state", node.Definition.Name))

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking node %q got into Ready state",
			node.Definition.Name)

		Eventually(func() bool {
			currentNode, err := nodes.Pull(APIClient, node.Definition.Name)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull node %q due to %v",
					node.Definition.Name, err)

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
			"Node hasn't reached Ready state after NMI trigger")

		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Error waiting for node %q to go into Ready state", node.Definition.Name))

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q successfully recovered after NMI", node.Definition.Name)

		verifyVmcoreDumpGenerated(ctx, node.Definition.Name)

		cleanupVarCrashDirectory(ctx, node.Definition.Name)
	}
}

// VerifyNMIRedfishOnControlPlane triggers NMI via RedFish on Control Plane nodes.
func VerifyNMIRedfishOnControlPlane(ctx SpecContext) {
	triggerNMIRedfish(RDSCoreConfig.NMIRedfishCPNodeLabel)
}

// VerifyNMIRedfishOnWorkerMCP triggers NMI via RedFish on nodes in "Worker" MCP.
func VerifyNMIRedfishOnWorkerMCP(ctx SpecContext) {
	triggerNMIRedfish(RDSCoreConfig.NMIRedfishWorkerMCPNodeLabel)
}

// VerifyNMIRedfishOnCNFMCP triggers NMI via RedFish on nodes in "CNF" MCP.
func VerifyNMIRedfishOnCNFMCP(ctx SpecContext) {
	triggerNMIRedfish(RDSCoreConfig.NMIRedfishCNFMCPNodeLabel)
}
