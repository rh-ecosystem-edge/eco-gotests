package tests

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/perfprofile"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type metricsTestResource struct {
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

			err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 1)
			if err != nil {
				Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
			}

			By("Validating SR-IOV interfaces")

			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = SriovOcpConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Fetching SR-IOV Vendor ID for interface under test")

			sriovVendorID, err = sriovoperator.DiscoverInterfaceUnderTestVendorID(
				APIClient, SriovOcpConfig.OcpSriovOperatorNamespace,
				sriovInterfacesUnderTest[0], workerNodeList[0].Definition.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch SR-IOV Vendor ID for interface under test")

			By("Enable Sriov Metrics Exporter feature in default SriovOperatorConfig CR")
			setMetricsExporterFlag(true)

			By("Verify new daemonset sriov-network-metrics-exporter is created and ready")
			Eventually(func() bool {
				sriovmetricsdaemonset, err = daemonset.Pull(
					APIClient, "sriov-network-metrics-exporter", SriovOcpConfig.OcpSriovOperatorNamespace)

				return err == nil
			}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "Daemonset sriov-network-metrics-exporter is not created")
			Expect(sriovmetricsdaemonset.IsReady(2*time.Minute)).Should(BeTrue(),
				"Daemonset sriov-network-metrics-exporter is not ready")

			By("Enable Prometheus scraping for the new Sriov Metrics Exporter by labeling operator namespace")

			sriovNs, err := namespace.Pull(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch Sriov Namespace")
			_, err = sriovNs.WithMultipleLabels(tsparams.ClusterMonitoringNSLabel).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to update Sriov Namespace")
		})

		AfterEach(func() {
			By("Removing SR-IOV configuration")

			err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				SriovOcpConfig.WorkerLabelEnvVar,
				SriovOcpConfig.OcpSriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Cleaning test namespace")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				tsparams.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
		})

		AfterAll(func() {
			By("Disable Sriov Metrics Exporter feature in default SriovOperatorConfig CR")
			setMetricsExporterFlag(false)
			Eventually(func() bool { return sriovmetricsdaemonset.Exists() }, 1*time.Minute, 1*time.Second).Should(BeFalse(),
				"sriov-metrics-exporter is not deleted yet")

			By("Remove cluster monitoring label for operator namespace to disable Prometheus scraping")

			sriovNs, err := namespace.Pull(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch Sriov Namespace")

			_, err = sriovNs.RemoveLabels(tsparams.ClusterMonitoringNSLabel).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove cluster-monitoring label from Sriov Namespace")
		})

		Context("Netdevice to Netdevice", func() {
			It("Same PF", reportxml.ID("74762"), func() {
				runMetricsNettoNetTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different PF", reportxml.ID("75929"), func() {
				By("Verifying we have 2 SR-IOV interfaces available")
				Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces")

				interfaces, err := SriovOcpConfig.GetSriovInterfaces(2)
				Expect(err).ToNot(HaveOccurred(), "Failed to retrieve 2 SR-IOV interfaces for testing")
				runMetricsNettoNetTests(interfaces[0], interfaces[1],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different Worker", reportxml.ID("75930"), func() {
				By("Verifying cluster has enough workers")

				err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 2)
				if err != nil {
					Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough workers: %v", err))
				}

				By("Validating SR-IOV interfaces on 2 workers")
				Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces on 2 workers")
				runMetricsNettoNetTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[1].Object.Name, sriovVendorID)
			})
		})

		Context("Netdevice to Vfiopci", func() {
			BeforeAll(func() {
				By("Deploying PerformanceProfile if it's not installed")

				err := perfprofile.DeployPerformanceProfile(
					APIClient,
					SriovOcpConfig.WorkerLabelMap,
					SriovOcpConfig.MCPLabel,
					"performance-profile-dpdk",
					"1,3,5,7,9,11,13,15,17,19,21,23,25",
					"0,2,4,6,8,10,12,14,16,18,20",
					24,
					tsparams.MCOWaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")
			})
			BeforeEach(func() {
				By("Clear MAC Table entry on switch for the test mac address")
				clearClientServerMacTableFromSwitch()
			})
			It("Same PF", reportxml.ID("74797"), func() {
				runMetricsNettoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different PF", reportxml.ID("75931"), func() {
				By("Verifying we have 2 SR-IOV interfaces available")
				Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces")

				interfaces, err := SriovOcpConfig.GetSriovInterfaces(2)
				Expect(err).ToNot(HaveOccurred(), "Failed to retrieve 2 SR-IOV interfaces for testing")
				runMetricsNettoVfioTests(interfaces[0], interfaces[1],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different Worker", reportxml.ID("75932"), func() {
				By("Verifying cluster has enough workers")

				err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 2)
				if err != nil {
					Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough workers: %v", err))
				}

				By("Validating SR-IOV interfaces on 2 workers")
				Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces on 2 workers")
				runMetricsNettoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[1].Object.Name, sriovVendorID)
			})
		})

		Context("Vfiopci to Vfiopci", func() {
			BeforeAll(func() {
				By("Deploying PerformanceProfile if it's not installed")

				err := perfprofile.DeployPerformanceProfile(
					APIClient,
					SriovOcpConfig.WorkerLabelMap,
					SriovOcpConfig.MCPLabel,
					"performance-profile-dpdk",
					"1,3,5,7,9,11,13,15,17,19,21,23,25",
					"0,2,4,6,8,10,12,14,16,18,20",
					24,
					tsparams.MCOWaitTimeout)
				Expect(err).ToNot(HaveOccurred(), "Fail to deploy PerformanceProfile")
			})
			BeforeEach(func() {
				By("Clear MAC Table entries of test mac addresses from Switch")
				clearClientServerMacTableFromSwitch()
			})
			It("Same PF", reportxml.ID("74800"), func() {
				runMetricsVfiotoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different PF", reportxml.ID("75933"), func() {
				By("Verifying we have 2 SR-IOV interfaces available")
				Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces")

				interfaces, err := SriovOcpConfig.GetSriovInterfaces(2)
				Expect(err).ToNot(HaveOccurred(), "Failed to retrieve 2 SR-IOV interfaces for testing")
				runMetricsVfiotoVfioTests(interfaces[0], interfaces[1],
					workerNodeList[0].Object.Name, workerNodeList[0].Object.Name, sriovVendorID)
			})
			It("Different Worker", reportxml.ID("75934"), func() {
				By("Verifying cluster has enough workers")

				err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 2)
				if err != nil {
					Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough workers: %v", err))
				}

				By("Validating SR-IOV interfaces on 2 workers")
				Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces on 2 workers")
				runMetricsVfiotoVfioTests(sriovInterfacesUnderTest[0], sriovInterfacesUnderTest[0],
					workerNodeList[0].Object.Name, workerNodeList[1].Object.Name, sriovVendorID)
			})
		})
	})

