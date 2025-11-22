package bmh

import (
	"fmt"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// HFSBuilder provides a struct to interface with HostFirmwareSettings resources on a specific cluster.
type HFSBuilder struct {
	// Definition of the HostFirmwareSettings used to create the object.
	Definition *bmhv1alpha1.HostFirmwareSettings
	// Object of the HostFirmwareSettings as it is on the cluster.
	Object    *bmhv1alpha1.HostFirmwareSettings
	apiClient goclient.Client
	errorMsg  string
}

// PullHFS pulls an existing HostFirmwareSettings from the cluster.
func PullHFS(apiClient *clients.Settings, name, nsname string) (*HFSBuilder, error) {
	klog.V(100).Infof("Pulling existing HostFirmwareSettings name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("hostFirmwareSettings 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(bmhv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add bmhv1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := &HFSBuilder{
		apiClient: apiClient.Client,
		Definition: &bmhv1alpha1.HostFirmwareSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the HostFirmwareSettings is empty")

		return nil, fmt.Errorf("hostFirmwareSettings 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the HostFirmwareSettings is empty")

		return nil, fmt.Errorf("hostFirmwareSettings 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("hostFirmwareSettings object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the HostFirmwareSettings object if found.
func (builder *HFSBuilder) Get() (*bmhv1alpha1.HostFirmwareSettings, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Getting HostFirmwareSettings object %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	hostFirmwareSettings := &bmhv1alpha1.HostFirmwareSettings{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, hostFirmwareSettings)
	if err != nil {
		klog.V(100).Infof(
			"HostFirmwareSettings object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return hostFirmwareSettings, nil
}

// Exists checks whether the given HostFirmwareSettings exists on the cluster.
func (builder *HFSBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if HostFirmwareSettings %s exists in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a HostFirmwareSettings on the cluster if it does not already exist.
func (builder *HFSBuilder) Create() (*HFSBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Creating HostFirmwareSettings %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes a HostFirmwareSettings from the cluster if it exists.
func (builder *HFSBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof(
		"Deleting HostFirmwareSettings %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof(
			"HostFirmwareSettings %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Object)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *HFSBuilder) validate() (bool, error) {
	resourceCRD := "hostFirmwareSettings"

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
