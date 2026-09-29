package tests

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var (
	vfZeroLineRegexp = regexp.MustCompile(`(?m)^\s*vf 0\b.*$`)
	vlanRegexp       = regexp.MustCompile(`\bvlan (\d+)`)
	maxTxRateRegexp  = regexp.MustCompile(`\bmax_tx_rate (\d+)`)
)

const (
	ipv4Family   = "IPv4"
	ipv6Family   = "IPv6"
	dualIPFamily = "dual"
)

var _ = Describe("ExternallyManaged", Ordered, Label(tsparams.LabelExternallyManagedTestCases),
	ContinueOnFailure, func() {
		Context("General", Label("generalexcreated"), func() {
			var (
				sriovInterfacesUnderTest []string
				workerNodeList           []*nodes.Builder
				vfNum                    int
			)

			BeforeAll(func() {
				By("Verifying if SR-IOV tests can be executed on given cluster")

				err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 1)
				if err != nil {
					Skip(fmt.Sprintf(
						"given cluster is not suitable for SR-IOV tests because it doesn't have enough nodes: %s", err.Error()))
				}

				vfNum, err = SriovOcpConfig.GetVFNum()
				Expect(err).ToNot(HaveOccurred(), "Failed to get VF number")

				By("Validating SR-IOV interfaces")

				workerNodeList, err = nodes.List(APIClient,
					metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
				Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

				Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
					"Failed to get required SR-IOV interfaces")

				sriovInterfacesUnderTest, err = SriovOcpConfig.GetSriovInterfaces(1)
				Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

				isMellanox, err := sriovocpenv.IsMellanoxDevice(
					sriovInterfacesUnderTest[0], workerNodeList[0].Object.Name,
				)
				Expect(err).ToNot(HaveOccurred(), "Failed to check if interface is a Mellanox device")

				if isMellanox {
					err = sriovocpenv.ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP(
						tsparams.MCOWaitTimeout,
						time.Minute,
						workerNodeList,
						sriovInterfacesUnderTest[0],
						true,
						vfNum,
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to configure Mellanox firmware")
				}

				By("Creating SR-IOV VFs on workers")

				err = configureVFsOnWorkersAndWait(
					workerNodeList,
					sriovInterfacesUnderTest[0],
					vfNum)
				Expect(err).ToNot(HaveOccurred(), "Failed to create VFs on workers")

				By("Configure SR-IOV with flag ExternallyManaged true")
				createExternallyManagedSriovConfiguration(
					tsparams.SriovResourceNameExManagedTrue, sriovInterfacesUnderTest[0], vfNum)
			})

			AfterAll(func() {
				if len(workerNodeList) == 0 || len(sriovInterfacesUnderTest) == 0 {
					return
				}

				By("Removing SR-IOV configuration")

				err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
					APIClient,
					SriovOcpConfig.WorkerLabelEnvVar,
					SriovOcpConfig.OcpSriovOperatorNamespace,
					tsparams.MCOWaitTimeout,
					tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

				By("Verifying that VFs still exist")

				err = sriovocpenv.WaitUntilVfsCreated(
					workerNodeList,
					sriovInterfacesUnderTest[0], vfNum, tsparams.DefaultTimeout,
				)
				Expect(err).ToNot(HaveOccurred(), "Unexpected amount of VF")

				err = sriovocpenv.AreVFsCreated(workerNodeList[0].Object.Name, sriovInterfacesUnderTest[0], vfNum)
				Expect(err).ToNot(HaveOccurred(), "VFs were removed during the test")

				By("Removing SR-IOV VFs on workers")

				err = configureVFsOnWorkersAndWait(
					workerNodeList,
					sriovInterfacesUnderTest[0],
					0)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove VFs on workers")
			})

			AfterEach(func() {
				By("Cleaning test namespace")

				err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					tsparams.DefaultTimeout, pod.GetGVR())

				Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
			})

			DescribeTable("Verifying connectivity with different IP protocols", reportxml.ID("63527"),
				func(ipStack string) {
					By("Verifying cluster has enough workers")

					err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 2)
					if err != nil {
						Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough workers: %v", err))
					}

					By("Validating SR-IOV interfaces on 2 workers")
					Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
						"Failed to get required SR-IOV interfaces on 2 workers")

					_, err = SriovOcpConfig.GetSriovInterfaces(2)
					Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

					By("Defining test parameters")

					clientIPs, serverIPs, err := defineExternallyManagedIterationParams(ipStack)
					Expect(err).ToNot(HaveOccurred(), "Failed to define test parameters")

					By("Creating test pods and checking connectivity")

					err = sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[1].Object.Name,
						tsparams.SriovResourceNameExManagedTrue, tsparams.SriovResourceNameExManagedTrue, "", "",
						clientIPs, serverIPs)
					Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
				},

				Entry("", ipv4Family, reportxml.SetProperty("IPStack", ipv4Family)),
				Entry("", ipv6Family, reportxml.SetProperty("IPStack", ipv6Family)),
				Entry("", dualIPFamily, reportxml.SetProperty("IPStack", dualIPFamily)),
			)

			It("Recreate VFs when SR-IOV policy is applied", reportxml.ID("63533"), func() {
				By("Creating test pods and checking connectivity")

				err := sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					tsparams.SriovResourceNameExManagedTrue, tsparams.SriovResourceNameExManagedTrue,
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")

				By("Removing created SR-IOV VFs on workers")

				err = configureVFsOnWorkersAndWait(
					workerNodeList,
					sriovInterfacesUnderTest[0],
					0)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove VFs on workers")

				By("Removing all test pods")

				err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					tsparams.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean all test pods")

				By("Creating SR-IOV VFs again on workers")

				err = configureVFsOnWorkersAndWait(
					workerNodeList,
					sriovInterfacesUnderTest[0],
					vfNum)
				Expect(err).ToNot(HaveOccurred(), "Failed to recreate VFs on workers")

				By("Re-create test pods and verify connectivity after recreating the VFs")

				err = sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					tsparams.SriovResourceNameExManagedTrue, tsparams.SriovResourceNameExManagedTrue,
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})

			It("SR-IOV network with options", reportxml.ID("63534"), func() {
				By("Collecting default MaxTxRate and Vlan values")

				defaultMaxTxRate, defaultVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
					sriovInterfacesUnderTest[0])

				By("Updating Vlan and MaxTxRate configurations in the SriovNetwork")

				newMaxTxRate := defaultMaxTxRate + 1
				newVlanID := defaultVlanID + 1
				sriovNetwork, err := sriov.PullNetwork(APIClient, tsparams.SriovResourceNameExManagedTrue,
					SriovOcpConfig.OcpSriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV network object")
				_, err = sriovNetwork.WithMaxTxRate(uint16(newMaxTxRate)).WithVLAN(uint16(newVlanID)).Update(false)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SR-IOV network with new configuration")

				By("Creating test pods and checking connectivity")

				err = sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					tsparams.SriovResourceNameExManagedTrue, tsparams.SriovResourceNameExManagedTrue,
					tsparams.ClientMacAddress, tsparams.ServerMacAddress,
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")

				By("Checking that VF configured with new VLAN and MaxTxRate values")
				Eventually(func() []int {
					currentmaxTxRate, currentVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
						sriovInterfacesUnderTest[0])

					return []int{currentmaxTxRate, currentVlanID}
				}, time.Minute, tsparams.RetryInterval).Should(Equal([]int{newMaxTxRate, newVlanID}),
					"MaxTxRate and VlanId have been not configured properly")

				By("Removing all test pods")

				err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					tsparams.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean all test pods")

				By("Checking that VF has initial configuration")

				Eventually(func() []int {
					currentmaxTxRate, currentVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
						sriovInterfacesUnderTest[0])

					return []int{currentmaxTxRate, currentVlanID}
				}, tsparams.DefaultTimeout, tsparams.RetryInterval).
					Should(Equal([]int{defaultMaxTxRate, defaultVlanID}),
						"MaxTxRate and VlanId configuration have not been reverted to the initial one")

				By("Removing SR-IOV configuration")

				err = sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
					APIClient,
					SriovOcpConfig.WorkerLabelEnvVar,
					SriovOcpConfig.OcpSriovOperatorNamespace,
					tsparams.MCOWaitTimeout,
					tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

				By("Checking that VF has initial configuration")
				Eventually(func() []int {
					currentmaxTxRate, currentVlanID := getVlanIDAndMaxTxRateForVf(workerNodeList[0].Object.Name,
						sriovInterfacesUnderTest[0])

					return []int{currentmaxTxRate, currentVlanID}
				}, time.Minute, tsparams.RetryInterval).Should(Equal([]int{defaultMaxTxRate, defaultVlanID}),
					"MaxTxRate and VlanId configurations have not been reverted to the initial one")

				By("Configure SR-IOV with flag ExternallyManaged true")
				createExternallyManagedSriovConfiguration(
					tsparams.SriovResourceNameExManagedTrue, sriovInterfacesUnderTest[0], vfNum)
			})

			It("SR-IOV operator removal", reportxml.ID("63537"), func() {
				By("Creating test pods and checking connectivity")

				err := sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					tsparams.SriovResourceNameExManagedTrue, tsparams.SriovResourceNameExManagedTrue, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")

				By("Collecting info about installed SR-IOV operator")

				sriovNamespace, sriovOperatorgroup, sriovSubscription := collectingInfoSriovOperator()

				By("Removing SR-IOV operator")
				removeSriovOperator(sriovNamespace)
				Expect(
					sriovoperator.IsSriovDeployed(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)).To(HaveOccurred(),
					"SR-IOV operator is not removed")

				By("Installing SR-IOV operator")
				installSriovOperator(sriovNamespace, sriovOperatorgroup, sriovSubscription)
				Eventually(func() error {
					return sriovoperator.IsSriovDeployed(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
				}, time.Minute, tsparams.RetryInterval).
					ShouldNot(HaveOccurred(), "SR-IOV operator is not installed")

				By("Verifying that VFs still exist after SR-IOV operator reinstallation")

				err = sriovocpenv.AreVFsCreated(workerNodeList[0].Object.Name, sriovInterfacesUnderTest[0], vfNum)
				Expect(err).ToNot(HaveOccurred(), "VFs were removed after SR-IOV operator reinstallation")

				By("Configure SR-IOV with flag ExternallyManaged true")
				createExternallyManagedSriovConfiguration(
					tsparams.SriovResourceNameExManagedTrue, sriovInterfacesUnderTest[0], vfNum)

				By("Recreating test pods and checking connectivity")

				err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					tsparams.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to remove test pods")

				err = sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					tsparams.SriovResourceNameExManagedTrue, tsparams.SriovResourceNameExManagedTrue, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})
		})
	})

