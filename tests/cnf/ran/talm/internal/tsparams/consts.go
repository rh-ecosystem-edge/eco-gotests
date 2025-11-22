package tsparams

import (
	"time"

	"k8s.io/klog/v2"
)

const (
	// LabelSuite is the label for all the tests in this suite.
	LabelSuite = "talm"
	// LabelBackupTestCases is the label for a particular test file.
	LabelBackupTestCases = "backup"
	// LabelBatchingTestCases is the label for a particular test file.
	LabelBatchingTestCases = "batching"
	// LabelBlockingCRTestCases is the label for a particular test file.
	LabelBlockingCRTestCases = "blockingcr"
	// LabelCanaryTestCases is the label for a particular test file.
	LabelCanaryTestCases = "canary"
	// LabelPreCacheTestCases is the label for a particular test file.
	LabelPreCacheTestCases = "precache"
	// LabelMissingSpokeTestCases is the label for a set of batching test cases.
	LabelMissingSpokeTestCases = "missingspoke"
	// LabelMissingPolicyTestCases is the label for a set of batching test cases.
	LabelMissingPolicyTestCases = "missingpolicy"
	// LabelCatalogSourceTestCases is the label for a set of batching test cases.
	LabelCatalogSourceTestCases = "catalogsource"
	// LabelTempNamespaceTestCases is the label for a set of batching test cases.
	LabelTempNamespaceTestCases = "tempnamespace"

	// TestNamespace is the testing namespace created on the hub.
	TestNamespace = "talm-test"
	// TemporaryNamespace is a temporary namespace for testing created on the spokes.
	TemporaryNamespace = "talm-namespace-temp"
	// CguName is the name for the test talm cgu.
	CguName = "talm-cgu"
	// PolicyName is the name for the test talm policy.
	PolicyName = "talm-policy"
	// PolicySetName is the name for the test talm policy set.
	PolicySetName = "talm-policyset"
	// PlacementRuleName is the name for the test talm placement rule.
	PlacementRuleName = "talm-placementrule"
	// PlacementBindingName is the name for the test talm placement binding.
	PlacementBindingName = "talm-placementbinding"
	// CatalogSourceName is the name for the test talm catalog source.
	CatalogSourceName = "talm-catsrc"
	// PreCachingConfigName is the name for the test talm pre caching config.
	PreCachingConfigName = "talm-precachingconfig"
	// NonExistentPolicyName is the name for non-existent policies.
	NonExistentPolicyName = "non-existent-policy"
	// NonExistentClusterName is the name for non-existent clusters.
	NonExistentClusterName = "non-existent-cluster"

	// TalmPodLabelSelector is the label selector to find talm pods.
	TalmPodLabelSelector = "pod-template-hash"
	// TalmCompleteLabel is the label applied during talm after completion.
	TalmCompleteLabel = "talmcomplete"

	// OpenshiftLoggingNamespace is the namespace where logging pods are.
	OpenshiftLoggingNamespace = "openshift-logging"

	// BackupPath is the path the temporary filesystem is mounted to.
	BackupPath = "/var/recovery"
	// RANTestPath is where the temporary filesystem file is.
	RANTestPath = "/var/ran-test-talm-recovery"
	// FSSize is the size of the temporary filesystem.
	FSSize = "100M"

	// ClustersSelectedType is the type for a CGU condition.
	ClustersSelectedType = "ClustersSelected"
	// ValidatedType is the type for a CGU condition.
	ValidatedType = "Validated"
	// PreCacheValidType is the type for a CGU condition.
	PreCacheValidType = "PrecacheSpecValid"
	// PreCacheSucceededType is the type for a CGU condition.
	PreCacheSucceededType = "PrecachingSuceeded"
	// ReadyType is the type for a CGU condition.
	ReadyType = "Ready"
	// SucceededType is the type for a CGU condition.
	SucceededType = "Succeeded"
	// ProgressingType is the type for a CGU condition.
	ProgressingType = "Progressing"
	// CompletedReason is the reason for a CGU condition.
	CompletedReason = "Completed"
	// TimedOutReason is the reason for a CGU condition.
	TimedOutReason = "TimedOut"
	// UpgradeCompletedReason is the reason for a CGU condition.
	UpgradeCompletedReason = "UpgradeCompleted"
	// PartiallyDoneReason is the reason for a CGU condition.
	PartiallyDoneReason = "PartiallyDone"
	// TalmTimeoutMessage is the message for a CGU condition.
	TalmTimeoutMessage = "Policy remediation took too long"
	// TalmCanaryTimeoutMessage is the message for a CGU condition.
	TalmCanaryTimeoutMessage = "Policy remediation took too long on canary clusters"
	// TalmBlockedMessage is the message for a CGU condition.
	TalmBlockedMessage = "Blocking CRs that are not completed: [%s]"
	// TalmMissingCRMessage is the message for a CGU condition.
	TalmMissingCRMessage = "Missing blocking CRs: [%s]"
	// PreCacheValidMessage is the message for a CGU condition.
	PreCacheValidMessage = "Precaching spec is valid and consistent"
	// PreCachePartialFailMessage  is the message for a CGU condition.
	PreCachePartialFailMessage = "Precaching failed for 1 clusters"

	// PreCacheContainerName is the name of the pre cache container.
	PreCacheContainerName = "pre-cache-container"
	// PreCachePodLabel is the label for the pre cache pod.
	PreCachePodLabel = "job-name=pre-cache"
	// PreCacheSpokeNS is the namespace from the pre cache test.
	PreCacheSpokeNS = "openshift-talo-pre-cache"
	// PreCacheOverrideName is the name of the config map for excluding images from precaching.
	PreCacheOverrideName = "cluster-group-upgrade-overrides"
	// PreCacheInvalidImage is a nonexistent image to use for pre caching tests.
	PreCacheInvalidImage = `quay.io/openshift-release-dev/ocp-v4.0-art-dev@` +
		`sha256:0000000000000000000000000000000000000000000000000000000000000000`
	// SpokeImageDeleteCommand is the command to delete the excluded pre cache image.
	SpokeImageDeleteCommand = `podman images --noheading  --filter "label=name=%s" --format {{.ID}}|` +
		`xargs podman rmi --force`
	// SpokeImageListCommand is the command to generate a list of cached images on the spoke cluster.
	SpokeImageListCommand = `podman images  --noheading --filter "label=name=%s"`
	// SpokeImageGetNameCommand is the command to get the name of an image.
	SpokeImageGetNameCommand = `podman images --noheading %s --format {{.Labels.name}}`
	// PrometheusPodName is the name of the prometheus pod.
	PrometheusPodName = "prometheus-k8s-0"
	// PrometheusNamespace is the namespace for prometheus.
	PrometheusNamespace = "openshift-monitoring"
	// TalmBackupNamespace is the namespace that the CGU operator uses when running backups.
	TalmBackupNamespace = "openshift-talo-backup"

	// MasterNodeSelector when used in a label selector finds all master nodes.
	MasterNodeSelector = "node-role.kubernetes.io/master="

	// TalmDefaultReconcileTime is the default time for talm to reconcile.
	TalmDefaultReconcileTime = 5 * time.Minute
	// TalmSystemStablizationTime is the default time to wait for talm to settle.
	TalmSystemStablizationTime = 15 * time.Second

	// LogLevel is the verbosity of glog statements in this test suite.
	LogLevel klog.Level = 90
)
