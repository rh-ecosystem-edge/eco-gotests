package bmh

import (
	"context"
	"fmt"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// HFCBuilder provides a struct to interface with HostFirmwareComponents resources on a specific cluster.
type HFCBuilder struct {
	// Definition of the HostFirmwareComponents used to create the object.
	Definition *bmhv1alpha1.HostFirmwareComponents
	// Object of the HostFirmwareComponents as it is on the cluster.
	Object    *bmhv1alpha1.HostFirmwareComponents
	apiClient goclient.Client
	errorMsg  string
}

// PullHFC pulls an existing HostFirmwareComponents from the cluster.
func PullHFC(apiClient *clients.Settings, name, nsname string) (*HFCBuilder, error) {
	klog.V(100).Infof("Pulling existing HostFirmwareComponents name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("hostFirmwareComponents 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(bmhv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add bmhv1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := &HFCBuilder{
		apiClient: apiClient.Client,
		Definition: &bmhv1alpha1.HostFirmwareComponents{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the HostFirmwareComponents is empty")

		return nil, fmt.Errorf("hostFirmwareComponents 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the HostFirmwareComponents is empty")

		return nil, fmt.Errorf("hostFirmwareComponents 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("hostFirmwareComponents object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the HostFirmwareComponents object if found.
func (builder *HFCBuilder) Get() (*bmhv1alpha1.HostFirmwareComponents, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Getting HostFirmwareComponents object %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	hostFirmwareComponents := &bmhv1alpha1.HostFirmwareComponents{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, hostFirmwareComponents)
	if err != nil {
		klog.V(100).Infof(
			"HostFirmwareComponents object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return hostFirmwareComponents, nil
}

// Exists checks whether the given HostFirmwareComponents exists on the cluster.
func (builder *HFCBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if HostFirmwareComponents %s exists in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *HFCBuilder) validate() (bool, error) {
	resourceCRD := "hostFirmwareComponents"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is uninitialized", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
