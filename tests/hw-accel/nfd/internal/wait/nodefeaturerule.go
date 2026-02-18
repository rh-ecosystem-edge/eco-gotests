package wait

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// WaitForLabelsFromRule waits for labels with the specified prefixes to appear on nodes.
func WaitForLabelsFromRule(apiClient *clients.Settings, labelPrefixes []string, timeout time.Duration) error {
	klog.V(nfdparams.LogLevel).Infof("Waiting for labels with prefixes %v (timeout: %v)", labelPrefixes, timeout)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			nodelabels, err := get.NodeFeatureLabels(apiClient, map[string]string{})
			if err != nil {
				klog.V(nfdparams.LogLevel).Infof("Error getting node labels: %v", err)
				return false, nil
			}

			if len(nodelabels) == 0 {
				klog.V(nfdparams.LogLevel).Info("No nodes found yet")
				return false, nil
			}

			// Check if any node has labels matching any of the prefixes
			for nodeName, labels := range nodelabels {
				allLabelsStr := strings.Join(labels, ",")
				foundCount := 0

				for _, prefix := range labelPrefixes {
					if strings.Contains(allLabelsStr, prefix) {
						foundCount++
					}
				}

				if foundCount > 0 {
					klog.V(nfdparams.LogLevel).Infof("Node %s has %d/%d expected label prefixes",
						nodeName, foundCount, len(labelPrefixes))
					return true, nil
				}
			}

			klog.V(nfdparams.LogLevel).Info("Labels not found yet, continuing to wait...")
			return false, nil
		})

	if err != nil {
		return fmt.Errorf("timeout waiting for labels with prefixes %v: %w", labelPrefixes, err)
	}

	klog.V(nfdparams.LogLevel).Info("Successfully found expected labels")
	return nil
}

// WaitForRuleProcessed waits for a NodeFeatureRule to be processed and labels to appear.
func WaitForRuleProcessed(apiClient *clients.Settings, ruleName string, expectedLabels []string, timeout time.Duration) error {
	klog.V(nfdparams.LogLevel).Infof("Waiting for NodeFeatureRule %s to be processed (timeout: %v)", ruleName, timeout)

	err := WaitForLabelsFromRule(apiClient, expectedLabels, timeout)
	if err != nil {
		return fmt.Errorf("rule %s was not processed within timeout: %w", ruleName, err)
	}

	klog.V(nfdparams.LogLevel).Infof("NodeFeatureRule %s processed successfully", ruleName)
	return nil
}