func runMetricsNettoNetTests(clientPf, serverPf, clientWorker, serverWorker, devID string) {
	By("Define and Create SriovNodePolicy, SriovNetwork and Pod Resources")

	clientResources := defineMetricsTestResources("client",
		clientPf, devID, "netdevice",
		clientWorker, 0, false)
	serverResources := defineMetricsTestResources("server",
		serverPf, devID, "netdevice",
		serverWorker, 1, false)

	cPod := createMetricsTestResources(clientResources, serverResources)

	By("ICMP check between client and server pods")
	Eventually(func() error {
		return sriovocpenv.ICMPConnectivityCheck(cPod, []string{tsparams.ServerIPv4IPAddress}, "net1")
	}, 1*time.Minute, 2*time.Second).Should(Not(HaveOccurred()), "ICMP Failed")

	checkMetricsWithPromQL()
}

func runMetricsNettoVfioTests(clientPf, serverPf, clientWorker, serverWorker, devID string) {
	By("Define and Create SriovNodePolicy, SriovNetwork and Pod Resources")

	clientResources := defineMetricsTestResources("client",
		clientPf, devID, "netdevice",
		clientWorker, 0, false)
	serverResources := defineMetricsTestResources("server",
		serverPf, devID, "vfiopci",
		serverWorker, 1, true)

	cPod := createMetricsTestResources(clientResources, serverResources)

	By("update ARP table to add server mac address in client pod")

	outputbuf, err := cPod.ExecCommand([]string{"bash", "-c", fmt.Sprintf("arp -s %s %s",
		strings.Split(tsparams.ServerIPv4IPAddress, "/")[0], tsparams.TestPodServerMAC)})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to add server mac address in client pod mac table. Output: %s", outputbuf.String()))

	By("ICMP check between client and server pods")
	Eventually(func() error {
		return sriovocpenv.ICMPConnectivityCheck(cPod, []string{tsparams.ServerIPv4IPAddress}, "net1")
	}, 1*time.Minute, 2*time.Second).Should(HaveOccurred(), "ICMP fail scenario could not be executed")

	checkMetricsWithPromQL()
}

