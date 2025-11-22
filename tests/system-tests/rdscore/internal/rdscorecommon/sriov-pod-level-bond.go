package rdscorecommon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
)

const (
	podLevelBondDeploymentRBACName = "privileged-rdscore-pod-level-bond"
	podLevelBondDeploymentRBACRole = "system:openshift:scc:privileged"
	podLevelBondDeploymentSAName   = "rdscore-pod-level-bond-sa"
	podLevelBondPodLabel           = "systemtest-test=rdscore-pod-level-bond-privileged"
	podLevelBondNetName            = "bond-net"
	tcpTestPassedMsg               = `TCP test passed as expected`
	mtuSize                        = "8900"
)

var (
	podLevelBondPodLabelMap = map[string]string{"systemtest-test": "rdscore-pod-level-bond-privileged"}
)

type podNetworkAnnotation struct {
	Name      string   `json:"name"`
	Interface string   `json:"interface"`
	Ips       []string `json:"ips,omitempty"`
	Mac       string   `json:"mac"`
	Default   bool     `json:"default,omitempty"`
	DNS       struct {
	} `json:"dns"`
	DeviceInfo struct {
		Type    string `json:"type"`
		Version string `json:"version"`
		Pci     struct {
			PciAddress string `json:"pci-address"`
		} `json:"pci"`
	} `json:"device-info,omitempty"`
}

//nolint:funlen
func createPrivilegedPodLevelBondDeployment(
	apiClient *clients.Settings,
	deploymentName,
	nsName,
	podLabel,
	scheduleOnHost,
	sriovNet1Name,
	sriovNet2Name,
	bondNetName,
	bondInfIPv4,
	bondInfIPv6,
	bondInfSubMaskIPv4,
	bondInfSubMaskIPv6,
	bondInfMacAddr string) error {
	klog.V(100).Infof("Ensuring deployment %q doesn't exist in %q namespace", deploymentName, nsName)

	err := cleanUpPodLevelBondDeployment(apiClient, deploymentName, nsName, podLabel)
	if err != nil {
		klog.V(100).Infof("Failed to cleanup deployment %s from the namespace %s: %v",
			deploymentName, nsName, err)

		return fmt.Errorf("failed to cleanup deployment %s from the namespace %s: %w",
			deploymentName, nsName, err)
	}

	klog.V(100).Infof("Removing ServiceAccount %q", podLevelBondDeploymentSAName)
	deleteServiceAccount(podLevelBondDeploymentSAName, nsName)

	klog.V(100).Infof("Creating ServiceAccount %q", podLevelBondDeploymentSAName)
	createServiceAccount(podLevelBondDeploymentSAName, nsName)

	klog.V(100).Infof("Removing Cluster RBAC %q in namespace %q", podLevelBondDeploymentRBACName, nsName)
	deleteClusterRBAC(podLevelBondDeploymentRBACName)

	klog.V(100).Infof("Creating Cluster RBAC %q in namespace %q", podLevelBondDeploymentRBACName, nsName)
	createClusterRBAC(
		podLevelBondDeploymentRBACName,
		podLevelBondDeploymentRBACRole,
		podLevelBondDeploymentSAName,
		nsName)

	klog.V(100).Infof("Defining container configuration")

	deploymentContainer := definePodLevelBondDeploymentContainer()

	klog.V(100).Infof("Obtaining container definition")

	deployContainerCfg, err := deploymentContainer.GetContainerCfg()
	if err != nil {
		klog.V(100).Infof("Failed to obtain container definition: %v", err)

		return fmt.Errorf("failed to obtain container definition: %w", err)
	}

	klog.V(100).Infof("Defining deployment %q in namespace %q configuration", deploymentName, nsName)

	testPodDeployment, err := definePodLevelBondTestPodDeployment(
		apiClient,
		deployContainerCfg,
		deploymentName,
		nsName,
		scheduleOnHost,
		sriovNet1Name,
		sriovNet2Name,
		bondNetName,
		bondInfIPv4,
		bondInfIPv6,
		bondInfSubMaskIPv4,
		bondInfSubMaskIPv6,
		bondInfMacAddr,
		podLevelBondPodLabelMap)
	if err != nil {
		klog.V(100).Infof("Failed to define deployment %s in namespace %s: %v",
			deploymentName, nsName, err)

		return fmt.Errorf("failed to define deployment %s in namespace %s: %w",
			deploymentName, nsName, err)
	}

	klog.V(100).Infof("Creating deployment %q in namespace %q configuration", deploymentName, nsName)

	testPodDeployment, err = testPodDeployment.CreateAndWaitUntilReady(5 * time.Minute)
	if err != nil {
		klog.V(100).Infof("Failed to create deployment %s in namespace %s: %v",
			deploymentName, nsName, err)

		return fmt.Errorf("failed to create deployment %s in namespace %s: %w",
			deploymentName, nsName, err)
	}

	if testPodDeployment == nil {
		klog.V(100).Infof("Deployment %s not found in namespace %s", deploymentName, nsName)

		return fmt.Errorf("deployment %s not found in namespace %s", deploymentName, nsName)
	}

	return nil
}

func cleanUpPodLevelBondDeployment(apiClient *clients.Settings, deploymentName, nsName, podLabel string) error {
	_, err := deployment.Pull(apiClient, deploymentName, nsName)
	if err != nil {
		klog.V(100).Infof("Deployment %s not found in namespace %s, %v", deploymentName, nsName, err)
	}

	klog.V(100).Infof("Ensure %s deployment does not exist in namespace %s", deploymentName, nsName)

	err = apiobjectshelper.DeleteDeployment(apiClient, deploymentName, nsName)
	if err != nil {
		klog.V(100).Infof("Failed to delete deployment %s from nsname %s due to %v",
			deploymentName, nsName, err)

		return fmt.Errorf("failed to delete deployment %s from nsname %s due to %w",
			deploymentName, nsName, err)
	}

	err = apiobjectshelper.EnsureAllPodsRemoved(apiClient, nsName, podLabel)
	if err != nil {
		klog.V(100).Infof("Failed to delete pods in namespace %s with the label %s: %v", nsName, podLabel, err)

		return fmt.Errorf("failed to delete pods in namespace %s with the label %s: %w", nsName, podLabel, err)
	}

	return nil
}

