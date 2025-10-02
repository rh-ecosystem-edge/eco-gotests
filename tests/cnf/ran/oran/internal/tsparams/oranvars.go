package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	pluginsv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/plugins/v1alpha1"
	hardwaremanagementv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/v1alpha1"
	inventoryv1alpha1 "github.com/openshift-kni/oran-o2ims/api/inventory/v1alpha1"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var (
	// Labels is the labels applied to all test cases in the suite.
	Labels = append(ranparam.Labels, LabelSuite)

	// ReporterHubNamespacesToDump tells the reporter which namespaces on the hub to collect pod logs from.
	ReporterHubNamespacesToDump = map[string]string{
		TestName:            "",
		O2IMSNamespace:      "",
		SubscriberNamespace: "",
	}

	// ReporterSpokeNamespacesToDump tells the reporter which namespaces on the spoke to collect pod logs from.
	ReporterSpokeNamespacesToDump = map[string]string{
		TestName: "",
	}

	// ReporterHubCRsToDump is the CRs the reporter should dump on the hub.
	ReporterHubCRsToDump = []k8sreporter.CRData{
		{Cr: &provisioningv1alpha1.ClusterTemplateList{}},
		{Cr: &provisioningv1alpha1.ProvisioningRequestList{}},
		{Cr: &hardwaremanagementv1alpha1.HardwarePluginList{}},
		{Cr: &hardwaremanagementv1alpha1.HardwareProfileList{}},
		{Cr: &hardwaremanagementv1alpha1.HardwareTemplateList{}},
		{Cr: &pluginsv1alpha1.AllocatedNodeList{}},
		{Cr: &pluginsv1alpha1.NodeAllocationRequestList{}},
		{Cr: &inventoryv1alpha1.InventoryList{}},
		{Cr: &policiesv1.PolicyList{}},
		{Cr: &siteconfigv1alpha1.ClusterInstanceList{}},
	}

	// ReporterSpokeCRsToDump is the CRs the reporter should dump on the spoke.
	ReporterSpokeCRsToDump = []k8sreporter.CRData{
		{Cr: &corev1.ConfigMapList{}, Namespace: ptr.To(TestName)},
		{Cr: &policiesv1.PolicyList{}},
	}
)

var (
	// PRValidationFailedCondition is the ProvisioningRequest condition where ProvisioningRequest validation failed.
	PRValidationFailedCondition = metav1.Condition{
		Type:   string(provisioningv1alpha1.PRconditionTypes.Validated),
		Reason: string(provisioningv1alpha1.CRconditionReasons.Failed),
		Status: metav1.ConditionFalse,
	}
	// PRValidationSucceededCondition is the ProvisioningRequest condition where ProvisioningRequest validation
	// succeeded.
	PRValidationSucceededCondition = metav1.Condition{
		Type:   string(provisioningv1alpha1.PRconditionTypes.Validated),
		Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
		Status: metav1.ConditionTrue,
	}
	// PRConfigurationAppliedCondition is the ProvisioningRequest condition where applying day2 configuration
	// succeeds.
	PRConfigurationAppliedCondition = metav1.Condition{
		Type:   string(provisioningv1alpha1.PRconditionTypes.ConfigurationApplied),
		Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
		Status: metav1.ConditionTrue,
	}
	// PRCIProcesssedCondition is the ProvisioningRequest condition where the ClusterInstance has successfully been
	// processed.
	PRCIProcesssedCondition = metav1.Condition{
		Type:   string(provisioningv1alpha1.PRconditionTypes.ClusterInstanceProcessed),
		Reason: string(provisioningv1alpha1.CRconditionReasons.Completed),
		Status: metav1.ConditionTrue,
	}

	// CTInvalidSchemaCondition is the ClusterTemplate condition where the validation failed due to invalid schema.
	CTInvalidSchemaCondition = metav1.Condition{
		Type:    string(provisioningv1alpha1.CTconditionTypes.Validated),
		Reason:  string(provisioningv1alpha1.CTconditionReasons.Failed),
		Status:  metav1.ConditionFalse,
		Message: "Error validating the clusterInstanceParameters schema",
	}
)
