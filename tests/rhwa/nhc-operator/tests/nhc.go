package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/nhc-operator/internal/nhcparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"NHC Post Deployment tests",
	Ordered,
	ContinueOnFailure,
	Label(nhcparams.Label), func() {
		BeforeAll(func() {
			By("Get NHC deployment object")

			nhcDeployment, err := deployment.Pull(
				APIClient, nhcparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NHC deployment")

			By("Verify NHC deployment is Ready")
			Expect(nhcDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(), "NHC deployment is not Ready")
		})

		It("Verify Node Health Check Operator pod is running", func() {
			listOptions := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", nhcparams.OperatorControllerPodLabel),
			}
			controllerPods, err := pod.WaitForAllPodsInNamespaceRunning(
				APIClient,
				rhwaparams.RhwaOperatorNs,
				rhwaparams.DefaultTimeout,
				listOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Pod is not ready")
			Expect(controllerPods).To(BeTrue(), "no controller pod matched selector for OperatorControllerPodLabel")
		})
	})