func definePodLevelBondDeploymentContainer() *pod.ContainerBuilder {
	cName := "test-pod"

	containerCmd := []string{
		"testcmd",
		"--listen",
		"-interface",
		"net3",
		"-protocol",
		"tcp",
		"-port",
		RDSCoreConfig.PodLevelBondPort,
		"-mtu",
		mtuSize,
	}

	klog.V(100).Infof("Creating container %q", cName)

	deploymentContainer := pod.NewContainerBuilder(cName, RDSCoreConfig.PodLevelBondDeployImage, containerCmd)

	klog.V(100).Infof("Defining SecurityContext")

	var trueFlag = true

	userUID := new(int64)

	*userUID = 0

	securityContext := &corev1.SecurityContext{
		RunAsUser:  userUID,
		Privileged: &trueFlag,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"NET_RAW", "NET_ADMIN", "SYS_ADMIN", "IPC_LOCK"},
		},
	}

	klog.V(100).Infof("Setting SecurityContext")

	deploymentContainer = deploymentContainer.WithSecurityContext(securityContext)

	klog.V(100).Infof("Dropping ALL security capability")

	deploymentContainer = deploymentContainer.WithDropSecurityCapabilities([]string{"ALL"}, true)

	klog.V(100).Infof("%q container's  definition:\n%#v", cName, deploymentContainer)

	return deploymentContainer
}

func definePodLevelBondTestPodDeployment(
	apiClient *clients.Settings,
	containerConfig *corev1.Container,
	deploymentName,
	nsName,
	scheduleOnHost,
	sriovNet1Name,
	sriovNet2Name,
	bondNetName,
	bondInfIPv4,
	bondInfIPv6,
	bondInfSubMaskIPv4,
	bondInfSubMaskIPv6,
	bondInfMacAddr string,
	deployLabels map[string]string) (*deployment.Builder, error) {
	klog.V(100).Infof("Defining deployment %q in %q ns", deploymentName, nsName)

	if bondInfIPv4 == "" {
		klog.V(100).Infof("Bond interface IPv4 address is missing")

		return nil, fmt.Errorf("bond interface IPv4 address is missing")
	}

	if bondInfIPv6 == "" {
		klog.V(100).Infof("Bond interface IPv6 address is missing")

		return nil, fmt.Errorf("bond interface IPv6 address is missing")
	}

	if bondInfSubMaskIPv4 == "" {
		klog.V(100).Infof("Bond interface IPv4 address subnet mask is missing")

		return nil, fmt.Errorf("bond interface IPv4 address subnet mask is missing")
	}

	if bondInfSubMaskIPv6 == "" {
		klog.V(100).Infof("Bond interface IPv6 address subnet mask is missing")

		return nil, fmt.Errorf("bond interface IPv6 address subnet mask is missing")
	}

	nodeSelector := map[string]string{"kubernetes.io/hostname": scheduleOnHost}

	netAnnotations := []*types.NetworkSelectionElement{
		{
			Name:      sriovNet1Name,
			Namespace: nsName,
		},
		{
			Name:      sriovNet2Name,
			Namespace: nsName,
		},
		{
			Name:      bondNetName,
			Namespace: nsName,
			IPRequest: []string{fmt.Sprintf("%s/%s", bondInfIPv4, bondInfSubMaskIPv4),
				fmt.Sprintf("%s/%s", bondInfIPv6, bondInfSubMaskIPv6)},
			MacRequest: bondInfMacAddr,
		},
	}

	podDeployment := deployment.NewBuilder(apiClient, deploymentName, nsName, deployLabels, *containerConfig)

	klog.V(100).Infof("Assigning ServiceAccount %q to the deployment", podLevelBondDeploymentSAName)

	podDeployment = podDeployment.WithServiceAccountName(podLevelBondDeploymentSAName)

	klog.V(100).Infof("Assigning NodeSelector %q to the deployment", nodeSelector)
	podDeployment = podDeployment.WithNodeSelector(nodeSelector)

	podDeployment = podDeployment.WithSecondaryNetwork(netAnnotations)

	return podDeployment, nil
}

//nolint:unparam
func generateTCPTraffic(
	clientPod *pod.Builder,
	serverIPAddr,
	serverPort,
	packetsNumber,
	timeout string) (string, error) {
	By("Open long TCP session and generate traffic")

	klog.V(100).Infof("Ensure pod %q in namespace %q is Ready",
		clientPod.Definition.Name, clientPod.Definition.Namespace)

	err := clientPod.WaitUntilReady(1 * time.Minute)
	if err != nil {
		klog.V(100).Infof("Failed to wait for pod %q in namespace %q to become Ready: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return "", fmt.Errorf("failed to wait for pod %q in namespace %q to become Ready: %w",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)
	}

	cmdToRun := []string{"bash", "-c",
		fmt.Sprintf("testcmd -protocol tcp -port %s -interface net3 -packages %s -timeoutTCP %s -server %s -mtu %s",
			serverPort, packetsNumber, timeout, serverIPAddr, mtuSize)}

	klog.V(100).Infof("Execute command: %q", cmdToRun)

	var output string

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*3,
		time.Minute*2,
		true,
		func(ctx context.Context) (bool, error) {
			result, err := clientPod.ExecCommand(cmdToRun, clientPod.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("Error running command from within a pod %q: %v",
					clientPod.Object.Name, err)

				return false, nil
			}

			klog.V(100).Infof("Successfully executed command from within a pod %q in namespace %q",
				clientPod.Object.Name, clientPod.Definition.Namespace)

			output = result.String()
			klog.V(100).Infof("Command's output:\n\t%v", output)

			return true, nil
		})
	if err != nil {
		return "", fmt.Errorf("failed to run command from within pod %s: %w", clientPod.Object.Name, err)
	}

	return output, nil
}

