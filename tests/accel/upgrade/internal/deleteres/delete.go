package deleteres

import (
	"context"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/accel/upgrade/internal/upgradeparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	retryDurationSecs = 360
	pollIntervalSecs  = 20
)

// Workload deletes a workload.
// Return nil on success, otherwise returns an error.
func Workload(apiClient *clients.Settings) error {
	var (
		oldPods []*pod.Builder
		err     error
	)

	pollSuccess := false

	err = wait.PollUntilContextTimeout(
		context.TODO(), pollIntervalSecs, retryDurationSecs, true, func(ctx context.Context) (bool, error) {
			oldPods, err = pod.List(apiClient, upgradeparams.TestNamespaceName,
				metav1.ListOptions{LabelSelector: upgradeparams.ContainerLabelsStr})
			if err != nil {
				return false, nil
			}

			pollSuccess = true

			klog.V(90).Infof("Found %d pods matching label %q ",
				len(oldPods), upgradeparams.ContainerLabelsStr)

			return true, nil
		})

	if !pollSuccess {
		klog.V(90).Infof("Error listing pods in %q namespace",
			upgradeparams.TestNamespaceName)

		return err
	}

	if len(oldPods) == 0 {
		klog.V(90).Infof("No pods matching label %q found in %q namespace",
			upgradeparams.ContainerLabelsStr, upgradeparams.TestNamespaceName)
	}

	for _, _pod := range oldPods {
		klog.V(90).Infof("Deleting pod %q in %q namspace",
			_pod.Definition.Name, _pod.Definition.Namespace)

		_pod, err = _pod.DeleteAndWait(300 * time.Second)
		if err != nil {
			klog.V(90).Infof("Failed to delete pod %q: %v",
				_pod.Definition.Name, err)

			return err
		}
	}

	return nil
}

// Service deletes a service.
// Returns nil on success, otherwise returns an error.
func Service(apiClient *clients.Settings) error {
	klog.V(90).Infof("Deleting Service %q in %q namespace",
		upgradeparams.DeploymentName, upgradeparams.TestNamespaceName)

	svcDemo, err := service.Pull(apiClient, upgradeparams.DeploymentName, upgradeparams.TestNamespaceName)

	if err != nil && svcDemo == nil {
		klog.V(90).Infof("Service %q not found in %q namespace",
			upgradeparams.DeploymentName, upgradeparams.TestNamespaceName)

		return err
	}

	err = svcDemo.Delete()
	if err != nil {
		klog.V(90).Infof("Error deleting service: %v", err)

		return err
	}

	klog.V(90).Infof("Deleted service %q in %q namespace",
		upgradeparams.DeploymentName, upgradeparams.TestNamespaceName)

	return nil
}

// Namespace deletes the workload test namespace.
func Namespace(apiClient *clients.Settings) error {
	klog.V(90).Infof("Deleting namespace %q", upgradeparams.TestNamespaceName)

	nsDemo, err := namespace.Pull(apiClient, upgradeparams.TestNamespaceName)

	if err != nil && nsDemo == nil {
		klog.V(90).Infof("Namespace %q not found", upgradeparams.TestNamespaceName)

		return err
	}

	err = nsDemo.DeleteAndWait(5 * time.Minute)
	if err != nil {
		klog.V(90).Infof("Error deleting namespace: %v", err)

		return err
	}

	klog.V(90).Infof("Deleted namespace %q", upgradeparams.TestNamespaceName)

	return nil
}
