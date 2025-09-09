package ran_du_system_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/reboot"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"KernelCrashKdump",
	Ordered,
	ContinueOnFailure,
	Label("KernelCrashKdump"), func() {
		It("Trigger kernel crash to generate kdump vmcore", reportxml.ID("56216"), Label("KernelCrashKdump"), func() {
			By("Pulling in OpenshiftAPI deployment")

			// pull openshift apiserver deployment object to wait for after the node reboot.
			openshiftAPIDeploy, err := deployment.Pull(APIClient, "apiserver", "openshift-apiserver")

			Expect(err).ToNot(HaveOccurred(), "Failed to pull in Openshift API deployment")

			By("Retrieve nodes list")
			nodeList, err := nodes.List(
				APIClient,
				metav1.ListOptions{},
			)
			Expect(err).ToNot(HaveOccurred(), "Error listing nodes.")

			for _, node := range nodeList {
				By("Trigger kernel crash")
				err = reboot.KernelCrashKdump(node.Definition.Name)
				Expect(err).ToNot(HaveOccurred(), "Error triggering a kernel crash on the node.")

				By("Waiting for the openshift apiserver deployment to be available")
				err = openshiftAPIDeploy.WaitUntilCondition("Available", 5*time.Minute)

				Expect(err).ToNot(HaveOccurred(), "OpenShift API server deployment not Availalble")

				By(fmt.Sprintf("Wait for %d minutes for the cluster resources to reconciliate their state",
					RanDuTestConfig.RebootRecoveryTime))
				time.Sleep(time.Duration(RanDuTestConfig.RebootRecoveryTime) * time.Minute)

				By("Assert vmcore dump was generated")
				cmdToExec := []string{"chroot", "/rootfs", "ls", "/var/crash"}

				coreDumps, err := remote.ExecuteOnNodeWithDebugPod(cmdToExec, node.Definition.Name)
				Expect(err).ToNot(HaveOccurred(), "could not execute command: %s", err)

				Expect(len(strings.Fields(coreDumps))).To(BeNumerically(">=", 1), "error: vmcore dump was not generated")
			}

		})
	})
