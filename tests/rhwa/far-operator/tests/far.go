package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/far-operator/internal/farparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"FAR Post Deployment tests",
	Ordered,
	ContinueOnFailure,
	Label(farparams.Label), func() {
		var farDeployment *deployment.Builder

		BeforeAll(func() {
			By("Get FAR deployment object")
			var err error
			farDeployment, err = deployment.Pull(
				APIClient, farparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get FAR deployment")

			By("Verify FAR deployment is Ready")
			Expect(farDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(), "FAR deployment is not Ready")
		})
		It("Verify Fence Agents Remediation Operator pod is running", reportxml.ID("66026"), func() {

			listOptions := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", farparams.OperatorControllerPodLabel),
			}
			_, err := pod.WaitForAllPodsInNamespaceRunning(
				APIClient,
				rhwaparams.RhwaOperatorNs,
				rhwaparams.DefaultTimeout,
				listOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Pod is not ready")
		})

		It("Verify FAR controller manager has correct number of replicas", reportxml.ID("OCP-61222"), func() {
			By("Checking cluster topology")
			infraConfig, err := infrastructure.Pull(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull infrastructure configuration")

			if infraConfig.Object.Status.ControlPlaneTopology == configv1.SingleReplicaTopologyMode {
				Skip("Skipping test on SNO (Single Node OpenShift) cluster")
			}

			By("Checking deployment replicas")
			Expect(farDeployment.Object.Spec.Replicas).ToNot(BeNil(), "Deployment replicas should not be nil")
			Expect(*farDeployment.Object.Spec.Replicas).To(Equal(farparams.ExpectedReplicas),
				"Expected %d replica(s), found %d", farparams.ExpectedReplicas, *farDeployment.Object.Spec.Replicas)

			By("Verifying ready replicas")
			Expect(farDeployment.Object.Status.ReadyReplicas).To(Equal(farparams.ExpectedReplicas),
				"Expected %d ready replica(s), found %d", farparams.ExpectedReplicas, farDeployment.Object.Status.ReadyReplicas)
		})
	})
