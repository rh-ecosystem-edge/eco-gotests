package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
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

		It("Verify FAR CSV has required annotations", reportxml.ID("70637"), func() {
			By("Getting FAR ClusterServiceVersion")
			farCSVs, err := olm.ListClusterServiceVersionWithNamePattern(
				APIClient, "fence-agents-remediation", rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to list FAR ClusterServiceVersions")
			Expect(len(farCSVs)).To(BeNumerically(">", 0), "At least one FAR ClusterServiceVersion should be found")

			By("Checking annotation values on FAR CSV")
			farCSV := farCSVs[0]
			Expect(farCSV.Object.Annotations).ToNot(BeNil(), "CSV annotations should not be nil")

			// Check each required annotation
			for annotationKey, expectedValue := range farparams.RequiredAnnotations {
				annotationValue, exists := farCSV.Object.Annotations[annotationKey]
				Expect(exists).To(BeTrue(), "Required annotation %q should exist on FAR CSV", annotationKey)
				Expect(annotationValue).To(Equal(expectedValue), "Annotation %q should have value %q", annotationKey, expectedValue)
			}
		})

		It("Verify FAR controller manager has correct number of replicas", reportxml.ID("61222"), func() {
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

		It("Verify FAR container runs as non-root user", reportxml.ID("61208"), func() {
			By("Getting FAR controller pod names")
			listOptions := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", farparams.OperatorControllerPodLabel),
			}
			farPods, err := pod.List(APIClient, rhwaparams.RhwaOperatorNs, listOptions)
			Expect(err).ToNot(HaveOccurred(), "Failed to get FAR controller pods")
			Expect(len(farPods)).To(BeNumerically(">", 0), "At least one FAR controller pod should be found")

			var errorMessages []string

			for _, farPod := range farPods {
				By(fmt.Sprintf("Verifying security context for pod %s", farPod.Object.Name))

				// Check 1: Verify runAsNonRoot security context
				By("Checking pod-level runAsNonRoot security context")
				if farPod.Object.Spec.SecurityContext == nil {
					errorMessages = append(errorMessages,
						fmt.Sprintf("Pod %s has nil SecurityContext", farPod.Object.Name))
				} else if farPod.Object.Spec.SecurityContext.RunAsNonRoot == nil {
					errorMessages = append(errorMessages,
						fmt.Sprintf("Pod %s has nil runAsNonRoot", farPod.Object.Name))
				} else if !*farPod.Object.Spec.SecurityContext.RunAsNonRoot {
					errorMessages = append(errorMessages,
						fmt.Sprintf("Incorrect runAsNonRoot for pod %s. Expected true, found: %v",
							farPod.Object.Name, *farPod.Object.Spec.SecurityContext.RunAsNonRoot))
				}

				// Check 2: Verify all containers' runAsUser
				By("Checking all containers' runAsUser")
				if len(farPod.Object.Spec.Containers) < 1 {
					errorMessages = append(errorMessages,
						fmt.Sprintf("Pod %s has no containers", farPod.Object.Name))
				} else {
					for i, container := range farPod.Object.Spec.Containers {
						if container.SecurityContext != nil && container.SecurityContext.RunAsUser != nil {
							if *container.SecurityContext.RunAsUser == 0 {
								errorMessages = append(errorMessages,
									fmt.Sprintf("Incorrect user running container [%d] in pod %s, expected non 0, found: %d",
										i, farPod.Object.Name, *container.SecurityContext.RunAsUser))
							}
						}
					}
				}
			}

			// Final validation
			if len(errorMessages) > 0 {
				errMsg := "Testing user running FAR container failed due to:\n"
				for _, msg := range errorMessages {
					errMsg += fmt.Sprintf("- %s\n", msg)
				}
				Fail(errMsg)
			}
		})
	})
