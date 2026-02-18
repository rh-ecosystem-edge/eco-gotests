package set

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nfd"
	"k8s.io/klog/v2"
)

// CreateNodeFeatureRuleFromJSON creates a NodeFeatureRule from a JSON/YAML string.
func CreateNodeFeatureRuleFromJSON(apiClient *clients.Settings, ruleJSON string) (*nfd.NodeFeatureRuleBuilder, error) {
	klog.V(100).Infof("Creating NodeFeatureRule from JSON/YAML")

	ruleBuilder := nfd.NewNodeFeatureRuleBuilderFromObjectString(apiClient, ruleJSON)
	if ruleBuilder == nil {
		return nil, fmt.Errorf("failed to create NodeFeatureRule builder from JSON")
	}

	// Check if rule already exists
	if ruleBuilder.Exists() {
		klog.V(100).Infof("NodeFeatureRule %s already exists", ruleBuilder.Definition.Name)
		return ruleBuilder, nil
	}

	// Create the rule
	createdRule, err := ruleBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create NodeFeatureRule: %w", err)
	}

	klog.V(100).Infof("Successfully created NodeFeatureRule %s", createdRule.Definition.Name)

	return createdRule, nil
}

// DeleteNodeFeatureRule deletes a NodeFeatureRule by name and namespace.
func DeleteNodeFeatureRule(apiClient *clients.Settings, name, namespace string) error {
	klog.V(100).Infof("Deleting NodeFeatureRule %s in namespace %s", name, namespace)

	ruleBuilder, err := nfd.PullFeatureRule(apiClient, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to pull NodeFeatureRule %s: %w", name, err)
	}

	if !ruleBuilder.Exists() {
		klog.V(100).Infof("NodeFeatureRule %s does not exist, skipping deletion", name)
		return nil
	}

	_, err = ruleBuilder.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete NodeFeatureRule %s: %w", name, err)
	}

	klog.V(100).Infof("Successfully deleted NodeFeatureRule %s", name)

	return nil
}
