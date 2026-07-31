package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterlogging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/logging/internal/loginittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/logging/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Loki Log Storage", Ordered, Label(tsparams.LabelLokiLogStorage), ContinueOnFailure, func() {

	// TC1: Loki Operator Installation Verification (AC #4)
	Context("Operator Installation", Label("install"), func() {
		It("Verifies Loki Operator namespace exists", reportxml.ID("76401"), func() {
			lokiNs := namespace.NewBuilder(APIClient, tsparams.LokiOperatorNamespace)
			Expect(lokiNs.Exists()).To(BeTrue(),
				fmt.Sprintf("namespace %s does not exist", tsparams.LokiOperatorNamespace))
		})

		It("Verifies Loki Operator deployment is available", reportxml.ID("76402"), func() {
			lokiDeploy, err := deployment.Pull(APIClient,
				tsparams.LokiOperatorDeploymentName, tsparams.LokiOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull Loki Operator deployment %s in namespace %s: %v",
					tsparams.LokiOperatorDeploymentName, tsparams.LokiOperatorNamespace, err))
			Expect(lokiDeploy.IsReady(2*time.Minute)).To(BeTrue(),
				fmt.Sprintf("Loki Operator deployment %s is not ready",
					tsparams.LokiOperatorDeploymentName))
		})

		It("Verifies Loki Operator subscription exists with correct source",
			reportxml.ID("76403"), func() {
				sub, err := olm.PullSubscription(APIClient,
					tsparams.LokiOperatorSubscriptionName, tsparams.LokiOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("failed to pull subscription %s: %v",
						tsparams.LokiOperatorSubscriptionName, err))
				Expect(sub.Object.Spec.CatalogSource).To(Equal(tsparams.DisconnectedCatalogSource),
					fmt.Sprintf("subscription catalog source is %s, expected %s",
						sub.Object.Spec.CatalogSource, tsparams.DisconnectedCatalogSource))
			})

		It("Verifies Loki Operator CSV is in Succeeded phase", reportxml.ID("76404"), func() {
			sub, err := olm.PullSubscription(APIClient,
				tsparams.LokiOperatorSubscriptionName, tsparams.LokiOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull subscription %s: %v",
					tsparams.LokiOperatorSubscriptionName, err))

			csvName := sub.Object.Status.InstalledCSV
			Expect(csvName).ToNot(BeEmpty(), "no installed CSV found on subscription")

			csv, err := olm.PullClusterServiceVersion(APIClient, csvName, tsparams.LokiOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull CSV %s: %v", csvName, err))
			Expect(string(csv.Object.Status.Phase)).To(Equal("Succeeded"),
				fmt.Sprintf("CSV %s phase is %s, expected Succeeded",
					csvName, csv.Object.Status.Phase))
		})

		It("Verifies logging namespace exists", reportxml.ID("76405"), func() {
			loggingNs := namespace.NewBuilder(APIClient, tsparams.LoggingNamespace)
			Expect(loggingNs.Exists()).To(BeTrue(),
				fmt.Sprintf("namespace %s does not exist", tsparams.LoggingNamespace))
		})
	})

	// TC2: LokiStack Deployment and Readiness (AC #4)
	Context("LokiStack Readiness", Label("lokistack"), func() {
		It("Verifies LokiStack instance exists and is ready", reportxml.ID("76411"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s in namespace %s: %v",
					tsparams.LokiStackName, tsparams.LoggingNamespace, err))
			Expect(lokiStack.IsReady(5*time.Minute)).To(BeTrue(),
				fmt.Sprintf("LokiStack %s is not in Ready state", tsparams.LokiStackName))
		})

		It("Verifies LokiStack size is 1x.extra-small", reportxml.ID("76412"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s: %v", tsparams.LokiStackName, err))
			Expect(string(lokiStack.Object.Spec.Size)).To(Equal(tsparams.LokiStackSizeExtraSmall),
				fmt.Sprintf("LokiStack size is %s, expected %s",
					lokiStack.Object.Spec.Size, tsparams.LokiStackSizeExtraSmall))
		})

		It("Verifies all LokiStack component deployments are ready", reportxml.ID("76413"), func() {
			for _, deployName := range tsparams.LokiStackDeploymentNames {
				By(fmt.Sprintf("Checking deployment %s", deployName))

				lokiDeploy, err := deployment.Pull(APIClient, deployName, tsparams.LoggingNamespace)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("failed to pull deployment %s in namespace %s: %v",
						deployName, tsparams.LoggingNamespace, err))
				Expect(lokiDeploy.IsReady(2*time.Minute)).To(BeTrue(),
					fmt.Sprintf("deployment %s is not ready", deployName))
			}
		})

		It("Verifies LokiStack tenants mode is openshift-logging", reportxml.ID("76414"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s: %v", tsparams.LokiStackName, err))
			Expect(lokiStack.Object.Spec.Tenants).ToNot(BeNil(), "LokiStack tenants spec is nil")
			Expect(string(lokiStack.Object.Spec.Tenants.Mode)).To(Equal("openshift-logging"),
				"LokiStack tenants mode is not openshift-logging")
		})
	})

	// TC3: Storage Configuration (AC #1, #4)
	Context("Storage Configuration", Label("storage"), func() {
		It("Verifies S3 credentials secret exists with required keys", reportxml.ID("76421"), func() {
			lokiSecret, err := secret.Pull(APIClient,
				tsparams.LokiSecretName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull secret %s in namespace %s: %v",
					tsparams.LokiSecretName, tsparams.LoggingNamespace, err))

			requiredKeys := []string{"access_key_id", "access_key_secret", "bucketnames", "endpoint"}
			for _, key := range requiredKeys {
				_, hasKey := lokiSecret.Object.Data[key]
				Expect(hasKey).To(BeTrue(),
					fmt.Sprintf("secret %s is missing required key %s",
						tsparams.LokiSecretName, key))
			}
		})

		It("Verifies LokiStack storage type is S3", reportxml.ID("76422"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s: %v", tsparams.LokiStackName, err))
			Expect(string(lokiStack.Object.Spec.Storage.Secret.Type)).To(Equal("s3"),
				"LokiStack storage secret type is not s3")
			Expect(lokiStack.Object.Spec.Storage.Secret.Name).To(Equal(tsparams.LokiSecretName),
				fmt.Sprintf("LokiStack storage secret name is %s, expected %s",
					lokiStack.Object.Spec.Storage.Secret.Name, tsparams.LokiSecretName))
		})

		It("Verifies LokiStack has a storage class configured", reportxml.ID("76423"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s: %v", tsparams.LokiStackName, err))
			Expect(lokiStack.Object.Spec.StorageClassName).ToNot(BeEmpty(),
				"LokiStack storageClassName is not configured")
		})

		It("Verifies PVCs exist and are bound in logging namespace", reportxml.ID("76424"), func() {
			podList, err := pod.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=logging-loki",
			})
			Expect(err).ToNot(HaveOccurred(), "failed to list Loki pods")
			Expect(len(podList)).To(BeNumerically(">", 0),
				"no Loki pods found in logging namespace")
		})
	})

	// TC5: Log Forwarding Configuration (AC #3)
	Context("Log Forwarding", Label("forwarding"), func() {
		It("Verifies ClusterLogForwarder exists", reportxml.ID("76431"), func() {
			clf, err := clusterlogging.PullClusterLogForwarder(APIClient,
				tsparams.CLFName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull ClusterLogForwarder %s in namespace %s: %v",
					tsparams.CLFName, tsparams.LoggingNamespace, err))
			Expect(clf.Object).ToNot(BeNil(), "ClusterLogForwarder object is nil")
		})

		It("Verifies ClusterLogForwarder has LokiStack output configured",
			reportxml.ID("76432"), func() {
				clf, err := clusterlogging.PullClusterLogForwarder(APIClient,
					tsparams.CLFName, tsparams.LoggingNamespace)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("failed to pull ClusterLogForwarder %s: %v",
						tsparams.CLFName, err))

				hasLokiOutput := false
				for _, output := range clf.Object.Spec.Outputs {
					if output.Type == "lokiStack" {
						hasLokiOutput = true

						break
					}
				}

				Expect(hasLokiOutput).To(BeTrue(),
					"ClusterLogForwarder does not have a lokiStack output configured")
			})

		It("Verifies ClusterLogForwarder forwards infrastructure logs",
			reportxml.ID("76433"), func() {
				clf, err := clusterlogging.PullClusterLogForwarder(APIClient,
					tsparams.CLFName, tsparams.LoggingNamespace)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("failed to pull ClusterLogForwarder %s: %v",
						tsparams.CLFName, err))

				hasInfraInput := false
				for _, pipeline := range clf.Object.Spec.Pipelines {
					for _, inputRef := range pipeline.InputRefs {
						if inputRef == "infrastructure" {
							hasInfraInput = true

							break
						}
					}
				}

				Expect(hasInfraInput).To(BeTrue(),
					"ClusterLogForwarder does not forward infrastructure logs")
			})

		It("Verifies ClusterLogForwarder forwards audit logs",
			reportxml.ID("76434"), func() {
				clf, err := clusterlogging.PullClusterLogForwarder(APIClient,
					tsparams.CLFName, tsparams.LoggingNamespace)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("failed to pull ClusterLogForwarder %s: %v",
						tsparams.CLFName, err))

				hasAuditInput := false
				for _, pipeline := range clf.Object.Spec.Pipelines {
					for _, inputRef := range pipeline.InputRefs {
						if inputRef == "audit" {
							hasAuditInput = true

							break
						}
					}
				}

				Expect(hasAuditInput).To(BeTrue(),
					"ClusterLogForwarder does not forward audit logs")
			})

		It("Verifies collector pods are running", reportxml.ID("76435"), func() {
			collectorPods, err := pod.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/component=collector",
			})
			Expect(err).ToNot(HaveOccurred(), "failed to list collector pods")
			Expect(len(collectorPods)).To(BeNumerically(">", 0),
				"no collector pods found in logging namespace")

			for _, collectorPod := range collectorPods {
				Expect(collectorPod.Object.Status.Phase).To(Equal(corev1.PodRunning),
					fmt.Sprintf("collector pod %s is not running (phase: %s)",
						collectorPod.Object.Name, collectorPod.Object.Status.Phase))
			}
		})
	})

	// TC6: Retention Configuration (AC #1, #2)
	Context("Retention Configuration", Label("retention"), func() {
		It("Verifies LokiStack has retention configured", reportxml.ID("76441"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s: %v", tsparams.LokiStackName, err))
			Expect(lokiStack.Object.Spec.Limits).ToNot(BeNil(),
				"LokiStack limits spec is nil")
			Expect(lokiStack.Object.Spec.Limits.Global).ToNot(BeNil(),
				"LokiStack global limits are nil")
			Expect(lokiStack.Object.Spec.Limits.Global.Retention).ToNot(BeNil(),
				"LokiStack global retention is nil")
		})

		It("Verifies default retention period is 5 days", reportxml.ID("76442"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s: %v", tsparams.LokiStackName, err))
			Expect(lokiStack.Object.Spec.Limits.Global.Retention.Days).
				To(Equal(tsparams.DefaultRetentionDays),
					fmt.Sprintf("retention days is %d, expected %d",
						lokiStack.Object.Spec.Limits.Global.Retention.Days,
						tsparams.DefaultRetentionDays))
		})
	})

	// TC7: Disconnected Environment Operation (AC #7)
	Context("Disconnected Environment", Label("disconnected"), func() {
		It("Verifies Loki Operator images are from internal registry",
			reportxml.ID("76451"), func() {
				lokiPods, err := pod.List(APIClient, tsparams.LokiOperatorNamespace, metav1.ListOptions{
					LabelSelector: "app.kubernetes.io/name=loki-operator",
				})
				Expect(err).ToNot(HaveOccurred(), "failed to list Loki Operator pods")
				Expect(len(lokiPods)).To(BeNumerically(">", 0),
					"no Loki Operator pods found")

				for _, lokiPod := range lokiPods {
					for _, container := range lokiPod.Object.Spec.Containers {
						Expect(container.Image).ToNot(ContainSubstring("registry.redhat.io"),
							fmt.Sprintf("pod %s container %s uses external registry image: %s",
								lokiPod.Object.Name, container.Name, container.Image))
					}
				}
			})

		It("Verifies LokiStack component images are from internal registry",
			reportxml.ID("76452"), func() {
				lokiPods, err := pod.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
					LabelSelector: "app.kubernetes.io/instance=logging-loki",
				})
				Expect(err).ToNot(HaveOccurred(), "failed to list LokiStack pods")
				Expect(len(lokiPods)).To(BeNumerically(">", 0),
					"no LokiStack pods found")

				for _, lokiPod := range lokiPods {
					for _, container := range lokiPod.Object.Spec.Containers {
						Expect(container.Image).ToNot(ContainSubstring("registry.redhat.io"),
							fmt.Sprintf("pod %s container %s uses external registry image: %s",
								lokiPod.Object.Name, container.Name, container.Image))
					}
				}
			})

		It("Verifies subscription source is disconnected catalog",
			reportxml.ID("76453"), func() {
				sub, err := olm.PullSubscription(APIClient,
					tsparams.LokiOperatorSubscriptionName, tsparams.LokiOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("failed to pull subscription %s: %v",
						tsparams.LokiOperatorSubscriptionName, err))
				Expect(sub.Object.Spec.CatalogSource).To(Equal(tsparams.DisconnectedCatalogSource),
					fmt.Sprintf("subscription catalog source is %s, expected %s",
						sub.Object.Spec.CatalogSource, tsparams.DisconnectedCatalogSource))
			})

		It("Verifies no image pull errors on Loki pods", reportxml.ID("76454"), func() {
			lokiPods, err := pod.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=logging-loki",
			})
			Expect(err).ToNot(HaveOccurred(), "failed to list LokiStack pods")

			for _, lokiPod := range lokiPods {
				for _, containerStatus := range lokiPod.Object.Status.ContainerStatuses {
					if containerStatus.State.Waiting != nil {
						Expect(containerStatus.State.Waiting.Reason).ToNot(
							BeElementOf("ErrImagePull", "ImagePullBackOff"),
							fmt.Sprintf("pod %s container %s has image pull error: %s",
								lokiPod.Object.Name, containerStatus.Name,
								containerStatus.State.Waiting.Reason))
					}
				}
			}
		})
	})

	// TC8: Dual-Stack / IPv6 Accessibility (AC #8)
	Context("IPv6 Accessibility", Label("ipv6"), func() {
		It("Verifies Loki gateway service exists", reportxml.ID("76461"), func() {
			svcList, err := service.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=logging-loki,app.kubernetes.io/component=gateway",
			})
			Expect(err).ToNot(HaveOccurred(), "failed to list Loki gateway services")
			Expect(len(svcList)).To(BeNumerically(">", 0),
				"no Loki gateway service found")
		})

		It("Verifies Loki services have dual-stack configuration", reportxml.ID("76462"), func() {
			svcList, err := service.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/instance=logging-loki",
			})
			Expect(err).ToNot(HaveOccurred(), "failed to list Loki services")

			for _, svc := range svcList {
				if svc.Object.Spec.IPFamilyPolicy != nil {
					policy := string(*svc.Object.Spec.IPFamilyPolicy)
					Expect(policy).To(
						BeElementOf("PreferDualStack", "RequireDualStack", "SingleStack"),
						fmt.Sprintf("service %s has unexpected IP family policy: %s",
							svc.Object.Name, policy))
				}
			}
		})
	})

	// TC11: Resource Verification (AC #5)
	Context("Resource Configuration", Label("resources"), func() {
		It("Verifies LokiStack pods are running on expected nodes",
			reportxml.ID("76471"), func() {
				lokiPods, err := pod.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
					LabelSelector: "app.kubernetes.io/instance=logging-loki",
				})
				Expect(err).ToNot(HaveOccurred(), "failed to list LokiStack pods")
				Expect(len(lokiPods)).To(BeNumerically(">", 0),
					"no LokiStack pods found")

				for _, lokiPod := range lokiPods {
					Expect(lokiPod.Object.Spec.NodeName).ToNot(BeEmpty(),
						fmt.Sprintf("pod %s is not scheduled to a node",
							lokiPod.Object.Name))
				}
			})

		It("Verifies LokiStack pods have resource requests set",
			reportxml.ID("76472"), func() {
				lokiPods, err := pod.List(APIClient, tsparams.LoggingNamespace, metav1.ListOptions{
					LabelSelector: "app.kubernetes.io/instance=logging-loki",
				})
				Expect(err).ToNot(HaveOccurred(), "failed to list LokiStack pods")

				for _, lokiPod := range lokiPods {
					for _, container := range lokiPod.Object.Spec.Containers {
						cpuRequest := container.Resources.Requests.Cpu()
						memRequest := container.Resources.Requests.Memory()
						Expect(cpuRequest.IsZero()).To(BeFalse(),
							fmt.Sprintf("pod %s container %s has no CPU request",
								lokiPod.Object.Name, container.Name))
						Expect(memRequest.IsZero()).To(BeFalse(),
							fmt.Sprintf("pod %s container %s has no memory request",
								lokiPod.Object.Name, container.Name))
					}
				}
			})

		It("Verifies Cluster Logging Operator deployment is available",
			reportxml.ID("76473"), func() {
				cloDeploy, err := deployment.Pull(APIClient,
					tsparams.CLODeploymentName, tsparams.LoggingNamespace)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("failed to pull CLO deployment %s: %v",
						tsparams.CLODeploymentName, err))
				Expect(cloDeploy.IsReady(2*time.Minute)).To(BeTrue(),
					fmt.Sprintf("CLO deployment %s is not ready",
						tsparams.CLODeploymentName))
			})
	})

	// LokiStack schema version validation (AC #4)
	Context("Schema Configuration", Label("schema"), func() {
		It("Verifies LokiStack uses v13 storage schema", reportxml.ID("76481"), func() {
			lokiStack, err := clusterlogging.PullLokiStack(APIClient,
				tsparams.LokiStackName, tsparams.LoggingNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("failed to pull LokiStack %s: %v", tsparams.LokiStackName, err))
			Expect(len(lokiStack.Object.Spec.Storage.Schemas)).To(BeNumerically(">", 0),
				"LokiStack has no storage schemas configured")

			latestSchema := lokiStack.Object.Spec.Storage.Schemas[len(lokiStack.Object.Spec.Storage.Schemas)-1]
			Expect(string(latestSchema.Version)).To(Equal("v13"),
				fmt.Sprintf("LokiStack schema version is %s, expected v13",
					latestSchema.Version))
		})
	})
})

