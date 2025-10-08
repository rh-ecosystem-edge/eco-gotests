package tsparams

const (
	// LabelSuite represents sriov label that can be used for test cases selection.
	LabelSuite = "sriov"
	// TestNamespaceName sriov namespace where all test cases are performed.
	TestNamespaceName = "sriov-tests"
	// TestNamespaceName1 sriov namespace where all test cases are performed.
	TestNamespaceName1 = "sriov-tests-1"
	// TestNamespaceName2 sriov namespace where all test cases are performed.
	TestNamespaceName2 = "sriov-tests-2"
	// LabelExternallyManagedTestCases represents ExternallyManaged label that can be used for test cases selection.
	LabelExternallyManagedTestCases = "externallymanaged"
	// LabelParallelDrainingTestCases represents parallel draining label that can be used for test cases selection.
	LabelParallelDrainingTestCases = "paralleldraining"
	// LabelQinQTestCases represents ExternallyManaged label that can be used for test cases selection.
	LabelQinQTestCases = "qinq"
	// LabelExposeMTUTestCases represents Expose MTU label that can be used for test cases selection.
	LabelExposeMTUTestCases = "exposemtu"
	// LabelSriovMetricsTestCases represents Sriov Metrics Exporter label that can be used for test cases selection.
	LabelSriovMetricsTestCases = "sriovmetrics"
	// LabelRdmaMetricsAPITestCases represents Rdma Metrics label that can be used for test cases selection.
	LabelRdmaMetricsAPITestCases = "rdmametricsapi"
	// LabelMlxSecureBoot represents Mellanox secure boot label that can be used for test cases selection.
	LabelMlxSecureBoot = "mlxsecureboot"
	// LabelWebhookInjector represents sriov webhook injector match conditions tests that can be used
	// for test cases selection.
	LabelWebhookInjector = "webhook-resource-injector"
	// LabelSriovNetAppNsTestCases represents sriov network application namespace label that can be used
	// for test cases selection.
	LabelSriovNetAppNsTestCases = "sriovnet-app-ns"
	// LabelSriovHWEnabled represents sriov HW Enabled tests that can be used
	// for test cases selection.
	LabelSriovHWEnabled = "sriov-hw-enabled"
)