func findInCmdExecOutput(cmdExecOutput, stringToFind string) (bool, error) {
	var err error

	matchesFound := 0

	if cmdExecOutput == "" {
		klog.V(100).Infof("The cmdExecOutput is empty")

		return false, fmt.Errorf("the cmdExecOutput is empty")
	}

	buf := new(bytes.Buffer)

	_, err = buf.WriteString(cmdExecOutput)
	if err != nil {
		klog.V(100).Infof("error in copying info from the cmdExecOutput to buffer: %v", err)

		return false, fmt.Errorf("error in copying info from the cmdExecOutput to buffer: %w", err)
	}

	stringToFindRegex, err := regexp.Compile(stringToFind)
	if err != nil {
		klog.V(100).Infof("Failed to compile stringToFind %s: %v", stringToFind, err)

		return false, fmt.Errorf("failed to compile stringToFind %s: %w", stringToFind, err)
	}

	scanner := bufio.NewScanner(buf)

	for scanner.Scan() {
		logLine := scanner.Text()
		if stringToFindRegex.MatchString(logLine) {
			klog.V(100).Infof("Match for the string %q was found", stringToFind)

			matchesFound++
		}
	}

	if matchesFound < 1 {
		klog.V(100).Infof("Expected string %q not found in the output: %s", stringToFind, cmdExecOutput)

		return false, fmt.Errorf("expected string %q not found in the output : %s", stringToFind, cmdExecOutput)
	}

	return true, nil
}

func scanClientPodTrafficOutput(clientPodOutput string) (bool, error) {
	klog.V(100).Infof("client pod output: %s", clientPodOutput)

	isFound, err := findInCmdExecOutput(clientPodOutput, tcpTestPassedMsg)
	if err != nil {
		klog.V(100).Infof("Failed to parse clientPodOutput due to %v", err)

		return false, fmt.Errorf("failed to parse clientPodOutput due to %w", err)
	}

	if !isFound {
		klog.V(100).Infof("TCP traffic transmission failure detected: %s", clientPodOutput)

		return false, fmt.Errorf("tcp traffic transmission failure detected: %s", clientPodOutput)
	}

	return true, nil
}

func getBondActiveInterface(clientPod *pod.Builder) (string, error) {
	klog.V(90).Infof("Getting bond active VF interface for the pod %s in namespace %s",
		clientPod.Definition.Name, clientPod.Definition.Namespace)

	var (
		output bytes.Buffer
		result string
		err    error
	)

	cmdToRun := []string{"bash", "-c", "cat /sys/class/net/net3/bonding/active_slave"}

	klog.V(100).Infof("Execute command: %q", cmdToRun)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*1,
		true,
		func(ctx context.Context) (bool, error) {
			output, err = clientPod.ExecCommand(cmdToRun, clientPod.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("Error running command from within a pod %q in namespace %q: %v",
					clientPod.Definition.Name, clientPod.Definition.Namespace, err)

				return false, nil
			}

			if strings.TrimSpace(output.String()) == "" {
				klog.V(100).Infof("The active_slave interface name is empty for a pod %q in namespace %q",
					clientPod.Definition.Name, clientPod.Definition.Namespace)

				return false, nil
			}

			klog.V(100).Infof("Successfully executed command from within a pod %q in namespace %q: %v",
				clientPod.Definition.Name, clientPod.Definition.Namespace, err)

			result = output.String()

			klog.V(100).Infof("Command's output:\n\t%v", result)

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to run command from within pod %q in namespace %q: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return "", fmt.Errorf("failed to run command from within pod %q in namespace %q: %w",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)
	}

	return strings.TrimRight(result, "\r\n"), nil
}

