package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kserve"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/kserve/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron KServe Inference Tests", Ordered, Label(params.Label), Label(tsparams.LabelSuite), func() {
	Context("KServe InferenceService", Label(tsparams.LabelSuite), func() {
		neuronConfig := neuronconfig.NewNeuronConfig()
		isvcName := "neuron-vllm-isvc"

		BeforeAll(func() {
			By("Verifying KServe configuration")

			if !neuronConfig.IsKServeConfigured() {
				Skip("KServe configuration is not set - HF_TOKEN and KSERVE_NAMESPACE are required")
			}

			By("Verifying ServingRuntime exists")

			_, err := kserve.PullServingRuntime(
				APIClient, tsparams.ServingRuntimeName, neuronConfig.KServeNamespace)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("ServingRuntime %s must exist in namespace %s (created by install-kserve-deps step)",
					tsparams.ServingRuntimeName, neuronConfig.KServeNamespace))
		})

		AfterAll(func() {
			By("Cleaning up InferenceService")

			builder, err := kserve.PullInferenceService(
				APIClient, isvcName, neuronConfig.KServeNamespace)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("InferenceService %s not found for cleanup: %v", isvcName, err)

				return
			}

			_, err = builder.Delete()
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Failed to delete InferenceService: %v", err)
			}
		})

		It("Should deploy InferenceService and reach Ready state",
			Label("kserve-001"), reportxml.ID("kserve-001"), func() {
				By("Creating InferenceService")

				storageURI := fmt.Sprintf("hf://%s", neuronConfig.KServeModelName)
				neuronDevices := int64(1)
				neuronQuantity := resource.MustParse(fmt.Sprintf("%d", neuronDevices))

				builder := kserve.NewInferenceServiceBuilder(
					APIClient, isvcName, neuronConfig.KServeNamespace).
					WithModelFormat(tsparams.ModelFormatName).
					WithRuntime(tsparams.ServingRuntimeName).
					WithStorageURI(storageURI).
					WithServiceAccountName(tsparams.ServiceAccountName).
					WithResources(
						corev1.ResourceList{
							"aws.amazon.com/neuron": neuronQuantity,
							corev1.ResourceMemory:   resource.MustParse("10Gi"),
						},
						corev1.ResourceList{
							"aws.amazon.com/neuron": neuronQuantity,
							corev1.ResourceMemory:   resource.MustParse("10Gi"),
						},
					).
					WithAnnotation("serving.knative.dev/progress-deadline", "1800s").
					WithAnnotation("sidecar.istio.io/inject", "true").
					WithAnnotation("serving.knative.openshift.io/enablePassthrough", "true")

				_, err := builder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create InferenceService")
				klog.V(params.NeuronLogLevel).Infof("Created InferenceService %s with model %s",
					isvcName, neuronConfig.KServeModelName)

				By("Waiting for InferenceService to reach Ready state")

				err = builder.WaitUntilReady(tsparams.InferenceServiceReadyTimeout)
				Expect(err).ToNot(HaveOccurred(),
					"InferenceService did not reach Ready within timeout")

				By("Verifying InferenceService URL is populated")

				url, err := builder.GetURL()
				Expect(err).ToNot(HaveOccurred(), "Failed to get InferenceService URL")
				Expect(url).ToNot(BeEmpty(), "InferenceService URL should not be empty")
				klog.V(params.NeuronLogLevel).Infof("InferenceService URL: %s", url)
			})

		It("Should return valid inference response",
			Label("kserve-002"), reportxml.ID("kserve-002"), func() {
				By("Getting InferenceService URL")

				url, err := kserve.PullInferenceService(
					APIClient, isvcName, neuronConfig.KServeNamespace)
				Expect(err).ToNot(HaveOccurred(), "InferenceService must exist")

				isvcURL, err := url.GetURL()
				Expect(err).ToNot(HaveOccurred(), "InferenceService must have a URL")

				By("Sending inference request")

				inferenceConfig := do.KServeInferenceConfig{
					InferenceServiceURL: isvcURL,
					Namespace:           neuronConfig.KServeNamespace,
					ModelName:           neuronConfig.KServeModelName,
					Timeout:             tsparams.InferenceRequestTimeout,
				}

				result, err := do.ExecuteKServeInference(APIClient, inferenceConfig)
				Expect(err).ToNot(HaveOccurred(), "Inference request failed")
				Expect(result).ToNot(BeEmpty(), "Inference response should not be empty")
				klog.V(params.NeuronLogLevel).Infof("Inference result: %s", result)
			})
	})
})
