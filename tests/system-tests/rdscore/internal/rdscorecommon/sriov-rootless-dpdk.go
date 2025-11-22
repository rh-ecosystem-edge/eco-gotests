package rdscorecommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/link"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"
	"k8s.io/apimachinery/pkg/util/wait"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
)

const (
	serverDPDKDeploymentName = "rootless-dpdk-server"
	dpdkServerMac            = "60:00:00:00:00:04"
	serverPodLabel           = "rds-app=rootless-dpdk-server"
	maxMulticastNoiseRate    = 5000
	minimumExpectedDPDKRate  = 1000000
	hugePages                = "2Gi"
	memory                   = "1Gi"
	cpu                      = 4

	tapOneInterfaceName            = "ext0"
	tapTwoInterfaceName            = "ext1"
	tapThreeInterfaceName          = "ext2"
	firstInterfaceBasedOnTapTwo    = "ext1.1"
	secondInterfaceBasedOnTapTwo   = "ext1.2"
	firstInterfaceBasedOnTapThree  = "ext2.1"
	secondInterfaceBasedOnTapThree = "ext2.2"

	dpdkTestpmdTimeout = 20 * time.Second
	clientRxCmdTimeout = dpdkTestpmdTimeout + 2*time.Second
	getLinkRxTimeout   = 3 * time.Second
)

var (
	deploymentNamespace          = RDSCoreConfig.RootlessDPDKNamespace
	dpdkVlanID                   = RDSCoreConfig.RootlessDPDKVlanID
	dummyVlanID                  = RDSCoreConfig.RootlessDPDKDummyVlanID
	dpdkDeploymentSAName         = RDSCoreConfig.RootlessDPDKDeploymentSA
	firstInterfaceBasedOnTapOne  = fmt.Sprintf("%s.%s", tapOneInterfaceName, dpdkVlanID)
	secondInterfaceBasedOnTapOne = fmt.Sprintf("%s.%s", tapOneInterfaceName, dummyVlanID)
	dpdkNetworkOne               = RDSCoreConfig.RootlessDPDKNetworkOne
	dpdkNetworkTwo               = RDSCoreConfig.RootlessDPDKNetworkTwo
	dpdkPolicyTwo                = RDSCoreConfig.RootlessDPDKPolicyTwo
	dpdkClientDeploymentName     = RDSCoreConfig.RootlessDPDKClientDeploymentName
	dpdkClientVlanMac            = RDSCoreConfig.RootlessDPDKClientVlanMac
	dpdkClientMacVlanMac         = RDSCoreConfig.RootlessDPDKClientMacVlanMac
	dpdkClientIPVlanMac          = RDSCoreConfig.RootlessDPDKClientIPVlanMac
	dpdkClientIPVlanIP           = RDSCoreConfig.RootlessDPDKClientIPVlanIPv4
	dpdkClientIPVlanIPDummy      = RDSCoreConfig.RootlessDPDKClientIPVlanIPv4Dummy

	rootUser         = int64(0)
	waitTimeout      = 3 * time.Minute
	psiWaitTimeout   = 10 * time.Second
	psiRetryInterval = time.Second

	serverDeploymentLabelMap = map[string]string{
		strings.Split(serverPodLabel, "=")[0]: strings.Split(serverPodLabel, "=")[1]}

	serverSC = corev1.SecurityContext{
		RunAsUser: &rootUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_ADMIN", "NET_RAW"},
		},
	}
)