func runMetricsVfiotoVfioTests(clientPf, serverPf, clientWorker, serverWorker, devID string) {
	By("Define and Create SriovNodePolicy, SriovNetwork and Pod Resources")

	clientResources := defineMetricsTestResources("client",
		clientPf, devID, "vfiopci",
		clientWorker, 0, true)
	serverResources := defineMetricsTestResources("server",
		serverPf, devID, "vfiopci",
		serverWorker, 1, true)

	createMetricsTestResources(clientResources, serverResources)

	checkMetricsWithPromQL()
}

func defineMetricsTestResources(
	role, pfName, nicVendor, deviceType, workerNode string, vfRange int, dpdk bool) metricsTestResource {
	var podBuilder *pod.Builder

	sriovPolicy := defineMetricsPolicy(role, deviceType, nicVendor, pfName, vfRange)

	sriovNetwork := defineMetricsNetwork(role, deviceType)

	if dpdk {
		podBuilder = defineMetricsDPDKPod(role, deviceType, workerNode)
	} else {
		podBuilder = defineMetricsPod(role, deviceType, workerNode)
	}

	return metricsTestResource{sriovPolicy, sriovNetwork, podBuilder}
}

func defineMetricsPolicy(role, devType, nicVendor, pfName string, vfRange int) *sriov.PolicyBuilder {
	var policy *sriov.PolicyBuilder

	switch devType {
	case "netdevice":
		policy = sriov.NewPolicyBuilder(APIClient,
			role+devType, SriovOcpConfig.OcpSriovOperatorNamespace, role+devType, SriovOcpConfig.VFNum, []string{pfName},
			SriovOcpConfig.WorkerLabelMap).
			WithDevType("netdevice").
			WithVFRange(vfRange, vfRange)
	case "vfiopci":
		if nicVendor != tsparams.MlxVendorID {
			policy = sriov.NewPolicyBuilder(APIClient,
				role+devType, SriovOcpConfig.OcpSriovOperatorNamespace, role+devType, SriovOcpConfig.VFNum, []string{pfName},
				SriovOcpConfig.WorkerLabelMap).
				WithDevType("vfio-pci").
				WithVFRange(vfRange, vfRange).
				WithRDMA(false)
		} else {
			policy = sriov.NewPolicyBuilder(APIClient,
				role+devType, SriovOcpConfig.OcpSriovOperatorNamespace, role+devType, SriovOcpConfig.VFNum, []string{pfName},
				SriovOcpConfig.WorkerLabelMap).
				WithDevType("netdevice").
				WithVFRange(vfRange, vfRange).
				WithRDMA(true)
		}
	default:
		Fail(fmt.Sprintf("Invalid device type: %s", devType))
	}

	return policy
}

func defineMetricsNetwork(role, devType string) *sriov.NetworkBuilder {
	network := sriov.NewNetworkBuilder(
		APIClient, role+devType, SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName, role+devType).
		WithMacAddressSupport().
		WithIPAddressSupport().
		WithStaticIpam().
		WithLogLevel("debug")

	return network
}

func defineMetricsPod(role, devType, worker string) *pod.Builder {
	var netAnnotation []*types.NetworkSelectionElement

	if role == "server" {
		netAnnotation = []*types.NetworkSelectionElement{
			{
				Name:       role + devType,
				MacRequest: tsparams.TestPodServerMAC,
				IPRequest:  []string{tsparams.ServerIPv4IPAddress},
			},
		}
	} else {
		netAnnotation = []*types.NetworkSelectionElement{
			{
				Name:       role + devType,
				MacRequest: tsparams.TestPodClientMAC,
				IPRequest:  []string{tsparams.ClientIPv4IPAddress},
			},
		}
	}

	return pod.NewBuilder(APIClient, role+"pod", tsparams.TestNamespaceName, SriovOcpConfig.OcpSriovTestContainer).
		WithNodeSelector(map[string]string{corev1.LabelHostname: worker}).
		WithPrivilegedFlag().
		WithSecondaryNetwork(netAnnotation)
}

