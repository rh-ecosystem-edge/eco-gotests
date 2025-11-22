package oran

import (
	"fmt"

	pluginsv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/plugins/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// AllocatedNodeBuilder provides a struct to inferface with Node resources on a specific cluster.
type AllocatedNodeBuilder struct {
	// Definition of the AllocatedNode used to create the resource.
	Definition *pluginsv1alpha1.AllocatedNode
	// Object of the AllocatedNode as it is on the cluster.
	Object *pluginsv1alpha1.AllocatedNode
	// apiClient used to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg used to store latest error message from functions that do not return errors.
	errorMsg string
}

// PullAllocatedNode pulls an existing AllocatedNode into a AllocatedNodeBuilder struct.
func PullAllocatedNode(apiClient *clients.Settings, name, nsname string) (*AllocatedNodeBuilder, error) {
	klog.V(100).Infof("Pulling existing AllocatedNode %s in namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the AllocatedNode is nil")

		return nil, fmt.Errorf("allocatedNode 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(pluginsv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Infof("Failed to add plugins v1alpha1 scheme to client schemes: %v", err)

		return nil, err
	}

	builder := &AllocatedNodeBuilder{
		apiClient: apiClient.Client,
		Definition: &pluginsv1alpha1.AllocatedNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the AllocatedNode is empty")

		return nil, fmt.Errorf("allocatedNode 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The nsname of the AllocatedNode is empty")

		return nil, fmt.Errorf("allocatedNode 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		klog.V(100).Infof("The AllocatedNode %s does not exist in namespace %s", name, nsname)

		return nil, fmt.Errorf("allocatedNode object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the AllocatedNode object if found.
func (builder *AllocatedNodeBuilder) Get() (*pluginsv1alpha1.AllocatedNode, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting AllocatedNode object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	node := &pluginsv1alpha1.AllocatedNode{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, node)
	if err != nil {
		return nil, err
	}

	return node, nil
}

// Exists checks whether this AllocatedNode exists on the cluster.
func (builder *AllocatedNodeBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if AllocatedNode %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	object, err := builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to get AllocatedNode object %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return false
	}

	builder.Object = object

	return true
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *AllocatedNodeBuilder) validate() (bool, error) {
	resourceCRD := "allocatedNode"

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