func createRootlessDPDKServerDeployment(
	apiClient *clients.Settings,
	serverSRIOVNetworkName,
	clientMac,
	sriovNodePolicyName,
	serverDeploymentHost,
	txIPs string) error {
	klog.V(100).Infof("Ensuring server deployment %q doesn't exist in %q namespace",
		serverDPDKDeploymentName, deploymentNamespace)

	err := cleanUpRootlessDPDKDeployment(apiClient, serverDPDKDeploymentName, deploymentNamespace, serverPodLabel)
	if err != nil {
		klog.V(100).Infof("Failed to cleanup deployment %s from the namespace %s: %v",
			serverDPDKDeploymentName, deploymentNamespace, err)

		return fmt.Errorf("failed to cleanup deployment %s from the namespace %s: %w",
			serverDPDKDeploymentName, deploymentNamespace, err)
	}

	klog.V(100).Infof("Creating server DPDK deployment %s in namespace %s",
		serverDPDKDeploymentName, deploymentNamespace)

	serverPodNetConfig := pod.StaticIPAnnotationWithMacAndNamespace(
		serverSRIOVNetworkName, deploymentNamespace, dpdkServerMac)
	if len(serverPodNetConfig) == 0 {
		klog.V(100).Infof("Failed to build rootless DPDK server pod network configuration for the "+
			"network %s from the namespace %s with the MAC address %s",
			serverSRIOVNetworkName, deploymentNamespace, dpdkServerMac)

		return fmt.Errorf("failed to build rootless DPDK server pod network configuration for the "+
			"network %s from the namespace %s with the MAC address %s",
			serverSRIOVNetworkName, deploymentNamespace, dpdkServerMac)
	}

	err = defineAndCreateDPDKDeployment(
		apiClient,
		serverDPDKDeploymentName,
		deploymentNamespace,
		serverDeploymentHost,
		dpdkDeploymentSAName,
		serverSC,
		nil,
		serverPodNetConfig,
		defineTestServerPmdCmd(clientMac, fmt.Sprintf("${PCIDEVICE_OPENSHIFT_IO_%s}",
			strings.ToUpper(sriovNodePolicyName)), txIPs),
		serverDeploymentLabelMap)
	if err != nil {
		klog.V(100).Infof("Failed to create server deployment %s in namespace %s: %v",
			serverDPDKDeploymentName, deploymentNamespace, err)

		return fmt.Errorf("failed to create server deployment %s in namespace %s: %w",
			serverDPDKDeploymentName, deploymentNamespace, err)
	}

	return nil
}

// CleanupRootlessDPDKServerDeployment cleaning up the rootless DPDK server deployment.
func CleanupRootlessDPDKServerDeployment() {
	By("Ensuring rootless DPDK server deployment was deleted")

	err := cleanUpRootlessDPDKDeployment(APIClient, serverDPDKDeploymentName, deploymentNamespace, serverPodLabel)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to cleanup deployment %s from the namespace %s: %v",
			serverDPDKDeploymentName, deploymentNamespace, err))
}

func cleanUpRootlessDPDKDeployment(
	apiClient *clients.Settings,
	deploymentName,
	nsName,
	podLabel string) error {
	if deploymentName == "" {
		klog.V(100).Infof("The rootless DPDK deployment name has to be provided")

		return fmt.Errorf("the rootless DPDK deployment name has to be provided")
	}

	if nsName == "" {
		klog.V(100).Infof("The rootless DPDK deployment namespace has to be provided")

		return fmt.Errorf("the rootless DPDK deployment namespace has to be provided")
	}

	_, err := deployment.Pull(apiClient, deploymentName, nsName)
	if err == nil {
		klog.V(100).Infof("Ensure %s deployment does not exist in namespace %s", deploymentName, nsName)

		err = apiobjectshelper.DeleteDeployment(apiClient, deploymentName, nsName)
		if err != nil {
			klog.V(100).Infof("Failed to delete deployment %s from nsname %s due to %v",
				deploymentName, nsName, err)

			return fmt.Errorf("failed to delete deployment %s from nsname %s due to %w",
				deploymentName, nsName, err)
		}
	}

	err = apiobjectshelper.EnsureAllPodsRemoved(apiClient, nsName, podLabel)
	if err != nil {
		klog.V(100).Infof("Failed to delete pods in namespace %s with the label %s: %v", nsName, podLabel, err)

		return fmt.Errorf("failed to delete pods in namespace %s with the label %s: %w", nsName, podLabel, err)
	}

	return nil
}

