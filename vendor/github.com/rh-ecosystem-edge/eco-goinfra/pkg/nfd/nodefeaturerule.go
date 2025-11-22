package nfd

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	nfdv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/nfd/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeFeatureRuleBuilder provides a struct for NodeFeatureRule object
// from the cluster and a NodeFeatureRule definition.
type NodeFeatureRuleBuilder struct {
	// Builder definition. Used to create
	// Builder object with minimum set of required elements.
	Definition *nfdv1.NodeFeatureRule
	// Created Builder object on the cluster.
	Object *nfdv1.NodeFeatureRule
	// api client to interact with the cluster.
	apiClient goclient.Client
	// errorMsg is processed before Builder object is created.
	errorMsg string
}

// NewNodeFeatureRuleBuilderFromObjectString creates a Builder object from CSV alm-examples.
func NewNodeFeatureRuleBuilderFromObjectString(apiClient *clients.Settings, almExample string) *NodeFeatureRuleBuilder {
	klog.V(100).Infof(
		"Initializing new Builder structure from almExample string")

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the NodeFeatureRule is nil")

		return nil
	}

	err := apiClient.AttachScheme(nfdv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nfd v1 scheme to client schemes")

		return nil
	}

	nodeFeatureRule, err := getNodeFeatureRuleFromAlmExample(almExample)

	klog.V(100).Infof(
		"Initializing Builder definition to NodeFeatureRule object")

	nodeFeatureRuleBuilder := NodeFeatureRuleBuilder{
		apiClient:  apiClient,
		Definition: nodeFeatureRule,
	}

	if err != nil {
		klog.V(100).Infof(
			"Error initializing NodeFeatureRule from alm-examples: %s", err.Error())

		nodeFeatureRuleBuilder.errorMsg = fmt.Sprintf("error initializing NodeFeatureRule from alm-examples: %s",
			err.Error())

		return &nodeFeatureRuleBuilder
	}

	if nodeFeatureRuleBuilder.Definition == nil {
		klog.V(100).Info("The NodeFeatureRule object definition is nil")

		nodeFeatureRuleBuilder.errorMsg = "nodeFeatureRule definition is nil"

		return &nodeFeatureRuleBuilder
	}

	return &nodeFeatureRuleBuilder
}

// PullFeatureRule loads an existing NodeFeatureRuleBuilder into Builder struct.
func PullFeatureRule(apiClient *clients.Settings, name, namespace string) (*NodeFeatureRuleBuilder, error) {
	klog.V(100).Infof("Pulling existing NodeFeatureRule name: %s in namespace: %s", name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the NodeFeatureRule is nil")

		return nil, fmt.Errorf("the apiClient of the NodeFeatureRule is nil")
	}

	err := apiClient.AttachScheme(nfdv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nfd v1 scheme to client schemes")

		return nil, err
	}

	ruleBuilder := &NodeFeatureRuleBuilder{
		apiClient: apiClient,
		Definition: &nfdv1.NodeFeatureRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("NodeFeatureRule name is empty")

		return nil, fmt.Errorf("nodeFeatureRule 'name' cannot be empty")
	}

	if namespace == "" {
		klog.V(100).Info("NodeFeatureRule namespace is empty")

		return nil, fmt.Errorf("nodeFeatureRule 'namespace' cannot be empty")
	}

	if !ruleBuilder.Exists() {
		return nil, fmt.Errorf("nodeFeatureRule object %s does not exist in namespace %s", name, namespace)
	}

	ruleBuilder.Definition = ruleBuilder.Object

	return ruleBuilder, nil
}

// getNodeFeatureRuleFromAlmExample extracts the NodeFeatureRule from the alm-examples block.
func getNodeFeatureRuleFromAlmExample(almExample string) (*nfdv1.NodeFeatureRule, error) {
	nodeFeatureRuleList := &nfdv1.NodeFeatureRuleList{}

	if almExample == "" {
		return nil, fmt.Errorf("almExample is an empty string")
	}

	err := json.Unmarshal([]byte(almExample), &nodeFeatureRuleList.Items)
	if err != nil {
		return nil, err
	}

	if len(nodeFeatureRuleList.Items) == 0 {
		return nil, fmt.Errorf("failed to get alm examples")
	}

	for i, item := range nodeFeatureRuleList.Items {
		if item.Kind == "NodeFeatureRule" {
			return &nodeFeatureRuleList.Items[i], nil
		}
	}

	return nil, fmt.Errorf("nodeFeatureRule is missing in alm-examples ")
}

// Create makes a NodeFeatureRule in the cluster and stores the created object in struct.
func (builder *NodeFeatureRuleBuilder) Create() (*NodeFeatureRuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the NodeFeatureRule %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Exists checks whether the given NodeFeatureRule exists.
func (builder *NodeFeatureRuleBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if NodeFeatureRule %s exists in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to collect NodeFeatureRule object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns NodeFeatureRule object if found.
func (builder *NodeFeatureRuleBuilder) Get() (*nfdv1.NodeFeatureRule, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collecting NodeFeatureRule object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	NodeFeatureRule := &nfdv1.NodeFeatureRule{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, NodeFeatureRule)
	if err != nil {
		klog.V(100).Infof("NodeFeatureRule object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return NodeFeatureRule, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NodeFeatureRuleBuilder) validate() (bool, error) {
	resourceCRD := "nodeFeatureRule"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
