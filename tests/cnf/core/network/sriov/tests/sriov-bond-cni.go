package tests

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

const (
	// bondCNINADName is the Bond CNI NAD used by xmitHashPolicy cases.
	bondCNINADName = "bond-cni-xor-nad"
	// bondCNIListenerNetworkName is a static-IPAM SriovNetwork for single-VF listener pods.
	bondCNIListenerNetworkName = "bond-cni-listener-vf"

	// Shared dual-stack layout on 192.168.0.0/24 and 2001::/64 (suite secondary subnets):
	// bonded client .10 / ::10; listener A .20/::20 + aliases .22/.23 and ::22/::23;
	// listener B .21/::21 + aliases .24/.25 and ::24/::25.
	// Per-host TCP listen ports: hostID 20→5000 … 25→5005 (port = 5000 + hostID - 20).
	// IPv4 and IPv6 twins share a port (one testcmd listener covers both).
	bondCNIIPv4Bond            = "192.168.0.10/24"
	bondCNIIPv6Bond            = "2001::10/64"
	bondCNIIPv4ListenerA       = "192.168.0.20/24"
	bondCNIIPv6ListenerA       = "2001::20/64"
	bondCNIIPv4ListenerB       = "192.168.0.21/24"
	bondCNIIPv6ListenerB       = "2001::21/64"
	bondCNIIPv4ListenerAAlias1 = "192.168.0.22/24"
	bondCNIIPv6ListenerAAlias1 = "2001::22/64"
	bondCNIIPv4ListenerAAlias2 = "192.168.0.23/24"
	bondCNIIPv6ListenerAAlias2 = "2001::23/64"
	bondCNIIPv4ListenerBAlias1 = "192.168.0.24/24"
	bondCNIIPv6ListenerBAlias1 = "2001::24/64"
	bondCNIIPv4ListenerBAlias2 = "192.168.0.25/24"
	bondCNIIPv6ListenerBAlias2 = "2001::25/64"
	bondCNITCPPortBase         = 5000
	bondCNITCPPortOctetOffset  = 20
	// bondCNIHashSkewPingCount is ICMP probes per dest for hash-skew TX measurement.
	// Matches bondCNIHashSkewMinTX so one successful ping burst can mark a slave used.
	bondCNIHashSkewPingCount = 10
	// bondCNIHashSkewMinTX is the minimum per-dest TX delta on a slave to count that slave
	// as used. Ignore small cross-slave noise (+1/+2) from ARP/ND that is not the hashed flow.
	bondCNIHashSkewMinTX = 10
	// bondCNIHashSkewAttempts is how many pool runs to allow before failing (TX-read flakes
	// or an unlucky dest order that does not show both slaves).
	bondCNIHashSkewAttempts = 2
	// bondCNIHAPingMinTX is the minimum ICMP probes that must be sent during HA toggles
	// so lost ≤ bondCNIHAPingMaxLoss is meaningful (ping is stopped when toggles finish).
	bondCNIHAPingMinTX = 10
	// bondCNIHAPingMaxLoss is the max lost ICMP probes allowed across the HA window
	// (LAG/slave toggles while ping runs; ping is stopped when toggles finish).
	bondCNIHAPingMaxLoss = 2

	// Listener MACs with opposite last-byte parity so layer2 xmit hash
	// (src[5]^dst[5])%2 maps the two peers to different bond slaves. Random VF MACs
	// can collide (see 89648 failure: …:c1 and …:ef both odd → all TX on net1).
	bondCNIListenerAMAC = "02:00:00:00:00:10"
	bondCNIListenerBMAC = "02:00:00:00:00:11"
)

// DiffPF slave SriovNetwork names for Bond CNI (default-MTU resource pool).
var bondCNINetworksV4DiffPF = []string{
	"sriov-bond-cni-v4-diffpf-pf1",
	"sriov-bond-cni-v4-diffpf-pf2",
}

// bondCNIHashSkewDestIPs is the dual-stack listener IP pool used for ICMP hash-skew
// (89648 / 89649 / 89650). Walked until both bond slaves show TX >= bondCNIHashSkewMinTX.
var bondCNIHashSkewDestIPs = []string{
	bondCNIIPv4ListenerA,
	bondCNIIPv6ListenerA,
	bondCNIIPv4ListenerAAlias1,
	bondCNIIPv6ListenerAAlias1,
	bondCNIIPv4ListenerAAlias2,
	bondCNIIPv6ListenerAAlias2,
	bondCNIIPv4ListenerB,
	bondCNIIPv6ListenerB,
	bondCNIIPv4ListenerBAlias1,
	bondCNIIPv6ListenerBAlias1,
	bondCNIIPv4ListenerBAlias2,
	bondCNIIPv6ListenerBAlias2,
}

var bondCNIListenerAIPs = []string{
	bondCNIIPv4ListenerA,
	bondCNIIPv6ListenerA,
	bondCNIIPv4ListenerAAlias1,
	bondCNIIPv6ListenerAAlias1,
	bondCNIIPv4ListenerAAlias2,
	bondCNIIPv6ListenerAAlias2,
}

