package tests

import (
	"context"

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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron DRA vLLM Inference Tests", Ordered,
	Label(params.Label, params.DRALabel), func() {
		Context("vLLM workload with a Neuron ResourceClaim", Label(tsparams.LabelSuite, "dra-vllm"), func() {
			neuronConfig := neuronconfig.NewNeuronConfig()

			var vllmConfig do.VLLMDeploymentConfig

			BeforeAll(func() {
				By("Verifying DRA and vLLM configuration")

				if !neuronConfig.IsDRAConfigured() {
					Skip("DRA not configured - ECO_HWACCEL_NEURON_DRA_DRIVER_IMAGE not set")
				}

				if !neuronConfig.IsVLLMConfigured() {
					Skip("vLLM not configured - ECO_HWACCEL_NEURON_HF_TOKEN not set")
				}

				vllmConfig = do.DefaultVLLMConfig(tsparams.DRAVLLMTestNamespace)
				vllmConfig.ModelName = neuronConfig.ModelName
				vllmConfig.ServedModelName = neuronConfig.ModelName
				vllmConfig.Image = neuronConfig.VLLMImage
				vllmConfig.StorageClassName = neuronConfig.StorageClassName

				By("Verifying the DeviceConfig is in DRA mode")

				err := await.DRADeviceConfig(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace, tsparams.DeviceConfigTimeout)
				Expect(err).ToNot(HaveOccurred(), "DRA DeviceConfig must exist")

				By("Waiting for the DRA driver and DeviceClass")

				err = await.DRADaemonSet(APIClient, params.NeuronNamespace, tsparams.DRADeployTimeout)
				Expect(err).ToNot(HaveOccurred(), "DRA driver DaemonSet should be ready")

				err = await.DeviceClassExists(
					APIClient, params.DRADefaultDeviceClassName, tsparams.DeviceClassTimeout)
				Expect(err).ToNot(HaveOccurred(), "Neuron DeviceClass should exist")

				By("Waiting for the DRA driver to publish Neuron ResourceSlices")

				err = await.DRAResourcesAvailable(APIClient, tsparams.DRADeployTimeout)
				Expect(err).ToNot(HaveOccurred(), "Neuron ResourceSlices should be published")

				_, deviceCount, err := check.SmallestDRANode(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Neuron ResourceSlices should be published")
				Expect(deviceCount).To(BeNumerically(">=", vllmConfig.NeuronDevices),
					"At least one DRA node should have enough Neuron devices")

				By("Creating the vLLM infrastructure")

				nsBuilder := namespace.NewBuilder(APIClient, tsparams.DRAVLLMTestNamespace)
				if !nsBuilder.Exists() {
					_, err = nsBuilder.WithMultipleLabels(map[string]string{
						"pod-security.kubernetes.io/enforce": "privileged",
					}).Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create vLLM namespace")
				}

				rctBuilder := resource.NewResourceClaimTemplateBuilder(
					APIClient, tsparams.DRAVLLMClaimTemplate, tsparams.DRAVLLMTestNamespace).
					WithDeviceRequest(
						params.DRADeviceRequestName,
						params.DRADefaultDeviceClassName,
						int64(vllmConfig.NeuronDevices),
					)
				Expect(rctBuilder).ToNot(BeNil())

				if !rctBuilder.Exists() {
					_, err = rctBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create ResourceClaimTemplate")
				}

				pvc := do.CreateVLLMPVC(vllmConfig)

				_, err = APIClient.CoreV1Interface.PersistentVolumeClaims(
					tsparams.DRAVLLMTestNamespace).Create(
					context.Background(), pvc, metav1.CreateOptions{})
				if err != nil && !apierrors.IsAlreadyExists(err) {
					Expect(err).ToNot(HaveOccurred(), "Failed to create PVC")
				}

				hfSecret := do.CreateHFTokenSecret(
					tsparams.DRAVLLMTestNamespace, vllmConfig.HFSecretName, neuronConfig.HuggingFaceToken)

				_, err = APIClient.CoreV1Interface.Secrets(tsparams.DRAVLLMTestNamespace).Create(
					context.Background(), hfSecret, metav1.CreateOptions{})
				if err != nil && !apierrors.IsAlreadyExists(err) {
					Expect(err).ToNot(HaveOccurred(), "Failed to create Hugging Face token secret")
				}

				serviceConfig := do.VLLMServiceConfig{
					Name:      vllmConfig.Name,
					Namespace: tsparams.DRAVLLMTestNamespace,
					Port:      vllmConfig.Port,
					Labels:    vllmConfig.Labels,
				}
				vllmService := do.CreateVLLMService(serviceConfig)

				_, err = APIClient.CoreV1Interface.Services(tsparams.DRAVLLMTestNamespace).Create(
					context.Background(), vllmService, metav1.CreateOptions{})
				if err != nil && !apierrors.IsAlreadyExists(err) {
					Expect(err).ToNot(HaveOccurred(), "Failed to create vLLM service")
				}

				vllmDeployment := do.CreateDRAVLLMDeployment(
					vllmConfig, tsparams.DRAVLLMClaimTemplate)
				_, err = APIClient.AppsV1Interface.Deployments(tsparams.DRAVLLMTestNamespace).Create(
					context.Background(), vllmDeployment, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to create DRA-aware vLLM deployment")
			})

			AfterAll(func() {
				By("Cleaning up DRA vLLM test resources")

				nsBuilder := namespace.NewBuilder(APIClient, tsparams.DRAVLLMTestNamespace)
				if nsBuilder.Exists() {
					err := nsBuilder.DeleteAndWait(params.DefaultTimeout)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("should allocate a Neuron device through DRA and use the default scheduler",
				reportxml.ID("90522"), func() {
					By("Verifying the ResourceClaim allocation and default scheduler")

					err := await.ResourceClaimAllocatedAndReserved(
						APIClient, tsparams.DRAVLLMTestNamespace, tsparams.DRAVLLMClaimTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"vLLM ResourceClaim should be allocated and reserved")

					err = await.VLLMPodsUseDefaultScheduler(
						APIClient, tsparams.DRAVLLMTestNamespace,
						vllmConfig.Labels, tsparams.DRAVLLMClaimTimeout)
					Expect(err).ToNot(HaveOccurred(),
						"vLLM pod should be scheduled by the default scheduler")

					usesResourceClaim, err := check.VLLMDeploymentUsesResourceClaim(
						APIClient, vllmConfig.Name, tsparams.DRAVLLMTestNamespace,
						params.DRADeviceRequestName, tsparams.DRAVLLMClaimTemplate)
					Expect(err).ToNot(HaveOccurred(), "Failed to inspect the vLLM deployment")
					Expect(usesResourceClaim).To(BeTrue(),
						"vLLM deployment should use its DRA claim without a legacy Neuron resource request")
				})

			It("should serve inference through the DRA-allocated Neuron device",
				reportxml.ID("90513"), func() {
					By("Waiting for the vLLM deployment to become ready")

					err := await.VLLMDeploymentReady(
						APIClient, vllmConfig.Name, tsparams.DRAVLLMTestNamespace,
						vllmConfig.Labels, tsparams.DRAVLLMStartupTimeout)
					Expect(err).ToNot(HaveOccurred(), "vLLM deployment should become ready")

					By("Sending an inference request through the existing vLLM helper")

					inferenceResult, err := do.ExecuteInferenceFromCluster(APIClient, do.InferenceConfig{
						ServiceName: vllmConfig.Name,
						Namespace:   tsparams.DRAVLLMTestNamespace,
						Port:        vllmConfig.Port,
						ModelName:   vllmConfig.ServedModelName,
						Timeout:     tsparams.DRAVLLMInferenceTimeout,
					})
					Expect(err).ToNot(HaveOccurred(), "DRA vLLM inference should succeed")
					Expect(inferenceResult).ToNot(BeEmpty(), "Inference should return a result")
					klog.V(params.NeuronLogLevel).Infof("DRA vLLM inference result: %s", inferenceResult)
				})
		})
	})
