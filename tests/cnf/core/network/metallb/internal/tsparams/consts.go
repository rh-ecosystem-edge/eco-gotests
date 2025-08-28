package tsparams

const (
	// LabelSuite represents metallb label that can be used for test cases selection.
	LabelSuite = "metallb"
	// LabelBFDTestCases represents bfd label that can be used for test cases selection.
	LabelBFDTestCases = "bfd"
	// LabelBGPTestCases represents bgp label that can be used for test cases selection.
	LabelBGPTestCases = "bgp"
	// LabelDynamicRemoteASTestCases represents dynamic-remote-as label that can be used for test cases selection.
	LabelDynamicRemoteASTestCases = "dynamic-remote-as"
	// LabelBGPUnnumbered represents bgp-unnumbered label that can be used for test cases selection.
	LabelBGPUnnumbered = "bgp-unnumbered"
	// LabelLayer2TestCases represents layer2 label that can be used for test cases selection.
	LabelLayer2TestCases = "layer2"
	// LabelFRRTestCases represents frrk8 label that can be used for test cases selection.
	LabelFRRTestCases = "frrk8s"
	// LabelFRRNode is a label configured on the worker nodes.
	LabelFRRNode = "app=frr-k8s"
	// BGPPassword var is used to set password for BGP session between FRR speakers.
	BGPPassword = "bgp-test"
	// LabelGRTestCases represents graceful restart label that can be used for test cases selection.
	LabelGRTestCases = "gracefulrestart"
	// MlbAddressListError an error message when the ECO_CNF_CORE_NET_MLB_ADDR_LIST is incorrect.
	MlbAddressListError = "An unexpected error occurred while " +
		"determining the IP addresses from the ECO_CNF_CORE_NET_MLB_ADDR_LIST environment variable."
	// NoAdvertiseCommunity routes in this community will not be propagated.
	NoAdvertiseCommunity = "65535:65282"
	// CustomCommunity routes in this community will not be included in this community.
	CustomCommunity = "500:500"
	// BgpPeerName1 bgp peer name 1.
	BgpPeerName1 = "bgppeer1"
	// BgpPeerName2 bgp peer name 2.
	BgpPeerName2 = "bgppeer2" // FrrK8WebHookServer is the web hook server running in namespace metallb-system.
	// BGPTestPeer is the bgppeer name.
	BGPTestPeer = "testpeer"
	// BgpPeerDynamicASiBGP variable for iBGP peers.
	BgpPeerDynamicASiBGP = "internal"
	// BgpPeerDynamicASeBGP variable for eBGP peers.
	BgpPeerDynamicASeBGP = "external"
	// BfdProfileName bfd profile name.
	BfdProfileName = "bfdprofile"
	// FrrK8WebHookServer is the web hook server running in namespace metallb-system.
	FrrK8WebHookServer = "frr-k8s-webhook-server"
	// MetallbServiceName is the name of the metallb service.
	MetallbServiceName = "service-1"
	// MetallbServiceName2 is the name of the second metallb service.
	MetallbServiceName2 = "service-2"
	// BGPAdvAndAddressPoolName BGPAdvAndAdressPoolName is the name of the BgpAdvertisement and IPAddressPool.
	BGPAdvAndAddressPoolName = "bgp-test"
	// LabelValue1 is the value name for the label1.
	LabelValue1 = "nginx1"
	// LabelValue2 is the value name for the label2.
	LabelValue2 = "nginx2"
	// MLBNginxPodName represents the pod name used for the MetalLB NGINX configuration.
	MLBNginxPodName = "mlbnginxtpod"
	// FRRBaseConfig represents FRR daemon minimal configuration.
	FRRBaseConfig = `!
frr defaults traditional
hostname frr-pod
log file /tmp/frr.log
log timestamp precision 3
!
debug zebra nht
debug bgp neighbor-events
!
bfd
!
`
	// FRRDefaultBGPPreConfig represents FRR daemon BGP minimal config.
	FRRDefaultBGPPreConfig = ` bgp router-id 10.10.10.11
  no bgp ebgp-requires-policy
  no bgp default ipv4-unicast
  no bgp network import-check
`
	// FRRDefaultConfigMapName represents default FRR configMap name.
	FRRDefaultConfigMapName = "frr-config"
	// LocalBGPASN represents local BGP AS number.
	LocalBGPASN = 64500
	// RemoteBGPASN represents remote BGP AS number.
	RemoteBGPASN = 64501
	// ETPLocal represents externalTrafficPolicy Local.
	ETPLocal = "Local"
	// ETPCluster represents externalTrafficPolicy Cluster.
	ETPCluster = "Cluster"
	// MetalLBIPv6 represents IPv6 label that can be used for test cases selection.
	MetalLBIPv6 = "metallbipv6"
	// MetalLBDual represents dual label that can be used for test cases selection.
	MetalLBDual = "metallbdual"
)
