package pod

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// List returns pod inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string, options ...metav1.ListOptions) ([]*Builder, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("podList 'apiClient' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("pod 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list pods, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing pods in the nsname %s", nsname)
	passedOptions := metav1.ListOptions{}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	} else if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	klog.V(100).Infof("%v", logMessage)

	podList, err := apiClient.Pods(nsname).List(logging.DiscardContext(), passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list pods in the nsname %s due to %s", nsname, err.Error())

		return nil, err
	}

	var podObjects []*Builder

	for _, runningPod := range podList.Items {
		copiedPod := runningPod
		podBuilder := &Builder{
			apiClient:  apiClient,
			Object:     &copiedPod,
			Definition: &copiedPod,
		}

		podObjects = append(podObjects, podBuilder)
	}

	return podObjects, nil
}

// ListInAllNamespaces returns a cluster-wide pod inventory.
func ListInAllNamespaces(apiClient *clients.Settings, options ...metav1.ListOptions) ([]*Builder, error) {
	logMessage := "Listing all pods in all namespaces"
	passedOptions := metav1.ListOptions{}

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("podList 'apiClient' cannot be empty")
	}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	podList, err := apiClient.Pods("").List(logging.DiscardContext(), passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list all pods due to %s", err.Error())

		return nil, err
	}

	var podObjects []*Builder

	for _, runningPod := range podList.Items {
		copiedPod := runningPod
		podBuilder := &Builder{
			apiClient:  apiClient,
			Object:     &copiedPod,
			Definition: &copiedPod,
		}

		podObjects = append(podObjects, podBuilder)
	}

	return podObjects, nil
}

// ListByNamePattern returns pod inventory in the given namespace filtered by name pattern.
func ListByNamePattern(apiClient *clients.Settings, namePattern, nsname string) ([]*Builder, error) {
	klog.V(100).Infof("Listing pods in the nsname %s filtered by the name pattern %s", nsname, namePattern)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("podList 'apiClient' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("pod 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list pods, 'nsname' parameter is empty")
	}

	podList, err := apiClient.Pods(nsname).List(logging.DiscardContext(), metav1.ListOptions{})
	if err != nil {
		klog.V(100).Infof("Failed to list pods filtered by the name pattern %s in the nsname %s due to %s",
			namePattern, nsname, err.Error())

		return nil, err
	}

	var podObjects []*Builder

	for _, runningPod := range podList.Items {
		if strings.Contains(runningPod.Name, namePattern) {
			copiedPod := runningPod
			podBuilder := &Builder{
				apiClient:  apiClient,
				Object:     &copiedPod,
				Definition: &copiedPod,
			}

			podObjects = append(podObjects, podBuilder)
		}
	}

	return podObjects, nil
}

// WaitForAllPodsInNamespaceRunning wait until all pods in namespace that match options are in running state.
func WaitForAllPodsInNamespaceRunning(
	apiClient *clients.Settings,
	nsname string,
	timeout time.Duration,
	options ...metav1.ListOptions) (bool, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return false, fmt.Errorf("podList 'apiClient' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("'nsname' parameter can not be empty")

		return false, fmt.Errorf("failed to list pods, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Waiting for all pods in %s namespace", nsname)
	passedOptions := metav1.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return false, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%s are in running state", logMessage)

	podList, err := List(apiClient, nsname, passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list all pods due to %s", err.Error())

		return false, err
	}

	for _, podObj := range podList {
		err = podObj.WaitUntilRunning(timeout)
		if err != nil {
			klog.V(100).Infof("Timout was reached while waiting for all pods in running state: %s", err.Error())

			return false, err
		}
	}

	return true, nil
}

// WaitForPodsInNamespacesHealthy waits up to timeout until every pod in namespaces is healthy. Failed pods with
// RestartPolicy of Never are ignored. It works by listing pods every 15 seconds until every listed pod is healthy.
func WaitForPodsInNamespacesHealthy(
	apiClient *clients.Settings, namespaces []string, timeout time.Duration, options ...metav1.ListOptions) error {
	logMessage := fmt.Sprintf("Waiting for all pods in namespaces %v to be healthy", namespaces)
	passedOptions := metav1.ListOptions{}

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return fmt.Errorf("podList 'apiClient' cannot be empty")
	}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Info(logMessage)

	return wait.PollUntilContextTimeout(
		context.TODO(), 15*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pods, err := listPodsInNamespaces(apiClient, namespaces, passedOptions)
			if err != nil {
				return false, nil
			}

			for _, pod := range pods {
				// We use this internal helper to avoid spamming the apiClient since otherwise we may be
				// sending hundreds of requests each iteration.
				if !pod.isObjectHealthy() {
					if pod.Object.Status.Phase == corev1.PodFailed && pod.Object.Spec.RestartPolicy == corev1.RestartPolicyNever {
						continue
					}

					return false, nil
				}
			}

			return true, nil
		})
}

// listPodsInNamespaces lists pods only in the provided namespaces or all namespaces if the provided slice is empty. It
// will not perform validation, passing arguments directly to ListInAllNamespaces or List.
func listPodsInNamespaces(
	apiClient *clients.Settings, namespaces []string, options ...metav1.ListOptions) ([]*Builder, error) {
	if len(namespaces) == 0 {
		return ListInAllNamespaces(apiClient, options...)
	}

	var allPods []*Builder

	for _, namespace := range namespaces {
		namespacePods, err := List(apiClient, namespace, options...)
		if err != nil {
			klog.V(100).Infof("Failed to list pods in namespace %s: %v", namespace, err)

			return nil, err
		}

		allPods = append(allPods, namespacePods...)
	}

	return allPods, nil
}
