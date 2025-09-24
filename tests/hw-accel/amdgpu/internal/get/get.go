package get

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
)

// PodsFromNamespaceByPrefixWithTimeout - Gets all pods in a namespace whose names start with a specified prefix.
func PodsFromNamespaceByPrefixWithTimeout(
	apiClient *clients.Settings, nsname string, prefix string) ([]*pod.Builder, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout period has been exceeded while "+
				"waiting for Pods with prefix '%s' in namespace '%s'", prefix, nsname)
		case <-time.After(amdgpuparams.DefaultSleepInterval):
			podBuilders, podsListErr := pod.List(apiClient, nsname)
			if podsListErr != nil {
				return nil, fmt.Errorf("failed to list Pods in namespace '%s'.\n%w", nsname, podsListErr)
			}

			var podsWithPrefix []*pod.Builder

			for _, podBuilder := range podBuilders {
				if strings.HasPrefix(podBuilder.Object.Name, prefix) {
					podsWithPrefix = append(podsWithPrefix, podBuilder)
				}
			}

			if len(podsWithPrefix) > 0 {
				return podsWithPrefix, nil
			}
		}
	}
}
