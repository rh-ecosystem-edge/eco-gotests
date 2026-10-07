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

	loadedMessage := get.ModuleLoadedMessage(moduleName, moduleNamespace)
	unloadedMessage := get.ModuleUnloadedMessage(moduleName, moduleNamespace)
	loadedCount, unloadedCount := 0, 0

	err := wait.PollUntilContextTimeout(context.TODO(), moduleEventsPollInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			eventList, err := events.List(apiClient, eventNamespace)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("error listing events: %v", err)

				return false, nil
			}

			loadedCount, unloadedCount = 0, 0
			for _, event := range eventList {
				if event.Object.Reason == kmmparams.ReasonModuleLoaded && event.Object.Message == loadedMessage {
					loadedCount++
				}

				if event.Object.Reason == kmmparams.ReasonModuleUnloaded && event.Object.Message == unloadedMessage {
					unloadedCount++
				}
			}

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
