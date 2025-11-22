package ocm

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListPoliciesInAllNamespaces returns a cluster-wide policy inventory.
func ListPoliciesInAllNamespaces(apiClient *clients.Settings,
	options ...runtimeclient.ListOptions) (
	[]*PolicyBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("Policies 'apiClient' parameter cannot be nil")

		return nil, fmt.Errorf("failed to list policies, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(policiesv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add Policy scheme to client schemes")

		return nil, err
	}

	logMessage := string("Listing all policies in all namespaces")
	passedOptions := runtimeclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	policyList := new(policiesv1.PolicyList)

	err = apiClient.List(context.TODO(), policyList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list all policies in all namespaces due to %s", err.Error())

		return nil, err
	}

	var policyObjects []*PolicyBuilder

	for _, policy := range policyList.Items {
		copiedPolicy := policy
		policyBuilder := &PolicyBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedPolicy,
			Definition: &copiedPolicy,
		}

		policyObjects = append(policyObjects, policyBuilder)
	}

	return policyObjects, nil
}

// WaitForAllPoliciesComplianceState wait up to timeout until all policies have complianceState. Policies are listed
// with options on every poll and then these policies have their compliance state checked.
func WaitForAllPoliciesComplianceState(
	apiClient *clients.Settings,
	complianceState policiesv1.ComplianceState,
	timeout time.Duration,
	options ...runtimeclient.ListOptions) error {
	if apiClient == nil {
		klog.V(100).Info("Policies 'apiClient' parameter cannot be nil")

		return fmt.Errorf("failed to wait for policies compliance state, 'apiClient' parameter is nil")
	}

	err := apiClient.AttachScheme(policiesv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add Policy scheme to client schemes")

		return err
	}

	logMessage := fmt.Sprintf("Waiting up to %s until policies have compliance state %s", timeout, complianceState)
	passedOptions := runtimeclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(", listing with the options %v", passedOptions)
	}

	klog.V(100).Info(logMessage)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			policies, err := ListPoliciesInAllNamespaces(apiClient, passedOptions)
			if err != nil {
				klog.V(100).Infof("Failed to list policies while waiting for compliance state: %v", err)

				return false, nil
			}

			for _, policy := range policies {
				policyComplianceState := policy.Definition.Status.ComplianceState
				if policyComplianceState != complianceState {
					klog.V(100).Infof("Policy %s in namespace %s has compliance state %s, not %s",
						policy.Definition.Name, policy.Definition.Namespace, policyComplianceState, complianceState)

					return false, nil
				}
			}

			return true, nil
		})
}