var bondCNIListenerBIPs = []string{
	bondCNIIPv4ListenerB,
	bondCNIIPv6ListenerB,
	bondCNIIPv4ListenerBAlias1,
	bondCNIIPv6ListenerBAlias1,
	bondCNIIPv4ListenerBAlias2,
	bondCNIIPv6ListenerBAlias2,
}

// Lab switch LAG on worker0 PFs only (numLags=1); worker1 stays non-LAG'd for single-VF
// listeners. Secondary Multus IPs are dual-stack (IPv4+IPv6); cluster dual-stack is not
// required. Cases: balance-xor xmitHashPolicy layer2 / layer2+3 / layer3+4.
var _ = Describe(
	"Bond CNI xmitHashPolicy",
	Ordered,
	Label(tsparams.LabelSuite, tsparams.LabelBondCNITestCases),
	ContinueOnFailure,
	func() {
		var (
			workerNodeList []*nodes.Builder
			worker0Name    string
			worker1Name    string
			pf1            string
			pf2            string
			err            error
		)

		BeforeAll(func() {
			By("Verifying cluster has enough nodes")

			err = netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 2)
			Expect(err).ToNot(HaveOccurred(), "Cluster needs at least 2 worker nodes for Bond CNI LAG tests")

			By("Discover and list worker nodes")

			workerNodeList, err = nodes.List(
				APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")
			Expect(len(workerNodeList)).To(BeNumerically(">=", 2), "Expected at least 2 worker nodes")

			worker0Name = workerNodeList[0].Definition.Name
			worker1Name = workerNodeList[1].Definition.Name

			By("Validating SR-IOV interfaces exist on nodes")
			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfaces, err := NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			pf1 = sriovInterfaces[0]
			pf2 = sriovInterfaces[1]

			By("Removing stale bond SR-IOV policies from prior suite revisions")
			Expect(sriovenv.DeleteBondSriovPolicies(sriovenv.BondStalePolicyNames)).To(Succeed(),
				"Failed to remove stale bond SR-IOV policies")

			// Default MTU only — DiffPF slaves + listener VF share the default resource pool.
			// Policy defines VF count/range; no pre-check of device totalvfs.
			By("Creating default-MTU SR-IOV policies and DiffPF bond slave networks")
			Expect(sriovenv.SetupBondStack(sriovenv.BondStackParams{
				PF1:            pf1,
				PF2:            pf2,
				PF1NumVFs:      sriovenv.BondMinVFsPerPF,
				PF2NumVFs:      sriovenv.BondMinVFsPerPF,
				VFLargeStart:   0,
				VFLargeEnd:     sriovenv.BondMinVFsPerPF - 1,
				DefaultMTUOnly: true,
				Networks: []sriovenv.BondNetworkConfig{
					{Name: bondCNINetworksV4DiffPF[0], Resource: sriovenv.BondResourcePF1Default},
					{Name: bondCNINetworksV4DiffPF[1], Resource: sriovenv.BondResourcePF2Default},
				},
			})).To(Succeed(), "Failed to set up bond CNI SR-IOV stack")

			By("Creating static-IPAM SR-IOV network for single-VF listener pods")
			Expect(sriovenv.CreateSriovNetworkWithStaticIPAM(
				bondCNIListenerNetworkName, sriovenv.BondResourcePF1Default)).
				To(Succeed(), "Failed to create bond CNI listener VF network")

			By("Configure static LAG on lab switch for worker0 bond PFs only")

			// One LAG on worker0's two PF ports; worker1 listener ports stay non-LAG'd.
			// Shared for all xmitHashPolicy cases — restore once in AfterAll.
			err = setupBondSwitchLAG(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to configure static LAG on lab switch")

			By(fmt.Sprintf(
				"Bond CNI LAG topology: bonded-client=%s listeners=%s pf1=%s pf2=%s",
				worker0Name, worker1Name, pf1, pf2))
		})

		AfterAll(func() {
			restoreBondSwitchLAGAfterActiveActiveTest()

			By("Removing SR-IOV configuration")

			err = sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				NetConfig.WorkerLabelEnvVar,
				NetConfig.SriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
		})

		AfterEach(func() {
			By("Deleting test pods and Bond CNI NADs")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR(), nad.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods and NADs")
		})

		It("Validate balance-xor with layer2 xmitHashPolicy", reportxml.ID("89648"), func() {
			validateBondCNIBalanceXorXmitHashPolicy(
				worker0Name, worker1Name, sriovenv.BondXmitHashPolicyLayer2)
		})

		It("Validate balance-xor with layer2+3 xmitHashPolicy", reportxml.ID("89649"), func() {
			validateBondCNIBalanceXorXmitHashPolicy(
				worker0Name, worker1Name, sriovenv.BondXmitHashPolicyLayer23)
		})

		It("Validate balance-xor with layer3+4 xmitHashPolicy", reportxml.ID("89650"), func() {
			validateBondCNIBalanceXorXmitHashPolicy(
				worker0Name, worker1Name, sriovenv.BondXmitHashPolicyLayer34)
		})
	})

// validateBondCNIBalanceXorXmitHashPolicy deploys two listening single-VF pods (testcmd on net1)
// and one DiffPF balance-xor bonded client. Egress is from bond0 so xmitHashPolicy applies.
// Shared helper for layer2 / layer2+3 / layer3+4 (89648 / 89649 / 89650).
// Secondary Multus addresses are dual-stack; cluster dual-stack is not required.
// Flow: Create Bond → ICMP → TCP → ICMP hash skew → HA ping → re-skew.
func validateBondCNIBalanceXorXmitHashPolicy(bondNode, listenerNode, xmitHashPolicy string) {
	bondPod, _, _ := deployBondCNIXmitHashPolicyStack(
		bondNode, listenerNode, xmitHashPolicy)

	By(fmt.Sprintf(
		"Verifying Bond CNI NAD has mode=%s and xmitHashPolicy=%s",
		sriovenv.BondModeBalanceXOR, xmitHashPolicy))
	assertBondCNINADConfig(bondCNINADName, sriovenv.BondModeBalanceXOR, xmitHashPolicy)

	By("Verifying Multus network-status: bonded client has net1/net2/bond0 (dual-stack source IPs)")
	verifyBondCNINetworkStatus(
		bondPod,
		bondCNINADName,
		bondCNIAddrsWithoutPrefix([]string{bondCNIIPv4Bond, bondCNIIPv6Bond}),
	)

	By(fmt.Sprintf("Verifying bond xmit_hash_policy is %s", xmitHashPolicy))
	Expect(sriovenv.VerifyBondXmitHashPolicy(
		bondPod, sriovenv.BondInterfaceName, xmitHashPolicy)).
		To(Succeed(), "Bond xmit_hash_policy mismatch")

	By("Verifying bond interface state")
	Expect(sriovenv.VerifyBondInterfaceState(
		bondPod, sriovenv.BondInterfaceName, sriovenv.BondModeBalanceXOR, 2)).
		To(Succeed(), "Bond interface state mismatch")

	By("Verifying both bond slaves report MII up")
	Expect(sriovenv.WaitForBondSlavesMIIUp(bondPod, sriovenv.BondInterfaceName)).
		To(Succeed(), "Bond slaves not MII up")

	hashSkewDests := bondCNIHashSkewDestIPs

	icmpDests := make([]string, 0, len(hashSkewDests))
	for _, dest := range hashSkewDests {
		icmpDests = append(icmpDests, bondICMPDestination(dest))
	}

	By("Verifying ICMP connectivity over bond0 to all dual-stack listener IPs")
	Expect(cmd.ICMPConnectivityCheck(bondPod, icmpDests, sriovenv.BondInterfaceName)).
		To(Succeed(), "ICMP connectivity over bond0 failed")

	By("Verifying TCP connectivity over bond0 to all dual-stack listener IPs")
	Expect(verifyBondCNITCPConnectivity(bondPod, hashSkewDests, sriovenv.BondMTUDefault)).
		To(Succeed(), "TCP connectivity over bond0 failed")

	By(fmt.Sprintf(
		"Verifying xmitHashPolicy=%s spreads ICMP egress across both bond slaves (dests=%v)",
		xmitHashPolicy, hashSkewDests))
	Expect(verifyBondCNIHashSkew(bondPod, hashSkewDests)).
		To(Succeed(), "Bond hash skew across slaves failed")

	By("Triggering LAG member failure during continuous ICMP and verifying little/no ping loss")
	Expect(toggleSwitchPortsDuringBondCNIContinuousPing(
		bondPod, bondCNIIPv4ListenerA)).
		To(Succeed(), "Bond HA after LAG/slave failure failed")

	By("Re-verifying xmitHashPolicy spreads ICMP egress across both bond slaves after HA")
	Expect(verifyBondCNIHashSkew(bondPod, hashSkewDests)).
		To(Succeed(), "Bond hash skew across slaves failed after HA")
}

// deployBondCNIXmitHashPolicyStack creates the Bond CNI NAD, one bonded egress pod, and two
// listening pods with SR-IOV VFs. Caller relies on AfterEach for pod/NAD cleanup.
func deployBondCNIXmitHashPolicyStack(
	bondNode, listenerNode, xmitHashPolicy string,
) (*pod.Builder, *pod.Builder, *pod.Builder) {
	By(fmt.Sprintf("Creating balance-xor Bond CNI NAD with xmitHashPolicy=%s", xmitHashPolicy))
	createBondCNINAD(bondCNINADName, xmitHashPolicy)

	By(fmt.Sprintf("Creating listener A with testcmd on %s (MAC %s)", listenerNode, bondCNIListenerAMAC))
	listenerA := createBondCNIListeningPod(
		"server-pod-1",
		listenerNode,
		bondCNIListenerNetworkName,
		bondCNIListenerAIPs,
		bondCNIListenerAMAC,
		sriovenv.BondMTUDefault,
	)

	By(fmt.Sprintf("Creating listener B with testcmd on %s (MAC %s)", listenerNode, bondCNIListenerBMAC))
	listenerB := createBondCNIListeningPod(
		"server-pod-2",
		listenerNode,
		bondCNIListenerNetworkName,
		bondCNIListenerBIPs,
		bondCNIListenerBMAC,
		sriovenv.BondMTUDefault,
	)

	By(fmt.Sprintf("Creating bonded client on %s (DiffPF slaves, egress via bond0)", bondNode))
	bondPod := createBondCNIBondedPod(
		bondCNINADName,
		"bond-cni-pod",
		bondNode,
		bondCNINetworksV4DiffPF,
		[]string{bondCNIIPv4Bond, bondCNIIPv6Bond},
	)

	return bondPod, listenerA, listenerB
}

// createBondCNIBondedPod creates a privileged DiffPF bonded pod.
func createBondCNIBondedPod(
	nadName, podName, nodeName string,
	slaveNetworks []string,
	bondIPsWithCIDR []string,
) *pod.Builder {
	annotation := pod.StaticIPBondAnnotationWithInterface(
		nadName,
		sriovenv.BondInterfaceName,
		slaveNetworks,
		bondIPsWithCIDR,
	)
	Expect(annotation).NotTo(BeNil(), "Failed to create bond annotation for bonded pod")

	bondPod, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotation).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create bonded pod")

	Expect(sriovenv.WaitForBondSlavesMIIUp(bondPod, sriovenv.BondInterfaceName)).
		To(Succeed(), "Bond slaves not MII up in pod %s", podName)

	return bondPod
}