func disableBondActiveVFInterface(clientPod *pod.Builder) error {
	var err error

	klog.V(100).Infof("Retrieve bond active interface name for the pod %s in namespace %s",
		clientPod.Definition.Name, clientPod.Definition.Namespace)

	interfaceName, err := getBondActiveInterface(clientPod)
	if err != nil {
		klog.V(100).Infof("Failed to retrieve bond active interface for the pod %s in namespace %s: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return fmt.Errorf("failed to retrieve bond active interface for the pod %s in namespace %s: %w",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)
	}

	err = changeInterfaceState(clientPod, interfaceName, true)
	if err != nil {
		klog.V(100).Infof("Failed to disable interface %s for the pod %q in namespace %q: %v",
			interfaceName, clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return fmt.Errorf("failed to disable interface %s for the pod %q in namespace %q: %w",
			interfaceName, clientPod.Definition.Name, clientPod.Definition.Namespace, err)
	}

	klog.V(100).Infof("Retrieve new bond active interface name for the pod %s in namespace %s",
		clientPod.Definition.Name, clientPod.Definition.Namespace)

	newInterfaceName, err := getBondActiveInterface(clientPod)
	if err != nil {
		klog.V(100).Infof("Failed to retrieve bond active interface for the pod %s in namespace %s: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return fmt.Errorf("failed to retrieve bond active interface for the pod %s in namespace %s: %w",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)
	}

	if newInterfaceName == interfaceName {
		klog.V(100).Infof("The bond active interface for the pod %s in namespace %s did not changed;"+
			"current bond active interface is %s, the original bond active interface is %s",
			clientPod.Definition.Name, clientPod.Definition.Namespace, newInterfaceName, interfaceName)

		return fmt.Errorf("the bond active interface for the pod %s in namespace %s did not changed;"+
			"current bond active interface is %s, the original bond active interface is %s",
			clientPod.Definition.Name, clientPod.Definition.Namespace, newInterfaceName, interfaceName)
	}

	klog.V(100).Infof("The bond active interface of the pod %s in namespace %s "+
		"successfully switched from the %s to the %s",
		clientPod.Definition.Name, clientPod.Definition.Namespace, interfaceName, newInterfaceName)

	return nil
}

//nolint:funlen
func changeInterfaceState(clientPod *pod.Builder, interfaceName string, toDisable bool) error {
	var (
		output           bytes.Buffer
		expectedInfState string
		result           string
		err              error
	)

	if toDisable {
		expectedInfState = "down"
	} else {
		expectedInfState = "up"
	}

	klog.V(100).Infof("Change pod-level bond interface %s for the pod %s in namespace %s state to the %s",
		interfaceName, clientPod.Definition.Name, clientPod.Definition.Namespace, expectedInfState)

	cmdToRun := []string{"bash", "-c", fmt.Sprintf("ip link set dev %s %s", interfaceName, expectedInfState)}

	klog.V(100).Infof("Execute command: %q", cmdToRun)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*1,
		true,
		func(ctx context.Context) (bool, error) {
			output, err = clientPod.ExecCommand(cmdToRun, clientPod.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("Error running command from within a pod %q in namespace %q: %v",
					clientPod.Definition.Name, clientPod.Definition.Namespace, err)

				return false, nil
			}

			klog.V(100).Infof("Successfully executed command from within a pod %q in namespace %q",
				clientPod.Definition.Name, clientPod.Definition.Namespace)

			result = output.String()

			klog.V(100).Infof("Command's output:\n\t%v", result)

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to run command from within pod %q in namespace %q: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return fmt.Errorf("failed to run command from within pod %q in namespace %q: %w",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)
	}

	klog.V(100).Infof("Change pod-level bond interface %s for the pod %s in namespace %s state to the %s",
		interfaceName, clientPod.Definition.Name, clientPod.Definition.Namespace, expectedInfState)

	cmdToRun = []string{"bash", "-c", "ip link show up"}

	klog.V(100).Infof("Execute command: %q", cmdToRun)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*1,
		true,
		func(ctx context.Context) (bool, error) {
			output, err = clientPod.ExecCommand(cmdToRun, clientPod.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("Error running command from within a pod %q in namespace %q: %v",
					clientPod.Definition.Name, clientPod.Definition.Namespace, err)

				return false, nil
			}

			klog.V(100).Infof("Successfully executed command from within a pod %q in namespace %q",
				clientPod.Definition.Name, clientPod.Definition.Namespace)

			result = output.String()

			klog.V(100).Infof("Command's output:\n\t%v", result)

			if toDisable && strings.Contains(result, fmt.Sprintf("%s:", interfaceName)) {
				klog.V(100).Infof("interface %q not in the state %q", interfaceName, expectedInfState)

				return false, nil
			}

			if !toDisable && !strings.Contains(result, fmt.Sprintf("%s:", interfaceName)) {
				klog.V(100).Infof("interface %q not in the state %q", interfaceName, expectedInfState)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to run command from within pod %q in namespace %q: %v",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)

		return fmt.Errorf("failed to run command from within pod %q in namespace %q: %w",
			clientPod.Definition.Name, clientPod.Definition.Namespace, err)
	}

	return nil
}

func inspectPodLevelBondedInterfaceConfig(podObj *pod.Builder, ipv4Addr, ipv6Addr string) (bool, error) {
	klog.V(100).Infof("Verify pod-level bonded interface configuration for pod %q in namespace %q",
		podObj.Definition.Name, podObj.Definition.Namespace)

	cmdToRun := []string{"bash", "-c", "ip a show type bond"}

	var output string

	klog.V(100).Infof("Execute command: %q", cmdToRun)

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*1,
		true,
		func(ctx context.Context) (bool, error) {
			result, err := podObj.ExecCommand(cmdToRun, podObj.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("Error running command from within a pod %q: %v",
					podObj.Object.Name, err)

				return false, nil
			}

			klog.V(100).Infof("Successfully executed command from within a pod %q in namespace %q",
				podObj.Object.Name, podObj.Definition.Namespace)

			output = result.String()

			if output == "" {
				klog.V(100).Infof("The command execution output is empty %q", output)

				return false, nil
			}

			klog.V(100).Infof("The command execution output:\n\t%v", output)

			return true, nil
		})
	if err != nil {
		return false, fmt.Errorf("failed to run command from within pod %s: %w", podObj.Object.Name, err)
	}

	if ipv4Addr != "" {
		klog.V(100).Infof("Ensure IPv4 %s address defined as expected", ipv4Addr)

		ipv4Found, err := findInCmdExecOutput(output, ipv4Addr)
		if err != nil {
			klog.V(100).Infof("Failed to parse output due to %v", err)

			return false, fmt.Errorf("failed to parse output due to %w", err)
		}

		if !ipv4Found {
			klog.V(100).Infof("IPv4 address %s not found configured for the bond interface "+
				"of the pod %s in namespace %s: %s",
				ipv4Addr, podObj.Definition.Name, podObj.Definition.Namespace, output)

			return false, fmt.Errorf("ipv4 address %s not found configured for the bond interface "+
				"of the pod %s in namespace %s: %s",
				ipv4Addr, podObj.Definition.Name, podObj.Definition.Namespace, output)
		}
	}

	if ipv6Addr != "" {
		klog.V(100).Infof("Ensure IPv6 %s address defined as expected", ipv6Addr)

		ipv6Found, err := findInCmdExecOutput(output, ipv6Addr)
		if err != nil {
			klog.V(100).Infof("Failed to parse output due to %v", err)

			return false, fmt.Errorf("failed to parse output due to %w", err)
		}

		if !ipv6Found {
			klog.V(100).Infof("IPv6 address %s not found configured for the bond interface "+
				"of the pod %s in namespace %s: %s",
				ipv6Addr, podObj.Definition.Name, podObj.Definition.Namespace, output)

			return false, fmt.Errorf("ipv6 address %s not found configured for the bond interface "+
				"of the pod %s in namespace %s: %s",
				ipv6Addr, podObj.Definition.Name, podObj.Definition.Namespace, output)
		}
	}

	return true, nil
}

func getPodObjectByNamePattern(apiClient *clients.Settings, podNamePattern, podNamespace string) (*pod.Builder, error) {
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

			// Filter out pods that are not running or marked for deletion
			var runningPods []*pod.Builder

			for _, _pod := range podObjList {
				switch {
				case _pod.Object.Status.Phase == corev1.PodRunning && _pod.Object.DeletionTimestamp == nil:
					klog.V(100).Infof("Pod %q is active (running and not marked for deletion)",
						_pod.Definition.Name)

					runningPods = append(runningPods, _pod)
				case _pod.Object.DeletionTimestamp != nil:
					klog.V(100).Infof("Pod %q is marked for deletion, skipping", _pod.Definition.Name)
				default:
					klog.V(100).Infof("Pod %q is in %q phase, skipping", _pod.Definition.Name, _pod.Object.Status.Phase)
				}
			}

			switch {
			case len(runningPods) == 0:
				klog.V(100).Infof("No running pods %s were found in namespace %q", podNamePattern, podNamespace)

				return false, nil
			case len(runningPods) > 1:
				klog.V(100).Infof("Wrong pod %s count %d was found in namespace %q",
					podNamePattern, len(runningPods), podNamespace)

				unexpectedRunningPods := make([]string, 0, len(runningPods))
				for _, _pod := range runningPods {
					unexpectedRunningPods = append(unexpectedRunningPods, _pod.Definition.Name)
				}

				klog.V(100).Infof("Unexpected running pods %v were found in namespace %q",
					unexpectedRunningPods, podNamespace)

				return false, nil
			}

			podObj = runningPods[0]

			klog.V(100).Infof("Selected pod %q in namespace %q", podObj.Definition.Name, podObj.Definition.Namespace)

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to retrieve pod %q in namespace %q: %v",
			podNamePattern, podNamespace, err)

		return nil, fmt.Errorf("failed to retrieve pod %q in namespace %q: %w",
			podNamePattern, podNamespace, err)
	}

	err = podObj.WaitUntilReady(time.Second * 30)
	if err != nil {
		klog.V(100).Infof("The pod-level bonded pod %s in namespace %s is not in Ready state: %v",
			podNamePattern, podNamespace, err)

		return nil, fmt.Errorf("the pod-level bonded pod %s in namespace %s is not in Ready state: %w",
			podNamePattern, podNamespace, err)
	}

	return podObj, nil
}

func getBondActiveInterfaceSrIovNetworkName(podObj *pod.Builder) (string, error) {
	podNetAnnotation := podObj.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]

	klog.V(100).Infof("%q pod's network annotation: %q", podObj.Definition.Name, podNetAnnotation)

	activeInterfaceName, err := getBondActiveInterface(podObj)
	if err != nil {
		klog.V(100).Infof("No active interface found for the pod %s in namespace %s: %v",
			podObj.Definition.Name, podObj.Definition.Namespace, err)

		return "", fmt.Errorf("no active interface found for the pod %s in namespace %s: %w",
			podObj.Definition.Name, podObj.Definition.Namespace, err)
	}

	var podNetworkStatusType []podNetworkAnnotation

	err = json.Unmarshal([]byte(podNetAnnotation), &podNetworkStatusType)
	if err != nil {
		klog.V(100).Infof("Error unmarshalling pod network status annotation %q: %v", podNetAnnotation, err)

		return "", fmt.Errorf("error unmarshalling pod network status annotation %q: %w", podNetAnnotation, err)
	}

	for _, networkAnnotation := range podNetworkStatusType {
		klog.V(100).Infof("Comparing network interface: %q with active interface: %q",
			networkAnnotation.Interface, activeInterfaceName)

		if networkAnnotation.Interface == activeInterfaceName {
			klog.V(100).Infof("Found sriov network name for the active interface %s in pod %s "+
				"in namespace %s: %s",
				activeInterfaceName, podObj.Object.Name, podObj.Object.Namespace, networkAnnotation.Name)

			netName := strings.Split(networkAnnotation.Name, "/")[1]

			return netName, nil
		}
	}

	klog.V(100).Infof("Failed to find sriov network name for the active interface %s in pod %s "+
		"in namespace %s: %v", activeInterfaceName, podObj.Object.Name, podObj.Object.Namespace, podNetworkStatusType)

	return "", fmt.Errorf("failed to find sriov network name for the active interface %s in pod %s "+
		"in namespace %s: %v", activeInterfaceName, podObj.Object.Name, podObj.Object.Namespace, podNetworkStatusType)
}

