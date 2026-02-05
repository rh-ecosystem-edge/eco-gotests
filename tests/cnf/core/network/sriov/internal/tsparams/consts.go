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

	// Net1Interface is the name of the first secondary network interface attached to pods.
	Net1Interface = "net1"

	// Pod names for SR-IOV connectivity tests.
	ClientPodMTU500      = "client-mtu500"
	ServerPodMTU500      = "server-mtu500"
	ClientPodMTU9000     = "client-mtu9000"
	ServerPodMTU9000     = "server-mtu9000"
	ClientPodWhereabouts = "client-whereabouts"
	ServerPodWhereabouts = "server-whereabouts"
	ClientPodVlanMTU500  = "client-vlan-mtu500"
	ServerPodVlanMTU500  = "server-vlan-mtu500"
	ClientPodVlanMTU9000 = "client-vlan-mtu9000"
	ServerPodVlanMTU9000 = "server-vlan-mtu9000"
)