// createBondCNIListeningPod creates a single-VF pod that listens with testcmd on net1.
func createBondCNIListeningPod(
	podName, nodeName, networkName string,
	ipRequests []string,
	macAddress string,
	mtu int,
) *pod.Builder {
	secNetwork := pod.StaticIPAnnotationWithMacAddress(networkName, ipRequests, macAddress)
	Expect(secNetwork).NotTo(BeNil(), "Failed to build listener network annotation for %s", podName)

	command := bondCNIListenerCommand(ipRequests, mtu)

	container, err := pod.NewContainerBuilder("server", NetConfig.CnfNetTestContainer, command).GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to build listener container for %s", podName)

	listenerPod, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(nodeName).
		RedefineDefaultContainer(*container).
		WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create listening VF pod %s", podName)

	Expect(sriovenv.WaitForServerReady(listenerPod, tsparams.WaitTimeout)).
		To(Succeed(), "Listener testcmd not ready on pod %s", podName)

	return listenerPod
}

// bondCNIAddrsWithoutPrefix returns RemovePrefix for each CIDR address.
func bondCNIAddrsWithoutPrefix(addrsWithCIDR []string) []string {
	out := make([]string, 0, len(addrsWithCIDR))
	for _, addr := range addrsWithCIDR {
		out = append(out, ipaddr.RemovePrefix(addr))
	}

	return out
}