//nolint:funlen
func verifyPodLevelBondWorkloads(
	clientDeploymentName,
	clientDeploymentNamespace,
	serverDeploymentName,
	serverDeploymentNamespace,
	clientIPv4,
	clientIPv6,
	serverIPv4,
	serverIPv6 string) {
	Expect(clientIPv4 == "" && clientIPv6 == "").ToNot(BeTrue(),
		"The client IPv4 and client IPv6 should not be empty")
	Expect(serverIPv4 == "" && serverIPv6 == "").ToNot(BeTrue(),
		"The server IPv4 and server IPv6 should not be empty")

	By("Ensure client/server pod deployment succeeded and get pods names")

	clientPodObj, err := getPodObjectByNamePattern(APIClient, clientDeploymentName, clientDeploymentNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve client pod-level bond %s object from namespace %s: %v",
			clientDeploymentName, clientDeploymentNamespace, err))

	serverPodObj, err := getPodObjectByNamePattern(APIClient, serverDeploymentName, serverDeploymentNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve server pod-level bond %s object from namespace %s: %v",
			serverDeploymentName, serverDeploymentNamespace, err))

	By("Inspecting bonded interface within the client pod")

	isFound, err := inspectPodLevelBondedInterfaceConfig(clientPodObj, clientIPv4, clientIPv6)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to verify client pod-level bond pod %s from namespace %s config: %v",
			clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, err))
	Expect(isFound).To(Equal(true),
		fmt.Sprintf("The pod-level bonded interface for the pod %s in namespace %s not as expected",
			clientPodObj.Definition.Name, clientPodObj.Definition.Namespace))

	By("Inspecting bonded interface within the server pod")

	isFound, err = inspectPodLevelBondedInterfaceConfig(serverPodObj, serverIPv4, serverIPv6)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to verify server pod-level bond pod %s from namespace %s config: %v",
			serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, err))
	Expect(isFound).To(Equal(true),
		fmt.Sprintf("The pod-level bonded interface for the pod %s in namespace %s not as expected",
			serverPodObj.Definition.Name, serverPodObj.Definition.Namespace))

	if serverIPv4 != "" {
		By("Send data from the client container to the IPv4 address used by the server container")

		output, err := generateTCPTraffic(clientPodObj, serverIPv4, RDSCoreConfig.PodLevelBondPort, "2", "5")
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to generate TCP traffic from the pod %s in namespace %s to the server %s: %v",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, serverIPv4, err))

		testPassed, err := scanClientPodTrafficOutput(output)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to parse client pod %s from namespace %s output: %v",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, err))
		Expect(testPassed).To(Equal(true),
			fmt.Sprintf("TCP traffic test verification failed for the pod %s in namespace %s; output %s",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, output))
	}

	if serverIPv6 != "" {
		By("Send data from the client container to the IPv6 address used by the server container")

		output, err := generateTCPTraffic(clientPodObj, serverIPv6, RDSCoreConfig.PodLevelBondPort, "2", "5")
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to generate TCP traffic from the pod %s in namespace %s to the server %s: %v",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, serverIPv6, err))

		testPassed, err := scanClientPodTrafficOutput(output)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to parse client pod %s from namespace %s output: %v",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, err))
		Expect(testPassed).To(Equal(true),
			fmt.Sprintf("TCP traffic test verification failed for the pod %s in namespace %s; output %s",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, output))
	}

	if clientIPv4 != "" {
		By("Send data from the server container to the IPv4 address used by the client container")

		output, err := generateTCPTraffic(serverPodObj, clientIPv4, RDSCoreConfig.PodLevelBondPort, "2", "5")
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to generate TCP traffic from the pod %s in namespace %s to the server %s: %v",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, clientIPv4, err))

		testPassed, err := scanClientPodTrafficOutput(output)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to parse server pod %s from namespace %s output: %v",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, err))
		Expect(testPassed).To(Equal(true),
			fmt.Sprintf("TCP traffic test verification failed for the pod %s in namespace %s; output %s",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, output))
	}

	if clientIPv6 != "" {
		By("Send data from the client container to the IPv6 address used by the server container")

		output, err := generateTCPTraffic(serverPodObj, clientIPv6, RDSCoreConfig.PodLevelBondPort, "2", "5")
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to generate TCP traffic from the pod %s in namespace %s to the server %s: %v",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, clientIPv6, err))

		testPassed, err := scanClientPodTrafficOutput(output)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to parse server pod %s from namespace %s output: %v",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, err))
		Expect(testPassed).To(Equal(true),
			fmt.Sprintf("TCP traffic test verification failed for the pod %s in namespace %s; output %s",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, output))
	}
}