func defineAndCreateDPDKDeployment(
	apiClient *clients.Settings,
	deploymentName,
	deploymentNamespace,
	scheduleOnHost,
	saName string,
	securityContext corev1.SecurityContext,
	podSC *corev1.PodSecurityContext,
	podNetConfig []*types.NetworkSelectionElement,
	containerCmd []string,
	deployLabels map[string]string) error {
	klog.V(100).Infof("Creating container %s definition", deploymentName)

	nodeSelector := map[string]string{"kubernetes.io/hostname": scheduleOnHost}

	dpdkContainerCfg, err := defineDPDKContainer(deploymentName, containerCmd, securityContext)
	if err != nil {
		klog.V(100).Infof("Failed to set DPDK container %s due to %v", deploymentName, err)

		return fmt.Errorf("failed to set DPDK container %s due to %w", deploymentName, err)
	}

	dpdkDeployment := deployment.NewBuilder(
		apiClient,
		deploymentName,
		deploymentNamespace,
		deployLabels,
		*dpdkContainerCfg).WithSecondaryNetwork(podNetConfig).
		WithNodeSelector(nodeSelector).WithHugePages()

	if podSC != nil {
		dpdkDeployment = dpdkDeployment.WithSecurityContext(podSC)
	}

	if saName != "" {
		dpdkDeployment = dpdkDeployment.WithServiceAccountName(saName)
	}

	_, err = dpdkDeployment.CreateAndWaitUntilReady(waitTimeout)
	if err != nil {
		klog.V(100).Infof("Failed to create DPDK deployment %s in namespace %s due to %v",
			deploymentName, deploymentNamespace, err)

		return fmt.Errorf("failed to create DPDK deployment %s in namespace %s: %w",
			deploymentName, deploymentNamespace, err)
	}

	klog.V(100).Infof("Wait for the Running pod status for the deployment %s in namespace %s",
		deploymentName, deploymentNamespace)

	_pod, err := getDPDKPod(apiClient, deploymentName, deploymentNamespace)

	if err != nil || _pod == nil {
		klog.V(100).Infof("no running pods found for the deployment %s in namespace %s: %v",
			deploymentName, deploymentNamespace, err)

		return fmt.Errorf("no running pods found for the deployment %s in namespace %s: %w",
			deploymentName, deploymentNamespace, err)
	}

	return nil
}

func defineDPDKContainer(
	cName string,
	containerCmd []string,
	securityContext corev1.SecurityContext) (*corev1.Container, error) {
	deploymentContainer := pod.NewContainerBuilder(
		cName,
		RDSCoreConfig.DpdkTestContainer,
		containerCmd)

	dpdkContainerCfg, err := deploymentContainer.WithSecurityContext(&securityContext).
		WithResourceLimit(hugePages, memory, cpu).
		WithResourceRequest(hugePages, memory, cpu).WithEnvVar("RUN_TYPE", "testcmd").
		GetContainerCfg()
	if err != nil {
		klog.V(100).Infof("Failed to define server container %s due to %v", cName, err)

		return nil, fmt.Errorf("failed to define server container %s due to %w", cName, err)
	}

	return dpdkContainerCfg, nil
}

func retrieveClientDPDKPod(apiClient *clients.Settings, podNamePattern, podNamespace string) (*pod.Builder, error) {
	klog.V(100).Infof("Retrieve client DPDK pod %s in namespace %s", podNamePattern, podNamespace)

	podObj, err := getDPDKPod(apiClient, podNamePattern, podNamespace)
	if err != nil {
		klog.V(100).Infof("Failed to retrieve client pod %q in namespace %q: %v",
			podNamePattern, podNamespace, err)

		return nil, fmt.Errorf("failed to retrieve client pod %q in namespace %q: %w",
			podNamePattern, podNamespace, err)
	}

	err = podObj.WaitUntilReady(time.Second * 30)
	if err != nil {
		klog.V(100).Infof("The rootless DPDK client pod %s in namespace %s is not in Ready condition: %v",
			podNamePattern, podNamespace, err)

		return nil, fmt.Errorf("the rootless DPDK client pod %s in namespace %s is not in Ready condition: %w",
			podNamePattern, podNamespace, err)
	}

	return podObj, nil
}

func getDPDKPod(apiClient *clients.Settings, podNamePattern, podNamespace string) (*pod.Builder, error) {
	var podObj *pod.Builder

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*1,
		true,
		func(ctx context.Context) (bool, error) {
			podObjList, err := pod.ListByNamePattern(apiClient, podNamePattern, podNamespace)
			if err != nil {
				klog.V(100).Infof("Error getting pod object by name pattern %q in namespace %q: %v",
					podNamePattern, podNamespace, err)

				return false, nil
			}

			if len(podObjList) == 0 {
				klog.V(100).Infof("No pods %s were found in namespace %q", podNamePattern, podNamespace)

				return false, nil
			}

			if len(podObjList) > 1 {
				klog.V(100).Infof("Wrong pods %s count was found in namespace %q",
					podNamePattern, podNamespace)

				for _, _pod := range podObjList {
					klog.V(100).Infof("Pod %q is in %q phase", _pod.Definition.Name, _pod.Object.Status.Phase)
				}

				return false, nil
			}

			podObj = podObjList[0]

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to retrieve pod %q in namespace %q: %v",
			podNamePattern, podNamespace, err)

		return nil, fmt.Errorf("failed to retrieve pod %q in namespace %q: %w",
			podNamePattern, podNamespace, err)
	}

	return podObj, nil
}

