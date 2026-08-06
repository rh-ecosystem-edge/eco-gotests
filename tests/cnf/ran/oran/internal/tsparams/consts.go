package tsparams

import "k8s.io/klog/v2"

const (
	// LabelSuite is the label applied to all cases in the oran suite.
	LabelSuite = "oran"
	// LabelPreProvision is the label applied to the pre-provision test cases and any other test cases that are
	// intended to be run before provisioning.
	LabelPreProvision = "pre-provision"
	// LabelProvision is the label applied to just the provision test cases.
	LabelProvision = "provision"
	// LabelPostProvision is the label applied to just the post-provision test cases.
	LabelPostProvision = "post-provision"
	// LabelMetal3Day2 is the label applied to just the day-2 Metal3 test cases. It is designed to be run after the
	// post-provision tests.
	LabelMetal3Day2 = "metal3-day2"
	// LabelTemplateInventory is the label applied to just the template inventory test cases.
	LabelTemplateInventory = "template-inventory"
	// LabelAlarms is the label applied to just the alarms test cases.
	LabelAlarms = "alarms"
	// LabelInventory is the label applied to just the inventory API test cases.
	LabelInventory = "inventory"
	// LabelSecurity is the label applied to O-Cloud Manager security hardening test cases.
	LabelSecurity = "security"
	// LabelClusterAPI is the label applied to just the cluster API test cases.
	LabelClusterAPI = "cluster-api"
)

const (
	// ClusterTemplateName is the default ClusterTemplate base name (without version) for SNO O-RAN tests. It is also
	// the namespace the ClusterTemplates are in. Prefer helper.GetClusterTemplateName() when building PRs so MNO labs
	// can resolve to MNOClusterTemplateName.
	ClusterTemplateName = "sno-ran-du"
	// MNOClusterTemplateName is the default ClusterTemplate base name for multi-node O-RAN provision tests.
	MNOClusterTemplateName = "mno-ran-du"
	// O2IMSNamespace is the namespace used by the oran-o2ims operator.
	O2IMSNamespace = "oran-o2ims"
	// ExtraManifestsName is the default generated extra manifests ConfigMap name for SNO. Prefer
	// helper.GetExtraManifestsName() so the name matches the resolved ClusterTemplate.
	ExtraManifestsName = "sno-ran-du-extra-manifest-1"
	// ClusterInstanceParamsKey is the key in the TemplateParameters map for the ClusterInstance parameters.
	ClusterInstanceParamsKey = "clusterInstanceParameters"
	// PolicyTemplateParamsKey is the key in the TemplateParameters map for the policy template parameters.
	PolicyTemplateParamsKey = "policyTemplateParameters"
	// HugePagesSizeKey is the key in TemplateParameters.policyTemplateParameters that sets the hugepages size.
	HugePagesSizeKey = "hugepages-size"
	// OCloudSiteID is the name of the site in the hardware manager to provision in.
	OCloudSiteID = "rdu3"
	// PolicySelectorLabel is the ExtraLabel applied to the managed cluster that determines which policies to apply.
	PolicySelectorLabel = "sno-ran-du-policy"
	// ClusterInstanceDefaultsKey is the key used for the ClusterInstance defaults in its ConfigMap.
	ClusterInstanceDefaultsKey = "clusterinstance-defaults"
	// PolicyTemplateDefaultsKey is the key used for the PolicyTemplate defaults in its ConfigMap.
	PolicyTemplateDefaultsKey = "policytemplate-defaults"

	// ImmutableMessage is the message to expect in a Policy's history when an immutable field cannot be updated.
	ImmutableMessage = "cannot be updated, likely due to immutable fields not matching"

	// PRValidationFailedDetailsSubstring is a substring of status.provisioningStatus.provisioningDetails when
	// ProvisioningRequest validation fails.
	PRValidationFailedDetailsSubstring = "Failed to validate the ProvisioningRequest"
	// PRFulfilledDetailsSubstring is a substring of status.provisioningStatus.provisioningDetails when
	// provisioning completes successfully.
	PRFulfilledDetailsSubstring = "Provisioning request has completed successfully"
	// PRNoHardwareMatchDetailsSubstring is a substring of provisioningDetails when no free hardware matches the
	// resource selector. The operator uses the same message when matching hardware exists but is already allocated.
	PRNoHardwareMatchDetailsSubstring = "not enough free resources matching"
	// PRMissingBootInterfaceDetailsSubstring is a substring of provisioningDetails when no NIC in the
	// ClusterInstance defaults matches the boot interface label value.
	PRMissingBootInterfaceDetailsSubstring = "no NIC found matching boot interface label value"
	// PRExistingNamespaceDetailsSubstring is a substring of provisioningDetails when clusterName matches an
	// existing namespace not owned by the ProvisioningRequest.
	PRExistingNamespaceDetailsSubstring = "not owned by ProvisioningRequest"
	// PRReservedNamespaceDetailsSubstring is a substring of API error detail when clusterName is a reserved namespace.
	PRReservedNamespaceDetailsSubstring = `targets a reserved namespace (prefix "default")`
	// PROpenshiftPrefixDetailsSubstring is a substring of API error detail when clusterName uses the openshift- prefix.
	PROpenshiftPrefixDetailsSubstring = `targets a reserved namespace (prefix "openshift-")`
	// PRKubePrefixDetailsSubstring is a substring of API error detail when clusterName uses the kube- prefix.
	PRKubePrefixDetailsSubstring = `targets a reserved namespace (prefix "kube-")`
	// PRDNS1123DetailsSubstring is a substring of API error detail when clusterName is not DNS-1123 compliant.
	PRDNS1123DetailsSubstring = "not a valid DNS-1123 label"
)