func prepareSecondPodLevelBondDeployment(sameNode, samePF bool) {
	By("Create privileged pod-level bond deployment")

	klog.V(100).Infof("Retrieve client pod-level bond pod %s from the namespace %s",
		RDSCoreConfig.PodLevelBondDeploymentOneName, RDSCoreConfig.PodLevelBondNamespace)

	clientPod, err := getPodObjectByNamePattern(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentOneName,
		RDSCoreConfig.PodLevelBondNamespace)

	if err != nil || clientPod == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod-Level Bond client deployment not found")

		Skip("Pod-Level Bond client deployment not found. Skipping...")
	}

	var schedulerOnHost string

	if sameNode && clientPod.Object.Spec.NodeName == RDSCoreConfig.PodLevelBondPodOneScheduleOnHost ||
		!sameNode && clientPod.Object.Spec.NodeName == RDSCoreConfig.PodLevelBondPodTwoScheduleOnHost {
		schedulerOnHost = RDSCoreConfig.PodLevelBondPodOneScheduleOnHost
	}

	if !sameNode && clientPod.Object.Spec.NodeName == RDSCoreConfig.PodLevelBondPodOneScheduleOnHost ||
		sameNode && clientPod.Object.Spec.NodeName == RDSCoreConfig.PodLevelBondPodTwoScheduleOnHost {
		schedulerOnHost = RDSCoreConfig.PodLevelBondPodTwoScheduleOnHost
	}

	Expect(schedulerOnHost).ToNot(Equal(""),
		fmt.Sprintf("Failed to setup schedulerOnHost value; client pod found: /n%q", clientPod.Definition))

	klog.V(100).Infof("Setup server deployment sriov networks")

	var netOne, netTwo string

	clientSRIOVNetForActiveInterface, err := getBondActiveInterfaceSrIovNetworkName(clientPod)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to find SRIOV network name for the active client interface: %v", err))

	if samePF && clientSRIOVNetForActiveInterface == RDSCoreConfig.PodLevelBondSRIOVNetOne ||
		!samePF && clientSRIOVNetForActiveInterface == RDSCoreConfig.PodLevelBondSRIOVNetTwo {
		netOne = RDSCoreConfig.PodLevelBondSRIOVNetOne
		netTwo = RDSCoreConfig.PodLevelBondSRIOVNetTwo
	}

	if !samePF && clientSRIOVNetForActiveInterface == RDSCoreConfig.PodLevelBondSRIOVNetOne ||
		samePF && clientSRIOVNetForActiveInterface == RDSCoreConfig.PodLevelBondSRIOVNetTwo {
		netOne = RDSCoreConfig.PodLevelBondSRIOVNetTwo
		netTwo = RDSCoreConfig.PodLevelBondSRIOVNetOne
	}

	Expect(netOne).ToNot(Equal(""),
		fmt.Sprintf("Failed to setup SRIOV networks values; client pod found: /n%q", clientPod.Definition))

	Expect(netTwo).ToNot(Equal(""),
		fmt.Sprintf("Failed to setup SRIOV networks values; client pod found: /n%q", clientPod.Definition))

	err = createPrivilegedPodLevelBondDeployment(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentTwoName,
		RDSCoreConfig.PodLevelBondNamespace,
		podLevelBondPodLabel,
		schedulerOnHost,
		netOne,
		netTwo,
		podLevelBondNetName,
		RDSCoreConfig.PodLevelBondDeploymentTwoIPv4,
		RDSCoreConfig.PodLevelBondDeploymentTwoIPv6,
		RDSCoreConfig.PodLevelBondPodSubnetMaskIPv4,
		RDSCoreConfig.PodLevelBondPodSubnetMaskIPv6,
		RDSCoreConfig.PodLevelBondPodMacAddr)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create priviledged pod-level bond deployment: %v", err))
}