func rxTrafficOnClientPod(clientPod *pod.Builder, clientRxCmd string) error {
	timeoutError := "command terminated with exit code 137"

	klog.V(90).Infof("Checking dpdk-pmd traffic command %s from the client pod %s",
		clientRxCmd, clientPod.Definition.Name)

	var clientOut bytes.Buffer

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*2,
		false,
		func(ctx context.Context) (bool, error) {
			clientOut, err = clientPod.ExecCommandWithTimeout([]string{"/bin/bash", "-c", clientRxCmd}, clientRxCmdTimeout)
			if err != nil {
				klog.V(100).Infof("Error running command: %v", err)

				if err.Error() != timeoutError {
					klog.V(100).Infof("Failed to run the dpdk-pmd command %s; %v", clientRxCmd, err)

					return false, nil
				}
			}

			return true, nil
		})
	if err != nil {
		if err.Error() != timeoutError {
			klog.V(100).Infof("Failed to run the dpdk-pmd command %s on the client pod %s in namespace %s; %v",
				clientRxCmd, clientPod.Definition.Name, clientPod.Definition.Namespace, err)

			return fmt.Errorf("failed to run the dpdk-pmd command %s on the client pod %s in namespace %s; %w",
				clientRxCmd, clientPod.Definition.Name, clientPod.Definition.Namespace, err)
		}

		klog.V(100).Infof("expected error message received: %v", err)
	}

	klog.V(90).Infof("Processing testpmd output from client pod \n%s", clientOut.String())

	outPutTrue := checkRxOnly(clientOut.String())

	if !outPutTrue {
		klog.V(100).Infof("Failed to parse the dpdk-pmd command execution output \n%s", clientOut.String())

		return fmt.Errorf("failed to parse the output from RxTrafficOnClientPod \n%s", clientOut.String())
	}

	return nil
}

func getCurrentLinkRx(runningPod *pod.Builder) (map[string]int, error) {
	var (
		linksRawInfo  bytes.Buffer
		linksInfoList []link.Link
		err           error
	)

	linksInfoMap := make(map[string]int)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*2,
		false,
		func(ctx context.Context) (bool, error) {
			linksRawInfo, err = runningPod.ExecCommandWithTimeout(
				[]string{"/bin/bash", "-c", "ip --json -s link show"}, getLinkRxTimeout)
			if err != nil {
				klog.V(100).Infof("The links info is not available for the pod %s in namespace %s with error %v",
					runningPod.Definition.Name, runningPod.Definition.Namespace, err)

				return false, nil
			}

			tmpLinksInfoList, err := link.NewListBuilder(linksRawInfo)
			if err != nil {
				klog.V(100).Infof("Failed to build a links object list for %q due to %v",
					linksRawInfo, err)

				return false, nil
			}

			linksInfoList = tmpLinksInfoList

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to get links info from pod %s in namespace %s with error %v",
			runningPod.Definition.Name, runningPod.Definition.Namespace, err)

		return nil, fmt.Errorf("failed to get links info from pod %s in namespace %s with error %w",
			runningPod.Definition.Name, runningPod.Definition.Namespace, err)
	}

	for _, linkInfo := range linksInfoList {
		klog.V(100).Infof("Found RxByte for interface %s: %d",
			linkInfo.Ifname, linkInfo.GetRxByte())

		linksInfoMap[linkInfo.Ifname] = linkInfo.GetRxByte()
	}

	return linksInfoMap, nil
}

func checkRxOnly(out string) bool {
	lines := strings.Split(out, "\n")
	for index, line := range lines {
		if strings.Contains(line, "NIC statistics for port") {
			if len(lines[index+1]) < 3 {
				klog.V(90).Info("Fail: line list contains less than 3 elements")

				return false
			}

			if len(lines) > index && getNumberOfPackets(lines[index+1], "RX") > 0 {
				return true
			}
		}
	}

	return false
}

