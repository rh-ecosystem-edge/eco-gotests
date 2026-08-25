package do

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
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

// CreateDeviceConfigUnstructured creates a DeviceConfig with the given spec using the dynamic client.
// This bypasses eco-goinfra builder validation, useful for testing CRD-level CEL validation.
func CreateDeviceConfigUnstructured(
	apiClient *clients.Settings, name string, spec map[string]interface{}) error {
	klog.V(params.NeuronLogLevel).Infof("Creating unstructured DeviceConfig %s", name)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.aws/v1beta1",
			"kind":       "DeviceConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": params.NeuronNamespace,
			},
			"spec": spec,
		},
	}

	_, err := apiClient.Resource(deviceConfigGVR).
		Namespace(params.NeuronNamespace).
		Create(context.TODO(), obj, metav1.CreateOptions{})

	return err
}

// DeleteDeviceConfigIfExists deletes a DeviceConfig by name, ignoring not-found errors.
func DeleteDeviceConfigIfExists(apiClient *clients.Settings, name string) {
	klog.V(params.NeuronLogLevel).Infof("Deleting DeviceConfig %s if it exists", name)

	_ = apiClient.Resource(deviceConfigGVR).
		Namespace(params.NeuronNamespace).
		Delete(context.TODO(), name, metav1.DeleteOptions{})
}
