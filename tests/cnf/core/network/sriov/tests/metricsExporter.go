package tests

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type testResource struct {
	policy  *sriov.PolicyBuilder
	network *sriov.NetworkBuilder
	pod     *pod.Builder
}

var serverPodRXPromQL = []string{"bash", "-c", "promtool query instant -o json " +
	"http://localhost:9090 \"sum(sriov_vf_rx_packets * on(pciAddr) group_left(pod) " +
	"sriov_kubepoddevice{pod=\\\"serverpod\\\"}) by (pod)\""}
var serverPodTXPromQL = []string{"bash", "-c", "promtool query instant -o json " +
	"http://localhost:9090 \"sum(sriov_vf_tx_packets * on(pciAddr) group_left(pod) " +
	"sriov_kubepoddevice{pod=\\\"serverpod\\\"}) by (pod)\""}

var _ = Describe(
	"SriovMetricsExporter", Ordered, Label(tsparams.LabelSriovMetricsTestCases, tsparams.LabelSriovHWEnabled),
	ContinueOnFailure, func() {

		var (
			workerNodeList           []*nodes.Builder
			sriovmetricsdaemonset    *daemonset.Builder
			sriovInterfacesUnderTest []string
			sriovVendorID            string
		)

		BeforeAll(func() {
			By("Verifying if Sriov Metrics Exporter tests can be executed on given cluster")
			err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 2)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support Sriov Metrics Exporter test cases as it doesn't have enough nodes")

			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Fetching SR-IOV Device ID for interface under test")
			sriovVendorID = discoverInterfaceUnderTestVendorID(sriovInterfacesUnderTest[0], workerNodeList[0].Definition.Name)
			Expect(sriovVendorID).ToNot(BeEmpty(), "Expected sriovDeviceID not to be empty")

			By("Enable Sriov Metrics Exporter feature in default SriovOperatorConfig CR")
			setMetricsExporter(true)

			By("Verify new daemonset sriov-network-metrics-exporter is created and ready")
			Eventually(func() bool {
				sriovmetricsdaemonset, err = daemonset.Pull(
					APIClient, "sriov-network-metrics-exporter", NetConfig.SriovOperatorNamespace)

				return err == nil
			}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "Daemonset sriov-network-metrics-exporter is not created")
			Expect(sriovmetricsdaemonset.IsReady(2*time.Minute)).Should(BeTrue(),
				"Daemonset sriov-network-metrics-exporter is not ready")

			By("Enable Prometheus scraping for the new Sriov Metrics Exporter by labeling operator namespace")
			sriovNs, err := namespace.Pull(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch Sriov Namespace")
			_, err = sriovNs.WithMultipleLabels(netparam.ClusterMonitoringNSLabel).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to update Sriov Namespace")
		})

		AfterEach(func() {
			By("Removing SR-IOV configuration")
			err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Cleaning test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		AfterAll(func() {
			By("Disable Sriov Metrics Exporter feature in default SriovOperatorConfig CR")
			setMetricsExporter(false)
			Eventually(func() bool { return sriovmetricsdaemonset.Exists() }, 1*time.Minute, 1*time.Second).Should(BeFalse(),
				"sriov-metrics-exporter is not deleted yet")

			By("Remove cluster monitoring label for operator namespace to disable Prometheus scraping")
			sriovNs, err := namespace.Pull(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch Sriov Namespace")

			_, err = sriovNs.RemoveLabels(netparam.ClusterMonitoringNSLabel).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove cluster-monitoring label from Sriov Namespace")
		})

		Context("Netdevice to Netdevice", func() {
			It("Same PF", reportxml.ID("74762"), func() {
				runNettoNetTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different PF", reportxml.ID("75929"), func() {
				runNettoNetTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[1],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different Worker", reportxml.ID("75930"), func() {
				runNettoNetTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[1].Object.Name, sriovVendorID)
			})
		})

		Context("Netdevice to Vfiopci", func() {
			BeforeAll(func() {
				By("Deploying PerformanceProfile if it's not installed")
				err := netenv.DeployPerformanceProfile(
					APIClient,
					NetConfig,
					"performance-profile-dpdk",
					"1,3,5,7,9,11,13,15,17,19,21,23,25",
					"0,2,4,6,8,10,12,14,16,18,20",
					24)
				Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")
			})
			BeforeEach(func() {
				By("Clear MAC Table entry on switch for the test mac address")
				clearClientServerMacTableFromSwitch()
			})
			It("Same PF", reportxml.ID("74797"), func() {
				runNettoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different PF", reportxml.ID("75931"), func() {
				runNettoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[1],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different Worker", reportxml.ID("75932"), func() {
				runNettoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[1].Object.Name, sriovVendorID)
			})
		})

		Context("Vfiopci to Vfiopci", func() {
			BeforeAll(func() {
				By("Deploying PerformanceProfile if it's not installed")
				err := netenv.DeployPerformanceProfile(
					APIClient,
					NetConfig,
					"performance-profile-dpdk",
					"1,3,5,7,9,11,13,15,17,19,21,23,25",
					"0,2,4,6,8,10,12,14,16,18,20",
					24)
				Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")
			})
			BeforeEach(func() {
				By("Clear MAC Table entries of test mac addresses from Switch")
				clearClientServerMacTableFromSwitch()

			})
			It("Same PF", reportxml.ID("74800"), func() {
				runVfiotoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different PF", reportxml.ID("75933"), func() {
				runVfiotoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[1],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different Worker", reportxml.ID("75934"), func() {
				runVfiotoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[1].Object.Name, sriovVendorID)
			})
		})

	})