func createExternallyManagedSriovConfiguration(sriovAndResName, sriovInterfaceName string, numVfs int) {
	By("Creating SR-IOV policy with flag ExternallyManaged true")

	err := createSriovPolicyWithExternallyManaged(sriovAndResName, numVfs, []string{sriovInterfaceName + "#0-1"})
	Expect(err).ToNot(HaveOccurred(), "Failed to create sriov policy")

	err = sriovenv.CreateSriovNetwork(sriovAndResName, sriovAndResName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to create sriov network")
}

func createSriovPolicyWithExternallyManaged(sriovAndResName string, numVfs int, pfDevices []string) error {
	sriovPolicy := sriov.NewPolicyBuilder(APIClient, sriovAndResName, SriovOcpConfig.OcpSriovOperatorNamespace,
		sriovAndResName,
		numVfs, pfDevices, SriovOcpConfig.WorkerLabelMap).WithExternallyManaged(true)

	err := sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
		APIClient,
		SriovOcpConfig.WorkerLabelEnvVar,
		SriovOcpConfig.OcpSriovOperatorNamespace,
		sriovPolicy,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration)
	if err != nil {
		return fmt.Errorf("failed to create sriov policy: %w", err)
	}

	return nil
}

func configureVFsOnWorkersAndWait(
	workerNodes []*nodes.Builder,
	sriovInterfaceName string,
	numVfs int,
) error {
	err := setSriovNumVfsOnWorkers(workerNodes, sriovInterfaceName, numVfs)
	if err != nil {
		return err
	}

	return sriovocpenv.WaitUntilVfsCreated(workerNodes, sriovInterfaceName, numVfs, tsparams.DefaultTimeout)
}

