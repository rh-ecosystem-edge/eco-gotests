package ocm

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	commonerrors "github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/errors"
	commonkey "github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/key"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListPoliciesInAllNamespaces returns a cluster-wide policy inventory.
func ListPoliciesInAllNamespaces(apiClient *clients.Settings,
	options ...runtimeclient.ListOptions) (
	[]*PolicyBuilder, error) {
	return common.List[policiesv1.Policy, policiesv1.PolicyList, PolicyBuilder](
		context.TODO(), apiClient, policiesv1.AddToScheme, common.ConvertListOptionsToOptions(options)...)
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

		return commonerrors.NewAPIClientNil(commonkey.NewResourceKey(policyGVK.Kind, "", ""))
	}

	logMessage := fmt.Sprintf("Waiting up to %s until policies have compliance state %s", timeout, complianceState)
	if len(options) > 0 {
		logMessage += fmt.Sprintf(", listing with the options %v", options)
	}

	klog.V(100).Info(logMessage)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			policies, err := ListPoliciesInAllNamespaces(apiClient, options...)
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
