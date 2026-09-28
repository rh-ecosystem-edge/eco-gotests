package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	upgradeTimeout     = 10 * time.Minute
	upgradePollTimeout = 5 * time.Minute
)

var _ = Describe("Neuron DRA Upgrade Tests", Ordered,
	Label(params.Label, params.DRALabel, "dra-upgrade"), func() {
		// DRA-7: DRA Driver Image Upgrade
		Context("DRA driver image upgrade", Label(tsparams.LabelSuite), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			var originalImage string

			var originalDSName string

			BeforeAll(func() {
				if !neuronCfg.IsDRAUpgradeConfigured() {
					Skip("DRA upgrade not configured - ECO_HWACCEL_NEURON_DRA_UPGRADE_DRIVER_IMAGE not set")
				}

				if !neuronCfg.IsValid() {
					Skip("Neuron configuration is not valid")
				}

				By("Verifying all required operators are ready")

				var options *neuronhelpers.NeuronInstallConfigOptions
				if neuronCfg.CatalogSource != "" {
					options = &neuronhelpers.NeuronInstallConfigOptions{
						CatalogSource: neuronhelpers.StringPtr(neuronCfg.CatalogSource),
					}
				}

				Expect(neuronhelpers.AreAllOperatorsReady(APIClient, options)).To(BeTrue(),
					"All operators (NFD, KMM, Neuron) must be pre-installed and ready")

				By("Verifying Neuron nodes exist")

				exists, err := check.NeuronNodesExist(APIClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "At least one Neuron node must exist")

				By("Ensuring DRA-mode DeviceConfig exists")

				if existingDC, _ := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace); existingDC != nil {
					if existingDC.Definition.Spec.DRADriverImage == neuronCfg.DRADriverImage {
						originalImage = neuronCfg.DRADriverImage

						klog.V(params.NeuronLogLevel).Infof(
							"Existing DRA DeviceConfig already uses initial image: %s", originalImage)
					} else {
						By("Deleting existing DeviceConfig to normalize to initial DRA image")

						_, err := existingDC.Delete()
						Expect(err).ToNot(HaveOccurred())

						Eventually(func() bool {
							_, pullErr := neuron.Pull(
								APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)

							return pullErr != nil
						}, upgradePollTimeout, 5*time.Second).Should(BeTrue())
					}
				}

				if originalImage == "" {
					originalImage = neuronCfg.DRADriverImage

					By("Creating DRA DeviceConfig with initial image")

					builder := neuron.NewBuilderWithDRA(
						APIClient,
						params.DefaultDeviceConfigName,
						params.NeuronNamespace,
						neuronCfg.DriversImage,
						neuronCfg.DriverVersion,
						neuronCfg.DRADriverImage,
					).WithSelector(map[string]string{
						params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
					}).WithNodeMetricsImage(neuronCfg.NodeMetricsImage)

					if neuronCfg.ImageRepoSecretName != "" {
						builder = builder.WithImageRepoSecret(neuronCfg.ImageRepoSecretName)
					}

					_, err := builder.Create()
					Expect(err).ToNot(HaveOccurred())

					err = neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
					Expect(err).ToNot(HaveOccurred())
				}

				By("Waiting for DRA DaemonSet to be ready")

				err = await.DRADaemonSet(
					APIClient, params.NeuronNamespace, tsparams.DRADeployTimeout)
				Expect(err).ToNot(HaveOccurred(), "DRA DaemonSet must be ready before upgrade")

				By("Recording original DRA DaemonSet name")

				dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
					params.NeuronNamespace).List(
					context.TODO(), metav1.ListOptions{
						LabelSelector: fmt.Sprintf("%s=%s",
							params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
					})
				Expect(err).ToNot(HaveOccurred())
				Expect(dsList.Items).ToNot(BeEmpty())

				originalDSName = dsList.Items[0].Name

				klog.V(params.NeuronLogLevel).Infof(
					"Original DRA DaemonSet: %s, image: %s", originalDSName, originalImage)
			})

			It("should update Module spec.dra after draDriverImage change",
				reportxml.ID("90515"), func() {
					By("Updating DeviceConfig with new DRA driver image")

					dcBuilder, err := neuron.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())

					dcBuilder.Definition.Spec.DRADriverImage = neuronCfg.UpgradeDRADriverImage

					_, err = dcBuilder.Update(false)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to update DeviceConfig with new DRA driver image")

					By("Verifying Module spec.dra.container.image updated")

					Eventually(func() string {
						module, pullErr := kmm.Pull(
							APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
						if pullErr != nil || module.Object.Spec.DRA == nil {
							return ""
						}

						return module.Object.Spec.DRA.Container.Image
					}, upgradePollTimeout, 5*time.Second).Should(
						Equal(neuronCfg.UpgradeDRADriverImage),
						"Module spec.dra.container.image should match new DRA driver image")
				})

			It("should roll the DRA DaemonSet to the updated image",
				reportxml.ID("90516"), func() {
					By("Waiting for the existing DRA DaemonSet to roll to the upgrade image")

					Eventually(func() bool {
						dsList, listErr := APIClient.K8sClient.AppsV1().DaemonSets(
							params.NeuronNamespace).List(
							context.TODO(), metav1.ListOptions{
								LabelSelector: fmt.Sprintf("%s=%s",
									params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
							})
						if listErr != nil {
							return false
						}

						var upgradedReady bool

						for idx := range dsList.Items {
							currentDS := &dsList.Items[idx]

							if currentDS.Name != originalDSName {
								continue
							}

							if len(currentDS.Spec.Template.Spec.Containers) > 0 &&
								currentDS.Spec.Template.Spec.Containers[0].Image == neuronCfg.UpgradeDRADriverImage &&
								currentDS.Status.ObservedGeneration == currentDS.Generation &&
								currentDS.Status.DesiredNumberScheduled > 0 &&
								currentDS.Status.UpdatedNumberScheduled == currentDS.Status.DesiredNumberScheduled &&
								currentDS.Status.NumberReady == currentDS.Status.DesiredNumberScheduled {
								upgradedReady = true
							}
						}

						return upgradedReady
					}, upgradeTimeout, 10*time.Second).Should(BeTrue(),
						"The existing DRA DaemonSet should roll to the upgrade image and be ready")

					By("Verifying the existing DRA DaemonSet has the upgrade image")

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{
							LabelSelector: fmt.Sprintf("%s=%s",
								params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
						})
					Expect(err).ToNot(HaveOccurred())
					Expect(dsList.Items).ToNot(BeEmpty())

					var upgradedDSFound bool

					for idx := range dsList.Items {
						currentDS := &dsList.Items[idx]
						if currentDS.Name != originalDSName {
							continue
						}

						Expect(currentDS.Spec.Template.Spec.Containers).ToNot(BeEmpty())
						Expect(currentDS.Spec.Template.Spec.Containers[0].Image).To(Equal(
							neuronCfg.UpgradeDRADriverImage),
							"DRA container image should be the upgrade target")

						upgradedDSFound = true

						break
					}

					Expect(upgradedDSFound).To(BeTrue(),
						"The original DRA DaemonSet should remain during an image rollout")

					klog.V(params.NeuronLogLevel).Infof(
						"DRA DaemonSet %s rolled from image %s to %s",
						originalDSName, originalImage, neuronCfg.UpgradeDRADriverImage)
				})

			It("should still have ResourceSlices published after upgrade",
				reportxml.ID("90517"), func() {
					By("Verifying ResourceSlices exist for neuron.aws.com driver")

					slices, err := resource.ListResourceSlicesByDriver(
						APIClient, params.DRADriverName)
					Expect(err).ToNot(HaveOccurred())
					Expect(slices).ToNot(BeEmpty(),
						"ResourceSlices should still be published after DRA upgrade")

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())

					klog.V(params.NeuronLogLevel).Infof(
						"Found %d ResourceSlices for %d Neuron nodes after upgrade",
						len(slices), len(neuronNodes))
				})

			It("should still have DeviceClass after upgrade",
				reportxml.ID("90514"), func() {
					By("Verifying DeviceClass exists")

					err := await.DeviceClassExists(
						APIClient, params.DRADefaultDeviceClassName, tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass should still exist after upgrade")
				})

			It("should report DRA availability in Module status after upgrade",
				reportxml.ID("90518"), func() {
					By("Verifying Module status.dra.availableNumber")

					module, err := kmm.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
					Expect(err).ToNot(HaveOccurred())

					dra := module.Object.Status.DRA
					Expect(dra.AvailableNumber).To(BeNumerically(">", 0),
						"Module status.dra.availableNumber should be > 0 after upgrade")

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())

					Expect(int(dra.AvailableNumber)).To(Equal(len(neuronNodes)),
						"DRA availableNumber should equal Neuron node count after upgrade")
				})
		})

		// DRA-4: Node Metrics Alongside DRA
		Context("Node metrics alongside DRA", Label(tsparams.LabelSuite, "dra-metrics"), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				if !neuronCfg.IsDRAConfigured() {
					Skip("DRA not configured - ECO_HWACCEL_NEURON_DRA_DRIVER_IMAGE not set")
				}

				if !neuronCfg.IsValid() {
					Skip("Neuron configuration is not valid - NodeMetricsImage may be missing")
				}

				By("Verifying DRA DeviceConfig exists")

				dcBuilder, err := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
				Expect(err).ToNot(HaveOccurred(), "DRA DeviceConfig must exist")
				Expect(dcBuilder.Definition.Spec.DRADriverImage).ToNot(BeEmpty(),
					"DeviceConfig must be in DRA mode")
			})

			It("should have node-metrics DaemonSet running alongside DRA",
				reportxml.ID("90519"), func() {
					By("Waiting for metrics DaemonSet to be ready")

					err := await.MetricsDaemonSet(
						APIClient, params.NeuronNamespace, tsparams.DRADeployTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"Node-metrics DaemonSet should be running alongside DRA")
				})

			It("should have metrics pods on all Neuron nodes",
				reportxml.ID("90520"), func() {
					By("Listing metrics DaemonSet")

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())

					metricsFound := false

					for idx := range dsList.Items {
						if strings.HasPrefix(dsList.Items[idx].Name, params.MetricsDaemonSetPrefix) {
							metricsFound = true

							Expect(int(dsList.Items[idx].Status.NumberReady)).To(Equal(len(neuronNodes)),
								"Metrics DaemonSet should have one ready pod per Neuron node")
						}
					}

					Expect(metricsFound).To(BeTrue(),
						"Node-metrics DaemonSet should exist in DRA mode")
				})

			It("should have both DRA and metrics DaemonSets coexisting",
				reportxml.ID("90521"), func() {
					By("Listing all DaemonSets in operator namespace")

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
						params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					draFound := false
					metricsFound := false

					for idx := range dsList.Items {
						daemonSet := &dsList.Items[idx]
						labels := daemonSet.Labels

						if labels != nil && labels[params.DRADaemonSetLabelKey] == params.DRADaemonSetLabelValue {
							draFound = true
						}

						if strings.HasPrefix(daemonSet.Name, params.MetricsDaemonSetPrefix) {
							metricsFound = true
						}
					}

					Expect(draFound).To(BeTrue(), "DRA DaemonSet should exist")
					Expect(metricsFound).To(BeTrue(), "Metrics DaemonSet should exist")

					klog.V(params.NeuronLogLevel).Info(
						"DRA and metrics DaemonSets coexist successfully")
				})
		})
	})