const (
	// TemplateValid is the valid ClusterTemplate used for the provision tests.
	TemplateValid = "v1"
	// TemplateInvalid is the ClusterTemplate version for the invalid ClusterTemplate test.
	TemplateInvalid = "v7"
	// TemplateUpdateDefaults is the ClusterTemplate version for the ClusterInstance defaults update test.
	TemplateUpdateDefaults = "v8"
	// TemplateUpdateExisting is the ClusterTemplate version for the update existing PG manifest test.
	TemplateUpdateExisting = "v9"
	// TemplateAddNew is the ClusterTemplate version for the add new manifest to existing PG test.
	TemplateAddNew = "v10"
	// TemplateUpdateSchema is the ClusterTemplate version for the policyTemplateParameters schema update test.
	TemplateUpdateSchema = "v11"
	// TemplateInlineBMCMissingSchema is the ClusterTemplate version for the missing inline BMC schema test (78245).
	TemplateInlineBMCMissingSchema = "v12"
	// TemplateBMCFirmwareUpdate is the ClusterTemplate version for BMC firmware upgrade test.
	TemplateBMCFirmwareUpdate = "v14"
	// TemplateBIOSFirmwareUpdate is the ClusterTemplate version for BIOS firmware upgrade test.
	TemplateBIOSFirmwareUpdate = "v15"
	// TemplateBIOSSettingsUpdate is the ClusterTemplate version for BIOS settings update test.
	TemplateBIOSSettingsUpdate = "v16"
	// TemplateNoHardwareMatch is the ClusterTemplate version for no hardware matching test.
	TemplateNoHardwareMatch = "v17"
	// TemplateMissingBootInterface is the ClusterTemplate version for missing boot interface test.
	TemplateMissingBootInterface = "v18"
	// TemplateNonexistentHWProfile is the ClusterTemplate version for nonexistent hardware profile test.
	TemplateNonexistentHWProfile = "v19"
	// TemplateHardwareAllocated is the ClusterTemplate version for allocated hardware test.
	TemplateHardwareAllocated = "v20"
)