func runNettoNetTests(clientPf, serverPf, clientWorker, serverWorker, devID string) {
	By("Define and Create SriovNodePolicy, SriovNetwork and Pod Resources")

	clientResources := defineTestResources("client",
		clientPf, devID, "netdevice",
		clientWorker, 0, false)
	serverResources := defineTestResources("server",
		serverPf, devID, "netdevice",
		serverWorker, 1, false)

	cPod, _ := createTestResources(clientResources, serverResources)

	By("ICMP check between client and server pods")
	Eventually(func() error {
		return cmd.ICMPConnectivityCheck(cPod, []string{tsparams.ServerIPv4IPAddress}, "net1")
	}, 1*time.Minute, 2*time.Second).Should(Not(HaveOccurred()), "ICMP Failed")

	checkMetricsWithPromQL()
}

func runNettoVfioTests(clientPf, serverPf, clientWorker, serverWorker, devID string) {
	By("Define and Create SriovNodePolicy, SriovNetwork and Pod Resources")

	clientResources := defineTestResources("client",
		clientPf, devID, "netdevice",
		clientWorker, 0, false)
	serverResources := defineTestResources("server",
		serverPf, devID, "vfiopci",
		serverWorker, 1, true)

	cPod, _ := createTestResources(clientResources, serverResources)

	By("update ARP table to add server mac address in client pod")

	outputbuf, err := cPod.ExecCommand([]string{"bash", "-c", fmt.Sprintf("arp -s %s %s",
		strings.Split(tsparams.ServerIPv4IPAddress, "/")[0], tsparams.ServerMacAddress)})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to add server mac address in client pod mac table. Output: %s", outputbuf.String()))

	By("ICMP check between client and server pods")
	Eventually(func() error {
		return cmd.ICMPConnectivityCheck(cPod, []string{tsparams.ServerIPv4IPAddress}, "net1")
	}, 1*time.Minute, 2*time.Second).Should(HaveOccurred(), "ICMP fail scenario could not be executed")

	checkMetricsWithPromQL()
}

func runVfiotoVfioTests(clientPf, serverPf, clientWorker, serverWorker, devID string) {
	By("Define and Create SriovNodePolicy, SriovNetwork and Pod Resources")

	clientResources := defineTestResources("client",
		clientPf, devID, "vfiopci",
		clientWorker, 0, true)
	serverResources := defineTestResources("server",
		serverPf, devID, "vfiopci",
		serverWorker, 1, true)

	_, _ = createTestResources(clientResources, serverResources)

	checkMetricsWithPromQL()
}

func defineTestResources(role, pfName, nicVendor, deviceType, workerNode string, vfRange int, dpdk bool) testResource {
	var podBuilder *pod.Builder

	sriovPolicy := definePolicy(role, deviceType, nicVendor, pfName, vfRange)

	sriovNetwork := defineNetwork(role, deviceType)

	if dpdk {
		podBuilder = defineDPDKPod(role, deviceType, workerNode)
	} else {
		podBuilder = sriovenv.DefinePod(role+"pod", role, role+deviceType, workerNode, true)
	}

	return testResource{sriovPolicy, sriovNetwork, podBuilder}
}