func verifyConnectivity() {
	verifyPodLevelBondWorkloads(
		RDSCoreConfig.PodLevelBondDeploymentOneName,
		RDSCoreConfig.PodLevelBondNamespace,
		RDSCoreConfig.PodLevelBondDeploymentTwoName,
		RDSCoreConfig.PodLevelBondNamespace,
		RDSCoreConfig.PodLevelBondDeploymentOneIPv4,
		RDSCoreConfig.PodLevelBondDeploymentOneIPv6,
		RDSCoreConfig.PodLevelBondDeploymentTwoIPv4,
		RDSCoreConfig.PodLevelBondDeploymentTwoIPv6)
}

// VerifyPodLevelBondWorkloadsOnSameNodeSamePF verifies TCP traffic works on the same node and different PFs.
func VerifyPodLevelBondWorkloadsOnSameNodeSamePF() {
	prepareSecondPodLevelBondDeployment(true, true)

	verifyConnectivity()
}

// VerifyPodLevelBondWorkloadsOnSameNodeDifferentPFs verifies TCP traffic works on the same node and different PFs.
func VerifyPodLevelBondWorkloadsOnSameNodeDifferentPFs() {
	prepareSecondPodLevelBondDeployment(true, false)

	verifyConnectivity()
}

// VerifyPodLevelBondWorkloadsOnDifferentNodesSamePF verifies TCP traffic works on the different nodes and same PF.
func VerifyPodLevelBondWorkloadsOnDifferentNodesSamePF() {
	prepareSecondPodLevelBondDeployment(false, true)

	verifyConnectivity()
}

// VerifyPodLevelBondWorkloadsOnDifferentNodesDifferentPFs verifies TCP traffic works on the
// different nodes and different PFs.
func VerifyPodLevelBondWorkloadsOnDifferentNodesDifferentPFs() {
	prepareSecondPodLevelBondDeployment(false, false)

	verifyConnectivity()
}