// bondCNITCPPortForIP maps a listener address to its TCP listen port (5000 + hostID - 20).
func bondCNITCPPortForIP(ipWithCIDR string) int {
	host := ipaddr.RemovePrefix(ipWithCIDR)

	var (
		hostID int
		err    error
	)

	if strings.Contains(host, ":") {
		last := host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			last = host[i+1:]
		}

		hostID, err = strconv.Atoi(last)
		Expect(err).ToNot(HaveOccurred(), "failed to parse IPv6 host id from %q", host)
	} else {
		parts := strings.Split(host, ".")
		Expect(parts).To(HaveLen(4), "expected IPv4 address, got %q", host)

		hostID, err = strconv.Atoi(parts[3])
		Expect(err).ToNot(HaveOccurred(), "failed to parse last octet of %q", host)
	}

	return bondCNITCPPortBase + hostID - bondCNITCPPortOctetOffset
}

// bondCNIListenerCommand starts a testcmd TCP listener per unique port on net1.
func bondCNIListenerCommand(ipRequests []string, mtu int) []string {
	packetSize := mtu - 100
	portsSeen := map[int]bool{}

	var listeners strings.Builder

	for _, ipWithCIDR := range ipRequests {
		port := bondCNITCPPortForIP(ipWithCIDR)
		if portsSeen[port] {
			continue
		}

		portsSeen[port] = true
		fmt.Fprintf(&listeners,
			"testcmd -listen -protocol tcp -port %d -interface %s -mtu %d & ",
			port, tsparams.Net1Interface, packetSize)
	}

	listeners.WriteString("sleep infinity")

	return []string{"bash", "-c", listeners.String()}
}

// createBondCNINAD creates a balance-xor Bond CNI NAD with the given xmitHashPolicy.
func createBondCNINAD(nadName, xmitHashPolicy string) {
	deleteBondNADBestEffort(nadName)

	var (
		bondBuilder *nad.Builder
		err         error
	)

	bondBuilder, err = sriovenv.CreateBondNADWithXmitHashPolicy(
		nadName, sriovenv.BondModeBalanceXOR, "static", sriovenv.BondMTUDefault, 2, xmitHashPolicy)
	Expect(err).ToNot(HaveOccurred(), "Failed to build bond NAD %s with xmitHashPolicy %s",
		nadName, xmitHashPolicy)

	_, err = bondBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create bond NAD %s", nadName)
}

