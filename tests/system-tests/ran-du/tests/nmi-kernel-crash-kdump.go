package ran_du_system_test

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
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randucommon"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
)

var _ = Describe(
	"NMIKernelCrashKdump",
	Ordered,
	ContinueOnFailure,
	Label("NMIKernelCrashKdump"), func() {
		It("Trigger NMI kernel crash via Redfish to generate kdump vmcore",
			reportxml.ID("85975"), Label("NMIKernelCrashKdump"), func(ctx SpecContext) {
				By("Checking if BMC credentials are configured")

				if len(RanDuTestConfig.NodesCredentialsMap) == 0 {
					klog.V(randuparams.RanDuLogLevel).Infof("BMC Details not specified")
					Skip("BMC Details not specified. Skipping...")
				}

				By("Retrieve nodes list")

				nodeList, err := nodes.List(
					APIClient,
					metav1.ListOptions{},
				)
				Expect(err).ToNot(HaveOccurred(), "Error listing nodes.")
				Expect(len(nodeList)).ToNot(Equal(0), "No nodes found in the cluster")

				for _, node := range nodeList {
					By(fmt.Sprintf("Cleaning up /var/crash directory on node %q", node.Definition.Name))
					randucommon.CleanupVarCrashDirectory(ctx, node.Definition.Name)

					By(fmt.Sprintf("Trigger NMI via Redfish on node %q", node.Definition.Name))
					klog.V(randuparams.RanDuLogLevel).Infof("Triggering NMI via Redfish on %q",
						node.Definition.Name)

					klog.V(randuparams.RanDuLogLevel).Infof(
						"NodesCredentialsMap:\n\t%#v", RanDuTestConfig.NodesCredentialsMap)

					var bmcClient *bmc.BMC

					klog.V(randuparams.RanDuLogLevel).Infof(
						"Creating BMC client for node %s", node.Definition.Name)

					if auth, ok := RanDuTestConfig.NodesCredentialsMap[node.Definition.Name]; !ok {
						klog.V(randuparams.RanDuLogLevel).Infof(
							"BMC Details for %q not found", node.Definition.Name)
						Fail(fmt.Sprintf("BMC Details for %q not found", node.Definition.Name))
					} else {
						bmcClient = bmc.New(auth.BMCAddress).
							WithRedfishUser(auth.Username, auth.Password).
							WithRedfishTimeout(6 * time.Minute)
					}

					By(fmt.Sprintf("Sending NMI reset action to %q", node.Definition.Name))

					err = wait.PollUntilContextTimeout(ctx, 15*time.Second, 6*time.Minute, true,
						func(ctx context.Context) (bool, error) {
							if err := bmcClient.SystemResetAction(redfish.NmiResetType); err != nil {
								klog.V(randuparams.RanDuLogLevel).Infof(
									"Failed to trigger NMI on %s -> %v", node.Definition.Name, err)

								return false, nil
							}

							klog.V(randuparams.RanDuLogLevel).Infof(
								"Successfully triggered NMI on %s", node.Definition.Name)

							return true, nil
						})

					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Failed to trigger NMI on node %s", node.Definition.Name))

					randucommon.WaitForNodeToBeNotReady(ctx, node.Definition.Name, 15*time.Second, 25*time.Minute)

					By(fmt.Sprintf("Waiting for node %q to return to Ready state", node.Definition.Name))

					klog.V(randuparams.RanDuLogLevel).Infof("Checking node %q got into Ready state",
						node.Definition.Name)

					Eventually(func() bool {
						currentNode, err := nodes.Pull(APIClient, node.Definition.Name)
						if err != nil {
							klog.V(randuparams.RanDuLogLevel).Infof("Failed to pull node %q due to %v",
								node.Definition.Name, err)

							return false
						}

						for _, condition := range currentNode.Object.Status.Conditions {
							if condition.Type == randucommon.ConditionTypeReadyString {
								if string(condition.Status) == randucommon.ConstantTrueString {
									klog.V(randuparams.RanDuLogLevel).Infof("Node %q is Ready",
										currentNode.Definition.Name)
									klog.V(randuparams.RanDuLogLevel).Infof("  Reason: %s", condition.Reason)

									return true
								}
							}
						}

						return false
					}).WithTimeout(25*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(BeTrue(),
						"Node hasn't reached Ready state after NMI trigger")

					klog.V(randuparams.RanDuLogLevel).Infof("Node %q successfully recovered after NMI",
						node.Definition.Name)

					randucommon.VerifyVmcoreDumpGenerated(ctx, node.Definition.Name)
				}
			})
	})
