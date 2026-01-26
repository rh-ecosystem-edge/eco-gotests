package ran_du_system_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/nmi"
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
				By("Retrieve nodes list")

				nodeList, err := nodes.List(
					APIClient,
					metav1.ListOptions{},
				)
				Expect(err).ToNot(HaveOccurred(), "Error listing nodes.")
				Expect(len(nodeList)).ToNot(Equal(0), "No nodes found in the cluster")

				isSNO := len(nodeList) == 1

				if isSNO {
					klog.V(randuparams.RanDuLogLevel).Infof("Detected SNO (Single Node OpenShift) deployment")
				} else {
					klog.V(randuparams.RanDuLogLevel).Infof("Detected multi-node deployment with %d nodes", len(nodeList))
				}

				By("Checking if BMC credentials are configured")

				if len(RanDuTestConfig.NodesCredentialsMap) == 0 {
					klog.V(randuparams.RanDuLogLevel).Infof("BMC Details not specified")
					Skip("BMC Details not specified. Skipping...")
				}

				for _, node := range nodeList {
					auth, ok := RanDuTestConfig.NodesCredentialsMap[node.Definition.Name]
					if !ok {
						klog.V(randuparams.RanDuLogLevel).Infof(
							"BMC Details for %q not found", node.Definition.Name)
						Fail(fmt.Sprintf("BMC Details for %q not found", node.Definition.Name))
					}

					bmcCredentials := nmi.BMCCredentials{
						BMCAddress: auth.BMCAddress,
						Username:   auth.Username,
						Password:   auth.Password,
					}

					By(fmt.Sprintf("Cleaning up /var/crash directory on node %q", node.Definition.Name))

					err = nmi.CleanupVarCrashDirectory(ctx, node.Definition.Name,
						randuparams.RanDuLogLevel, 5*time.Second, 2*time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Failed to cleanup /var/crash on node %s", node.Definition.Name))

					By(fmt.Sprintf("Triggering NMI via Redfish on node %q", node.Definition.Name))

					err = nmi.TriggerNMIViaRedfish(ctx, node.Definition.Name, bmcCredentials,
						randuparams.RanDuLogLevel, 15*time.Second, 6*time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Failed to trigger NMI on node %s", node.Definition.Name))

					By(fmt.Sprintf("Waiting for node %q to become unavailable", node.Definition.Name))

					err = nmi.WaitForNodeToBecomeUnavailable(ctx, APIClient, node.Definition.Name, isSNO,
						randuparams.RanDuLogLevel, 15*time.Second, 25*time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Node %s didn't become unavailable", node.Definition.Name))

					By(fmt.Sprintf("Waiting for node %q to return to Ready state", node.Definition.Name))

					err = nmi.WaitForNodeToBecomeReady(ctx, APIClient, node.Definition.Name, isSNO,
						randuparams.RanDuLogLevel, 15*time.Second, 25*time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Node %s didn't return to Ready state", node.Definition.Name))

					klog.V(randuparams.RanDuLogLevel).Infof("Node %q successfully recovered after NMI",
						node.Definition.Name)

					By(fmt.Sprintf("Verifying vmcore dump was generated on node %q", node.Definition.Name))

					err = nmi.VerifyVmcoreDumpGenerated(ctx, node.Definition.Name,
						randuparams.RanDuLogLevel, 15*time.Second, 5*time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Vmcore dump was not generated on node %s", node.Definition.Name))
				}
			})
	})
