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

	// SR-IOV resource names - consolidated by PF to avoid multiple reboots.
	// All contexts share these resources, each context creates its own SriovNetwork.

	// SriovResourcePF1MTU500 is the SR-IOV resource name for PF1 MTU 500 (IPv4).
	SriovResourcePF1MTU500 = "sriovpf1mtu500"
	// SriovResourcePF1MTU1280 is the SR-IOV resource name for PF1 MTU 1280 (IPv6 minimum).
	SriovResourcePF1MTU1280 = "sriovpf1mtu1280"
	// SriovResourcePF1MTU9000 is the SR-IOV resource name for PF1 MTU 9000.
	SriovResourcePF1MTU9000 = "sriovpf1mtu9000"
	// SriovResourcePF2MTU500 is the SR-IOV resource name for PF2 MTU 500 (IPv4).
	SriovResourcePF2MTU500 = "sriovpf2mtu500"
	// SriovResourcePF2MTU1280 is the SR-IOV resource name for PF2 MTU 1280 (IPv6 minimum).
	SriovResourcePF2MTU1280 = "sriovpf2mtu1280"
	// SriovResourcePF2MTU9000 is the SR-IOV resource name for PF2 MTU 9000.
	SriovResourcePF2MTU9000 = "sriovpf2mtu9000"

	// SR-IOV network names for Same Node Same PF tests.

	// SriovNetworkSamePFMTU500 is the SR-IOV network name for Same Node Same PF MTU 500 (IPv4).
	SriovNetworkSamePFMTU500 = "sriov-net-samepf-mtu500"
	// SriovNetworkSamePFMTU1280 is the SR-IOV network name for Same Node Same PF MTU 1280 (IPv6).
	SriovNetworkSamePFMTU1280 = "sriov-net-samepf-mtu1280"
	// SriovNetworkSamePFMTU9000 is the SR-IOV network name for Same Node Same PF MTU 9000.
	SriovNetworkSamePFMTU9000 = "sriov-net-samepf-mtu9000"

	// SR-IOV network names for Same Node Different PF tests.

	// SriovNetworkDiffPFClientMTU500 is the SR-IOV network name for Different PF client MTU 500 (IPv4).
	SriovNetworkDiffPFClientMTU500 = "sriov-net-diffpf-client-mtu500"
	// SriovNetworkDiffPFServerMTU500 is the SR-IOV network name for Different PF server MTU 500 (IPv4).
	SriovNetworkDiffPFServerMTU500 = "sriov-net-diffpf-server-mtu500"
	// SriovNetworkDiffPFClientMTU1280 is the SR-IOV network name for Different PF client MTU 1280 (IPv6).
	SriovNetworkDiffPFClientMTU1280 = "sriov-net-diffpf-client-mtu1280"
	// SriovNetworkDiffPFServerMTU1280 is the SR-IOV network name for Different PF server MTU 1280 (IPv6).
	SriovNetworkDiffPFServerMTU1280 = "sriov-net-diffpf-server-mtu1280"
	// SriovNetworkDiffPFClientMTU9000 is the SR-IOV network name for Different PF client MTU 9000.
	SriovNetworkDiffPFClientMTU9000 = "sriov-net-diffpf-client-mtu9000"
	// SriovNetworkDiffPFServerMTU9000 is the SR-IOV network name for Different PF server MTU 9000.
	SriovNetworkDiffPFServerMTU9000 = "sriov-net-diffpf-server-mtu9000"

	// SR-IOV network names for Different Node tests.

	// SriovNetworkDiffNodeMTU500 is the SR-IOV network name for Different Node MTU 500 (IPv4).
	SriovNetworkDiffNodeMTU500 = "sriov-net-diffnode-mtu500"
	// SriovNetworkDiffNodeMTU1280 is the SR-IOV network name for Different Node MTU 1280 (IPv6).
	SriovNetworkDiffNodeMTU1280 = "sriov-net-diffnode-mtu1280"
	// SriovNetworkDiffNodeMTU9000 is the SR-IOV network name for Different Node MTU 9000.
	SriovNetworkDiffNodeMTU9000 = "sriov-net-diffnode-mtu9000"

	// Pod name prefixes for connectivity tests.

	// ClientPodMTU500 is the pod name prefix for client MTU 500 (IPv4).
	ClientPodMTU500 = "client-mtu500"
	// ServerPodMTU500 is the pod name prefix for server MTU 500 (IPv4).
	ServerPodMTU500 = "server-mtu500"
	// ClientPodMTU1280 is the pod name prefix for client MTU 1280 (IPv6).
	ClientPodMTU1280 = "client-mtu1280"
	// ServerPodMTU1280 is the pod name prefix for server MTU 1280 (IPv6).
	ServerPodMTU1280 = "server-mtu1280"
	// ClientPodMTU9000 is the pod name prefix for client MTU 9000.
	ClientPodMTU9000 = "client-mtu9000"
	// ServerPodMTU9000 is the pod name prefix for server MTU 9000.
	ServerPodMTU9000 = "server-mtu9000"
)