func getNumberOfPackets(line, firstFieldSubstr string) int {
	klog.V(100).Infof("Parsing line %s", line)

	splitLine := strings.Fields(line)

	klog.V(100).Infof("Parsing field %s", splitLine)

	if !strings.Contains(splitLine[0], firstFieldSubstr) {
		klog.V(90).Infof("Failed to find expected substring %s", firstFieldSubstr)

		return 0
	}

	if len(splitLine) != 6 {
		klog.V(90).Info("the slice doesn't contain 6 elements")

		return 0
	}

	numberOfPackets, err := strconv.Atoi(splitLine[1])
	if err != nil {
		klog.V(90).Infof("failed to convert string to integer %s", err)

		return 0
	}

	return numberOfPackets
}

func defineTestServerPmdCmd(ethPeer, pciAddress, txIPs string) []string {
	baseCmd := fmt.Sprintf("dpdk-testpmd -a %s -- --forward-mode txonly --eth-peer=0,%s", pciAddress, ethPeer)
	if txIPs != "" {
		baseCmd += fmt.Sprintf(" --tx-ip=%s", txIPs)
	}

	baseCmd += " --stats-period 5"

	return []string{"/bin/bash", "-c", baseCmd}
}

func defineTestPmdCmd(interfaceName string, pciAddress string) string {
	return fmt.Sprintf("timeout -s SIGKILL %d dpdk-testpmd "+
		"--vdev=virtio_user0,path=/dev/vhost-net,queues=2,queue_size=1024,iface=%s "+
		"-a %s -- --stats-period 5", int(dpdkTestpmdTimeout.Seconds()), interfaceName, pciAddress)
}

func checkRxOutputRateForInterfaces(
	clientPod *pod.Builder,
	origInterfaceTrafficRateMap,
	interfaceTrafficRateMap map[string]int) error {
	for interfaceName, TrafficRate := range interfaceTrafficRateMap {
		originalRate := origInterfaceTrafficRateMap[interfaceName]

		klog.V(100).Infof("Original Rate for the interface %s: %d", interfaceName, originalRate)

		currentRate, err := getLinkRx(clientPod, interfaceName)
		if err != nil {
			klog.V(100).Infof("Failed to collect link %s info from pod %s in namespace %s with error %v",
				interfaceName, clientPod.Definition.Name, clientPod.Definition.Namespace, err)

			return fmt.Errorf("failed to get link %s info from pod %s in namespace %s with error %w",
				interfaceName, clientPod.Definition.Name, clientPod.Definition.Namespace, err)
		}

		klog.V(100).Infof("Current Rate for the interface %s: %d", interfaceName, currentRate)

		currentRate -= originalRate

		klog.V(100).Infof("Current run Rate for the interface %s: %d", interfaceName, currentRate)

		if TrafficRate == maxMulticastNoiseRate && currentRate > TrafficRate ||
			TrafficRate != maxMulticastNoiseRate && currentRate < TrafficRate {
			klog.V(100).Infof("Failed traffic rate is not in expected range; "+
				"current rate is %d, TrafficRate is %d, interfaceName is %s", currentRate, TrafficRate, interfaceName)

			return fmt.Errorf("failed traffic rate is not in expected range; "+
				"current rate is %d, TrafficRate is %d, interfaceName is %s", currentRate, TrafficRate, interfaceName)
		}
	}

	return nil
}

func getLinkRx(runningPod *pod.Builder, linkName string) (int, error) {
	var (
		linkRawInfo bytes.Buffer
		err         error
	)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		5*time.Second,
		time.Minute,
		false,
		func(ctx context.Context) (bool, error) {
			linkRawInfo, err = runningPod.ExecCommandWithTimeout(
				[]string{"/bin/bash", "-c", fmt.Sprintf("ip --json -s link show dev %s", linkName)}, getLinkRxTimeout)
			if err != nil {
				klog.V(100).Infof("The link %s info is not available for the pod %s in namespace %s "+
					"with error %v",
					linkName, runningPod.Definition.Name, runningPod.Definition.Namespace, err)

				return false, nil
			}

			klog.V(100).Infof("Validating output of link %s info from pod %s in namespace %s",
				linkName, runningPod.Definition.Name, runningPod.Definition.Namespace)

			_, err = link.NewBuilder(linkRawInfo)
			if err != nil {
				klog.V(100).Infof("Failed to parse %q link's info from pod %s in namespace %s with error %v",
					linkName, runningPod.Definition.Name, runningPod.Definition.Namespace, err)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to get link %s info from pod %s in namespace %s with error %v",
			linkName, runningPod.Definition.Name, runningPod.Definition.Namespace, err)

		return 0, fmt.Errorf("failed to get link %s info from pod %s in namespace %s with error %w",
			linkName, runningPod.Definition.Name, runningPod.Definition.Namespace, err)
	}

	linkInfo, err := link.NewBuilder(linkRawInfo)
	if err != nil {
		klog.V(100).Infof("Failed to collect link %s info from pod %s in namespace %s with error %v",
			linkName, runningPod.Definition.Name, runningPod.Definition.Namespace, err)

		return 0, fmt.Errorf("failed to collect link %s info from pod %s in namespace %s with error %w",
			linkName, runningPod.Definition.Name, runningPod.Definition.Namespace, err)
	}

	return linkInfo.GetRxByte(), nil
}

