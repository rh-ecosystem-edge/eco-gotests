package tests

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/events"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/frrconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	mlbcmd "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Layer2", Ordered, Label(tsparams.LabelLayer2TestCases), ContinueOnFailure, func() {
	var (
		clientTestPod *pod.Builder
		err           error
	)
	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

		By("Creating a new instance of MetalLB Speakers on workers")
		err = metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, workerLabelMap)
		}
	})

	BeforeEach(func() {
		By("Creating an IPAddressPool and L2Advertisement")
		ipAddressPool := setupL2Advertisement(ipv4metalLbIPList)

		By("Creating a MetalLB service")
		setupMetalLbService(
			tsparams.MetallbServiceName,
			netparam.IPV4Family,
			tsparams.LabelValue1,
			ipAddressPool,
			corev1.ServiceExternalTrafficPolicyTypeCluster)

		By("Creating external Network Attachment Definition")
		err = define.CreateExternalNad(APIClient, frrconfig.ExternalMacVlanNADName, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

		By("Creating client test pod")
		clientTestPod, err = pod.NewBuilder(
			APIClient, "l2clientpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
			DefineOnNode(masterNodeList[0].Object.Name).
			WithTolerationToMaster().
			WithSecondaryNetwork(pod.StaticIPAnnotation(frrconfig.ExternalMacVlanNADName,
				[]string{fmt.Sprintf("%s/%s", ipv4metalLbIPList[1], netparam.IPSubnet24)})).
			WithPrivilegedFlag().CreateAndWaitUntilRunning(time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to create client test pod")
	})

	AfterEach(func() {
		if labelExists(workerNodeList[1], tsparams.TestLabel) {
			By("Remove custom test label from nodes")
			removeNodeLabel(workerNodeList, tsparams.TestLabel)
		}

		By("Cleaning MetalLb operator namespace")
		metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
		err = metalLbNs.CleanObjects(
			tsparams.DefaultTimeout,
			metallb.GetL2AdvertisementGVR(),
			metallb.GetIPAddressPoolGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

		By("Cleaning test namespace")
		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout,
			pod.GetGVR(),
			service.GetGVR(),
			nad.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
	})

	It("Validate MetalLB Layer 2 functionality", reportxml.ID("42936"), func() {
		By("Creating nginx test pod on worker node")
		setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
			workerNodeList[0].Definition.Name,
			tsparams.LabelValue1)

		By("Getting announcing node name")
		announcingNodeName := getLBServiceAnnouncingNodeName()

		By("Running traffic test")
		trafficTest(clientTestPod, announcingNodeName)
	})

	It("Failure of MetalLB announcing speaker node", reportxml.ID("42751"), func() {
		By("Changing the label selector for Metallb and adding a label for Workers")
		metalLbIo, err := metallb.Pull(APIClient, tsparams.MetalLbIo, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metallb.io object")
		_, err = metalLbIo.WithSpeakerNodeSelector(tsparams.TestLabel).Update(false)
		Expect(err).ToNot(HaveOccurred(), "Failed to update metallb object with the new MetalLb label")

		By("Adding test label to compute nodes")

		addNodeLabel(workerNodeList, tsparams.TestLabel)

		By("Validating all metalLb speaker daemonset are running")
		metalLbDaemonSetShouldMatchConditionAndBeInReadyState(
			BeEquivalentTo(len(workerNodeList)), "Failed to run metalLb speakers on top of nodes with test label")

		By("Creating nginx test pod on worker nodes")
		setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[0].Definition.Name,
			workerNodeList[0].Definition.Name,
			tsparams.LabelValue1)
		setupNGNXPod(tsparams.MLBNginxPodName+workerNodeList[1].Definition.Name,
			workerNodeList[1].Definition.Name,
			tsparams.LabelValue1)

		By("Getting announcing node name")
		announcingNodeName := getLBServiceAnnouncingNodeName()

		By("Removing test label from announcing node")
		applyTestLabelActionToNode(announcingNodeName, removeNodeLabel)

		metalLbDaemonSetShouldMatchConditionAndBeInReadyState(
			BeEquivalentTo(len(workerNodeList)-1), "Failed to run metalLb speakers on top of nodes with test label")

		By("Should have a new MetalLB announcing node during failure of announcing speaker")
		var announcingNodeNameDuringFailure string

		Eventually(func() string {
			announcingNodeNameDuringFailure = getLBServiceAnnouncingNodeName()

			return announcingNodeNameDuringFailure
		}, tsparams.DefaultTimeout, tsparams.DefaultRetryInterval).ShouldNot(Equal(announcingNodeName),
			fmt.Sprintf("Announcing node %s is not changed", announcingNodeNameDuringFailure))

		By("Running traffic test")
		trafficTest(clientTestPod, announcingNodeNameDuringFailure)

		By("Returning back test label to the original announcing node")
		applyTestLabelActionToNode(announcingNodeName, addNodeLabel)

		metalLbDaemonSetShouldMatchConditionAndBeInReadyState(
			BeEquivalentTo(len(workerNodeList)), "Failed to run metalLb speakers on top of nodes with test label")

		By("Should have node return to announcing node after failure")
		Eventually(getLBServiceAnnouncingNodeName,
			tsparams.DefaultTimeout, tsparams.DefaultRetryInterval).Should(Equal(announcingNodeName),
			fmt.Sprintf("Announcing node %s is not changed back", announcingNodeNameDuringFailure))

		By("Running traffic test")
		trafficTest(clientTestPod, announcingNodeName)
	})
})

func labelExists(nodeObject *nodes.Builder, givenLabel map[string]string) bool {
	if !nodeObject.Exists() {
		return false
	}

	nodeLabels := nodeObject.Object.GetLabels()
	for key, value := range givenLabel {
		if nodeLabels[key] != value {
			return false
		}
	}

	return true
}

func trafficTest(clientTestPod *pod.Builder, nodeName string) {
	By("Running Arping test")
	arpingTest(clientTestPod, ipv4metalLbIPList[0], nodeName)

	By("Running http check")

	httpOutput, err := mlbcmd.Curl(clientTestPod, ipv4metalLbIPList[1], ipv4metalLbIPList[0], netparam.IPV4Family)
	Expect(err).ToNot(HaveOccurred(), httpOutput)
}

// getLBServiceAnnouncingNodeName searches for node name in following string example:
// announcing from node "nodeName".
func getLBServiceAnnouncingNodeName() string {
	serviceEvents, err := events.List(
		APIClient, tsparams.TestNamespaceName, metav1.ListOptions{FieldSelector: "reason=nodeAssigned"})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get events in namespace %s", tsparams.TestNamespaceName))

	var allEvents []string
	//nolint:varnamelen
	sort.Slice(serviceEvents, func(i int, j int) bool {
		// Primary sort: LastTimestamp
		if serviceEvents[i].Object.LastTimestamp.Time.Equal(serviceEvents[j].Object.LastTimestamp.Time) {
			// Secondary sort: FirstTimestamp
			return serviceEvents[i].Object.FirstTimestamp.Time.Before(serviceEvents[j].Object.FirstTimestamp.Time)
		}

		return serviceEvents[i].Object.LastTimestamp.Time.Before(serviceEvents[j].Object.LastTimestamp.Time)
	})
	Expect(len(serviceEvents)).To(BeNumerically(">", 0), "No events were found")

	lastSortedEvent := serviceEvents[len(serviceEvents)-1]
	for _, index := range strings.Split(lastSortedEvent.Object.String(), "}") {
		if strings.Contains(index, "announcing from node") {
			re := regexp.MustCompile(`"([^\"]+)"`)
			event := re.FindString(index)
			allEvents = append(allEvents, event)
		}
	}

	return strings.Trim(allEvents[len(allEvents)-1], "\"")
}

func arpingTest(client *pod.Builder, destIPAddr, nodeName string) {
	arpingOutput, err := mlbcmd.Arping(client, destIPAddr)
	Expect(err).ToNot(HaveOccurred(), "Failed to run arping command")

	output := strings.Split(arpingOutput, "\n")
	lineCount := 0

	for _, reply := range output {
		if strings.Contains(reply, "Unicast") {
			lineCount++
		}
	}

	// When using the NAD interface the mac address of eth0 is included in the arp replies adding an extra line count.
	Expect(lineCount).To(Equal(3), "An incorrect number of arp replies were received")
	// Verifies the output mac addresses matches the announcing node mac address
	nodeMac := speakerNodeMac(nodeName)
	Expect(strings.Join(output, "\n")).Should(ContainSubstring(strings.ToUpper(nodeMac)),
		"ARP request was not received from the announcing node")
}

// speakerNodeMac locates the MAC address of the node interface br-ex found in func GetLBServiceNodeName()
// {"mode":"shared","interface-id":"br-ex_testcluster.com","mac-address":"34:66:ed:f3:88:66",
// "ip-addresses":["10.60.60.60/24"],"ip-address":"10.60.60.60/24","next-hops":["10.60.60.254"],"next-hop":
// "10.60.60.254","node-port-enable":"true","vlan-id":"0"}.
func speakerNodeMac(metallbNodeName string) string {
	metallbNode, err := nodes.Pull(APIClient, metallbNodeName)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull the node %s", metallbNodeName))

	val := metallbNode.Object.Annotations["k8s.ovn.org/l3-gateway-config"]

	for _, index := range strings.Split(val, ",") {
		if strings.Contains(index, "mac-address") {
			re := regexp.MustCompile("([0-9a-fA-F]{2}[:]){5}([0-9a-fA-F]{2})")
			mFind := re.FindAllString(index, -1)

			return strings.Join(mFind, "")
		}
	}

	return ""
}

func applyTestLabelActionToNode(nodeName string, actionFunc func([]*nodes.Builder, map[string]string)) {
	var found bool

	for _, workerNode := range workerNodeList {
		if workerNode.Object.Name == nodeName {
			actionFunc([]*nodes.Builder{workerNode}, tsparams.TestLabel)

			found = true

			break
		}
	}

	Expect(found).To(BeTrue(), fmt.Sprintf("Failed to find worker with name %s", nodeName))
}
