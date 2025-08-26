package helper

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewProvisioningRequest creates a ProvisioningRequest builder with templateVersion, setting all the required
// parameters and using the affix from RANConfig.
func NewProvisioningRequest(client runtimeclient.Client, templateVersion string) *oran.ProvisioningRequestBuilder {
	versionWithAffix := RANConfig.ClusterTemplateAffix + "-" + templateVersion
	prBuilder := oran.NewPRBuilder(client, tsparams.TestPRName, tsparams.ClusterTemplateName, versionWithAffix).
		WithTemplateParameter("nodeClusterName", RANConfig.Spoke1Name).
		WithTemplateParameter("oCloudSiteId", tsparams.OCloudSiteID).
		WithTemplateParameter("policyTemplateParameters", map[string]any{}).
		WithTemplateParameter("clusterInstanceParameters", map[string]any{
			"clusterName": RANConfig.Spoke1Name,
			"nodes": []map[string]any{{
				"hostName": RANConfig.Spoke1Hostname,
			}},
		})

	return prBuilder
}

// NewNoTemplatePR creates a ProvisioningRequest builder with templateVersion, following the schema for no
// HardwareTemplate. All required parameters and the affix are set from RANConfig. The BMC and network data are
// incorrect so that a ClusterInstance is generated but will not actually provision.
func NewNoTemplatePR(client runtimeclient.Client, templateVersion string) *oran.ProvisioningRequestBuilder {
	versionWithAffix := RANConfig.ClusterTemplateAffix + "-" + templateVersion
	prBuilder := oran.NewPRBuilder(client, tsparams.TestPRName, tsparams.ClusterTemplateName, versionWithAffix).
		WithTemplateParameter("nodeClusterName", RANConfig.Spoke1Name).
		WithTemplateParameter("oCloudSiteId", tsparams.OCloudSiteID).
		WithTemplateParameter("policyTemplateParameters", map[string]any{}).
		WithTemplateParameter("clusterInstanceParameters", map[string]any{
			"clusterName": RANConfig.Spoke1Name,
			"nodes": []map[string]any{{
				"hostName": "fake.apps." + RANConfig.Spoke1Hostname,
				// 192.0.2.0 is a reserved test address so we never accidentally use a valid IP.
				"bmcAddress": "redfish-VirtualMedia://192.0.2.0/redfish/v1/Systems/System.Embedded.1",
				"bmcCredentialsDetails": map[string]any{
					"username": tsparams.TestBase64Credential,
					"password": tsparams.TestBase64Credential,
				},
				"bootMACAddress": "01:23:45:67:89:AB",
				"nodeNetwork": map[string]any{
					"interfaces": []map[string]any{{
						"macAddress": "01:23:45:67:89:AB",
					}},
				},
			}},
		})

	return prBuilder
}

// WaitForNoncompliantImmutable waits up to timeout until one of the policies in namespace is NonCompliant and the
// message history shows it is due to an immutable field.
func WaitForNoncompliantImmutable(client *clients.Settings, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			policies, err := ocm.ListPoliciesInAllNamespaces(client, runtimeclient.ListOptions{Namespace: namespace})
			if err != nil {
				glog.V(tsparams.LogLevel).Infof("Failed to list all policies in namespace %s: %v", namespace, err)

				return false, nil
			}

			for _, policy := range policies {
				if policy.Definition.Status.ComplianceState == policiesv1.NonCompliant {
					glog.V(tsparams.LogLevel).Infof("Policy %s in namespace %s is not compliant, checking history",
						policy.Definition.Name, policy.Definition.Namespace)

					details := policy.Definition.Status.Details
					if len(details) != 1 {
						continue
					}

					history := details[0].History
					if len(history) < 1 {
						continue
					}

					if strings.Contains(history[0].Message, tsparams.ImmutableMessage) {
						glog.V(tsparams.LogLevel).Infof("Policy %s in namespace %s is not compliant due to an immutable field",
							policy.Definition.Name, policy.Definition.Namespace)

						return true, nil
					}
				}
			}

			return false, nil
		})
}

// WaitForValidPRClusterInstance waits up to timeout until the ClusterInstance for the ProvisioningRequest has condition
// RenderedTemplatesApplied.
func WaitForValidPRClusterInstance(client *clients.Settings, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			clusterInstance, err := siteconfig.PullClusterInstance(client, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			if err != nil {
				glog.V(tsparams.LogLevel).Infof("Failed to pull ClusterInstance %s: %v", RANConfig.Spoke1Name, err)

				return false, nil
			}

			return slices.ContainsFunc(clusterInstance.Definition.Status.Conditions, func(condition metav1.Condition) bool {
				return condition.Type == string(siteconfigv1alpha1.RenderedTemplatesApplied) &&
					condition.Status == metav1.ConditionTrue
			}), nil
		})
}

// WaitForPolicyVersion waits up to timeout until all of the policies in the namespace have the specified version.
// Version is defined as the first hyphen-delimited part of the policy name. Since it lists policies in the spoke
// namespace, it first splits on the period that separates the policy namespace and name before checking the version.
func WaitForPolicyVersion(client *clients.Settings, version string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			policies, err := ocm.ListPoliciesInAllNamespaces(client, runtimeclient.ListOptions{Namespace: RANConfig.Spoke1Name})
			if err != nil {
				glog.V(tsparams.LogLevel).Infof("Failed to list all policies in namespace %s: %v", RANConfig.Spoke1Name, err)

				return false, nil
			}

			for _, policy := range policies {
				policySegments := strings.SplitN(policy.Definition.Name, ".", 2)
				policyName := policySegments[len(policySegments)-1]

				policyVersion := strings.SplitN(policyName, "-", 2)[0]
				if policyVersion != version {
					glog.V(tsparams.LogLevel).Infof("Policy %s in namespace %s has version %s, expected %s",
						policy.Definition.Name, policy.Definition.Namespace, policyVersion, version)

					return false, nil
				}
			}

			return true, nil
		})
}