const (
	// TestName is the name to use for various test items, such as labels, annotations, and the test ConfigMap in
	// post-provision tests. This constant consolidates all these names so there is only one rather than a separate
	// TestLabel, TestAnnotation, etc. constants that are all the same.
	TestName = "oran-test"
	// TestName2 is the secondary test name to use for various test items, for example, the second test ConfigMap
	// for test cases that use it in the post-provision tests.
	TestName2 = "oran-test-2"
	// TestOriginalValue is the original value to expect when checking the test ConfigMap.
	TestOriginalValue = "original-value"
	// TestNewValue is the new value to set in the test ConfigMap.
	TestNewValue = "new-value"
	// TestPRName is the UUID used for naming ProvisioningRequests. Since metadata.name must be a UUID, just use a
	// constant one for consistency.
	TestPRName = "9c5372f3-ea1d-4a96-8157-b3b874a55cf9"
	// TestPRName2 is the second UUID used for naming ProvisioningRequests. Metal3 tests require a second PR applied
	// to verify the case of all hardware already allocated.
	TestPRName2 = "a1b2c3d4-e5f6-7890-1234-567890abcdef"
	// TestPRNameSecurity is the UUID used for security hardening ProvisioningRequest tests.
	TestPRNameSecurity = "b2c3d4e5-f6a7-8901-2345-678901bcdef0"
	// TestExistingNamespace is a namespace created manually for clusterName ownership validation tests.
	TestExistingNamespace = "oran-test-existing-ns"

	// TestLocationAlpha is the first Location name used by inventory filter tests.
	TestLocationAlpha = "test-location-alpha"
	// TestLocationBeta is the second Location name used by inventory filter tests.
	TestLocationBeta = "test-location-beta"
	// TestInventoryResourcePool is the ResourcePool name used by inventory subscription notification tests.
	TestInventoryResourcePool = "oran-test-inventory-pool"
	// ResourcePoolNameLabel is the BMH label that associates a host with a ResourcePool.
	ResourcePoolNameLabel = "resources.clcm.openshift.io/resourcePoolName"
	// NonExistentUUID is a well-known UUID used for not-found O2IMS API requests.
	NonExistentUUID = "00000000-0000-0000-0000-000000000000"
	// TestNotificationLabel is the ManagedCluster label key used to trigger cluster change notifications.
	TestNotificationLabel = "oran-test-notification"
	// ClusterVendorLabel is the ManagedCluster label mapped to NodeCluster extensions.vendor.
	ClusterVendorLabel = "vendor"
	// ClusterIDLabel is the ManagedCluster label used as NodeCluster.nodeClusterId.
	ClusterIDLabel = "clusterID"
	// OpenshiftVersionLabel is the ManagedCluster label mapped to NodeCluster extensions.openshiftVersion.
	OpenshiftVersionLabel = "openshiftVersion"
	// LocalClusterLabel is the ManagedCluster label that identifies the ACM hub cluster.
	LocalClusterLabel = "local-cluster"
	// ClusterModelExtension is the NodeCluster / NodeClusterType extensions key for hub vs spoke model.
	ClusterModelExtension = "model"
	// ClusterVersionExtension is the NodeClusterType extensions key for OpenShift version.
	ClusterVersionExtension = "version"
	// ClusterModelHubCluster is the model value for the ACM hub (local-cluster).
	ClusterModelHubCluster = "hub-cluster"
	// ClusterModelManagedCluster is the model value for spoke ManagedClusters.
	ClusterModelManagedCluster = "managed-cluster"
	// HardwareManagerNodeIDLabel is the Agent label mapped to ClusterResource.resourceId.
	HardwareManagerNodeIDLabel = "clcm.openshift.io/hwMgrNodeId"
	// AlarmDefinitionSeverityField is the AlarmDefinition additionalFields key for severity.
	AlarmDefinitionSeverityField = "severity"
	// ClusterAPIVersion is the expected first version in cluster API version responses.
	ClusterAPIVersion = "1.0.0"
	// ClusterAPIURIPrefix is the expected uriPrefix in cluster API version responses.
	ClusterAPIURIPrefix = "/o2ims-infrastructureCluster/v1"
	// InventoryAPIVersion is the expected first version in inventory API version responses.
	InventoryAPIVersion = "2.0.0"
	// InventoryAPIURIPrefix is the expected uriPrefix in inventory API version responses.
	InventoryAPIURIPrefix = "/o2ims-infrastructureInventory/v2"
)

// LogLevel is the glog verbosity level to use for logs in this suite or its helpers.
const LogLevel klog.Level = 80
