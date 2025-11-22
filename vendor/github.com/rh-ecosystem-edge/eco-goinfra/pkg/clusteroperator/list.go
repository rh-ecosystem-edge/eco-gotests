package clusteroperator

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// List returns clusterOperators inventory.
func List(apiClient *clients.Settings, options ...metav1.ListOptions) ([]*Builder, error) {
	logMessage := "Listing all clusterOperators"
	passedOptions := metav1.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	coList, err := apiClient.ClusterOperators().List(logging.DiscardContext(), passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list clusterOperators due to %s", err.Error())

		return nil, err
	}

	var coObjects []*Builder

	for _, clusterOperator := range coList.Items {
		copiedCo := clusterOperator
		coBuilder := &Builder{
			apiClient:  apiClient.Client,
			Object:     &copiedCo,
			Definition: &copiedCo,
		}

		coObjects = append(coObjects, coBuilder)
	}

	return coObjects, nil
}

// WaitForAllClusteroperatorsAvailable waits until all clusterOperators are in available state.
func WaitForAllClusteroperatorsAvailable(
	apiClient *clients.Settings, timeout time.Duration, options ...metav1.ListOptions) (bool, error) {
	klog.V(100).Info("Waiting for all clusterOperators to be in available state")

	err := wait.PollUntilContextTimeout(context.TODO(), fiveScds, timeout, true, func(ctx context.Context) (bool, error) {
		coList, err := List(apiClient, options...)
		if err != nil {
			klog.V(100).Infof("Failed to list all clusterOperators due to %s", err.Error())

			return false, err
		}

		for _, clusteroperator := range coList {
			if !clusteroperator.IsAvailable() {
				klog.V(100).Infof("The %s clusterOperator is not available",
					clusteroperator.Object.Name)

				return false, nil
			}
		}

		return true, nil
	})
	if err == nil {
		klog.V(100).Infof("All clusterOperators were found available before timeout: %v",
			timeout)

		return true, nil
	}

	// Here err is "timed out waiting for the condition"
	klog.V(100).Infof("Not all clusterOperators were found available before timeout: %v",
		timeout)

	return false, err
}

// WaitForAllClusteroperatorsStopProgressing waits until all clusterOperators stopped progressing.
func WaitForAllClusteroperatorsStopProgressing(
	apiClient *clients.Settings, timeout time.Duration, options ...metav1.ListOptions) (bool, error) {
	klog.V(100).Info("Waiting for all clusteroperators to stop progressing")

	coList, err := List(apiClient, options...)
	if err != nil {
		klog.V(100).Infof("Failed to list all clusterOperators due to %s", err.Error())

		return false, err
	}

	err = wait.PollUntilContextTimeout(context.TODO(), fiveScds, timeout, true, func(ctx context.Context) (bool, error) {
		for _, clusteroperator := range coList {
			if clusteroperator.IsProgressing() {
				klog.V(100).Infof("The %s clusterOperator is still progressing",
					clusteroperator.Object.Name)

				return false, nil
			}
		}

		return true, nil
	})
	if err == nil {
		klog.V(100).Infof("All clusterOperators stopped progressing before timeout: %v",
			timeout)

		return true, nil
	}

	// Here err is "timed out waiting for the condition"
	klog.V(100).Infof("Not all clusterOperators stopped progressing before timeout: %v",
		timeout)

	return false, err
}

// VerifyClusterOperatorsVersion checks if all the clusterOperators have version desiredVersion.
func VerifyClusterOperatorsVersion(desiredVersion string, clusterOperatorList []*Builder,
	options ...metav1.ListOptions) (bool, error) {
	if desiredVersion == "" {
		return false, fmt.Errorf("desiredVersion can't be empty")
	}

	if clusterOperatorList == nil {
		return false, fmt.Errorf("clusterOperatorList is invalid")
	}

	if len(clusterOperatorList) == 0 {
		return false, fmt.Errorf("clusterOperatorList can't be empty")
	}

	klog.V(100).Infof("Checking if all the operators have version desiredVersion %s", desiredVersion)

	for _, operator := range clusterOperatorList {
		klog.V(100).Infof("Checking %s clusterOperator version", operator.Definition.Name)

		hasDesiredVersion, err := operator.HasDesiredVersion(desiredVersion)
		if err != nil {
			return false, err
		}

		if !hasDesiredVersion {
			return false, fmt.Errorf("the clusterOperator %s doesn't have the desired version %s",
				operator.Definition.Name, desiredVersion)
		}
	}

	return true, nil
}
