package oran

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	pluginsv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/plugins/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NARBuilder provides a struct to inferface with NodeAllocationRequest resources on a specific cluster.
type NARBuilder struct {
	// Definition of the NodeAllocationRequest used to create the resource.
	Definition *pluginsv1alpha1.NodeAllocationRequest
	// Object of the NodeAllocationRequest as it is on the cluster.
	Object *pluginsv1alpha1.NodeAllocationRequest
	// apiClient used to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg used to store latest error message from functions that do not return errors.
	errorMsg string
}

// PullNodeAllocationRequest pulls an existing NodeAllocationRequest into a NARBuilder struct.
func PullNodeAllocationRequest(apiClient *clients.Settings, name, nsname string) (*NARBuilder, error) {
	glog.V(100).Infof("Pulling existing NodeAllocationRequest %s in namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient of the NodeAllocationRequest is nil")

		return nil, fmt.Errorf("nodeAllocationRequest 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(pluginsv1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add plugins v1alpha1 scheme to client schemes: %v", err)

		return nil, err
	}

	builder := &NARBuilder{
		apiClient: apiClient.Client,
		Definition: &pluginsv1alpha1.NodeAllocationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the NodeAllocationRequest is empty")

		return nil, fmt.Errorf("nodeAllocationRequest 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Info("The nsname of the NodeAllocationRequest is empty")

		return nil, fmt.Errorf("nodeAllocationRequest 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		glog.V(100).Info("The NodeAllocationRequest %s does not exist in namespace %s", name, nsname)

		return nil, fmt.Errorf("nodeAllocationRequest object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the NodeAllocationRequest object if found.
func (builder *NARBuilder) Get() (*pluginsv1alpha1.NodeAllocationRequest, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting NodeAllocationRequest object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	nodeAllocationRequest := &pluginsv1alpha1.NodeAllocationRequest{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, nodeAllocationRequest)
	if err != nil {
		glog.V(100).Infof("Failed to get NodeAllocationRequest object %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return nodeAllocationRequest, nil
}

// Exists checks whether this NodeAllocationRequest exists on the cluster.
func (builder *NARBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if NodeAllocationRequest %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	object, err := builder.Get()
	if err != nil {
		glog.V(100).Infof("Failed to get NodeAllocationRequest object %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return false
	}

	builder.Object = object

	return true
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *NARBuilder) validate() (bool, error) {
	resourceCRD := "nodeAllocationRequest"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is uninitialized", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