func getPCIAddressListFromSrIovNetworkName(podNetworkStatus, networkName string) ([]string, error) {
	var podNetworkStatusType []podNetworkAnnotation

	err := json.Unmarshal([]byte(podNetworkStatus), &podNetworkStatusType)
	if err != nil {
		klog.V(100).Infof("Failed to unmarshal pod network status %s with error %v", podNetworkStatus, err)

		return nil, err
	}

	var pciAddressList []string

	for _, networkAnnotation := range podNetworkStatusType {
		if strings.Contains(networkAnnotation.Name, networkName) {
			pciAddressList = append(pciAddressList, networkAnnotation.DeviceInfo.Pci.PciAddress)
		}
	}

	klog.V(100).Infof("PCI address list: %v", pciAddressList)

	return pciAddressList, nil
}

func isPCIAddressAvailable(clientPod *pod.Builder) bool {
	networkStatusAnnotation := "k8s.v1.cni.cncf.io/network-status"

	if !clientPod.Exists() {
		klog.V(100).Infof("Pod %s doesn't exist in namespace %s",
			clientPod.Definition.Name, clientPod.Definition.Namespace)

		return false
	}

	podNetAnnotation := clientPod.Object.Annotations[networkStatusAnnotation]
	if podNetAnnotation == "" {
		klog.V(100).Infof("Pod %s from namespace %s network annotation field %s is not available",
			clientPod.Object.Name, clientPod.Object.Namespace, networkStatusAnnotation)

		return false
	}

	var err error

	pciAddressList, err := getPCIAddressListFromSrIovNetworkName(podNetAnnotation, dpdkNetworkOne)
	if err != nil {
		klog.V(100).Infof("Failed to get PCI address list from pod %s in namespace %s with error %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return false
	}

	if len(pciAddressList) < 1 {
		klog.V(100).Infof("Pod %s from namespace %s network annotation field %s not found",
			clientPod.Definition.Name, clientPod.Definition.Namespace, networkStatusAnnotation)

		return false
	}

	return true
}

// VerifyRootlessDPDKOnTheSameNodeSingleVFMultipleVlans deploy workloads with Rootless DPDK pods
// on the same node using single VF with multiple VLANs.
func VerifyRootlessDPDKOnTheSameNodeSingleVFMultipleVlans(ctx SpecContext) {
	By("Create Rootless DPDK server deployment on the same node")

	err := createRootlessDPDKServerDeployment(
		APIClient,
		dpdkNetworkTwo,
		dpdkClientVlanMac,
		dpdkPolicyTwo,
		RDSCoreConfig.RootlessDPDKNodeOne,
		"")
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create rootless DPDK deployment in namespace %s: %v", deploymentNamespace, err))

	By("Retrieve client rootless DPDK pod object")

	clientPod, err := retrieveClientDPDKPod(
		APIClient,
		dpdkClientDeploymentName,
		deploymentNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve client rootless DPDK pod %s from namespace %s: %v",
			dpdkClientDeploymentName, deploymentNamespace, err))

	By("Collecting PCI Address")
	Eventually(
		isPCIAddressAvailable, psiWaitTimeout, psiRetryInterval).WithArguments(clientPod).Should(BeTrue())

	pciAddressList, err := getPCIAddressListFromSrIovNetworkName(
		clientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"], dpdkNetworkOne)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Fail to collect PCI addresses for the pod %s in namespace %s: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err))

	klog.V(90).Infof("Getting original link network devices rate values")

	originalLinksRxRateMap, err := getCurrentLinkRx(clientPod)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to retrieve current link interfaces rate values; %v", err)

	By("Running client dpdk-testpmd")

	err = rxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapOneInterfaceName, pciAddressList[0]))
	Expect(err).ToNot(HaveOccurred(),
		"The Receive traffic test on the client pod failed; %v", err)

	By("Checking the rx output of tap ext0 device")

	err = checkRxOutputRateForInterfaces(
		clientPod,
		originalLinksRxRateMap,
		map[string]int{
			tapOneInterfaceName:          minimumExpectedDPDKRate,
			firstInterfaceBasedOnTapOne:  minimumExpectedDPDKRate,
			secondInterfaceBasedOnTapOne: maxMulticastNoiseRate},
	)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("The Receive traffic test on the client pod %s in namespace %s failed: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err))
}