// assertBondCNINADConfig pulls the NAD and checks Spec.Config for bond mode and xmitHashPolicy.
func assertBondCNINADConfig(nadName, expectedMode, expectedXmitHashPolicy string) {
	pulled, err := nad.Pull(APIClient, nadName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull bond NAD %s", nadName)
	Expect(pulled.Definition.Spec.Config).NotTo(BeEmpty(), "Bond NAD %s has empty Spec.Config", nadName)

	var cfg map[string]interface{}
	Expect(json.Unmarshal([]byte(pulled.Definition.Spec.Config), &cfg)).
		To(Succeed(), "Failed to unmarshal bond NAD %s Spec.Config: %s",
			nadName, pulled.Definition.Spec.Config)

	Expect(cfg["type"]).To(Equal("bond"), "Bond NAD %s type mismatch", nadName)
	Expect(cfg["mode"]).To(Equal(expectedMode), "Bond NAD %s mode mismatch", nadName)
	Expect(cfg["xmitHashPolicy"]).To(Equal(expectedXmitHashPolicy),
		"Bond NAD %s xmitHashPolicy mismatch", nadName)
}

// bondCNINetworkStatusEntry is a minimal Multus network-status object for assertions.
type bondCNINetworkStatusEntry struct {
	Name      string   `json:"name"`
	Interface string   `json:"interface"`
	IPs       []string `json:"ips"`
	Mac       string   `json:"mac"`
}

// parseBondCNINetworkStatus returns Multus network-status entries keyed by interface name.
func parseBondCNINetworkStatus(podBuilder *pod.Builder) map[string]bondCNINetworkStatusEntry {
	pulled, err := pod.Pull(APIClient, podBuilder.Definition.Name, podBuilder.Definition.Namespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull pod %s for network-status", podBuilder.Definition.Name)

	annotation := pulled.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
	Expect(annotation).NotTo(BeEmpty(), "Pod %s missing network-status annotation", pulled.Definition.Name)

	var statuses []bondCNINetworkStatusEntry
	Expect(json.Unmarshal([]byte(annotation), &statuses)).
		To(Succeed(), "Failed to unmarshal network-status for pod %s: %s", pulled.Definition.Name, annotation)

	By(fmt.Sprintf("network-status for %s: %s", pulled.Definition.Name, annotation))

	ifaces := make(map[string]bondCNINetworkStatusEntry, len(statuses))
	for _, status := range statuses {
		if status.Interface != "" {
			ifaces[status.Interface] = status
		}
	}

	return ifaces
}

// networkStatusIPs returns host addresses from a network-status entry (strips CIDR if present).
func networkStatusIPs(status bondCNINetworkStatusEntry) []string {
	gotIPs := make([]string, 0, len(status.IPs))
	for _, ipWithPrefix := range status.IPs {
		gotIPs = append(gotIPs, strings.Split(ipWithPrefix, "/")[0])
	}

	return gotIPs
}

// verifyBondCNINetworkStatus asserts k8s.v1.cni.cncf.io/network-status after ADD includes
// Multus-assigned slave interfaces (net1, net2) and bond0 with the expected IPs and NAD name.
func verifyBondCNINetworkStatus(podBuilder *pod.Builder, bondNADName string, expectedBondIPs []string) {
	ifaces := parseBondCNINetworkStatus(podBuilder)

	Expect(ifaces).To(HaveKey(sriovenv.BondSlave1IfName),
		"network-status missing slave interface %s", sriovenv.BondSlave1IfName)
	Expect(ifaces).To(HaveKey(sriovenv.BondSlave2IfName),
		"network-status missing slave interface %s", sriovenv.BondSlave2IfName)
	Expect(ifaces).To(HaveKey(sriovenv.BondInterfaceName),
		"network-status missing bond interface %s", sriovenv.BondInterfaceName)

	bondStatus := ifaces[sriovenv.BondInterfaceName]
	Expect(bondStatus.Name).To(ContainSubstring(bondNADName),
		"bond network-status name %q should reference NAD %s", bondStatus.Name, bondNADName)
	Expect(bondStatus.Mac).NotTo(BeEmpty(), "bond network-status missing MAC")

	gotIPs := networkStatusIPs(bondStatus)
	for _, expectedIP := range expectedBondIPs {
		Expect(gotIPs).To(ContainElement(expectedIP),
			"bond network-status IPs %v missing expected %s", gotIPs, expectedIP)
	}
}

// bondCNITCPTestCmd builds the bond0→dest TCP testcmd used by TCP connectivity checks.
func bondCNITCPTestCmd(destHost string, tcpPort, packetSize int) string {
	return fmt.Sprintf("testcmd -protocol tcp -port %d -interface %s -server %s -mtu %d",
		tcpPort, sriovenv.BondInterfaceName, destHost, packetSize)
}

// verifyBondCNITCPConnectivity runs testcmd TCP from bond0 to each dest's listen port.
func verifyBondCNITCPConnectivity(bondPod *pod.Builder, destIPsWithCIDR []string, mtu int) error {
	packetSize := mtu - 100

	for _, dest := range destIPsWithCIDR {
		host := ipaddr.RemovePrefix(dest)
		tcpPort := bondCNITCPPortForIP(dest)

		if err := sriovenv.RunProtocolTest(bondPod, "TCP",
			bondCNITCPTestCmd(host, tcpPort, packetSize)); err != nil {
			return fmt.Errorf("TCP to %s:%d failed: %w", host, tcpPort, err)
		}
	}

	return nil
}

// verifyBondCNIHashSkew runs ICMP hash-skew until both bond slaves are used (net1 and
// net2 TX >= bondCNIHashSkewMinTX for at least one dest each), stopping early once both
// are seen. Retries up to bondCNIHashSkewAttempts if a full pool pass fails.
func verifyBondCNIHashSkew(bondPod *pod.Builder, destIPsWithCIDR []string) error {
	var lastErr error

	for attempt := 1; attempt <= bondCNIHashSkewAttempts; attempt++ {
		lastErr = checkBondCNIHashSkewAcrossSlaves(bondPod, destIPsWithCIDR)
		if lastErr == nil {
			if attempt > 1 {
				klog.Infof("hash skew succeeded on attempt %d/%d", attempt, bondCNIHashSkewAttempts)
			}

			return nil
		}

		klog.Infof("hash skew attempt %d/%d failed: %v", attempt, bondCNIHashSkewAttempts, lastErr)
	}

	return fmt.Errorf("hash skew failed after %d attempts: %w", bondCNIHashSkewAttempts, lastErr)
}

// checkBondCNIHashSkewAcrossSlaves is one hash-skew pool walk (no retries).
func checkBondCNIHashSkewAcrossSlaves(bondPod *pod.Builder, destIPsWithCIDR []string) error {
	var usedNet1, usedNet2 bool

	for _, dest := range destIPsWithCIDR {
		deltaNet1, deltaNet2, err := measureBondCNIICMPSlaveTX(bondPod, dest)
		if err != nil {
			return err
		}

		usedNet1 = usedNet1 || deltaNet1 >= bondCNIHashSkewMinTX
		usedNet2 = usedNet2 || deltaNet2 >= bondCNIHashSkewMinTX

		if usedNet1 && usedNet2 {
			return nil
		}
	}

	return fmt.Errorf(
		"xmitHashPolicy did not spread ICMP across both slaves with >=%d TX each "+
			"(net1=%t net2=%t, dests=%v)",
		bondCNIHashSkewMinTX, usedNet1, usedNet2, destIPsWithCIDR)
}

// measureBondCNIICMPSlaveTX pings dest from bond0 and returns net1/net2 TX deltas.
func measureBondCNIICMPSlaveTX(
	bondPod *pod.Builder, destWithCIDR string,
) (deltaNet1, deltaNet2 uint64, err error) {
	host := ipaddr.RemovePrefix(destWithCIDR)

	txBeforeNet1, txBeforeNet2, err := getBondCNISlaveTXPackets(bondPod)
	if err != nil {
		return 0, 0, err
	}

	pingCmd := fmt.Sprintf("ping -I %s -c %d -W 1 %s",
		sriovenv.BondInterfaceName, bondCNIHashSkewPingCount, host)
	if parsed := net.ParseIP(host); parsed != nil && parsed.To4() == nil {
		pingCmd = fmt.Sprintf("ping -6 -I %s -c %d -W 1 %s",
			sriovenv.BondInterfaceName, bondCNIHashSkewPingCount, host)
	}

	if out, pingErr := bondPod.ExecCommand([]string{"bash", "-c", pingCmd}); pingErr != nil {
		return 0, 0, fmt.Errorf("ICMP to hash-skew dest %s failed: %w (out=%s)",
			host, pingErr, out.String())
	}

	txAfterNet1, txAfterNet2, err := getBondCNISlaveTXPackets(bondPod)
	if err != nil {
		return 0, 0, err
	}

	deltaNet1 = txDelta(txBeforeNet1, txAfterNet1)
	deltaNet2 = txDelta(txBeforeNet2, txAfterNet2)

	klog.V(90).Infof("hash skew ICMP %s: net1 TX %d->%d (+%d) net2 TX %d->%d (+%d) minTX=%d",
		host, txBeforeNet1, txAfterNet1, deltaNet1,
		txBeforeNet2, txAfterNet2, deltaNet2, bondCNIHashSkewMinTX)

	return deltaNet1, deltaNet2, nil
}

// txDelta returns after-before, or 0 if the counter decreased (e.g. interface re-plumb).
func txDelta(before, after uint64) uint64 {
	if after < before {
		return 0
	}

	return after - before
}

// getBondCNISlaveTXPackets reads net1 and net2 tx_packets from the bonded pod.
func getBondCNISlaveTXPackets(podBuilder *pod.Builder) (net1TX, net2TX uint64, err error) {
	net1TX, err = getBondCNIIfaceTXPackets(podBuilder, sriovenv.BondSlave1IfName)
	if err != nil {
		return 0, 0, err
	}

	net2TX, err = getBondCNIIfaceTXPackets(podBuilder, sriovenv.BondSlave2IfName)
	if err != nil {
		return 0, 0, err
	}

	return net1TX, net2TX, nil
}

// getBondCNIIfaceTXPackets reads /sys/class/net/<iface>/statistics/tx_packets inside the pod.
func getBondCNIIfaceTXPackets(podBuilder *pod.Builder, iface string) (uint64, error) {
	out, err := podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/statistics/tx_packets", iface)})
	if err != nil {
		return 0, fmt.Errorf("failed to read %s tx_packets: %w (out=%s)", iface, err, out.String())
	}

	count, err := strconv.ParseUint(strings.TrimSpace(out.String()), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s tx_packets %q: %w", iface, out.String(), err)
	}

	return count, nil
}

// toggleSwitchPortsDuringBondCNIContinuousPing toggles each switch LAG member during a continuous ping on bond0.
func toggleSwitchPortsDuringBondCNIContinuousPing(bondPod *pod.Builder, listenerIP string) error {
	if bondSwitchCredentials == nil || len(bondSwitchInterfaces) < 2 || len(bondSwitchLagNames) < 1 {
		return fmt.Errorf("switch LAG not configured (need credentials, 2+ interfaces, LAG name)")
	}

	jnpr, err := cmd.NewSession(
		bondSwitchCredentials.SwitchIP, bondSwitchCredentials.User, bondSwitchCredentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	disable := func(iface string) error {
		return jnpr.Config([]string{fmt.Sprintf("set interfaces %s disable", iface)})
	}
	enable := func(iface string) error {
		return jnpr.Config([]string{fmt.Sprintf("delete interfaces %s disable", iface)})
	}

	if err := startBondCNIHAContinuousPing(bondPod, listenerIP); err != nil {
		return err
	}

	// Give the first probes a moment to start before the first disable.
	time.Sleep(2 * time.Second)

	if err := cycleSwitchPortDuringBondCNIHAPing(
		bondPod, jnpr, bondSwitchInterfaces[0], disable, enable); err != nil {
		return err
	}

	if err := cycleSwitchPortDuringBondCNIHAPing(
		bondPod, jnpr, bondSwitchInterfaces[1], disable, enable); err != nil {
		return err
	}

	return collectBondCNIHAContinuousPing(bondPod)
}

// restoreBondCNISwitchPortBestEffort tries to re-enable a switch port after a failed HA step.
func restoreBondCNISwitchPortBestEffort(enable func(string) error, iface string) {
	if restoreErr := enable(iface); restoreErr != nil {
		klog.Warningf("best-effort switch restore failed for %s: %v", iface, restoreErr)
	}
}

// cycleSwitchPortDuringBondCNIHAPing disables one switch LAG member, waits for bond degrade,
// re-enables it, waits for the switch port up, then waits for both slaves MII up.
func cycleSwitchPortDuringBondCNIHAPing(
	bondPod *pod.Builder,
	jnpr *cmd.Junos,
	port string,
	disable, enable func(string) error,
) error {
	if err := disable(port); err != nil {
		_ = collectBondCNIHAContinuousPing(bondPod)

		return fmt.Errorf("failed to disable switch interface %s: %w", port, err)
	}

	if err := sriovenv.WaitForBondDegradedOneSlaveDown(bondPod, sriovenv.BondInterfaceName); err != nil {
		restoreBondCNISwitchPortBestEffort(enable, port)

		_ = collectBondCNIHAContinuousPing(bondPod)

		return fmt.Errorf("bond did not degrade after disabling %s: %w", port, err)
	}

	if err := enable(port); err != nil {
		restoreBondCNISwitchPortBestEffort(enable, port)

		_ = collectBondCNIHAContinuousPing(bondPod)

		return fmt.Errorf("failed to re-enable switch interface %s: %w", port, err)
	}

	if err := waitForSwitchInterfaceUp(jnpr, port, time.Minute); err != nil {
		_ = collectBondCNIHAContinuousPing(bondPod)

		return err
	}

	if err := sriovenv.WaitForBondSlavesMIIUp(bondPod, sriovenv.BondInterfaceName); err != nil {
		_ = collectBondCNIHAContinuousPing(bondPod)

		return fmt.Errorf("bond did not recover after re-enabling %s: %w", port, err)
	}

	return nil
}

const (
	bondCNIHAPingOutFile  = "/tmp/bond-cni-ha-ping.out"
	bondCNIHAPingDoneFile = "/tmp/bond-cni-ha-ping.done"
	bondCNIHAPingPIDFile  = "/tmp/bond-cni-ha-ping.pid"
)

// startBondCNIHAContinuousPing launches an unbounded ping on bond0 in the background.
func startBondCNIHAContinuousPing(bondPod *pod.Builder, listenerIP string) error {
	dest := ipaddr.RemovePrefix(listenerIP)
	cmdLine := fmt.Sprintf(
		"rm -f %s %s %s; "+
			"nohup bash -c '"+
			"ping -I %s -W 1 %s > %s 2>&1 & "+
			"echo $! > %s; "+
			"wait $!; "+
			"touch %s"+
			"' >/dev/null 2>&1 & "+
			// Poll briefly: the wrapper writes the PID file asynchronously.
			"for _ in $(seq 1 50); do "+
			"if test -s %s && kill -0 \"$(cat %s)\" 2>/dev/null; then exit 0; fi; "+
			"sleep 0.1; "+
			"done; "+
			"exit 1",
		bondCNIHAPingOutFile, bondCNIHAPingDoneFile, bondCNIHAPingPIDFile,
		sriovenv.BondInterfaceName, dest, bondCNIHAPingOutFile,
		bondCNIHAPingPIDFile,
		bondCNIHAPingDoneFile,
		bondCNIHAPingPIDFile, bondCNIHAPingPIDFile)

	if _, err := bondPod.ExecCommand([]string{"bash", "-c", cmdLine}); err != nil {
		return fmt.Errorf("failed to start continuous HA ping: %w", err)
	}

	return nil
}

// collectBondCNIHAContinuousPing stops the background ping by PID (SIGINT so it prints a
// summary), waits for the wrapper to mark done, and asserts loss stays within limits.
func collectBondCNIHAContinuousPing(bondPod *pod.Builder) error {
	// Interrupt only the ping PID; the wrapper waits on it then touches the done file.
	_, _ = bondPod.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf(
			"if test -s %s; then kill -INT \"$(cat %s)\" 2>/dev/null || true; fi",
			bondCNIHAPingPIDFile, bondCNIHAPingPIDFile)})

	deadline := time.Now().Add(30 * time.Second)

	for {
		out, err := bondPod.ExecCommand([]string{"bash", "-c",
			fmt.Sprintf("test -f %s && echo done || echo wait", bondCNIHAPingDoneFile)})
		if err == nil && strings.Contains(out.String(), "done") {
			break
		}

		if time.Now().After(deadline) {
			_, _ = bondPod.ExecCommand([]string{"bash", "-c",
				fmt.Sprintf(
					"if test -s %s; then kill -KILL \"$(cat %s)\" 2>/dev/null || true; fi; touch %s",
					bondCNIHAPingPIDFile, bondCNIHAPingPIDFile, bondCNIHAPingDoneFile)})

			partial, _ := bondPod.ExecCommand([]string{"bash", "-c",
				fmt.Sprintf("cat %s 2>/dev/null || echo '(no ping output yet)'", bondCNIHAPingOutFile)})

			return fmt.Errorf("continuous HA ping did not finish within 30s after stop; partial out:\n%s",
				partial.String())
		}

		time.Sleep(tsparams.NADWaitTimeout)
	}

	out, err := bondPod.ExecCommand([]string{"bash", "-c", "cat " + bondCNIHAPingOutFile})
	if err != nil {
		return fmt.Errorf("failed to read continuous HA ping output: %w", err)
	}

	pingOut := out.String()

	transmitted, received, lost, parseErr := parsePingPacketCounts(pingOut)
	if parseErr != nil {
		return fmt.Errorf("failed to parse continuous HA ping output: %w (out=%s)", parseErr, pingOut)
	}

	klog.Infof("Bond CNI HA continuous ping: transmitted=%d received=%d lost=%d (minTX=%d maxAllowed=%d)",
		transmitted, received, lost, bondCNIHAPingMinTX, bondCNIHAPingMaxLoss)

	if transmitted < bondCNIHAPingMinTX {
		return fmt.Errorf(
			"continuous HA ping sent too few probes: tx=%d minRequired=%d (rx=%d lost=%d)\n%s",
			transmitted, bondCNIHAPingMinTX, received, lost, pingOut)
	}

	if lost > bondCNIHAPingMaxLoss {
		return fmt.Errorf(
			"continuous HA ping loss too high: lost=%d maxAllowed=%d (tx=%d rx=%d)\n%s",
			lost, bondCNIHAPingMaxLoss, transmitted, received, pingOut)
	}

	return nil
}

var pingPacketCountRE = regexp.MustCompile(
	`(\d+)\s+packets transmitted,\s+(\d+)\s+received`)

// parsePingPacketCounts extracts transmitted/received/lost from ping summary output.
func parsePingPacketCounts(output string) (transmitted, received, lost int, err error) {
	match := pingPacketCountRE.FindStringSubmatch(output)
	if match == nil {
		return 0, 0, 0, fmt.Errorf("no ping summary line found")
	}

	transmitted, err = strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, 0, err
	}

	received, err = strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, 0, err
	}

	if received > transmitted {
		received = transmitted
	}

	return transmitted, received, transmitted - received, nil
}