func definePolicy(role, devType, nicVendor, pfName string, vfRange int) *sriov.PolicyBuilder {
	var policy *sriov.PolicyBuilder

	switch devType {
	case "netdevice":
		policy = sriov.NewPolicyBuilder(APIClient,
			role+devType, NetConfig.SriovOperatorNamespace, role+devType, 6, []string{pfName}, NetConfig.WorkerLabelMap).
			WithDevType("netdevice").
			WithVFRange(vfRange, vfRange)
	case "vfiopci":
		if nicVendor != netparam.MlxVendorID {
			policy = sriov.NewPolicyBuilder(APIClient,
				role+devType, NetConfig.SriovOperatorNamespace, role+devType, 6, []string{pfName}, NetConfig.WorkerLabelMap).
				WithDevType("vfio-pci").
				WithVFRange(vfRange, vfRange).
				WithRDMA(false)
		} else {
			policy = sriov.NewPolicyBuilder(APIClient,
				role+devType, NetConfig.SriovOperatorNamespace, role+devType, 6, []string{pfName}, NetConfig.WorkerLabelMap).
				WithDevType("netdevice").
				WithVFRange(vfRange, vfRange).
				WithRDMA(true)
		}
	}

	return policy
}

func defineNetwork(role, devType string) *sriov.NetworkBuilder {
	network := sriov.NewNetworkBuilder(
		APIClient, role+devType, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName, role+devType).
		WithMacAddressSupport().
		WithIPAddressSupport().
		WithStaticIpam().
		WithLogLevel(netparam.LogLevelDebug)

	return network
}

func defineDPDKPod(role, devType, worker string) *pod.Builder {
	var (
		rootUser      int64
		testpmdCmd    []string
		netAnnotation []*types.NetworkSelectionElement
	)

	securityContext := corev1.SecurityContext{
		RunAsUser: &rootUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"},
		},
	}

	switch role {
	case "client":
		netAnnotation = []*types.NetworkSelectionElement{
			{
				Name:       role + devType,
				MacRequest: tsparams.ClientMacAddress,
				IPRequest:  []string{tsparams.ClientIPv4IPAddress},
			},
		}
		testpmdCmd = []string{"bash", "-c", fmt.Sprintf("testpmd -a ${PCIDEVICE_OPENSHIFT_IO_%s} --iova-mode=va -- "+
			"--portmask=0x1 --nb-cores=2 --forward-mode=txonly --port-topology=loop --no-mlockall "+
			"--stats-period 5 --eth-peer=0,%s", strings.ToUpper(role+devType), tsparams.ServerMacAddress)}
	case "server":
		netAnnotation = []*types.NetworkSelectionElement{
			{
				Name:       role + devType,
				MacRequest: tsparams.ServerMacAddress,
				IPRequest:  []string{tsparams.ServerIPv4IPAddress},
			},
		}
		testpmdCmd = []string{"bash", "-c", fmt.Sprintf("testpmd -a ${PCIDEVICE_OPENSHIFT_IO_%s} --iova-mode=va -- "+
			"--portmask=0x1 --nb-cores=2 --forward-mode=macswap --port-topology=loop --no-mlockall "+
			"--stats-period 5", strings.ToUpper(role+devType))}
	}

	dpdkContainer, err := pod.NewContainerBuilder("testpmd", NetConfig.DpdkTestContainer, testpmdCmd).
		WithSecurityContext(&securityContext).
		WithResourceLimit("1Gi", "1Gi", 4).
		WithResourceRequest("1Gi", "1Gi", 4).
		WithEnvVar("RUN_TYPE", "testcmd").
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to Get Container Builder Configuration")

	dpdkPod := pod.NewBuilder(APIClient, role+"pod", tsparams.TestNamespaceName, NetConfig.DpdkTestContainer).
		RedefineDefaultContainer(*dpdkContainer).
		WithHugePages().
		WithPrivilegedFlag().
		DefineOnNode(worker).
		WithSecondaryNetwork(netAnnotation)

	return dpdkPod
}

