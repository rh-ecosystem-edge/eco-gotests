package siteconfig

import (
	"fmt"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

// NodeBuilder provides struct for the siteconfig NodeSpec object.
type NodeBuilder struct {
	// Node definition. Used to create a Node object.
	definition *siteconfigv1alpha1.NodeSpec
	// Used in functions that define or mutate Node definition. errorMsg is processed before the Node
	// object is created.
	errorMsg string
}

// NewNodeBuilder creates a new instance of NodeBuilder.
func NewNodeBuilder(
	name,
	bmcAddress,
	bootMACAddress,
	bmcCredentialsName,
	templateName,
	templateNamespace string) *NodeBuilder {
	klog.V(100).Infof(
		"Initializing new siteconfig Node structure with the following params: "+
			"name: %s, bmcAddress: %s, bootMACAddress: %s, bmcCredentialsName: %s, templateName: %s, templateNamespace: %s",
		name, bmcAddress, bootMACAddress, bmcCredentialsName, templateName, templateNamespace)

	builder := NodeBuilder{
		definition: &siteconfigv1alpha1.NodeSpec{
			HostName:       name,
			BmcAddress:     bmcAddress,
			BootMACAddress: bootMACAddress,
			BmcCredentialsName: siteconfigv1alpha1.BmcCredentialsName{
				Name: bmcCredentialsName,
			},
			TemplateRefs: []siteconfigv1alpha1.TemplateRef{
				{
					Name:      templateName,
					Namespace: templateNamespace,
				},
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The siteconfig node name is empty")

		builder.errorMsg = "siteconfig node 'name' cannot be empty"

		return &builder
	}

	if bmcAddress == "" {
		klog.V(100).Info("The siteconfig node bmcAddress is empty")

		builder.errorMsg = "siteconfig node 'bmcAddress' cannot be empty"

		return &builder
	}

	if bootMACAddress == "" {
		klog.V(100).Info("The siteconfig node bootMACAddress is empty")

		builder.errorMsg = "siteconfig node 'bootMACAddress' cannot be empty"

		return &builder
	}

	if bmcCredentialsName == "" {
		klog.V(100).Info("The siteconfig node bmcCredentialsName is empty")

		builder.errorMsg = "siteconfig node 'bmcCredentialsName' cannot be empty"

		return &builder
	}

	if templateName == "" {
		klog.V(100).Info("The siteconfig node templateName is empty")

		builder.errorMsg = "siteconfig node 'templateName' cannot be empty"

		return &builder
	}

	if templateNamespace == "" {
		klog.V(100).Info("The siteconfig node templateNamespace is empty")

		builder.errorMsg = "siteconfig node 'templateNamespace' cannot be empty"

		return &builder
	}

	return &builder
}

// WithAutomatedCleaningMode adds the automatedCleaningMode field to the node.
func (builder *NodeBuilder) WithAutomatedCleaningMode(mode string) *NodeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting automatedCleaningMode to %s on siteconfig node", mode)

	if !slices.Contains([]string{"disabled", "metadata"}, mode) {
		builder.errorMsg = "siteconfig node automatedCleaningMode must be one of: disabled, metadata"

		return builder
	}

	builder.definition.AutomatedCleaningMode = bmhv1alpha1.AutomatedCleaningMode(mode)

	return builder
}

// WithNodeNetwork adds a node network configuration to the node.
func (builder *NodeBuilder) WithNodeNetwork(networkConfig *v1beta1.NMStateConfigSpec) *NodeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Adding networking config to siteconfig node")

	if networkConfig == nil {
		klog.V(100).Info("The siteconfig node networkConfig is nil")

		builder.errorMsg = "siteconfig node networkConfig cannot be nil"

		return builder
	}

	builder.definition.NodeNetwork = networkConfig

	return builder
}

// Generate returns the NodeSpec struct from the NodeBuilder.
func (builder *NodeBuilder) Generate() (*siteconfigv1alpha1.NodeSpec, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Info("Generating siteconfig nodeSpec from node builder")

	return builder.definition, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NodeBuilder) validate() (bool, error) {
	resourceCRD := "siteconfig node"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
