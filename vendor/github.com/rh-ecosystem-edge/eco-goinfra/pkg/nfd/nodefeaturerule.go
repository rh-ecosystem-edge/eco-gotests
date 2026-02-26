package nfd

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	nfdv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/nfd/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ruleNameEmptyError is the error message when rule name is empty.
	ruleNameEmptyError = "rule 'name' cannot be empty"
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

// NewNodeFeatureRuleBuilder creates a new instance of NodeFeatureRuleBuilder.
func NewNodeFeatureRuleBuilder(apiClient *clients.Settings, name, namespace string) *NodeFeatureRuleBuilder {
	klog.V(100).Infof(
		"Initializing new NodeFeatureRule structure with name: %s, namespace: %s",
		name, namespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the NodeFeatureRule is nil")

		return nil
	}

	err := apiClient.AttachScheme(nfdv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nfd v1 scheme to client schemes")

		return nil
	}

	builder := &NodeFeatureRuleBuilder{
		apiClient: apiClient,
		Definition: &nfdv1.NodeFeatureRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: nfdv1.NodeFeatureRuleSpec{
				Rules: []nfdv1.Rule{},
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the NodeFeatureRule is empty")

		builder.errorMsg = "nodeFeatureRule 'name' cannot be empty"

		return builder
	}

	if namespace == "" {
		klog.V(100).Info("The namespace of the NodeFeatureRule is empty")

		builder.errorMsg = "nodeFeatureRule 'namespace' cannot be empty"

		return builder
	}

	return builder
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

// PullFeatureRule loads an existing NodeFeatureRule into Builder struct.
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

// WithRule adds a rule to the NodeFeatureRule.
func (builder *NodeFeatureRuleBuilder) WithRule(rule nfdv1.Rule) *NodeFeatureRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Adding rule %s to NodeFeatureRule %s in namespace %s",
		rule.Name, builder.Definition.Name, builder.Definition.Namespace)

	if rule.Name == "" {
		klog.V(100).Info("Rule 'name' cannot be empty")

		builder.errorMsg = ruleNameEmptyError

		return builder
	}

	builder.Definition.Spec.Rules = append(builder.Definition.Spec.Rules, rule)

	return builder
}

// WithRules sets the rules for the NodeFeatureRule, replacing any existing rules.
func (builder *NodeFeatureRuleBuilder) WithRules(rules []nfdv1.Rule) *NodeFeatureRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting %d rules on NodeFeatureRule %s in namespace %s",
		len(rules), builder.Definition.Name, builder.Definition.Namespace)

	if len(rules) == 0 {
		klog.V(100).Info("Rules list cannot be empty")

		builder.errorMsg = "rules list cannot be empty"

		return builder
	}

	for _, rule := range rules {
		if rule.Name == "" {
			klog.V(100).Info("Rule 'name' cannot be empty")

			builder.errorMsg = ruleNameEmptyError

			return builder
		}
	}

	builder.Definition.Spec.Rules = rules

	return builder
}

// WithSimplePCIRule adds a simple PCI device matching rule with labels.
// This is a convenience method for common PCI device detection patterns like the Neuron NFD rule.
func (builder *NodeFeatureRuleBuilder) WithSimplePCIRule(
	ruleName string,
	labels map[string]string,
	vendorIDs []string,
	deviceIDs []string,
) *NodeFeatureRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Adding simple PCI rule %s to NodeFeatureRule %s in namespace %s",
		ruleName, builder.Definition.Name, builder.Definition.Namespace)

	if ruleName == "" {
		klog.V(100).Info("Rule 'name' cannot be empty")

		builder.errorMsg = ruleNameEmptyError

		return builder
	}

	if len(labels) == 0 {
		klog.V(100).Info("Rule 'labels' cannot be empty")

		builder.errorMsg = "rule 'labels' cannot be empty"

		return builder
	}

	if len(vendorIDs) == 0 {
		klog.V(100).Info("vendorIDs cannot be empty")

		builder.errorMsg = "vendorIDs cannot be empty"

		return builder
	}

	matchExpressions := nfdv1.MatchExpressionSet{
		"vendor": &nfdv1.MatchExpression{
			Op:    nfdv1.MatchIn,
			Value: vendorIDs,
		},
	}

	if len(deviceIDs) > 0 {
		matchExpressions["device"] = &nfdv1.MatchExpression{
			Op:    nfdv1.MatchIn,
			Value: deviceIDs,
		}
	}

	rule := nfdv1.Rule{
		Name:   ruleName,
		Labels: labels,
		MatchAny: []nfdv1.MatchAnyElem{
			{
				MatchFeatures: nfdv1.FeatureMatcher{
					{
						Feature:          "pci.device",
						MatchExpressions: &matchExpressions,
					},
				},
			},
		},
	}

	builder.Definition.Spec.Rules = append(builder.Definition.Spec.Rules, rule)

	return builder
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

// Delete removes the NodeFeatureRule from the cluster.
func (builder *NodeFeatureRuleBuilder) Delete() (*NodeFeatureRuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting NodeFeatureRule %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("NodeFeatureRule %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete NodeFeatureRule: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update modifies the existing NodeFeatureRule in the cluster.
func (builder *NodeFeatureRuleBuilder) Update(force bool) (*NodeFeatureRuleBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating NodeFeatureRule %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("NodeFeatureRule %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return builder, fmt.Errorf("cannot update non-existent NodeFeatureRule")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%s", msg.FailToUpdateNotification("NodeFeatureRule",
				builder.Definition.Name, builder.Definition.Namespace))

			deletedBuilder, deleteErr := builder.Delete()
			if deleteErr != nil {
				klog.V(100).Infof("%s", msg.FailToUpdateError("NodeFeatureRule",
					builder.Definition.Name, builder.Definition.Namespace))

				return nil, deleteErr
			}

			if deletedBuilder == nil {
				return nil, fmt.Errorf("failed to delete NodeFeatureRule: builder is nil")
			}

			deletedBuilder.Definition.ResourceVersion = ""

			return deletedBuilder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
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

	return err == nil
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
