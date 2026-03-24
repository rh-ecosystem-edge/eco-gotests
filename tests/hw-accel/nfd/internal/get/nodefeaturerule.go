package get

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nfd"
	"k8s.io/klog/v2"
)

// NodeFeatureRule retrieves a NodeFeatureRule by name and namespace.
func NodeFeatureRule(apiClient *clients.Settings, name, namespace string) (*nfd.NodeFeatureRuleBuilder, error) {
	klog.V(100).Infof("Getting NodeFeatureRule %s in namespace %s", name, namespace)

	ruleBuilder, err := nfd.PullFeatureRule(apiClient, name, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get NodeFeatureRule %s: %w", name, err)
	}

	if !ruleBuilder.Exists() {
		return nil, fmt.Errorf("NodeFeatureRule %s does not exist", name)
	}

	return ruleBuilder, nil
}

// NodesWithLabel returns a list of node names that have labels matching the given pattern.
func NodesWithLabel(apiClient *clients.Settings, labelPattern string) ([]string, error) {
	klog.V(100).Infof("Getting nodes with label pattern: %s", labelPattern)

	nodelabels, err := NodeFeatureLabels(apiClient, map[string]string{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node feature labels: %w", err)
	}

	matchingNodes := []string{}

	for nodeName, labels := range nodelabels {
		for _, label := range labels {
			if strings.Contains(label, labelPattern) {
				matchingNodes = append(matchingNodes, nodeName)

				break
			}
		}
	}

	klog.V(100).Infof("Found %d nodes with label pattern %s", len(matchingNodes), labelPattern)

	return matchingNodes, nil
}
