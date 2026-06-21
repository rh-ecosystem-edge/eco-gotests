package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/inclusterbuild/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron In-Cluster Build Tests", Ordered,
	Label(params.Label, params.LabelSuite, params.InClusterBuildLabel), func() {
		Context("Module with in-cluster build", Label(tsparams.LabelSuite), func() {
			neuronConfig := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				By("Verifying in-cluster build configuration")

				if !neuronConfig.IsValid() {
					Skip("Neuron configuration is not valid - DriverVersion and DevicePluginImage are required")
				}

				if !neuronConfig.IsInClusterBuild() {
					Skip("Not configured for in-cluster build - DriversImage is set, skipping in-cluster build tests")
				}

				By("Verifying all required operators are ready")

				var options *neuronhelpers.NeuronInstallConfigOptions
				if neuronConfig.CatalogSource != "" {
					options = &neuronhelpers.NeuronInstallConfigOptions{
						CatalogSource: neuronhelpers.StringPtr(neuronConfig.CatalogSource),
					}
				}

				Expect(neuronhelpers.AreAllOperatorsReady(APIClient, options)).To(BeTrue(),
					"All operators (NFD, KMM, Neuron) must be pre-installed and ready")

				By("Creating DeviceConfig with in-cluster build (no DriversImage)")

				builder := neuron.NewBuilderWithInClusterBuild(
					APIClient,
					params.DefaultDeviceConfigName,
					params.NeuronNamespace,
					neuronConfig.DriverVersion,
					neuronConfig.DevicePluginImage,
				).WithSelector(map[string]string{
					params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
				}).WithNodeMetricsImage(neuronConfig.NodeMetricsImage)

				if neuronConfig.SchedulerImage != "" && neuronConfig.SchedulerExtensionImage != "" {
					builder = builder.WithScheduler(neuronConfig.SchedulerImage, neuronConfig.SchedulerExtensionImage)
				}

				if neuronConfig.ImageRepoSecretName != "" {
					builder = builder.WithImageRepoSecret(neuronConfig.ImageRepoSecretName)
				}

				if builder.Exists() {
					existingDC, pullErr := neuron.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(pullErr).ToNot(HaveOccurred(), "Failed to pull existing DeviceConfig")

					if existingDC.Definition.Spec.DriversImage != "" {
						klog.V(params.NeuronLogLevel).Infof(
							"Existing DeviceConfig has DriversImage set, recreating for in-cluster build")

						_, deleteErr := existingDC.Delete()
						Expect(deleteErr).ToNot(HaveOccurred(), "Failed to delete existing DeviceConfig")

						Eventually(func() bool {
							_, checkErr := neuron.Pull(
								APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)

							return checkErr != nil
						}, 5*time.Minute, 5*time.Second).Should(BeTrue(),
							"DeviceConfig should be fully deleted")

						_, createErr := builder.Create()
						Expect(createErr).ToNot(HaveOccurred(), "Failed to create DeviceConfig for in-cluster build")
					}
				} else {
					_, err := builder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create DeviceConfig for in-cluster build")
				}

				By("Waiting for cluster stability after DeviceConfig")

				err := neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Cluster not stable after DeviceConfig")

				By("Waiting for Neuron nodes to be labeled")

				err = await.NeuronNodesLabeled(APIClient, tsparams.OperatorDeployTimeout)
				Expect(err).ToNot(HaveOccurred(), "No Neuron-labeled nodes found")
			})

			AfterAll(func() {
				By("Cleaning up in-cluster build test resources")

				nsBuilder := namespace.NewBuilder(APIClient, tsparams.InClusterBuildTestNamespace)
				if nsBuilder.Exists() {
					err := nsBuilder.Delete()
					if err != nil {
						klog.V(params.NeuronLogLevel).Infof("Failed to delete test namespace: %v", err)
					}
				}
			})

			It("should create the Dockerfile ConfigMap", reportxml.ID("88201"), func() {
				By("Waiting for the build ConfigMap to be created by the operator")

				err := await.BuildConfigMapCreated(
					APIClient,
					params.NeuronNamespace,
					params.DefaultDeviceConfigName,
					tsparams.BuildConfigMapTimeout,
				)
				Expect(err).ToNot(HaveOccurred(), "Dockerfile ConfigMap was not created")

				By("Verifying the ConfigMap exists")

				exists, err := check.BuildConfigMapExists(
					APIClient,
					params.NeuronNamespace,
					params.DefaultDeviceConfigName,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "Dockerfile ConfigMap should exist")
			})

			It("should deploy device plugin via in-cluster built driver",
				reportxml.ID("88202"), func() {
					By("Waiting for device plugin deployment to be ready")

					err := await.DevicePluginDeployment(
						APIClient,
						params.NeuronNamespace,
						tsparams.DevicePluginReadyTimeout,
					)
					Expect(err).ToNot(HaveOccurred(), "Device plugin deployment failed with in-cluster build")

					By("Verifying device plugin pods are running on all Neuron nodes")

					running, err := check.DevicePluginPodsRunning(APIClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(running).To(BeTrue(), "Device plugin pods should be running on all Neuron nodes")
				})

			It("should have Neuron device capacity on nodes",
				reportxml.ID("88203"), func() {
					By("Waiting for Neuron resources to be available on all nodes")

					err := await.AllNeuronNodesResourceAvailable(APIClient, tsparams.DevicePluginReadyTimeout)
					Expect(err).ToNot(HaveOccurred(), "Neuron resources not available on nodes")

					By("Verifying Neuron device capacity on each node")

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(neuronNodes).ToNot(BeEmpty(), "Should have at least one Neuron node")

					for _, node := range neuronNodes {
						devices, cores, err := check.GetNeuronCapacity(APIClient, node.Object.Name)
						Expect(err).ToNot(HaveOccurred())
						Expect(devices).To(BeNumerically(">", 0),
							"Node %s should have Neuron device capacity", node.Object.Name)

						klog.V(params.NeuronLogLevel).Infof(
							"Node %s: %d Neuron devices, %d NeuronCores",
							node.Object.Name, devices, cores)
					}
				})
		})
	})
