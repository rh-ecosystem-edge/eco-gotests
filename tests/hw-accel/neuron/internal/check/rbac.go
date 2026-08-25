package check

import (
	"context"
	"slices"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// GetClusterRole retrieves a ClusterRole by name.
func GetClusterRole(apiClient *clients.Settings, name string) (*rbacv1.ClusterRole, error) {
	klog.V(params.NeuronLogLevel).Infof("Getting ClusterRole %s", name)

	return apiClient.K8sClient.RbacV1().ClusterRoles().Get(
		context.TODO(), name, metav1.GetOptions{})
}

// HasPolicyRule checks if a set of PolicyRules grants all specified verbs
// for a given API group and resource within a single rule.
func HasPolicyRule(rules []rbacv1.PolicyRule, apiGroup, resource string, verbs []string) bool {
	for _, rule := range rules {
		if !slices.Contains(rule.APIGroups, apiGroup) || !slices.Contains(rule.Resources, resource) {
			continue
		}

		allFound := true

		for _, verb := range verbs {
			if !slices.Contains(rule.Verbs, verb) {
				allFound = false

				break
			}
		}

		if allFound {
			return true
		}
	}

	return false
}

// HasSCCRule checks if a set of PolicyRules grants 'use' on a named SecurityContextConstraint.
func HasSCCRule(rules []rbacv1.PolicyRule, sccName string) bool {
	for _, rule := range rules {
		if slices.Contains(rule.APIGroups, "security.openshift.io") &&
			slices.Contains(rule.Resources, "securitycontextconstraints") &&
			slices.Contains(rule.Verbs, "use") &&
			slices.Contains(rule.ResourceNames, sccName) {
			return true
		}
	}

	return false
}
