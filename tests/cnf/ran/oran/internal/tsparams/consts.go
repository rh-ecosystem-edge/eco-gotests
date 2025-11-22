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
	// LabelTemplateInventory is the label applied to just the template inventory test cases.
	LabelTemplateInventory = "template-inventory"
	// LabelAlarms is the label applied to just the alarms test cases.
	LabelAlarms = "alarms"
)

const (
	// ClusterTemplateName is the name without the version of the ClusterTemplate used in the ORAN tests. It is also
	// the namespace the ClusterTemplates are in.
	ClusterTemplateName = "sno-ran-du"
	// O2IMSNamespace is the namespace used by the oran-o2ims operator.
	O2IMSNamespace = "oran-o2ims"
	// ExtraManifestsName is the name of the generated extra manifests ConfigMap in the cluster Namespace.
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
	// TemplateMissingSchema is the ClusterTemplate version for the missing schema without HardwareTemplate test.
	TemplateMissingSchema = "v12"
	// TemplateNoHWTemplate is the ClusterTemplate version for the successful no HardwareTemplate test.
	TemplateNoHWTemplate = "v13"
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
	// TestBase64Credential is a base64 encoded version of the string "wrongpassword" for when an obviously invalid
	// credential is needed.
	TestBase64Credential = "d3JvbmdwYXNzd29yZA=="
)

const (
	// SubscriberSubdomain is the subdomain for the subscriber deployment. It is expected that there exists a *.apps
	// route for this subdomain.
	SubscriberSubdomain = "oran-subscriber"
	// SubscriberNamespace is the namespace for the subscriber deployment.
	SubscriberNamespace = "oran-subscriber"
)

// LogLevel is the glog verbosity level to use for logs in this suite or its helpers.
const LogLevel klog.Level = 80
