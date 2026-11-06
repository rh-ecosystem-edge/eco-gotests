package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/snr-operator/internal/snrparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"SNR Post Deployment tests",
	Ordered,
	ContinueOnFailure,
	Label(snrparams.Label), func() {
		BeforeAll(func() {
			By("Get SNR deployment object")

			snrDeployment, err := deployment.Pull(
				APIClient, snrparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get SNR deployment")

			By("Verify SNR deployment is Ready")
			Expect(snrDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(), "SNR deployment is not Ready")
		})

		It("Verify Self Node Remediation Operator pod is running", func() {
			listOptions := metav1.ListOptions{
				LabelSelector: snrparams.OperatorControllerPodLabel,
			}

			pods, err := pod.List(APIClient, rhwaparams.RhwaOperatorNs, listOptions)
			Expect(err).ToNot(HaveOccurred(), "Failed to list pods")
			Expect(pods).ToNot(BeEmpty(),
				fmt.Sprintf("No pods found matching label %s", snrparams.OperatorControllerPodLabel))

			allRunning, err := pod.WaitForAllPodsInNamespaceRunning(
				APIClient,
				rhwaparams.RhwaOperatorNs,
				rhwaparams.DefaultTimeout,
				listOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Error waiting for pods to be ready")
			Expect(allRunning).To(BeTrue(), "Not all SNR operator pods are running")
		})
	})
