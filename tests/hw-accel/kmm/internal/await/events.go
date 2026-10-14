package await

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const moduleEventsPollInterval = 5 * time.Second

const moduleEventAnnotation = "module"

// ModuleLifecycleEvents waits until the expected number of ModuleLoaded and
// ModuleUnloaded events for a Module are present in the specified namespace.
func ModuleLifecycleEvents(apiClient *clients.Settings, eventNamespace, moduleNamespace, moduleName string,
	expectedCount int, timeout time.Duration) error {
	if apiClient == nil {
		return fmt.Errorf("apiClient cannot be nil")
	}

	if eventNamespace == "" || moduleNamespace == "" || moduleName == "" {
		return fmt.Errorf("eventNamespace, moduleNamespace, and moduleName cannot be empty")
	}

	if expectedCount < 1 {
		return fmt.Errorf("expectedCount must be greater than zero")
	}

	if timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}

	loadedCount, unloadedCount := 0, 0

	err := wait.PollUntilContextTimeout(context.TODO(), moduleEventsPollInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			eventList, err := events.List(apiClient, eventNamespace)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("error listing events: %v", err)

				return false, nil
			}

			loadedCount = countModuleEvents(eventList, kmmparams.ReasonModuleLoaded,
				moduleNamespace, moduleName)
			unloadedCount = countModuleEvents(eventList, kmmparams.ReasonModuleUnloaded,
				moduleNamespace, moduleName)

			klog.V(kmmparams.KmmLogLevel).Infof(
				"Module %s/%s events: loaded=%d unloaded=%d expected=%d",
				moduleNamespace, moduleName, loadedCount, unloadedCount, expectedCount)

			return loadedCount >= expectedCount && unloadedCount >= expectedCount, nil
		})
	if err != nil {
		return fmt.Errorf("timed out waiting for Module %s/%s lifecycle events: loaded=%d unloaded=%d expected=%d: %w",
			moduleNamespace, moduleName, loadedCount, unloadedCount, expectedCount, err)
	}

	return nil
}

// countModuleEvents counts event occurrences rather than Event objects. The
// Kubernetes event recorder can aggregate repeated occurrences into one Event
// and stores the occurrence count in Event.Count or Event.Series.Count.
func countModuleEvents(eventList []*events.Builder, reason, moduleNamespace, moduleName string) int {
	moduleRef := fmt.Sprintf("%s/%s", moduleNamespace, moduleName)
	message := ""

	switch reason {
	case kmmparams.ReasonModuleLoaded:
		message = get.ModuleLoadedMessage(moduleName, moduleNamespace)
	case kmmparams.ReasonModuleUnloaded:
		message = get.ModuleUnloadedMessage(moduleName, moduleNamespace)
	default:
		return 0
	}

	count := 0

	for _, event := range eventList {
		if !isModuleEvent(event, reason, moduleRef, message) {
			continue
		}

		occurrences := int(event.Object.Count)
		if event.Object.Series != nil && int(event.Object.Series.Count) > occurrences {
			occurrences = int(event.Object.Series.Count)
		}

		if occurrences == 0 {
			occurrences = 1
		}

		count += occurrences
	}

	return count
}

func isModuleEvent(event *events.Builder, reason, moduleRef, message string) bool {
	if event == nil || event.Object == nil || event.Object.Reason != reason ||
		event.Object.InvolvedObject.Kind != "Node" {
		return false
	}

	if event.Object.Annotations != nil {
		annotation, exists := event.Object.Annotations[moduleEventAnnotation]
		if exists {
			return annotation == moduleRef
		}
	}

	// Older KMM versions did not add the module annotation. Keep the message
	// fallback for those versions while preferring the unambiguous annotation.
	return event.Object.Message == message
}
