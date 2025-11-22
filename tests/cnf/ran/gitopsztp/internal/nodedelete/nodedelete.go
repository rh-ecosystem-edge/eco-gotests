package nodedelete

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// GetBmhNamespace returns the namespace for the specified BareMetalHost, if it exists.
func GetBmhNamespace(client *clients.Settings, bmhName string) (string, error) {
	bmhList, err := bmh.ListInAllNamespaces(client)
	if err != nil {
		return "", err
	}

	for _, bmhBuilder := range bmhList {
		if bmhBuilder.Definition.Name == bmhName {
			return bmhBuilder.Definition.Namespace, nil
		}
	}

	return "", fmt.Errorf("BareMetalHost %s not found", bmhName)
}

// WaitForBMHDeprovisioning waits up to the specified timeout till the BMH and agent with the provided name and
// namespace are no longer found.
func WaitForBMHDeprovisioning(client *clients.Settings, name, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), tsparams.ArgoCdChangeInterval, timeout, true, func(ctx context.Context) (bool, error) {
			klog.V(tsparams.LogLevel).Infof("Checking if BareMetalHost %s in namespace %s is deprovisioned", name, namespace)

			_, err := bmh.Pull(client, name, namespace)
			if err == nil {
				return false, nil
			}

			_, err = assisted.PullAgent(client, name, namespace)
			if err == nil {
				return false, nil
			}

			return true, nil
		})
}
