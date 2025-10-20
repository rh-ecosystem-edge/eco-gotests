package ocloudparams

import "time"

const (
	// Label represents O-Cloud system tests label that can be used for test cases selection.
	Label = "ocloud"

	// OCloudLogLevel configures logging level for O-Cloud related tests.
	OCloudLogLevel = 90

	// BMHAvailabilityTimeout is the timeout for BMH availability.
	BMHAvailabilityTimeout = 60 * time.Minute

	// AcmNamespace is the namespace for ACM.
	AcmNamespace = "rhacm"
	// AcmSubscriptionName is the name of the ACM operator subscription.
	AcmSubscriptionName = "acm-operator-subscription"

	// OpenshiftGitOpsNamespace is the namespace for the GitOps operator.
	OpenshiftGitOpsNamespace = "openshift-operators"
	// OpenshiftGitOpsSubscriptionName is the name of the GitOps operator subscription.
	OpenshiftGitOpsSubscriptionName = "openshift-gitops-operator-subscription"

	// OranO2ImsNamespace is the namespace for the O-Cloud manager operator.
	OranO2ImsNamespace = "oran-o2ims"
	// OCloudO2ImsSubscriptionName is the name of the O-Cloud manager operator subscription.
	OCloudO2ImsSubscriptionName = "oran-o2ims-operator-subscription"

	//nolint:lll
	// OCloudHardwareManagerPluginSubscriptionName is the name of the O-Cloud hardware manager plugin operator subscription.
	OCloudHardwareManagerPluginSubscriptionName = "oran-hwmgr-plugin-operator-subscription"

	// PtpNamespace is the namespace for the PTP operator.
	PtpNamespace = "openshift-ptp"
	// PtpOperatorSubscriptionName is the name of the PTP operator subscription.
	PtpOperatorSubscriptionName = "ptp-operator-subscription"
	// PtpDeploymentName is the name of the PTP deployment.
	PtpDeploymentName = "ptp-operator"
	// PtpContainerName is the name of the PTP container.
	PtpContainerName = "ptp-operator"

	// SriovNamespace is the namespace for the SR-IOV operator.
	SriovNamespace = "openshift-sriov-network-operator"

	// LifecycleAgentNamespace is the namespace for the Lifecycle Agent operator.
	LifecycleAgentNamespace = "openshift-lifecycle-agent"
)

const (
	// SubscriberSubdomain is the subdomain for the subscriber deployment. It is expected that there exists a *.apps
	// route for this subdomain.
	SubscriberSubdomain = "oran-subscriber"
	// SubscriberNamespace is the namespace for the subscriber deployment.
	SubscriberNamespace = "oran-subscriber"
)

const (
	// ConfigFilesFolder path to the config files folder.
	ConfigFilesFolder = "./internal/ocloudconfig/config-files/"
)