func createTestResources(cRes, sRes testResource) (*pod.Builder, *pod.Builder) {
	for _, res := range []testResource{cRes, sRes} {
		By("Create SriovNetworkNodePolicy")

		_, err := res.policy.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to Create SriovNetworkNodePolicy %s", res.policy.Definition.Name))

		By("Create SriovNetwork")

		_, err = res.network.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to Create SriovNetwork")
	}

	err := netenv.WaitForSriovAndMCPStable(APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
		NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)

	By("Wait for NAD Creation")

	for _, res := range []testResource{cRes, sRes} {
		Eventually(func() error {
			_, err = nad.Pull(APIClient, res.network.Object.Name, tsparams.TestNamespaceName)

			return err
		}, 10*time.Second, 1*time.Second).Should(BeNil(), "Failed to pull NAD created by SriovNetwork")
	}

	By(fmt.Sprintf("Creating %s Pod", cRes.pod.Definition.Name))

	cPod, err := cRes.pod.CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Pod")

	By(fmt.Sprintf("Creating %s Pod", sRes.pod.Definition.Name))

	sPod, err := sRes.pod.CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Pod")

	return cPod, sPod
}

func checkMetricsWithPromQL() {
	By("Wait until promQL gives serverpod metrics")
	Eventually(func() bool {
		return strings.Contains(execPromQLandReturnString(serverPodRXPromQL), "serverpod")
	},
		130*time.Second, 30*time.Second).Should(BeTrue(), "PromQL output does not contain server pod metrics")

	By("Verify RX and TX packets counters are > 0")
	Eventually(func() int { return fetchScalarFromPromQLoutput(execPromQLandReturnString(serverPodRXPromQL)) },
		2*time.Minute, 30*time.Second).Should(BeNumerically(">", 0), "RX counters are zero")
	Eventually(func() int { return fetchScalarFromPromQLoutput(execPromQLandReturnString(serverPodTXPromQL)) },
		2*time.Minute, 30*time.Second).Should(BeNumerically(">", 0), "TX counters are zero")
}

func execPromQLandReturnString(query []string) string {
	By("Running PromQL to fetch metrics of serverpod")

	promPods, err := pod.List(APIClient,
		NetConfig.PrometheusOperatorNamespace, metav1.ListOptions{LabelSelector: "prometheus=k8s"})
	Expect(err).ToNot(HaveOccurred(), "Failed to get prometheus pods")

	By(fmt.Sprintf("Running PromQL query: %s", query))
	output, err := promPods[0].ExecCommand(query, "prometheus")
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to get promQL output from prometheus pod. Output: %s", output.String()))

	By(fmt.Sprintf("Received PromQL output: %s", output.String()))

	return output.String()
}

func fetchScalarFromPromQLoutput(res string) int {
	type podMetricPromQLoutput []struct {
		Metric struct {
			Pod string `json:"pod,omitempty"`
		}
		Value []interface{} `json:"value,omitempty"`
	}

	By("Fetch the final value from the PromQL output")

	var outValue podMetricPromQLoutput

	err := json.Unmarshal([]byte(res), &outValue)
	Expect(err).ToNot(HaveOccurred(), "Failed to Unmarshal promQL output from prometheus pod")

	//nolint:forcetypeassert
	finalVal, err := strconv.Atoi(outValue[0].Value[1].(string))
	Expect(err).To(Not(HaveOccurred()), "Failed to convert counter value to int")

	return finalVal
}

func setMetricsExporter(flag bool) {
	defaultOperatorConfig, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch default Sriov Operator Config")

	if defaultOperatorConfig.Definition.Spec.FeatureGates == nil {
		defaultOperatorConfig.Definition.Spec.FeatureGates = map[string]bool{"metricsExporter": flag}
	} else {
		defaultOperatorConfig.Definition.Spec.FeatureGates["metricsExporter"] = flag
	}

	_, err = defaultOperatorConfig.Update()
	Expect(err).ToNot(HaveOccurred(), "Failed to update metricsExporter flag in default Sriov Operator Config")
}

func clearClientServerMacTableFromSwitch() {
	switchCredentials, err := sriovenv.NewSwitchCredentials()
	Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

	jnpr, err := cmd.NewSession(switchCredentials.SwitchIP, switchCredentials.User, switchCredentials.Password)
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch Switch Credentials")

	defer jnpr.Close()

	_, err = jnpr.RunCommand(fmt.Sprintf("clear ethernet-switching table %s", tsparams.ServerMacAddress))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to clear mac table for %s", tsparams.ServerMacAddress))

	_, err = jnpr.RunCommand(fmt.Sprintf("clear ethernet-switching table %s", tsparams.ClientMacAddress))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to clear mac table for %s", tsparams.ClientMacAddress))
}
