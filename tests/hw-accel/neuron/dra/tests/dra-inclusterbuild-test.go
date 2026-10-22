package tests

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron DRA In-Cluster Build Tests", Ordered,
	Label(params.Label, params.DRALabel, params.InClusterBuildLabel), func() {
		Context("DRA with an in-cluster kernel driver build",
			Label(tsparams.LabelSuite, tsparams.DRAInClusterBuildLabel), func() {
				neuronConfig := neuronconfig.NewNeuronConfig()

				var originalDeviceConfig *do.DeviceConfigState

				BeforeAll(func() {
					By("Verifying the DRA in-cluster build inputs")

					if !neuronConfig.IsDRAInClusterBuildConfigured() {
						Skip("DRA in-cluster build requires driver version, DRA driver image, and node metrics image")
					}

					By("Verifying Neuron nodes exist")

					exists, err := check.NeuronNodesExist(APIClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(exists).To(BeTrue(), "At least one Neuron node must exist")

					By("Replacing the DeviceConfig with DRA and in-cluster build enabled")

					builder := neuronhelpers.NewDRAInClusterBuildDeviceConfigBuilder(APIClient, neuronConfig)
					Expect(builder).ToNot(BeNil(), "Failed to initialize the DRA in-cluster build DeviceConfig")

					originalDeviceConfig, err = do.ReplaceDeviceConfig(
						APIClient, builder, params.DefaultTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to install the DRA in-cluster build DeviceConfig")

					By("Waiting for the operator and KMM to build and deploy the DRA stack")

					err = neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
					Expect(err).ToNot(HaveOccurred(), "Cluster did not stabilize after replacing DeviceConfig")

					err = await.BuildConfigMapCreated(
						APIClient, params.NeuronNamespace, params.DefaultDeviceConfigName,
						tsparams.DRAInClusterBuildTimeout)
					if err != nil {
						neuronhelpers.LogDRAInClusterBuildDiagnostics(APIClient)
					}

					Expect(err).ToNot(HaveOccurred(), "The Neuron in-cluster build ConfigMap was not created")

					err = await.DRADaemonSet(
						APIClient, params.NeuronNamespace, tsparams.DRAInClusterBuildTimeout)
					if err != nil {
						neuronhelpers.LogDRAInClusterBuildDiagnostics(APIClient)
					}

					Expect(err).ToNot(HaveOccurred(), "The DRA DaemonSet did not become ready after the build")

					err = await.DeviceClassExists(
						APIClient, params.DRADefaultDeviceClassName, tsparams.DeviceClassTimeout)
					Expect(err).ToNot(HaveOccurred(), "The Neuron DeviceClass was not created")

					err = await.DRAResourcesAvailable(APIClient, tsparams.DRAInClusterBuildTimeout)
					Expect(err).ToNot(HaveOccurred(), "The DRA driver did not publish Neuron devices")

					By("Creating the DRA consumer infrastructure")

					nsBuilder := namespace.NewBuilder(APIClient, tsparams.DRAInClusterBuildTestNamespace)
					if !nsBuilder.Exists() {
						_, err = nsBuilder.Create()
						Expect(err).ToNot(HaveOccurred(), "Failed to create the DRA consumer namespace")
					}

					claimTemplate := resource.NewResourceClaimTemplateBuilder(
						APIClient, tsparams.DRAInClusterBuildClaimTemplate,
						tsparams.DRAInClusterBuildTestNamespace).
						WithDeviceRequest(
							params.DRADeviceRequestName, params.DRADefaultDeviceClassName, 1)
					Expect(claimTemplate).ToNot(BeNil())

					if !claimTemplate.Exists() {
						_, err = claimTemplate.Create()
						Expect(err).ToNot(HaveOccurred(), "Failed to create the ResourceClaimTemplate")
					}
				})

				AfterAll(func() {
					By("Cleaning up DRA in-cluster build test resources")

					cleanupErrors := make([]error, 0, 3)

					nsBuilder := namespace.NewBuilder(APIClient, tsparams.DRAInClusterBuildTestNamespace)
					if nsBuilder.Exists() {
						err := nsBuilder.DeleteAndWait(params.DefaultTimeout)
						if err != nil {
							cleanupErrors = append(cleanupErrors,
								fmt.Errorf("failed to delete the DRA consumer namespace: %w", err))
						}
					}

					if originalDeviceConfig != nil {
						err := do.RestoreDeviceConfig(
							APIClient, originalDeviceConfig, params.DefaultTimeout)
						if err != nil {
							cleanupErrors = append(cleanupErrors,
								fmt.Errorf("failed to restore the original DeviceConfig: %w", err))
						}

						err = neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
						if err != nil {
							cleanupErrors = append(cleanupErrors,
								fmt.Errorf("cluster did not stabilize after restoring DeviceConfig: %w", err))
						}
					}

					if cleanupErr := errors.Join(cleanupErrors...); cleanupErr != nil {
						klog.Errorf("DRA in-cluster build cleanup completed with errors: %v", cleanupErr)
					}
				})

				It("should build the Neuron driver in cluster and allocate a device through DRA",
					reportxml.ID("90523"), func() {
						By("Verifying the DeviceConfig and generated Module combine in-cluster build with DRA")

						configured, err := check.DRAInClusterBuildConfigured(APIClient)
						Expect(err).ToNot(HaveOccurred())
						Expect(configured).To(BeTrue(),
							"DeviceConfig and Module should combine in-cluster build with DRA")

						buildConfigMapExists, err := check.BuildConfigMapExists(
							APIClient, params.NeuronNamespace, params.DefaultDeviceConfigName)
						Expect(err).ToNot(HaveOccurred())
						Expect(buildConfigMapExists).To(BeTrue(), "The build ConfigMap should exist")

						By("Creating a ResourceClaim consumer and waiting for device allocation")

						err = do.CreateDRAConsumerPodAndWait(
							APIClient, tsparams.DRAInClusterBuildConsumerPod,
							tsparams.DRAInClusterBuildTestNamespace,
							tsparams.DRAInClusterBuildClaimTemplate, params.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "The DRA consumer pod should reach Running")

						err = await.ResourceClaimAllocatedAndReserved(
							APIClient, tsparams.DRAInClusterBuildTestNamespace, params.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "The ResourceClaim should be allocated and reserved")

						hasDevices, err := check.PodHasNeuronDevices(
							APIClient, tsparams.DRAInClusterBuildConsumerPod,
							tsparams.DRAInClusterBuildTestNamespace)
						Expect(err).ToNot(HaveOccurred())
						Expect(hasDevices).To(BeTrue(), "The DRA consumer should see a Neuron device")
					})
			})
	})