func setSriovNumVfsOnWorkers(workerNodes []*nodes.Builder, sriovInterfaceName string, numVfs int) error {
	for _, workerNode := range workerNodes {
		cmd := fmt.Sprintf("echo %d > /host/sys/class/net/%s/device/sriov_numvfs",
			numVfs, sriovInterfaceName)
		if numVfs > 0 {
			cmd = fmt.Sprintf(
				"echo 0 > /host/sys/class/net/%s/device/sriov_numvfs; echo %d > /host/sys/class/net/%s/device/sriov_numvfs",
				sriovInterfaceName, numVfs, sriovInterfaceName)
		}

		if _, err := execOnSriovConfigDaemon(workerNode.Object.Name, cmd); err != nil {
			return err
		}
	}

	return nil
}

func execOnSriovConfigDaemon(nodeName, command string) (string, error) {
	pods, err := pod.List(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace, metav1.ListOptions{
		LabelSelector: "app=sriov-network-config-daemon",
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list config daemon pods on node %s: %w", nodeName, err)
	}

	if len(pods) == 0 {
		return "", fmt.Errorf("failed to find config daemon pod on node %s", nodeName)
	}

	output, err := pods[0].ExecCommand([]string{"bash", "-c", command})
	if err != nil {
		return "", fmt.Errorf("failed to execute command on node %s: %s %w",
			nodeName, output.String(), err)
	}

	return output.String(), nil
}

func getVlanIDAndMaxTxRateForVf(nodeName, sriovInterfaceName string) (maxTxRate, vlanID int) {
	output, err := execOnSriovConfigDaemon(nodeName, fmt.Sprintf("ip link show %s", sriovInterfaceName))
	Expect(err).ToNot(HaveOccurred(), "Failed to get VLAN and MaxTxRate for VF")

	vfLine := vfZeroLineRegexp.FindString(output)
	Expect(vfLine).ToNot(BeEmpty(), "vf 0 not found on interface %s node %s", sriovInterfaceName, nodeName)

	return parseIntFromRegexp(maxTxRateRegexp, vfLine), parseIntFromRegexp(vlanRegexp, vfLine)
}

func parseIntFromRegexp(re *regexp.Regexp, input string) int {
	match := re.FindStringSubmatch(input)
	if len(match) < 2 {
		return 0
	}

	value, err := strconv.Atoi(strings.TrimSpace(match[1]))
	if err != nil {
		return 0
	}

	return value
}

func defineExternallyManagedIterationParams(ipFamily string) (clientIPs, serverIPs []string, err error) {
	switch ipFamily {
	case ipv4Family:
		return []string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress}, nil
	case ipv6Family:
		return []string{tsparams.ClientIPv6IPAddress}, []string{tsparams.ServerIPv6IPAddress}, nil
	case dualIPFamily:
		return []string{tsparams.ClientIPv4IPAddress, tsparams.ClientIPv6IPAddress},
			[]string{tsparams.ServerIPv4IPAddress, tsparams.ServerIPv6IPAddress}, nil
	}

	return nil, nil, fmt.Errorf(
		"ipStack parameter %s is invalid; allowed values are %s, %s, %s ",
		ipFamily, ipv4Family, ipv6Family, dualIPFamily)
}