// VerifyRootlessDPDKWorkloadsOnDifferentNodesMultipleVlans deploy workloads with Rootless DPDK pods
// on the different nodes with multiple VLANs.
func VerifyRootlessDPDKWorkloadsOnDifferentNodesMultipleVlans(ctx SpecContext) {
	By("Create Rootless DPDK server deployment on different node with multiple VLANs")

	err := createRootlessDPDKServerDeployment(
		APIClient,
		dpdkNetworkTwo,
		dpdkClientVlanMac,
		dpdkPolicyTwo,
		RDSCoreConfig.RootlessDPDKNodeTwo,
		"")
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create rootless DPDK deployment in namespace %s: %v", deploymentNamespace, err))

	By("Retrieve client rootless DPDK pod object")

	clientPod, err := retrieveClientDPDKPod(
		APIClient,
		dpdkClientDeploymentName,
		deploymentNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve client rootless DPDK pod %s from namespace %s: %v",
			dpdkClientDeploymentName, deploymentNamespace, err))

	By("Collecting PCI Address")
	Eventually(
		isPCIAddressAvailable, psiWaitTimeout, psiRetryInterval).WithArguments(clientPod).Should(BeTrue())

	pciAddressList, err := getPCIAddressListFromSrIovNetworkName(
		clientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"], dpdkNetworkOne)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Fail to collect PCI addresses for the pod %s in namespace %s: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err))

	klog.V(90).Infof("Getting original link network devices rate values")

	originalLinksRxRateMap, err := getCurrentLinkRx(clientPod)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to retrieve current link interfaces rate values; %v", err)

	By("Running client dpdk-testpmd")

	err = rxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapOneInterfaceName, pciAddressList[0]))
	Expect(err).ToNot(HaveOccurred(),
		"The Receive traffic test on the client pod failed")

	By("Checking the rx output of tap ext0 device")

	err = checkRxOutputRateForInterfaces(
		clientPod,
		originalLinksRxRateMap,
		map[string]int{
			tapOneInterfaceName:          minimumExpectedDPDKRate,
			firstInterfaceBasedOnTapOne:  minimumExpectedDPDKRate,
			secondInterfaceBasedOnTapOne: maxMulticastNoiseRate},
	)
	Expect(err).ToNot(HaveOccurred(),
		"The Receive traffic test on the client pod %s in namespace %s failed: %v",
		clientPod.Definition.Name, clientPod.Definition.Namespace, err)
}