func defineMetricsDPDKPod(role, devType, worker string) *pod.Builder {
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
				MacRequest: tsparams.TestPodClientMAC,
				IPRequest:  []string{tsparams.ClientIPv4IPAddress},
			},
		}
		testpmdCmd = []string{"bash", "-c", fmt.Sprintf("testpmd -a ${PCIDEVICE_OPENSHIFT_IO_%s} --iova-mode=va -- "+
			"--portmask=0x1 --nb-cores=2 --forward-mode=txonly --port-topology=loop --no-mlockall "+
			"--stats-period 5 --eth-peer=0,%s", strings.ToUpper(role+devType), tsparams.TestPodServerMAC)}
	case "server":
		netAnnotation = []*types.NetworkSelectionElement{
			{
				Name:       role + devType,
				MacRequest: tsparams.TestPodServerMAC,
				IPRequest:  []string{tsparams.ServerIPv4IPAddress},
			},
		}
		testpmdCmd = []string{"bash", "-c", fmt.Sprintf("testpmd -a ${PCIDEVICE_OPENSHIFT_IO_%s} --iova-mode=va -- "+
			"--portmask=0x1 --nb-cores=2 --forward-mode=macswap --port-topology=loop --no-mlockall "+
			"--stats-period 5", strings.ToUpper(role+devType))}
	}

	dpdkContainer, err := pod.NewContainerBuilder("testpmd", SriovOcpConfig.DpdkTestContainer, testpmdCmd).
		WithSecurityContext(&securityContext).
		WithResourceLimit("1Gi", "1Gi", 4).
		WithResourceRequest("1Gi", "1Gi", 4).
		WithEnvVar("RUN_TYPE", "testcmd").
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to Get Container Builder Configuration")

	dpdkPod := pod.NewBuilder(APIClient, role+"pod", tsparams.TestNamespaceName, SriovOcpConfig.DpdkTestContainer).
		RedefineDefaultContainer(*dpdkContainer).
		WithHugePages().
		WithPrivilegedFlag().
		DefineOnNode(worker).
		WithSecondaryNetwork(netAnnotation)

	return dpdkPod
}

func createMetricsTestResources(cRes, sRes metricsTestResource) *pod.Builder {
	for _, res := range []metricsTestResource{cRes, sRes} {
		By("Create SriovNetworkNodePolicy")

		_, err := res.policy.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to Create SriovNetworkNodePolicy %s", res.policy.Definition.Name))

		By("Create SriovNetwork")

		_, err = res.network.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create SriovNetwork %s", res.network.Definition.Name))

		err = sriovenv.WaitForNADCreation(res.network.Definition.Name, tsparams.TestNamespaceName, tsparams.NADTimeout)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to wait for NAD creation for Sriov Network %s with error %v",
			res.network.Definition.Name, err)
	}

	err := sriovoperator.WaitForSriovAndMCPStable(APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
		SriovOcpConfig.MCPLabel, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable before creating test resources")

	By("Wait for NAD Creation")

	for _, res := range []metricsTestResource{cRes, sRes} {
		Eventually(func() error {
			_, err = nad.Pull(APIClient, res.network.Object.Name, tsparams.TestNamespaceName)

			return err
		}, 10*time.Second, 1*time.Second).Should(BeNil(), "Failed to pull NAD created by SriovNetwork")
	}

	By(fmt.Sprintf("Creating %s Pod", cRes.pod.Definition.Name))

	cPod, err := cRes.pod.CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Pod")

	By(fmt.Sprintf("Creating %s Pod", sRes.pod.Definition.Name))

	_, err = sRes.pod.CreateAndWaitUntilRunning(2 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Pod")

	return cPod
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
		SriovOcpConfig.PrometheusOperatorNamespace, metav1.ListOptions{LabelSelector: "prometheus=k8s"})
	Expect(err).ToNot(HaveOccurred(), "Failed to get prometheus pods")
	Expect(len(promPods)).To(BeNumerically(">", 0), "No prometheus pods found with label prometheus=k8s")

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

	Expect(len(outValue)).To(BeNumerically(">", 0), "PromQL output contains no metrics")
	Expect(len(outValue[0].Value)).To(BeNumerically(">", 1), "PromQL metric value array is incomplete")

	valueStr, ok := outValue[0].Value[1].(string)
	Expect(ok).To(BeTrue(), "PromQL metric value is not a string")

	finalVal, err := strconv.Atoi(valueStr)
	Expect(err).To(Not(HaveOccurred()), "Failed to convert counter value to int")

	return finalVal
}

func setMetricsExporterFlag(flag bool) {
	defaultOperatorConfig, err := sriov.PullOperatorConfig(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
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
	switchCredentials, err := sriovocpenv.NewSwitchCredentials()
	Expect(err).ToNot(HaveOccurred(), "Failed to get switch credentials")

	jnpr, err := sriovocpenv.NewJunosSession(
		switchCredentials.SwitchIP, switchCredentials.User, switchCredentials.Password)
	Expect(err).ToNot(HaveOccurred(), "Failed to create new Junos Session")

	defer jnpr.Close()

	_, err = jnpr.RunCommand(fmt.Sprintf("clear ethernet-switching table %s", tsparams.TestPodServerMAC))
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to clear mac table for %s", tsparams.TestPodServerMAC))

	_, err = jnpr.RunCommand(fmt.Sprintf("clear ethernet-switching table %s", tsparams.TestPodClientMAC))
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to clear mac table for %s", tsparams.TestPodClientMAC))
}
