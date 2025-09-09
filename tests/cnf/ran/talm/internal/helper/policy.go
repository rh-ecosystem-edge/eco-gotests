package helper

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policiesv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// CreatePolicy creates a single policy enforcing the compliance type of the provided object.
func CreatePolicy(
	client *clients.Settings,
	object runtime.Object,
	suffix string) (*ocm.PolicyBuilder, error) {
	configurationPolicy := getConfigurationPolicy(tsparams.PolicyName+suffix, object)
	policy := ocm.NewPolicyBuilder(client, tsparams.PolicyName+suffix, tsparams.TestNamespace, &policiesv1.PolicyTemplate{
		ObjectDefinition: runtime.RawExtension{
			Object: &configurationPolicy,
		},
	}).WithRemediationAction(policiesv1.Inform)

	return policy.Create()
}

// CreatePolicyComponents defines and creates the policy components like the policy set, placement field, placement
// rule, and placement binding.
func CreatePolicyComponents(
	client *clients.Settings,
	suffix string,
	clusters []string,
	clusterSelector metav1.LabelSelector) error {
	policySet := ocm.NewPolicySetBuilder(
		client,
		tsparams.PolicySetName+suffix,
		tsparams.TestNamespace,
		policiesv1beta1.NonEmptyString(tsparams.PolicyName+suffix))

	_, err := policySet.Create()
	if err != nil {
		return err
	}

	fields := getPlacementField(clusters, clusterSelector)
	placementRule := ocm.NewPlacementRuleBuilder(client, tsparams.PlacementRuleName+suffix, tsparams.TestNamespace)
	placementRule.Definition.Spec.GenericPlacementFields = fields

	_, err = placementRule.Create()
	if err != nil {
		return err
	}

	placementBinding := ocm.NewPlacementBindingBuilder(
		client,
		tsparams.PlacementBindingName+suffix,
		tsparams.TestNamespace,
		policiesv1.PlacementSubject{
			Name:     tsparams.PlacementRuleName + suffix,
			APIGroup: "apps.open-cluster-management.io",
			Kind:     "PlacementRule",
		}, policiesv1.Subject{

			Name:     tsparams.PolicySetName + suffix,
			APIGroup: "policy.open-cluster-management.io",
			Kind:     "PolicySet",
		})

	_, err = placementBinding.Create()

	return err
}

// GetPolicyNameWithPrefix returns the name of the first policy to start with prefix in the provided namespace, or an
// empty string if no such policy exists.
func GetPolicyNameWithPrefix(client *clients.Settings, prefix, namespace string) (string, error) {
	policies, err := ocm.ListPoliciesInAllNamespaces(client, runtimeclient.ListOptions{Namespace: namespace})
	if err != nil {
		return "", err
	}

	for _, policy := range policies {
		if strings.HasPrefix(policy.Object.Name, prefix) {
			return policy.Object.Name, nil
		}
	}

	return "", nil
}

// getConfigurationPolicy is used to get a configuration policy that contains the provided object.
func getConfigurationPolicy(policyName string, object runtime.Object) configurationPolicyv1.ConfigurationPolicy {
	return configurationPolicyv1.ConfigurationPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigurationPolicy",
			APIVersion: "policy.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-config", policyName),
		},
		Spec: &configurationPolicyv1.ConfigurationPolicySpec{
			Severity:          "low",
			RemediationAction: configurationPolicyv1.Inform,
			NamespaceSelector: configurationPolicyv1.Target{
				Include: []configurationPolicyv1.NonEmptyString{"kube-*"},
				Exclude: []configurationPolicyv1.NonEmptyString{"*"},
			},
			ObjectTemplates: []*configurationPolicyv1.ObjectTemplate{
				{
					ComplianceType: configurationPolicyv1.MustHave,
					ObjectDefinition: runtime.RawExtension{
						Object: object,
					},
				},
			},
			EvaluationInterval: configurationPolicyv1.EvaluationInterval{
				Compliant:    "10s",
				NonCompliant: "10s",
			},
		},
	}
}

// getPlacementField is used to get a generic placement field for use with a placement rule.
func getPlacementField(clusters []string, clusterSelector metav1.LabelSelector) placementrulev1.GenericPlacementFields {
	clustersPlacementField := []placementrulev1.GenericClusterReference{}
	for _, cluster := range clusters {
		clustersPlacementField = append(clustersPlacementField, placementrulev1.GenericClusterReference{Name: cluster})
	}

	return placementrulev1.GenericPlacementFields{
		Clusters:        clustersPlacementField,
		ClusterSelector: &clusterSelector,
	}
}