// VerifyRootlessDPDKWorkloadsOnDifferentNodesMultipleMacVlans deploy workloads with Rootless DPDK pods
// on the different nodes with multiple MAC-VLANs.
func VerifyRootlessDPDKWorkloadsOnDifferentNodesMultipleMacVlans(ctx SpecContext) {
	By("Create Rootless DPDK server deployment on different node with multiple MAC-VLANs")

	err := createRootlessDPDKServerDeployment(
		APIClient,
		dpdkNetworkTwo,
		dpdkClientMacVlanMac,
		dpdkPolicyTwo,
		RDSCoreConfig.RootlessDPDKNodeTwo,
		"")
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create rootless DPDK deployment in namespace %s: %v", deploymentNamespace, err))

	By("Retrieve client rootless DPDK pod object")

	clientPod, err := retrieveClientDPDKPod(
		APIClient,
		dpdkClientDeploymentName,
		deploymentNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve client rootless DPDK pod %s from namespace %s: %v",
			dpdkClientDeploymentName, deploymentNamespace, err))

	By("Collecting PCI Address")
	Eventually(
		isPCIAddressAvailable, psiWaitTimeout, psiRetryInterval).WithArguments(clientPod).Should(BeTrue())

	pciAddressList, err := getPCIAddressListFromSrIovNetworkName(
		clientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"], dpdkNetworkOne)
	Expect(err).ToNot(HaveOccurred(), "Fail to collect PCI addresses for the pod %s in namespace %s: %w",
		clientPod.Definition.Name, clientPod.Definition.Namespace, err)

	klog.V(90).Infof("Getting original link network devices rate values")

	originalLinksRxRateMap, err := getCurrentLinkRx(clientPod)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to retrieve current link interfaces rate values; %v", err)

	By("Running client dpdk-testpmd")

	err = rxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapTwoInterfaceName, pciAddressList[1]))
	Expect(err).ToNot(HaveOccurred(),
		"The Receive traffic test on the client pod failed")

	By("Checking the rx output of tap ext1 device")

	err = checkRxOutputRateForInterfaces(
		clientPod,
		originalLinksRxRateMap,
		map[string]int{
			tapTwoInterfaceName:          minimumExpectedDPDKRate,
			tapOneInterfaceName:          maxMulticastNoiseRate,
			firstInterfaceBasedOnTapTwo:  minimumExpectedDPDKRate,
			secondInterfaceBasedOnTapTwo: maxMulticastNoiseRate},
	)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("The Receive traffic test on the client pod %s in namespace %s failed for the "+
			"tap interface %s: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, tapTwoInterfaceName, err))
}

// VerifyRootlessDPDKWorkloadsOnDifferentNodesMultipleIPVlans deploy workloads with Rootless DPDK pods
// on the different nodes with multiple IP-VLANs.
func VerifyRootlessDPDKWorkloadsOnDifferentNodesMultipleIPVlans(ctx SpecContext) {
	By("Create Rootless DPDK server deployment on different node with multiple IP-VLANs")

	err := createRootlessDPDKServerDeployment(
		APIClient,
		dpdkNetworkTwo,
		dpdkClientIPVlanMac,
		dpdkPolicyTwo,
		RDSCoreConfig.RootlessDPDKNodeTwo,
		fmt.Sprintf("%s,%s", dpdkClientIPVlanIPDummy, dpdkClientIPVlanIP))
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create rootless DPDK deployment in namespace %s: %v", deploymentNamespace, err))

	By("Retrieve client rootless DPDK pod object")

	clientPod, err := retrieveClientDPDKPod(
		APIClient,
		dpdkClientDeploymentName,
		deploymentNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve client rootless DPDK pod %s from namespace %s: %v",
			dpdkClientDeploymentName, deploymentNamespace, err))

	By("Collecting PCI Address")
	Eventually(
		isPCIAddressAvailable, psiWaitTimeout, psiRetryInterval).WithArguments(clientPod).Should(BeTrue())

	pciAddressList, err := getPCIAddressListFromSrIovNetworkName(
		clientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"], dpdkNetworkOne)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Fail to collect PCI addresses for the pod %s in namespace %s: %v",
		clientPod.Definition.Name, clientPod.Definition.Namespace, err))

	klog.V(90).Infof("Getting original link network devices rate values")

	originalLinksRxRateMap, err := getCurrentLinkRx(clientPod)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve current link interfaces rate values; %v", err))

	By("Running client dpdk-testpmd")

	err = rxTrafficOnClientPod(clientPod, defineTestPmdCmd(tapThreeInterfaceName, pciAddressList[2]))
	Expect(err).ToNot(HaveOccurred(),
		"The Receive traffic test on the client pod failed")

	By("Checking the rx output of tap ext1 device")

	err = checkRxOutputRateForInterfaces(
		clientPod,
		originalLinksRxRateMap,
		map[string]int{
			tapThreeInterfaceName:          minimumExpectedDPDKRate,
			tapOneInterfaceName:            maxMulticastNoiseRate,
			tapTwoInterfaceName:            maxMulticastNoiseRate,
			firstInterfaceBasedOnTapThree:  minimumExpectedDPDKRate,
			secondInterfaceBasedOnTapThree: maxMulticastNoiseRate},
	)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("The Receive traffic test on the client pod %s in namespace %s failed for the "+
			"tap interface %s: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, tapThreeInterfaceName, err))
}
