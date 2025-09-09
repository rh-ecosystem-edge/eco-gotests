package tsparams

import (
	"fmt"

	cguv1alpha1 "github.com/openshift-kni/cluster-group-upgrades-operator/pkg/api/clustergroupupgrades/v1alpha1"
	"github.com/openshift-kni/k8sreporter"
	olmv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policiesv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(ranparam.Labels, LabelSuite)

	// ReporterHubNamespacesToDump tells to the reporter which namespaces on the hub to collect pod logs from.
	ReporterHubNamespacesToDump = map[string]string{
		TestNamespace:                       "",
		ranparam.OpenshiftOperatorNamespace: "",
		ranparam.AcmOperatorNamespace:       "",
	}

	// ReporterSpokeNamespacesToDump tells the reporter which namespaces on the spokes to collect pod logs from.
	ReporterSpokeNamespacesToDump = map[string]string{
		TemporaryNamespace:  "",
		PreCacheSpokeNS:     "",
		TalmBackupNamespace: "",
	}

	// ReporterHubCRsToDump is the CRs the reporter should dump on the hub.
	ReporterHubCRsToDump = []k8sreporter.CRData{
		{Cr: &corev1.NamespaceList{}},
		{Cr: &corev1.PodList{}},
		{Cr: &cguv1alpha1.ClusterGroupUpgradeList{}, Namespace: ptr.To(TestNamespace)},
		{Cr: &cguv1alpha1.PreCachingConfigList{}, Namespace: ptr.To(TestNamespace)},
		{Cr: &policiesv1.PolicyList{}},
		{Cr: &policiesv1.PlacementBindingList{}, Namespace: ptr.To(TestNamespace)},
		{Cr: &placementrulev1.PlacementRuleList{}, Namespace: ptr.To(TestNamespace)},
		{Cr: &policiesv1beta1.PolicySetList{}, Namespace: ptr.To(TestNamespace)},
	}

	// ReporterSpokeCRsToDump is the CRs the reporter should dump on the spokes.
	ReporterSpokeCRsToDump = []k8sreporter.CRData{
		{Cr: &corev1.NamespaceList{}},
		{Cr: &corev1.PodList{}},
		{Cr: &olmv1alpha1.CatalogSourceList{}, Namespace: ptr.To(TemporaryNamespace)},
		{Cr: &policiesv1.PolicyList{}},
	}

	// TalmNonExistentClusterMessage is the condition message for when a cluster is non-existent.
	TalmNonExistentClusterMessage = fmt.Sprintf(
		"Unable to select clusters: cluster %s is not a ManagedCluster", NonExistentClusterName)
	// TalmNonExistentPolicyMessage is the condition message for when a policy is non-existent.
	TalmNonExistentPolicyMessage = fmt.Sprintf("Missing managed policies: [%s]", NonExistentPolicyName)

	// CguTimeoutReasonCondition is the condition when the CGU has timed out.
	CguTimeoutReasonCondition = metav1.Condition{Type: SucceededType, Reason: TimedOutReason}
	// CguTimeoutMessageCondition is the condition when the CGU has timed out since policy remidation took too long.
	CguTimeoutMessageCondition = metav1.Condition{Type: SucceededType, Message: TalmTimeoutMessage}
	// CguTimeoutCanaryCondition is the condition when the CGU has timed out due to canary policy remediation.
	CguTimeoutCanaryCondition = metav1.Condition{Type: SucceededType, Message: TalmCanaryTimeoutMessage}
	// CguSuccessfulFinishCondition is the condition when the CGU has completed.
	CguSuccessfulFinishCondition = metav1.Condition{Type: SucceededType, Reason: CompletedReason}
	// CguSucceededCondition is the condition when the CGU has succeeded for any reason.
	CguSucceededCondition = metav1.Condition{Type: SucceededType, Status: metav1.ConditionTrue}
	// CguNonExistentClusterCondition is the condition when the CGU has a non-existent cluster.
	CguNonExistentClusterCondition = metav1.Condition{Type: ClustersSelectedType, Message: TalmNonExistentClusterMessage}
	// CguNonExistentPolicyCondition is the condition when the CGU has a non-existent policy.
	CguNonExistentPolicyCondition = metav1.Condition{Type: ValidatedType, Message: TalmNonExistentPolicyMessage}
	// CguPreCacheValidCondition is the condition when the CGU pre-caching is valid.
	CguPreCacheValidCondition = metav1.Condition{
		Type:    PreCacheValidType,
		Status:  metav1.ConditionTrue,
		Message: PreCacheValidMessage,
	}
	// CguPreCachePartialCondition is the condition when the CGU pre-caching could only partially complete.
	CguPreCachePartialCondition = metav1.Condition{
		Type:    PreCacheSucceededType,
		Status:  metav1.ConditionTrue,
		Message: PreCachePartialFailMessage,
		Reason:  PartiallyDoneReason,
	}
)
