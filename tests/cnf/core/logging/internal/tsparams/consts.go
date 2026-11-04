package tsparams

const (
	// LabelSuite represents logging label that can be used for test cases selection.
	LabelSuite = "logging"
	// LabelLokiLogStorage represents loki log storage label for test cases selection.
	LabelLokiLogStorage = "loki-log-storage"

	// LokiOperatorNamespace is the namespace where the Loki Operator is deployed.
	LokiOperatorNamespace = "openshift-operators-redhat"
	// LoggingNamespace is the namespace where LokiStack and CLO resources are deployed.
	LoggingNamespace = "openshift-logging"
	// LokiOperatorDeploymentName is the Loki Operator controller manager deployment name.
	LokiOperatorDeploymentName = "loki-operator-controller-manager"
	// LokiOperatorSubscriptionName is the Loki Operator subscription name.
	LokiOperatorSubscriptionName = "loki-operator"
	// LokiStackName is the LokiStack instance name.
	LokiStackName = "logging-loki"
	// LokiSecretName is the Loki S3 credentials secret name.
	LokiSecretName = "logging-loki-s3"
	// CLFName is the ClusterLogForwarder instance name.
	CLFName = "instance"
	// CLODeploymentName is the Cluster Logging Operator deployment name.
	CLODeploymentName = "cluster-logging-operator"

	// LokiStackSizeExtraSmall is the expected LokiStack size profile.
	LokiStackSizeExtraSmall = "1x.extra-small"
	// DefaultRetentionDays is the expected default retention period in days.
	DefaultRetentionDays uint = 5
	// DisconnectedCatalogSource is the expected catalog source for disconnected environments.
	DisconnectedCatalogSource = "redhat-operators-disconnected"

	// TestNamespaceName is the namespace used for test resources.
	TestNamespaceName = "loki-validation-test"
	// TestServiceAccountName is the ServiceAccount used for Loki API queries.
	TestServiceAccountName = "loki-test-sa"
)
