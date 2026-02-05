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

	// ClientPodMTU500 is the name of the client pod for MTU 500 tests.
	ClientPodMTU500 = "client-mtu500"
	// ServerPodMTU500 is the name of the server pod for MTU 500 tests.
	ServerPodMTU500 = "server-mtu500"
	// ClientPodMTU1280 is the name of the client pod for MTU 1280 tests.
	ClientPodMTU1280 = "client-mtu1280"
	// ServerPodMTU1280 is the name of the server pod for MTU 1280 tests.
	ServerPodMTU1280 = "server-mtu1280"
	// ClientPodMTU9000 is the name of the client pod for MTU 9000 tests.
	ClientPodMTU9000 = "client-mtu9000"
	// ServerPodMTU9000 is the name of the server pod for MTU 9000 tests.
	ServerPodMTU9000 = "server-mtu9000"
	// ClientPodWhereabouts is the name of the client pod for whereabouts IPAM tests.
	ClientPodWhereabouts = "client-whereabouts"
	// ServerPodWhereabouts is the name of the server pod for whereabouts IPAM tests.
	ServerPodWhereabouts = "server-whereabouts"
	// ClientPodVlanMTU500 is the name of the client pod for VLAN MTU 500 tests.
	ClientPodVlanMTU500 = "client-vlan-mtu500"
	// ServerPodVlanMTU500 is the name of the server pod for VLAN MTU 500 tests.
	ServerPodVlanMTU500 = "server-vlan-mtu500"
	// ClientPodVlanMTU1280 is the name of the client pod for VLAN MTU 1280 tests.
	ClientPodVlanMTU1280 = "client-vlan-mtu1280"
	// ServerPodVlanMTU1280 is the name of the server pod for VLAN MTU 1280 tests.
	ServerPodVlanMTU1280 = "server-vlan-mtu1280"
	// ClientPodVlanMTU9000 is the name of the client pod for VLAN MTU 9000 tests.
	ClientPodVlanMTU9000 = "client-vlan-mtu9000"
	// ServerPodVlanMTU9000 is the name of the server pod for VLAN MTU 9000 tests.
	ServerPodVlanMTU9000 = "server-vlan-mtu9000"

	// IPv4 multicast groups and MACs.

	// MulticastIPv4Group is the default IPv4 multicast group address.
	MulticastIPv4Group = "239.100.0.250"
	// MulticastIPv4MAC is the Ethernet multicast MAC for 239.100.0.250.
	MulticastIPv4MAC = "01:00:5e:64:00:fa"
	// MulticastIPv4GroupLargeMTU is the IPv4 multicast group for MTU > 1500.
	MulticastIPv4GroupLargeMTU = "239.100.100.250"
	// MulticastIPv4MACLargeMTU is the Ethernet multicast MAC for 239.100.100.250.
	MulticastIPv4MACLargeMTU = "01:00:5e:64:64:fa"

	// IPv6 multicast group and MAC.

	// MulticastIPv6Group is the IPv6 site-local multicast group address.
	MulticastIPv6Group = "ff05:5::5"
	// MulticastIPv6MAC is the Ethernet multicast MAC for ff05:5::5.
	MulticastIPv6MAC = "33:33:00:00:00:05"
)
