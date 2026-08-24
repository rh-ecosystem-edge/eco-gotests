package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	neuronscheme "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/neuron/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron DRA DeviceConfig Tests", Ordered,
	Label(params.Label, params.DRALabel), func() {
		Context("DRA mode setup and verification", Label(tsparams.LabelSuite), func() {
			neuronConfig := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				By("Verifying DRA configuration")

				if !neuronConfig.IsDRAConfigured() {
					Skip("DRA not configured - ECO_HWACCEL_NEURON_DRA_DRIVER_IMAGE not set")
				}

				if !neuronConfig.IsValid() {
					Skip("Neuron configuration is not valid")
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

				By("Verifying Neuron nodes exist")

				exists, err := check.NeuronNodesExist(APIClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "At least one Neuron node must exist")

				By("Creating DeviceConfig with DRA mode")

				if existingDC, _ := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace); existingDC != nil {
					klog.V(params.NeuronLogLevel).Info("Deleting existing DeviceConfig for DRA test")

					_, err := existingDC.Delete()
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() bool {
						_, pullErr := neuron.Pull(
							APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)

						return pullErr != nil
					}, 5*time.Minute, 5*time.Second).Should(BeTrue(),
						"Existing DeviceConfig should be deleted")
				}

				builder := neuron.NewBuilderWithDRA(
					APIClient,
					params.DefaultDeviceConfigName,
					params.NeuronNamespace,
					neuronConfig.DriversImage,
					neuronConfig.DriverVersion,
					neuronConfig.DRADriverImage,
				).WithSelector(map[string]string{
					params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
				}).WithNodeMetricsImage(neuronConfig.NodeMetricsImage)

				if neuronConfig.ImageRepoSecretName != "" {
					builder = builder.WithImageRepoSecret(neuronConfig.ImageRepoSecretName)
				}

				_, err = builder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create DRA-mode DeviceConfig")

				By("Waiting for cluster stability after DeviceConfig")

				err = neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Cluster not stable after DeviceConfig")
			})

			// DRA-1: DeviceConfig to Module DRA Mapping
			It("should create KMM Module with correct DRA spec",
				reportxml.ID("90390"), func() {
					By("Pulling the KMM Module")

					module, err := kmm.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred(), "KMM Module should exist")

					By("Verifying Module has spec.dra set")

					moduleDRA := module.Object.Spec.DRA
					Expect(moduleDRA).ToNot(BeNil(), "Module spec.dra should be set")

					By("Verifying DRA driver name")

					Expect(moduleDRA.DriverName).To(Equal(params.DRADriverName),
						"DRA driverName should be %s", params.DRADriverName)

					By("Verifying DRA container image")

					Expect(moduleDRA.Container.Image).To(Equal(neuronConfig.DRADriverImage),
						"DRA container image should match draDriverImage")

					By("Verifying DRA service account")

					Expect(moduleDRA.ServiceAccountName).To(Equal(params.DRAServiceAccountName),
						"DRA serviceAccountName should be %s", params.DRAServiceAccountName)

					By("Verifying default DeviceClass in Module spec")

					Expect(moduleDRA.DeviceClasses).To(HaveLen(1),
						"Module should have exactly one default DeviceClass")
					Expect(moduleDRA.DeviceClasses[0].Name).To(Equal(params.DRADefaultDeviceClassName),
						"Default DeviceClass name should be %s", params.DRADefaultDeviceClassName)

					By("Verifying spec.devicePlugin is NOT set")

					Expect(module.Object.Spec.DevicePlugin).To(BeNil(),
						"Module spec.devicePlugin should not be set in DRA mode")
				})

			It("should have DRA DaemonSet running on Neuron nodes",
				reportxml.ID("90391"), func() {
					By("Waiting for DRA DaemonSet to be ready")

					err := await.DRADaemonSet(
						APIClient, params.NeuronNamespace, tsparams.DRADeployTimeout)
					Expect(err).ToNot(HaveOccurred(), "DRA DaemonSet should be ready")

					By("Verifying DRA pods are running on all Neuron nodes")

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{
							LabelSelector: fmt.Sprintf("%s=%s",
								params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
						})
					Expect(err).ToNot(HaveOccurred())
					Expect(dsList.Items).ToNot(BeEmpty(), "DRA DaemonSet should exist")

					draDS := dsList.Items[0]
					Expect(int(draDS.Status.NumberReady)).To(Equal(len(neuronNodes)),
						"DRA DaemonSet should have one ready pod per Neuron node")
				})

			It("should have DeviceClass created",
				reportxml.ID("90392"), func() {
					By("Waiting for default DeviceClass to exist")

					err := await.DeviceClassExists(
						APIClient, params.DRADefaultDeviceClassName, tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass %s should exist", params.DRADefaultDeviceClassName)

					By("Verifying DeviceClass has KMM ownership labels")

					dcBuilder, err := resource.PullDeviceClass(
						APIClient, params.DRADefaultDeviceClassName)
					Expect(err).ToNot(HaveOccurred())

					labels := dcBuilder.Object.Labels
					Expect(labels).ToNot(BeNil(), "DeviceClass should have labels")
					Expect(labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.name", params.DefaultDeviceConfigName))
					Expect(labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.namespace", params.NeuronNamespace))

					klog.V(params.NeuronLogLevel).Infof("DeviceClass labels: %v", labels)
				})

			It("should have ResourceSlices published",
				reportxml.ID("90393"), func() {
					By("Listing ResourceSlices for neuron.aws.com driver")

					slices, err := resource.ListResourceSlicesByDriver(
						APIClient, params.DRADriverName)
					Expect(err).ToNot(HaveOccurred())
					Expect(slices).ToNot(BeEmpty(),
						"ResourceSlices should be published for driver %s", params.DRADriverName)

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())

					klog.V(params.NeuronLogLevel).Infof(
						"Found %d ResourceSlices for %d Neuron nodes",
						len(slices), len(neuronNodes))
				})

			It("should report DRA availability in Module status",
				reportxml.ID("90394"), func() {
					By("Checking Module status.dra")

					module, err := kmm.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())

					dra := module.Object.Status.DRA
					Expect(dra.AvailableNumber).To(BeNumerically(">", 0),
						"Module status.dra.availableNumber should be > 0")

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())

					Expect(int(dra.AvailableNumber)).To(Equal(len(neuronNodes)),
						"DRA availableNumber should equal number of Neuron nodes")
				})

			// DRA-2: No Scheduler in DRA Mode
			It("should NOT deploy custom scheduler in DRA mode",
				reportxml.ID("90395"), func() {
					By("Listing all deployments in operator namespace")

					deployList, err := APIClient.K8sClient.AppsV1().Deployments(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					By("Verifying no scheduler-related deployments exist")

					for _, deploy := range deployList.Items {
						Expect(deploy.Name).ToNot(ContainSubstring("scheduler"),
							"No scheduler deployment should exist in DRA mode, found: %s",
							deploy.Name)
					}
				})

			It("should NOT deploy device-plugin DaemonSet in DRA mode",
				reportxml.ID("90396"), func() {
					By("Listing DaemonSets in operator namespace")

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					By("Verifying no device-plugin DaemonSet exists")

					for _, ds := range dsList.Items {
						Expect(ds.Name).ToNot(HavePrefix(params.DevicePluginDaemonSetPrefix),
							"Device-plugin DaemonSet should not exist in DRA mode, found: %s",
							ds.Name)
					}
				})

			// DRA-3: Custom DeviceClasses via DeviceConfig
			It("should support custom DeviceClasses via DeviceConfig",
				reportxml.ID("90397"), func() {
					By("Patching DeviceConfig with custom DeviceClasses")

					dcBuilder, err := neuron.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())

					customClasses := []neuronscheme.DeviceClassSpec{
						{
							Name: "neuron-training",
							Selectors: []resourcev1.DeviceSelector{
								{CEL: &resourcev1.CELDeviceSelector{
									Expression: fmt.Sprintf("device.driver == %q", params.DRADriverName),
								}},
							},
						},
						{
							Name: "neuron-inference",
							Selectors: []resourcev1.DeviceSelector{
								{CEL: &resourcev1.CELDeviceSelector{
									Expression: fmt.Sprintf("device.driver == %q", params.DRADriverName),
								}},
							},
						},
					}

					dcBuilder = dcBuilder.WithDeviceClasses(customClasses)
					_, err = dcBuilder.Update(false)
					Expect(err).ToNot(HaveOccurred(), "Failed to update DeviceConfig with custom DeviceClasses")

					By("Waiting for custom DeviceClasses to be created")

					err = await.DeviceClassExists(APIClient, "neuron-training", tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(), "DeviceClass neuron-training should exist")

					err = await.DeviceClassExists(APIClient, "neuron-inference", tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(), "DeviceClass neuron-inference should exist")

					By("Verifying default DeviceClass was removed by KMM convergence")

					Eventually(func() bool {
						_, pullErr := resource.PullDeviceClass(
							APIClient, params.DRADefaultDeviceClassName)

						return pullErr != nil
					}, tsparams.DeviceClassTimeout, 5*time.Second).Should(BeTrue(),
						"Default DeviceClass %s should be removed", params.DRADefaultDeviceClassName)

					By("Verifying Module spec has custom DeviceClasses")

					module, err := kmm.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())
					Expect(module.Object.Spec.DRA.DeviceClasses).To(HaveLen(2))

					By("Reverting to default DeviceClass")

					dcBuilder, err = neuron.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())

					dcBuilder.Definition.Spec.DeviceClasses = nil

					_, err = dcBuilder.Update(false)
					Expect(err).ToNot(HaveOccurred(), "Failed to revert DeviceConfig DeviceClasses")

					By("Waiting for default DeviceClass to be restored")

					err = await.DeviceClassExists(
						APIClient, params.DRADefaultDeviceClassName, tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"Default DeviceClass %s should be restored", params.DRADefaultDeviceClassName)

					By("Waiting for custom DeviceClasses to be removed")

					err = await.DeviceClassGone(APIClient, "neuron-training", tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(), "Custom DeviceClass neuron-training should be deleted")

					err = await.DeviceClassGone(APIClient, "neuron-inference", tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(), "Custom DeviceClass neuron-inference should be deleted")
				})
		})
	})
