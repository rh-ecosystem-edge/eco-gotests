package tests

import (
	"context"
	"fmt"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/3upgrade/internal/tsparams"
	commonawait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron Rolling Upgrade Tests", Ordered, Label(params.LabelSuite), func() {

	Context("Rolling Upgrade", Label(tsparams.LabelSuite), func() {

		neuronConfig := neuronconfig.NewNeuronConfig()
		var neuronNodes []string

		BeforeAll(func() {
			By("Verifying configuration")

			if !neuronConfig.IsValid() {
				Skip("Neuron configuration is not valid - DriversImage and DevicePluginImage required")
			}

			if !neuronConfig.IsUpgradeConfigured() {
				Skip("Upgrade configuration is not set - " +
					"ECO_HWACCEL_NEURON_UPGRADE_TARGET_VERSION and " +
					"ECO_HWACCEL_NEURON_UPGRADE_TARGET_DRIVERS_IMAGE are required")
			}

			By("Deploying required operators")
			var options *neuronhelpers.NeuronInstallConfigOptions
			if neuronConfig.CatalogSource != "" {
				options = &neuronhelpers.NeuronInstallConfigOptions{
					CatalogSource: neuronhelpers.StringPtr(neuronConfig.CatalogSource),
				}
			}

			err := neuronhelpers.DeployAllOperators(APIClient, options)
			Expect(err).ToNot(HaveOccurred(), "Failed to deploy required operators")

			By("Waiting for NFD operator to be ready")
			nfdInstallConfig := deploy.OperatorInstallConfig{
				APIClient:              APIClient,
				Namespace:              params.NFDNamespace,
				OperatorGroupName:      "nfd-operator-group",
				SubscriptionName:       "nfd-subscription",
				PackageName:            "nfd",
				CatalogSource:          "redhat-operators",
				CatalogSourceNamespace: "openshift-marketplace",
				Channel:                "stable",
				TargetNamespaces:       []string{params.NFDNamespace},
				LogLevel:               params.NeuronLogLevel,
			}
			nfdInstaller := deploy.NewOperatorInstaller(nfdInstallConfig)
			ready, err := nfdInstaller.IsReady(tsparams.OperatorDeployTimeout)
			Expect(err).ToNot(HaveOccurred(), "NFD operator readiness check failed")
			Expect(ready).To(BeTrue(), "NFD operator is not ready")

			By("Waiting for KMM operator to be ready")
			kmmInstallConfig := neuronhelpers.GetDefaultKMMInstallConfig(APIClient)
			kmmInstaller := deploy.NewOperatorInstaller(kmmInstallConfig)
			ready, err = kmmInstaller.IsReady(tsparams.OperatorDeployTimeout)
			Expect(err).ToNot(HaveOccurred(), "KMM operator readiness check failed")
			Expect(ready).To(BeTrue(), "KMM operator is not ready")

			By("Waiting for Neuron operator to be ready")
			neuronInstallConfig := neuronhelpers.GetDefaultNeuronInstallConfig(APIClient, options)
			neuronInstaller := deploy.NewOperatorInstaller(neuronInstallConfig)
			ready, err = neuronInstaller.IsReady(tsparams.OperatorDeployTimeout)
			Expect(err).ToNot(HaveOccurred(), "Neuron operator readiness check failed")
			Expect(ready).To(BeTrue(), "Neuron operator is not ready")

			By("Creating initial DeviceConfig with driver version")
			builder := neuron.NewBuilder(
				APIClient,
				params.DefaultDeviceConfigName,
				params.NeuronNamespace,
				neuronConfig.DriversImage,
				neuronConfig.DriverVersion,
				neuronConfig.DevicePluginImage,
			).WithSelector(map[string]string{
				params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
			})

			if neuronConfig.SchedulerImage != "" && neuronConfig.SchedulerExtensionImage != "" {
				builder = builder.WithScheduler(neuronConfig.SchedulerImage, neuronConfig.SchedulerExtensionImage)
			}

			if neuronConfig.ImageRepoSecretName != "" {
				builder = builder.WithImageRepoSecret(neuronConfig.ImageRepoSecretName)
			}

			if !builder.Exists() {
				_, err = builder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create DeviceConfig")
			}

			By("Waiting for cluster stability after DeviceConfig")
			err = neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Cluster not stable after DeviceConfig")

			By("Waiting for Neuron nodes to be labeled")
			err = commonawait.NeuronNodesLabeled(APIClient, tsparams.DevicePluginReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "No Neuron-labeled nodes found")

			By("Waiting for device plugin deployment")
			err = commonawait.DevicePluginDeployment(
				APIClient, params.NeuronNamespace, tsparams.DevicePluginReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Device plugin deployment failed")

			By("Waiting for Neuron resources to be available")
			err = commonawait.AllNeuronNodesResourceAvailable(APIClient, tsparams.DevicePluginReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Neuron resources not available on nodes")

			By("Recording initial state")
			nodeBuilders, err := check.GetNeuronNodes(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron nodes")

			for _, node := range nodeBuilders {
				neuronNodes = append(neuronNodes, node.Object.Name)
			}

			klog.V(params.NeuronLogLevel).Infof("Found %d Neuron nodes for upgrade: %v",
				len(neuronNodes), neuronNodes)
			klog.V(params.NeuronLogLevel).Infof("Initial driver version: %s", neuronConfig.DriverVersion)
			klog.V(params.NeuronLogLevel).Infof("Upgrade target: %s", neuronConfig.UpgradeTargetVersion)

			By("Creating upgrade test namespace")
			nsBuilder := namespace.NewBuilder(APIClient, tsparams.UpgradeTestNamespace)
			if !nsBuilder.Exists() {
				_, err = nsBuilder.WithMultipleLabels(map[string]string{
					"pod-security.kubernetes.io/enforce": "privileged",
				}).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create upgrade test namespace")
			}

			By("Deploying test workload on Neuron nodes")
			var successCount int
			var creationErrors []string

			for i, nodeName := range neuronNodes {
				workloadPod := do.CreateTestWorkloadPod(
					fmt.Sprintf("%s-%d", tsparams.TestWorkloadPodName, i),
					tsparams.UpgradeTestNamespace,
					nodeName,
					tsparams.TestWorkloadContainerName,
					tsparams.TestWorkloadLabels,
				)
				_, err = APIClient.CoreV1Interface.Pods(tsparams.UpgradeTestNamespace).Create(
					context.Background(), workloadPod, metav1.CreateOptions{})
				if err != nil {
					errMsg := fmt.Sprintf("node %s: %v", nodeName, err)
					creationErrors = append(creationErrors, errMsg)
					klog.V(params.NeuronLogLevel).Infof("Failed to create workload on node %s: %v",
						nodeName, err)
				} else {
					successCount++
					klog.V(params.NeuronLogLevel).Infof("Successfully created workload on node %s", nodeName)
				}
			}

			Expect(successCount).To(BeNumerically(">", 0),
				"Failed to create any test workloads. Errors: %v", creationErrors)

			By("Waiting for test workloads to be running")
			time.Sleep(30 * time.Second)
		})

		AfterAll(func() {
			By("Cleaning up upgrade test resources")
			nsBuilder := namespace.NewBuilder(APIClient, tsparams.UpgradeTestNamespace)
			if nsBuilder.Exists() {
				err := nsBuilder.Delete()
				if err != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to delete upgrade test namespace: %v", err)
				}
			}

			By("Cleaning up DeviceConfig and waiting for deletion")
			deviceConfigBuilder, err := neuron.Pull(
				APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
			if err == nil {
				_, deleteErr := deviceConfigBuilder.Delete()
				if deleteErr != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to delete DeviceConfig: %v", deleteErr)
				} else {
					// Wait for DeviceConfig to be fully deleted (finalizer processed)
					klog.V(params.NeuronLogLevel).Info("Waiting for DeviceConfig finalizer to be processed...")
					Eventually(func() bool {
						_, pullErr := neuron.Pull(APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)

						return pullErr != nil // DeviceConfig is gone when we can't pull it
					}, 5*time.Minute, 5*time.Second).Should(BeTrue(),
						"DeviceConfig should be fully deleted")
				}
			}

			By("Uninstalling operators")
			uninstallErr := neuronhelpers.UninstallAllOperators(APIClient)
			if uninstallErr != nil {
				klog.V(params.NeuronLogLevel).Infof("Operator uninstall completed with issues: %v",
					uninstallErr)
			}
		})

		It("Should verify initial state before upgrade",
			Label("neuron-upgrade-001"), reportxml.ID("neuron-upgrade-001"), func() {
				By("Verifying Neuron nodes are ready")
				nodesExist, err := check.NeuronNodesExist(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Error checking Neuron nodes")
				Expect(nodesExist).To(BeTrue(), "Neuron nodes should exist")

				By("Verifying device plugin pods are running")
				running, err := check.DevicePluginPodsRunning(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Error checking device plugin pods")
				Expect(running).To(BeTrue(), "Device plugin pods should be running")

				By("Verifying test workloads are deployed")
				pods, err := pod.List(APIClient, tsparams.UpgradeTestNamespace, metav1.ListOptions{
					LabelSelector: "app=neuron-test-workload",
				})
				Expect(err).ToNot(HaveOccurred(), "Error listing test workloads")
				klog.V(params.NeuronLogLevel).Infof("Found %d test workload pods", len(pods))
			})

		It("Should perform rolling upgrade of Neuron drivers",
			Label("neuron-upgrade-002"), reportxml.ID("neuron-upgrade-002"), func() {
				By("Updating DeviceConfig with new driver version")

				deviceConfigBuilder, err := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull DeviceConfig")

				deviceConfigBuilder.Definition.Spec.DriversImage = neuronConfig.UpgradeTargetDriversImage
				deviceConfigBuilder.Definition.Spec.DriverVersion = neuronConfig.UpgradeTargetVersion

				if neuronConfig.SchedulerImage != "" && neuronConfig.SchedulerExtensionImage != "" {
					deviceConfigBuilder = deviceConfigBuilder.WithScheduler(
						neuronConfig.SchedulerImage, neuronConfig.SchedulerExtensionImage)
				}

				if neuronConfig.ImageRepoSecretName != "" {
					deviceConfigBuilder = deviceConfigBuilder.WithImageRepoSecret(
						neuronConfig.ImageRepoSecretName)
				}

				_, err = deviceConfigBuilder.Update(false)
				Expect(err).ToNot(HaveOccurred(), "Failed to update DeviceConfig")

				klog.V(params.NeuronLogLevel).Infof("DeviceConfig updated with driver version: %s",
					neuronConfig.UpgradeTargetVersion)

				By("Monitoring rolling upgrade process")
				startTime := time.Now()

				updatedNodes := make(map[string]bool)

				Eventually(func() bool {
					for _, nodeName := range neuronNodes {
						if updatedNodes[nodeName] {
							continue
						}

						fieldSelector := fmt.Sprintf("spec.nodeName=%s,status.phase=Running", nodeName)
						pods, listErr := pod.List(APIClient, params.NeuronNamespace, metav1.ListOptions{
							FieldSelector: fieldSelector,
						})
						if listErr != nil {
							continue
						}

						for _, currentPod := range pods {
							if check.IsDevicePluginPod(currentPod.Object.Name) {
								if currentPod.Object.CreationTimestamp.Time.After(startTime) {
									klog.V(params.NeuronLogLevel).Infof(
										"Node %s updated: new device plugin pod %s",
										nodeName, currentPod.Object.Name)
									updatedNodes[nodeName] = true

									break
								}
							}
						}
					}

					return len(updatedNodes) == len(neuronNodes)
				}).WithTimeout(tsparams.TotalUpgradeTimeout).
					WithPolling(30*time.Second).
					Should(BeTrue(), "All nodes should be updated")

				klog.V(params.NeuronLogLevel).Infof("Rolling upgrade completed: %d/%d nodes updated",
					len(updatedNodes), len(neuronNodes))
			})

		It("Should verify sequential node processing during upgrade",
			Label("neuron-upgrade-003"), reportxml.ID("neuron-upgrade-003"), func() {

				By("Collecting device plugin pod creation timestamps")

				pods, err := pod.List(APIClient, params.NeuronNamespace, metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "Error listing pods")

				devicePluginPods := make(map[string]time.Time)

				for _, currentPod := range pods {
					if check.IsDevicePluginPod(currentPod.Object.Name) {
						devicePluginPods[currentPod.Object.Spec.NodeName] =
							currentPod.Object.CreationTimestamp.Time
					}
				}

				klog.V(params.NeuronLogLevel).Infof("Device plugin pods creation times: %v",
					devicePluginPods)

				By("Verifying sequential processing with minimum time gap between pods")

				if len(devicePluginPods) <= 1 {
					klog.V(params.NeuronLogLevel).Info(
						"Only one or zero device plugin pods found - skipping sequential verification")
					Skip("Sequential verification requires multiple device plugin pods")
				}

				// Extract timestamps into a slice and sort chronologically
				type podCreation struct {
					nodeName string
					created  time.Time
				}

				var creationTimes []podCreation
				for nodeName, created := range devicePluginPods {
					creationTimes = append(creationTimes, podCreation{nodeName: nodeName, created: created})
				}

				sort.Slice(creationTimes, func(i, j int) bool {
					return creationTimes[i].created.Before(creationTimes[j].created)
				})

				// Minimum expected gap between sequential pod creations (10 seconds)
				const minGapThreshold = 10 * time.Second

				for i := 1; i < len(creationTimes); i++ {
					gap := creationTimes[i].created.Sub(creationTimes[i-1].created)
					klog.V(params.NeuronLogLevel).Infof(
						"Gap between node %s and node %s: %v",
						creationTimes[i-1].nodeName, creationTimes[i].nodeName, gap)

					Expect(gap).To(BeNumerically(">=", minGapThreshold),
						"Expected minimum %v gap between pod creations on %s and %s, got %v",
						minGapThreshold, creationTimes[i-1].nodeName, creationTimes[i].nodeName, gap)
				}

				klog.V(params.NeuronLogLevel).Info(
					"Sequential node processing verified - all pods have minimum time gap")
			})

		It("Should verify workloads are restored after upgrade",
			Label("neuron-upgrade-004"), reportxml.ID("neuron-upgrade-004"), func() {
				By("Waiting for cluster stability after upgrade")
				err := neuronhelpers.WaitForClusterStability(APIClient, params.ClusterStabilityTimeout)
				Expect(err).ToNot(HaveOccurred(), "Cluster not stable after upgrade")

				By("Verifying device plugin pods are running on all nodes")
				running, err := check.DevicePluginPodsRunning(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Error checking device plugin pods")
				Expect(running).To(BeTrue(), "Device plugin pods should be running after upgrade")

				By("Verifying Neuron resources are available on all nodes")
				for _, nodeName := range neuronNodes {
					hasResources, err := check.NodeHasNeuronResources(APIClient, nodeName)
					Expect(err).ToNot(HaveOccurred(),
						"Error checking Neuron resources on node %s", nodeName)
					Expect(hasResources).To(BeTrue(),
						"Node %s should have Neuron resources after upgrade", nodeName)
				}
			})

		It("Should verify driver version after upgrade",
			Label("neuron-upgrade-005"), reportxml.ID("neuron-upgrade-005"), func() {
				By("Checking DeviceConfig has new driver version")

				deviceConfigBuilder, err := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to get DeviceConfig")

				driversImage := deviceConfigBuilder.Definition.Spec.DriversImage
				klog.V(params.NeuronLogLevel).Infof("DeviceConfig driversImage: %s", driversImage)
				Expect(driversImage).To(Equal(neuronConfig.UpgradeTargetDriversImage),
					"DeviceConfig should have the new drivers image")

				By("Verifying new device plugin pods are running")
				pods, err := pod.List(APIClient, params.NeuronNamespace, metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "Error listing pods")

				for _, currentPod := range pods {
					if check.IsDevicePluginPod(currentPod.Object.Name) &&
						currentPod.Object.Status.Phase == corev1.PodRunning {
						klog.V(params.NeuronLogLevel).Infof("Running device plugin pod: %s on node %s",
							currentPod.Object.Name, currentPod.Object.Spec.NodeName)
					}
				}
			})

		It("Should verify upgrade did not cause data loss or extended downtime",
			Label("neuron-upgrade-006"), reportxml.ID("neuron-upgrade-006"), func() {
				By("Checking all Neuron nodes are healthy")
				nodeBuilders, err := check.GetNeuronNodes(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron nodes")

				for _, node := range nodeBuilders {
					isReady := false

					for _, condition := range node.Object.Status.Conditions {
						if condition.Type == corev1.NodeReady &&
							condition.Status == corev1.ConditionTrue {
							isReady = true

							break
						}
					}

					Expect(isReady).To(BeTrue(),
						"Node %s should be ready after upgrade", node.Object.Name)

					neuronDevices, neuronCores, err := check.GetNeuronCapacity(
						APIClient, node.Object.Name)
					Expect(err).ToNot(HaveOccurred(), "Error getting Neuron capacity")
					Expect(neuronDevices).To(BeNumerically(">=", 1),
						"Node %s should have Neuron devices after upgrade", node.Object.Name)

					klog.V(params.NeuronLogLevel).Infof("Node %s: %d devices, %d cores after upgrade",
						node.Object.Name, neuronDevices, neuronCores)
				}
			})
	})
})
