package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
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
		BeforeAll(func() {
			By("Get FAR deployment object")
			farDeployment, err := deployment.Pull(
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

		It("Verify FAR CSV has required annotations", reportxml.ID("OCP-70637"), func() {
			By("Getting FAR ClusterServiceVersion")
			farCSVs, err := olm.ListClusterServiceVersionWithNamePattern(
				APIClient, "fence-agents-remediation", rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to list FAR ClusterServiceVersions")
			Expect(len(farCSVs)).To(BeNumerically(">", 0), "No FAR ClusterServiceVersion found")

			By("Checking annotation values on FAR CSV")
			farCSV := farCSVs[0]
			Expect(farCSV.Object.Annotations).ToNot(BeNil(), "CSV annotations should not be nil")

			// Check each required annotation
			for annotationKey, expectedValue := range farparams.RequiredAnnotations {
				annotationValue, exists := farCSV.Object.Annotations[annotationKey]
				Expect(exists).To(BeTrue(), fmt.Sprintf("Required annotation '%s' should exist on FAR CSV", annotationKey))
				Expect(annotationValue).To(Equal(expectedValue), fmt.Sprintf("Annotation '%s' should have value '%s'", annotationKey, expectedValue))
			}
		})
	})
