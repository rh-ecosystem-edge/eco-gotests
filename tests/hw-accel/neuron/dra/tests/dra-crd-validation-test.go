package tests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	neuronscheme "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/neuron/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

var deviceConfigGVR = schema.GroupVersionResource{
	Group:    "k8s.aws",
	Version:  "v1beta1",
	Resource: "deviceconfigs",
}

func createDeviceConfigUnstructured(spec map[string]interface{}) error {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.aws/v1beta1",
			"kind":       "DeviceConfig",
			"metadata": map[string]interface{}{
				"name":      "validation-test",
				"namespace": params.NeuronNamespace,
			},
			"spec": spec,
		},
	}

	_, err := APIClient.Resource(deviceConfigGVR).
		Namespace(params.NeuronNamespace).
		Create(context.TODO(), obj, metav1.CreateOptions{})

	return err
}

func deleteDeviceConfigIfExists() {
	_ = APIClient.Resource(deviceConfigGVR).
		Namespace(params.NeuronNamespace).
		Delete(context.TODO(), "validation-test", metav1.DeleteOptions{})
}

var _ = Describe("Neuron DRA CRD Validation Tests", Ordered,
	Label(params.Label, params.DRALabel, tsparams.LabelValidation), func() {
		BeforeAll(func() {
			Expect(neuronscheme.AddToScheme(APIClient.Scheme())).ToNot(HaveOccurred())
		})

		BeforeEach(func() {
			deleteDeviceConfigIfExists()
		})

		AfterAll(func() {
			deleteDeviceConfigIfExists()
		})

		Context("Mutual exclusivity — CEL rule 1", Label(tsparams.LabelSuite), func() {
			It("should reject DeviceConfig with draDriverImage AND devicePluginImage",
				reportxml.ID("90380"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"draDriverImage":    "registry.example.com/dra-driver:latest",
						"devicePluginImage": "registry.example.com/device-plugin:latest",
						"nodeMetricsImage":  "registry.example.com/metrics:latest",
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("draDriverImage is mutually exclusive"))

					klog.V(params.NeuronLogLevel).Infof("Correctly rejected: %v", err)
				})

			It("should reject DeviceConfig with draDriverImage AND customSchedulerImage",
				reportxml.ID("90381"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"draDriverImage":       "registry.example.com/dra-driver:latest",
						"customSchedulerImage": "registry.example.com/scheduler:latest",
						"nodeMetricsImage":     "registry.example.com/metrics:latest",
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("draDriverImage is mutually exclusive"))

					klog.V(params.NeuronLogLevel).Infof("Correctly rejected: %v", err)
				})

			It("should reject DeviceConfig with draDriverImage AND schedulerExtensionImage",
				reportxml.ID("90382"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"draDriverImage":          "registry.example.com/dra-driver:latest",
						"schedulerExtensionImage": "registry.example.com/scheduler-ext:latest",
						"nodeMetricsImage":        "registry.example.com/metrics:latest",
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("draDriverImage is mutually exclusive"))

					klog.V(params.NeuronLogLevel).Infof("Correctly rejected: %v", err)
				})

			It("should reject DeviceConfig with draDriverImage AND all three device-plugin fields",
				reportxml.ID("90383"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"draDriverImage":          "registry.example.com/dra-driver:latest",
						"devicePluginImage":       "registry.example.com/device-plugin:latest",
						"customSchedulerImage":    "registry.example.com/scheduler:latest",
						"schedulerExtensionImage": "registry.example.com/scheduler-ext:latest",
						"nodeMetricsImage":        "registry.example.com/metrics:latest",
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("draDriverImage is mutually exclusive"))

					klog.V(params.NeuronLogLevel).Infof("Correctly rejected: %v", err)
				})
		})

		Context("Triad completeness — CEL rule 2", Label(tsparams.LabelSuite), func() {
			It("should reject DeviceConfig with devicePluginImage but missing schedulerExtensionImage",
				reportxml.ID("90384"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"devicePluginImage":    "registry.example.com/device-plugin:latest",
						"customSchedulerImage": "registry.example.com/scheduler:latest",
						"nodeMetricsImage":     "registry.example.com/metrics:latest",
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("must all be set together"))

					klog.V(params.NeuronLogLevel).Infof("Correctly rejected: %v", err)
				})

			It("should reject DeviceConfig with devicePluginImage but missing customSchedulerImage",
				reportxml.ID("90385"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"devicePluginImage":       "registry.example.com/device-plugin:latest",
						"schedulerExtensionImage": "registry.example.com/scheduler-ext:latest",
						"nodeMetricsImage":        "registry.example.com/metrics:latest",
					})
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("must all be set together"))

					klog.V(params.NeuronLogLevel).Infof("Correctly rejected: %v", err)
				})
		})

		Context("Valid configurations — happy paths", Label(tsparams.LabelSuite), func() {
			It("should accept DeviceConfig with only draDriverImage",
				reportxml.ID("90386"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"draDriverImage":   "registry.example.com/dra-driver:latest",
						"nodeMetricsImage": "registry.example.com/metrics:latest",
					})
					Expect(err).ToNot(HaveOccurred(), "Valid DRA-only DeviceConfig should be accepted")

					klog.V(params.NeuronLogLevel).Info("DRA-only DeviceConfig accepted")
				})

			It("should accept DeviceConfig with full device-plugin triad",
				reportxml.ID("90387"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"devicePluginImage":       "registry.example.com/device-plugin:latest",
						"customSchedulerImage":    "registry.example.com/scheduler:latest",
						"schedulerExtensionImage": "registry.example.com/scheduler-ext:latest",
						"nodeMetricsImage":        "registry.example.com/metrics:latest",
					})
					Expect(err).ToNot(HaveOccurred(), "Valid device-plugin triad should be accepted")

					klog.V(params.NeuronLogLevel).Info("Device-plugin triad DeviceConfig accepted")
				})
		})

		Context("DeviceClasses validation", Label(tsparams.LabelSuite), func() {
			It("should accept DRA DeviceConfig with valid deviceClasses",
				reportxml.ID("90388"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"draDriverImage":   "registry.example.com/dra-driver:latest",
						"nodeMetricsImage": "registry.example.com/metrics:latest",
						"deviceClasses": []interface{}{
							map[string]interface{}{
								"name": "neuron.aws.com",
								"selectors": []interface{}{
									map[string]interface{}{
										"cel": map[string]interface{}{
											"expression": "device.driver == \"neuron.aws.com\"",
										},
									},
								},
							},
						},
					})
					Expect(err).ToNot(HaveOccurred(),
						"DRA DeviceConfig with valid deviceClasses should be accepted")

					klog.V(params.NeuronLogLevel).Info("DRA DeviceConfig with deviceClasses accepted")
				})

			It("should reject DeviceConfig with invalid deviceClass name pattern",
				reportxml.ID("90389"), func() {
					err := createDeviceConfigUnstructured(map[string]interface{}{
						"draDriverImage":   "registry.example.com/dra-driver:latest",
						"nodeMetricsImage": "registry.example.com/metrics:latest",
						"deviceClasses": []interface{}{
							map[string]interface{}{
								"name": "INVALID_NAME",
							},
						},
					})
					Expect(err).To(HaveOccurred())

					klog.V(params.NeuronLogLevel).Infof("Correctly rejected invalid name: %v", err)
				})
		})
	})