// VerifyPodLevelBondWorkloadsAfterVFFailOver verifies TCP traffic after bond active interface failure
// (fail-over procedure).
func VerifyPodLevelBondWorkloadsAfterVFFailOver() {
	prepareSecondPodLevelBondDeployment(false, true)

	verifyConnectivity()

	By("Retrieve client pod-level bond pod object")

	clientPodObj, err := getPodObjectByNamePattern(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentOneName,
		RDSCoreConfig.PodLevelBondNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve client pod-level bond %s object from namespace %s: %v",
			RDSCoreConfig.PodLevelBondDeploymentOneName, RDSCoreConfig.PodLevelBondNamespace, err))

	By("Retrieve server pod-level bond pod object")

	serverPodObj, err := getPodObjectByNamePattern(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentTwoName,
		RDSCoreConfig.PodLevelBondNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve server pod-level bond %s object from namespace %s: %v",
			RDSCoreConfig.PodLevelBondDeploymentOneName, RDSCoreConfig.PodLevelBondNamespace, err))

	By(fmt.Sprintf("Getting bond's active interface for pod %q in namespace %q",
		serverPodObj.Definition.Name, serverPodObj.Definition.Namespace))

	activeInf, err := getBondActiveInterface(serverPodObj)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve bond active interface for the pod deployment %s in namespace %s: %v",
			serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, err))

	go func() {
		By("Send data from the client container to the IPv4 address used by the server container")

		output, err := generateTCPTraffic(
			clientPodObj,
			RDSCoreConfig.PodLevelBondDeploymentTwoIPv4,
			RDSCoreConfig.PodLevelBondPort,
			"10",
			"5")
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to generate TCP traffic from the pod %s in namespace %s to the server %s: %v",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace,
				RDSCoreConfig.PodLevelBondDeploymentTwoIPv4, err))

		testPassed, err := scanClientPodTrafficOutput(output)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to parse client pod %s from namespace %s output: %v",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, err))
		Expect(testPassed).To(Equal(true),
			fmt.Sprintf("TCP traffic test verification failed for the pod %s in namespace %s; output %s",
				clientPodObj.Definition.Name, clientPodObj.Definition.Namespace, output))
	}()

	go func() {
		time.Sleep(time.Second * 2)

		err = disableBondActiveVFInterface(serverPodObj)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to disable bond active interface for the pod %s in namespace %s: %v",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, err))
	}()

	var ctx SpecContext

	Eventually(func() bool {
		newActiveInf, err := getBondActiveInterface(serverPodObj)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to retrieve new bond active interface for the pod %s in namespace %s: %v",
				serverPodObj.Definition.Name, serverPodObj.Definition.Namespace, err)

			return false
		}

		if newActiveInf == activeInf {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"The bond active interface did not changed yet %q", newActiveInf)

			return false
		}

		return true
	}).WithContext(ctx).WithPolling(time.Second).WithTimeout(30*time.Second).Should(BeTrue(),
		"Fail-Over procedure failure; failed to switch to the new bond active interface")

	verifyConnectivity()
}

// VerifyPodLevelBondWorkloadsAfterBondInterfaceFailure verifies TCP traffic after pod bonded interface
// recovering after failure.
func VerifyPodLevelBondWorkloadsAfterBondInterfaceFailure() {
	prepareSecondPodLevelBondDeployment(false, true)

	verifyConnectivity()

	By("Retrieve tested pod-level bond pod object")

	testPodObj, err := getPodObjectByNamePattern(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentTwoName,
		RDSCoreConfig.PodLevelBondNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve test pod-level bond pod %s object from namespace %s: %v",
			RDSCoreConfig.PodLevelBondDeploymentTwoName, RDSCoreConfig.PodLevelBondNamespace, err))

	By("Disable tested pod bond interface")

	err = changeInterfaceState(testPodObj, "net3", true)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to disable interface net3 for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	By("Enable tested pod bond interface")

	err = changeInterfaceState(testPodObj, "net3", false)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to enable interface net3 for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	activeInf, err := getBondActiveInterface(testPodObj)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve bond active interface for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	klog.V(100).Infof("DEBUG LOG: active interface found - %s", activeInf)

	verifyConnectivity()
}

// VerifyPodLevelBondWorkloadsAfterBothVFsFailure verifies TCP traffic after bond
// interface recovering after both VFs failure.
func VerifyPodLevelBondWorkloadsAfterBothVFsFailure() {
	prepareSecondPodLevelBondDeployment(false, true)

	verifyConnectivity()

	By("Retrieve tested pod-level bond pod object")

	testPodObj, err := getPodObjectByNamePattern(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentTwoName,
		RDSCoreConfig.PodLevelBondNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve test pod-level bond pod %s object from namespace %s: %v",
			RDSCoreConfig.PodLevelBondDeploymentTwoName, RDSCoreConfig.PodLevelBondNamespace, err))

	By("Disable first VF interface (net1)")

	err = changeInterfaceState(testPodObj, "net1", true)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to disable interface net1 for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	By("Disable second VF interface (net2)")

	err = changeInterfaceState(testPodObj, "net2", true)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to disable interface net2 for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	time.Sleep(time.Second * 3)

	By("Enable first VF interface (net1)")

	err = changeInterfaceState(testPodObj, "net1", false)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to enable interface net1 for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	By("Enable second VF interface (net2)")

	err = changeInterfaceState(testPodObj, "net2", false)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to disable interface net2 for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	activeInf, err := getBondActiveInterface(testPodObj)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve bond active interface for the pod deployment %s in namespace %s: %v",
			testPodObj.Definition.Name, testPodObj.Definition.Namespace, err))

	klog.V(100).Infof("DEBUG LOG: active interface found - %s", activeInf)

	verifyConnectivity()
}

// VerifyPodLevelBondWorkloadsAfterPodCrashing verifies TCP traffic works after pod crashing.
func VerifyPodLevelBondWorkloadsAfterPodCrashing() {
	prepareSecondPodLevelBondDeployment(false, true)

	verifyConnectivity()

	By("Retrieve tested pod-level bond pod object")

	testPodObj, err := getPodObjectByNamePattern(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentTwoName,
		RDSCoreConfig.PodLevelBondNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve test pod-level bond pod %s object from namespace %s: %v",
			RDSCoreConfig.PodLevelBondDeploymentTwoName, RDSCoreConfig.PodLevelBondNamespace, err))

	By("Delete test pod")

	_, err = testPodObj.DeleteAndWait(time.Second * 30)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to delete test pod-level bond pod %s object from namespace %s: %v",
			RDSCoreConfig.PodLevelBondDeploymentTwoName, RDSCoreConfig.PodLevelBondNamespace, err))

	By("Wait new test pod-level bond pod will be created")

	_, err = getPodObjectByNamePattern(
		APIClient,
		RDSCoreConfig.PodLevelBondDeploymentTwoName,
		RDSCoreConfig.PodLevelBondNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve test pod-level bond pod %s object from namespace %s: %v",
			RDSCoreConfig.PodLevelBondDeploymentTwoName, RDSCoreConfig.PodLevelBondNamespace, err))

	verifyConnectivity()
}
