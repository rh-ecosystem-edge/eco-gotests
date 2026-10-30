package hwolenv

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
)

// CreateOwnedPod creates a pod atomically and waits for the same UID to run.
func CreateOwnedPod(builder *pod.Builder, runID string, timeout time.Duration) (*pod.Builder, error) {
	if APIClient == nil || APIClient.K8sClient == nil || builder == nil || builder.Definition == nil || runID == "" {
		return nil, fmt.Errorf("API client, pod definition and run ID are required")
	}
	if builder.Definition.Labels == nil {
		builder.Definition.Labels = map[string]string{}
	}
	builder.Definition.Labels[tsparams.TestRunLabelKey] = runID
	pods := APIClient.K8sClient.CoreV1().Pods(builder.Definition.Namespace)
	created, err := pods.Create(context.TODO(), builder.Definition, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod %s/%s: %w", builder.Definition.Namespace, builder.Definition.Name, err)
	}
	if created.UID == "" {
		return builder, fmt.Errorf("created pod %s/%s has no UID; refusing unsafe cleanup", created.Namespace, created.Name)
	}
	builder.Definition.UID = created.UID
	builder.Object = created
	if err := waitForOwnedPod(pods, created.Name, created.UID, runID, timeout); err != nil {
		if cleanupErr := deleteOwnedPod(pods, created.Name, created.UID, runID, timeout); cleanupErr != nil {
			return builder, fmt.Errorf("pod did not become running: %v; cleanup failed: %w", err, cleanupErr)
		}
		return builder, err
	}
	return builder, nil
}

// DeleteOwnedPod deletes only the exact pod created by this run and waits for deletion.
func DeleteOwnedPod(builder *pod.Builder, timeout time.Duration) error {
	if APIClient == nil || APIClient.K8sClient == nil || builder == nil || builder.Definition == nil {
		return fmt.Errorf("API client and pod definition are required")
	}
	return deleteOwnedPod(APIClient.K8sClient.CoreV1().Pods(builder.Definition.Namespace),
		builder.Definition.Name, builder.Definition.UID, builder.Definition.Labels[tsparams.TestRunLabelKey], timeout)
}

// RefreshOwnedPod verifies identity before consuming a pod's status or executing a command.
func RefreshOwnedPod(builder *pod.Builder) error {
	if APIClient == nil || APIClient.K8sClient == nil || builder == nil || builder.Definition == nil ||
		builder.Definition.UID == "" || builder.Definition.Labels[tsparams.TestRunLabelKey] == "" {
		return fmt.Errorf("API client and owned pod identity are required")
	}
	current, err := APIClient.K8sClient.CoreV1().Pods(builder.Definition.Namespace).
		Get(context.TODO(), builder.Definition.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if current.UID != builder.Definition.UID ||
		current.Labels[tsparams.TestRunLabelKey] != builder.Definition.Labels[tsparams.TestRunLabelKey] {
		return fmt.Errorf("pod %s/%s was replaced or its ownership changed", current.Namespace, current.Name)
	}
	builder.Object = current
	return nil
}

func waitForOwnedPod(pods coreclient.PodInterface, name string, uid types.UID, runID string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		current, err := pods.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if current.UID != uid || current.Labels[tsparams.TestRunLabelKey] != runID {
			return false, fmt.Errorf("pod %s was replaced or its ownership changed", name)
		}
		return current.Status.Phase == corev1.PodRunning, nil
	})
}

func deleteOwnedPod(pods coreclient.PodInterface, name string, uid types.UID, runID string, timeout time.Duration) error {
	if pods == nil || name == "" || uid == "" || runID == "" {
		return fmt.Errorf("pod client, name, UID and run ID are required")
	}
	current, err := pods.Get(context.TODO(), name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.UID != uid || current.Labels[tsparams.TestRunLabelKey] != runID {
		return fmt.Errorf("refusing to delete pod %s: UID or ownership changed", name)
	}
	rv := current.ResourceVersion
	err = pods.Delete(context.TODO(), name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{
		UID: &uid, ResourceVersion: &rv,
	}})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete owned pod %s: %w", name, err)
	}
	return wait.PollUntilContextTimeout(context.Background(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		current, err := pods.Get(ctx, name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if current.UID != uid {
			return false, fmt.Errorf("pod %s was replaced during cleanup", name)
		}
		return false, nil
	})
}
