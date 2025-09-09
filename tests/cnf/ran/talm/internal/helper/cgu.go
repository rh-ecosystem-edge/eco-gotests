package helper

import (
	"context"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	operatorsv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
)

// WaitForCguBlocked waits up to the timeout until the provided cguBuilder matches the condition for being blocked.
func WaitForCguBlocked(cguBuilder *cgu.CguBuilder, message string) error {
	blockedCondition := metav1.Condition{
		Type:    tsparams.ProgressingType,
		Status:  metav1.ConditionFalse,
		Message: message,
	}

	_, err := cguBuilder.WaitForCondition(blockedCondition, 6*time.Minute)

	return err
}

// SetupCguWithNamespace creates the policy with a namespace and its components for a cguBuilder then creates the
// cguBuilder.
func SetupCguWithNamespace(cguBuilder *cgu.CguBuilder, suffix string) (*cgu.CguBuilder, error) {
	// The client doesn't matter since we only want the definition. Kind and APIVersion are necessary for TALM.
	tempNs := namespace.NewBuilder(HubAPIClient, tsparams.TemporaryNamespace+suffix)
	tempNs.Definition.Kind = "Namespace"
	tempNs.Definition.APIVersion = corev1.SchemeGroupVersion.Version

	_, err := CreatePolicy(HubAPIClient, tempNs.Definition, suffix)
	if err != nil {
		return nil, err
	}

	err = CreatePolicyComponents(
		HubAPIClient, suffix, cguBuilder.Definition.Spec.Clusters, metav1.LabelSelector{})
	if err != nil {
		return nil, err
	}

	err = waitForPolicyComponentsExist(HubAPIClient, suffix)
	if err != nil {
		return nil, err
	}

	return cguBuilder.Create()
}

// SetupCguWithCatSrc creates the policy with a catalog source and its components for a cguBuilder then creates the
// cguBuilder.
func SetupCguWithCatSrc(cguBuilder *cgu.CguBuilder) (*cgu.CguBuilder, error) {
	// The client doesn't matter since we only want the definition. Kind and APIVersion are necessary for TALM.
	catsrc := olm.NewCatalogSourceBuilder(
		HubAPIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace)
	catsrc.Definition.Spec.SourceType = operatorsv1alpha1.SourceTypeInternal
	catsrc.Definition.Spec.Priority = 1
	catsrc.Definition.Kind = "CatalogSource"
	catsrc.Definition.APIVersion = "operators.coreos.com/v1alpha1"

	_, err := CreatePolicy(HubAPIClient, catsrc.Definition, "")
	if err != nil {
		return nil, err
	}

	err = CreatePolicyComponents(
		HubAPIClient, "", cguBuilder.Definition.Spec.Clusters, metav1.LabelSelector{})
	if err != nil {
		return nil, err
	}

	err = waitForPolicyComponentsExist(HubAPIClient, "")
	if err != nil {
		return nil, err
	}

	return cguBuilder.Create()
}

// WaitToEnableCgu waits for the TalmSystemStablizationTime before enabling the CGU.
func WaitToEnableCgu(cguBuilder *cgu.CguBuilder) (*cgu.CguBuilder, error) {
	time.Sleep(tsparams.TalmSystemStablizationTime)

	cguBuilder.Definition.Spec.Enable = ptr.To(true)

	return cguBuilder.Update(true)
}

// SetupCguWithClusterVersion creates the policy with the provided clustrer version and its components for a cguBuilder
// then creates the cguBuilder.
func SetupCguWithClusterVersion(
	cguBuilder *cgu.CguBuilder, clusterVersion *configv1.ClusterVersion) (*cgu.CguBuilder, error) {
	_, err := CreatePolicy(HubAPIClient, clusterVersion, "")
	if err != nil {
		return nil, err
	}

	err = CreatePolicyComponents(
		HubAPIClient, "", cguBuilder.Definition.Spec.Clusters, metav1.LabelSelector{})
	if err != nil {
		return nil, err
	}

	err = waitForPolicyComponentsExist(HubAPIClient, "")
	if err != nil {
		return nil, err
	}

	return cguBuilder.Create()
}

// waitForPolicyComponentsExist waits until all the policy components exist on the provided client.
func waitForPolicyComponentsExist(client *clients.Settings, suffix string) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 15*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
			// Check for definitions being nil too since sometimes the err is nil when the resource does not
			// exist on the cluster.
			policy, err := ocm.PullPolicy(client, tsparams.PolicyName+suffix, tsparams.TestNamespace)
			if err != nil || policy.Definition == nil {
				return false, nil
			}

			policySet, err := ocm.PullPolicySet(client, tsparams.PolicySetName+suffix, tsparams.TestNamespace)
			if err != nil || policySet.Definition == nil {
				return false, nil
			}

			placementRule, err := ocm.PullPlacementRule(client, tsparams.PlacementRuleName+suffix, tsparams.TestNamespace)
			if err != nil || placementRule.Definition == nil {
				return false, nil
			}

			placementBinding, err := ocm.PullPlacementBinding(
				client, tsparams.PlacementBindingName+suffix, tsparams.TestNamespace)
			if err != nil || placementBinding.Definition == nil {
				return false, nil
			}

			return true, nil
		})
}
