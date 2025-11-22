package rdscorecommon

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	scc "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/scc"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/egressip"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	deployRBACRole       = "system:openshift:scc:privileged"
	nonEgressIPPodLabel  = "env=centos"
	nonEgressIPNamespace = "non-egressip-ns"
)

var (
	egressIPPodLabelsMap = map[string]string{
		strings.Split(RDSCoreConfig.EgressIPPodLabel, "=")[0]: strings.Split(RDSCoreConfig.EgressIPPodLabel, "=")[1]}

	egressIPPodSelector = metav1.ListOptions{
		LabelSelector: RDSCoreConfig.EgressIPPodLabel,
	}
	nonEgressIPPodSelector = metav1.ListOptions{
		LabelSelector: nonEgressIPPodLabel,
	}
)

// validateRemoteIPAddress validates remote IP address is set.
func validateRemoteIPAddress(ipv6 bool) {
	klog.V(100).Infof("Validating remote IP address")

	switch {
	case ipv6 && RDSCoreConfig.EgressIPRemoteIPv6 == "":
		Skip("Remote IPv6 address(EgressIPRemoteIPv6) is not set")
	case !ipv6 && RDSCoreConfig.EgressIPRemoteIPv4 == "":
		Skip("Remote IPv4 address(EgressIPRemoteIPv4) is not set")
	}
}

// createAgnhostRBAC creates the SCC privileged, serviceAccount and RBAC.
func createAgnhostRBAC(
	apiClient *clients.Settings,
	egressIPNamespace string) (string, error) {
	deploySAName := fmt.Sprintf("rdscore-egressip-sa-%s", egressIPNamespace)
	deployRBACName := fmt.Sprintf("privileged-rdscore-rbac-%s", egressIPNamespace)

	var err error

	klog.V(100).Infof("Adding SCC privileged to the agnhost namespace")

	err = scc.AddPrivilegedSCCtoDefaultSA(egressIPNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to add SCC privileged to the agnhost namespace %s: %w",
			egressIPNamespace, err)
	}

	klog.V(100).Infof("Removing ServiceAccount")

	err = apiobjectshelper.DeleteServiceAccount(apiClient, deploySAName, egressIPNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to remove serviceAccount %q from egressIPNamespace %q",
			deploySAName, egressIPNamespace)
	}

	klog.V(100).Infof("Creating ServiceAccount")

	err = apiobjectshelper.CreateServiceAccount(apiClient, deploySAName, egressIPNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to create serviceAccount %q in egressIPNamespace %q",
			deploySAName, egressIPNamespace)
	}

	klog.V(100).Infof("Removing Cluster RBAC")

	err = apiobjectshelper.DeleteClusterRBAC(apiClient, deployRBACName)
	if err != nil {
		return "", fmt.Errorf("failed to delete deployment RBAC %q", deployRBACName)
	}

	klog.V(100).Infof("Creating Cluster RBAC")

	err = apiobjectshelper.CreateClusterRBAC(apiClient, deployRBACName, deployRBACRole,
		deploySAName, egressIPNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to create deployment RBAC %q in egressIPNamespace %s",
			deployRBACName, egressIPNamespace)
	}

	return deploySAName, nil
}

// cleanUpDeployments removes all qe deployments leftovers from the cluster according to the label.
func cleanUpDeployments(apiClient *clients.Settings) error {
	namespacesList := []string{RDSCoreConfig.EgressIPNamespaceOne, RDSCoreConfig.EgressIPNamespaceTwo,
		nonEgressIPNamespace}
	podLabelsList := []string{RDSCoreConfig.EgressIPPodLabel, nonEgressIPPodLabel}

	for _, nsName := range namespacesList {
		deploymentsList, err := deployment.List(APIClient, nsName)
		if err != nil {
			klog.V(100).Infof("Error listing deployments in the namespace %s, %v", nsName, err)

			return err
		}

		for _, deploy := range deploymentsList {
			klog.V(100).Infof("Ensure %s deploy don't exist in namespace %s",
				deploy.Definition.Name, deploy.Definition.Namespace)

			err := apiobjectshelper.DeleteDeployment(
				apiClient,
				deploy.Definition.Name,
				nsName)
			if err != nil {
				return fmt.Errorf("failed to delete deploy %s from nsname %s",
					deploy.Definition.Name, nsName)
			}
		}

		for _, label := range podLabelsList {
			err = apiobjectshelper.EnsureAllPodsRemoved(APIClient, nsName, label)
			if err != nil {
				return fmt.Errorf("failed to delete pods in namespace %s: %w", nsName, err)
			}
		}
	}

	return nil
}

// createAgnhostDeployment creates the agnhost deployment that will be used as a source for EgressIP tests.
func createAgnhostDeployment(
	apiClient *clients.Settings,
	nsName,
	deploySAName,
	scheduleOnHost string,
	podLabelsMap map[string]string) error {
	replicaCnt := 1
	nameSuffix := strings.Split(scheduleOnHost, ".")[0]
	randSuffix := randomString(3)
	deploymentName := fmt.Sprintf("%s-agnhost-%s-%s", nsName, nameSuffix, randSuffix)

	var err error

	klog.V(100).Infof("Defining container configuration")

	containerCmd := []string{
		"/agnhost",
		"netexec",
		"--http-port",
		RDSCoreConfig.EgressIPTcpPort,
	}

	deployContainer := defineDeployContainer(RDSCoreConfig.EgressIPDeploymentImage, containerCmd)

	klog.V(100).Infof("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	if err != nil {
		return fmt.Errorf("failed to obtain container definition: %w", err)
	}

	klog.V(100).Infof("Defining deployment configuration")

	testPodDeployment := defineTestPodDeployment(
		apiClient,
		deployContainerCfg,
		deploymentName,
		nsName,
		deploySAName,
		scheduleOnHost,
		replicaCnt,
		podLabelsMap)

	klog.V(100).Infof("Creating deployment")

	testPodDeployment, err = testPodDeployment.CreateAndWaitUntilReady(5 * time.Minute)
	if err != nil {
		klog.V(100).Infof("Failed to create deployment %s in namespace %s: %v",
			deploymentName, nsName, err)

		return fmt.Errorf("failed to create deployment %s in namespace %s: %w",
			deploymentName, nsName, err)
	}

	if testPodDeployment == nil {
		return fmt.Errorf("failed to create deployment %s in namespace %s",
			deploymentName, nsName)
	}

	return nil
}

func randomString(length int) string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"

	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}

	return string(result)
}

func defineTestPodDeployment(
	apiClient *clients.Settings,
	containerConfig *corev1.Container,
	deployName, deployNs, saName string,
	scheduleOnHost string,
	replicaCnt int,
	deployLabels map[string]string) *deployment.Builder {
	klog.V(100).Infof("Defining deployment %q in %q ns", deployName, deployNs)

	nodeAffinity := corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{scheduleOnHost},
							},
						},
					},
				},
			},
		},
	}

	podsDeployment := deployment.NewBuilder(apiClient, deployName, deployNs, deployLabels, *containerConfig)

	klog.V(100).Infof("Assigning ServiceAccount %q to the deployment", saName)

	podsDeployment = podsDeployment.WithServiceAccountName(saName)

	klog.V(100).Infof("Setting Replicas count")

	podsDeployment = podsDeployment.WithReplicas(int32(replicaCnt))

	podsDeployment = podsDeployment.WithHostNetwork(false)

	podsDeployment = podsDeployment.WithAffinity(&nodeAffinity)

	return podsDeployment
}

func defineDeployContainer(cImage string, cCmd []string) *pod.ContainerBuilder {
	cName := "agnhost"

	klog.V(100).Infof("Creating container %q", cName)

	deployContainer := pod.NewContainerBuilder(cName, cImage, cCmd)

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

	deployContainer = deployContainer.WithSecurityContext(securityContext)

	klog.V(100).Infof("Dropping ALL security capability")

	deployContainer = deployContainer.WithDropSecurityCapabilities([]string{"ALL"}, true)

	klog.V(100).Infof("%q container's  definition:\n%#v", cName, deployContainer)

	return deployContainer
}

func sendTrafficCheckIP(clientPods []*pod.Builder, isIPv6 bool, expectedIPs []string) error {
	By("Validating pods source address")

	targetIP := RDSCoreConfig.EgressIPRemoteIPv4

	if isIPv6 {
		targetIP = fmt.Sprintf("[%s]", RDSCoreConfig.EgressIPRemoteIPv6)
	}

	cmdToRun := []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 5 -Ls http://%s:%s/clientip",
			targetIP, RDSCoreConfig.EgressIPTcpPort)}

	klog.V(100).Infof("Execute command: %q", cmdToRun)

	for _, clientPod := range clientPods {
		var parsedIP string

		klog.V(100).Infof("Wait 5 minutes for pod %q to be Ready", clientPod.Definition.Name)

		err := clientPod.WaitUntilReady(5 * time.Minute)

		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q in %q namespace is not Ready",
			clientPod.Definition.Name, clientPod.Definition.Namespace))

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second*5,
			time.Minute*1,
			true,
			func(ctx context.Context) (bool, error) {
				result, err := clientPod.ExecCommand(cmdToRun, clientPod.Object.Spec.Containers[0].Name)
				if err != nil {
					klog.V(100).Infof("Error running command from within a pod %q: %v",
						clientPod.Object.Name, err)

					return false, nil
				}

				klog.V(100).Infof("Successfully executed command from within a pod %q: %v",
					clientPod.Object.Name, err)
				klog.V(100).Infof("Command's output:\n\t%v", result.String())

				parsedIP, _, err = net.SplitHostPort(result.String())
				if err != nil {
					klog.V(100).Infof("Failed to parse %q for host/port pair", result.String())

					return false, nil
				}

				klog.V(100).Infof("Verify IP version type correctness")

				myIP, err := netip.ParseAddr(parsedIP)
				if err != nil {
					klog.V(100).Infof("Failed to parse used ip address %q", parsedIP)

					return false, nil
				}

				if isIPv6 && myIP.Is4() || !isIPv6 && myIP.Is6() {
					klog.V(100).Infof("Wrong IP version detected; %q", parsedIP)

					return false, nil
				}

				klog.V(100).Infof("Comparing %q with expected %q", parsedIP, expectedIPs)

				for _, expectedIP := range expectedIPs {
					if parsedIP == expectedIP {
						return true, nil
					}
				}

				klog.V(100).Infof("Mismatched IP address. Expected %q got %q", expectedIPs, parsedIP)

				return false, nil
			})
		if err != nil {
			return fmt.Errorf("failed to run command from within pod %s: %w", clientPod.Object.Name, err)
		}
	}

	return nil
}

func getEgressIPMap() (map[string]string, error) {
	By("Getting a map of source nodes and assigned Egress IPs for these nodes")

	egressIPObj, err := egressip.Pull(APIClient, RDSCoreConfig.EgressIPName)
	if err != nil {
		klog.V(100).Infof("Failed to pull egressIP %q object: %v", RDSCoreConfig.EgressIPName, err)

		return nil, fmt.Errorf("failed to retrieve egressIP %s object: %w", RDSCoreConfig.EgressIPName, err)
	}

	egressIPMap, err := egressIPObj.GetAssignedEgressIPMap()
	if err != nil {
		klog.V(100).Infof("Failed to retrieve egressIP %s assigned egressIPs map: %v",
			RDSCoreConfig.EgressIPName, err)

		return nil, fmt.Errorf("failed to retrieve egressIP %s assigned egressIPs map: %w",
			RDSCoreConfig.EgressIPName, err)
	}

	if len(egressIPMap) == 0 {
		return nil, fmt.Errorf("configuration failure - EgressIP doesn't have IP addresses assigned to the nodes: "+
			"%v", egressIPMap)
	}

	return egressIPMap, nil
}

func getEgressIPList() ([]string, error) {
	By("Build an EgressIP list")

	var eIPList []string

	egressIPMap, err := getEgressIPMap()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve EgressIP map due to %w", err)
	}

	for eIPNodeName, eIPValue := range egressIPMap {
		if eIPValue != RDSCoreConfig.EgressIPv4 && eIPValue != RDSCoreConfig.EgressIPv6 {
			return nil, fmt.Errorf("EgressIP address assigned to the node %s not correctly configured; "+
				"current value: %s, expected values to be set %s or %s", eIPNodeName, eIPValue,
				RDSCoreConfig.EgressIPv4, RDSCoreConfig.EgressIPv6)
		}

		eIPList = append(eIPList, eIPValue)
	}

	return eIPList, nil
}

func verifyEgressIPConnectivityBalancedTraffic(isIPv6 bool) {
	By("Validating remote IP address is set")

	validateRemoteIPAddress(isIPv6)

	By("Verifying egress IP connectivity balanced traffic; single namespace")

	expectedIPs, err := getEgressIPList()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve configured EgressIP addresses list from the egressIP %s: %v",
			RDSCoreConfig.EgressIPName, err))

	podObjects, err := pod.List(APIClient, RDSCoreConfig.EgressIPNamespaceOne, egressIPPodSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve pods list from namespace %s with label %v: %v",
			RDSCoreConfig.EgressIPNamespaceOne, egressIPPodSelector, err))

	err = sendTrafficCheckIP(podObjects, isIPv6, expectedIPs)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Server response was note received: %v", err))
}

func verifyEgressIPConnectivityThreeNodes(isIPv6 bool) {
	By("Validating remote IP address is set")

	validateRemoteIPAddress(isIPv6)

	By("Verifying egress IP connectivity for the mixed nodes and namespaces")

	expectedIPs, err := getEgressIPList()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve configured EgressIP addresses list from the egressIP %s: %v",
			RDSCoreConfig.EgressIPName, err))

	podObjects, err := pod.List(APIClient, RDSCoreConfig.EgressIPNamespaceOne, egressIPPodSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve pods list from namespace %s with label %s: %v",
			RDSCoreConfig.EgressIPNamespaceOne, RDSCoreConfig.EgressIPPodLabel, err))

	err = sendTrafficCheckIP(podObjects, isIPv6, expectedIPs)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Server response was note received: %v", err))

	podObjects, err = pod.List(APIClient, RDSCoreConfig.EgressIPNamespaceTwo, egressIPPodSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve pods list from namespace %s with label %s: %v",
			RDSCoreConfig.EgressIPNamespaceTwo, RDSCoreConfig.EgressIPPodLabel, err))

	err = sendTrafficCheckIP(podObjects, isIPv6, expectedIPs)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Server response was not received: %v", err))
}

//nolint:funlen, gocognit
func gracefulNodeReboot(nodeName string) error {
	By("Execute graceful node reboot")

	nodeObj, err := nodes.Pull(APIClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to retrieve node %s object due to: %w", nodeName, err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cordoning node %q", nodeName)

	err = nodeObj.Cordon()
	if err != nil {
		klog.V(100).Infof("Failed to cordon node %q due to %v", nodeName, err)

		return fmt.Errorf("failed to cordon node %q due to %w", nodeName, err)
	}

	time.Sleep(5 * time.Second)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Draining node %q", nodeName)

	err = nodeObj.Drain()
	if err != nil {
		klog.V(100).Infof("Failed to drain node %q due to %v", nodeName, err)

		return fmt.Errorf("failed to drain node %q due to %w", nodeName, err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(
		fmt.Sprintf("NodesCredentialsMap:\n\t%#v", RDSCoreConfig.NodesCredentialsMap))

	var bmcClient *bmc.BMC

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(
		fmt.Sprintf("Creating BMC client for node %s", nodeName))

	if auth, ok := RDSCoreConfig.NodesCredentialsMap[nodeName]; !ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			fmt.Sprintf("BMC Details for %q not found", nodeName))
		Fail(fmt.Sprintf("BMC Details for %q not found", nodeName))
	} else {
		bmcClient = bmc.New(auth.BMCAddress).
			WithRedfishUser(auth.Username, auth.Password).
			WithRedfishTimeout(6 * time.Minute)
	}

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*5,
		time.Minute*5,
		true,
		func(ctx context.Context) (bool, error) {
			if err := bmcClient.SystemForceReset(); err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Info(
					fmt.Sprintf("Failed to power cycle %s -> %v", nodeName, err))

				return false, nil
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Info(
				fmt.Sprintf("Successfully powered cycle %s", nodeName))

			return true, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to reboot node %s", nodeName)

		return fmt.Errorf("failed to reboot node %s", nodeName)
	}

	By(fmt.Sprintf("Checking node %s got into NotReady", nodeName))

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute*25,
		true,
		func(ctx context.Context) (bool, error) {
			currentNode, err := nodes.Pull(APIClient, nodeName)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull node: %v", err)

				return false, nil
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString {
					if condition.Status != rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is notReady", currentNode.Definition.Name)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

						return true, nil
					}
				}
			}

			return false, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("The node %s hasn't reached notReady state", nodeName)

		return fmt.Errorf("node %s hasn't reached notReady state", nodeName)
	}

	By(fmt.Sprintf("Checking node %q got into Ready", nodeName))

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute*25,
		true,
		func(ctx context.Context) (bool, error) {
			currentNode, err := nodes.Pull(APIClient, nodeName)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error pulling in node: %v", err)

				return false, nil
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString {
					if condition.Status == rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

						return true, nil
					}
				}
			}

			return false, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("The node %s hasn't reached Ready state", nodeName)

		return fmt.Errorf("node %s hasn't reached Ready state", nodeName)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Uncordoning node %q", nodeName)

	err = nodeObj.Uncordon()
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to uncordon %q due to %v", nodeName, err)

		return fmt.Errorf("failed to uncordon %q due to %w", nodeName, err)
	}

	time.Sleep(15 * time.Second)

	return nil
}

func getNodeForReboot(isIPv6 bool) (string, string, error) {
	By("Find cluster node name to reboot")

	egressIPMap, err := getEgressIPMap()
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to retrieve egressIP map due to %v", err)

		return "", "", fmt.Errorf("failed to retrieve egressIP map due to %w", err)
	}

	for egressIPNode, egressIPValue := range egressIPMap {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("IP %q is assigned to %q", egressIPValue, egressIPNode)

		myIP, err := netip.ParseAddr(egressIPValue)
		if err != nil {
			klog.V(100).Infof("Failed to parse used ip address %q", egressIPValue)

			return "", "", fmt.Errorf("failed to parse used ip address %q", egressIPValue)
		}

		if !isIPv6 && myIP.Is4() || isIPv6 && myIP.Is6() {
			klog.V(100).Infof("Selected node %q with IP %q address", egressIPNode, egressIPValue)

			return egressIPValue, egressIPNode, nil
		}
	}

	return "", "", fmt.Errorf("no egress IP address found in egressIP map")
}

func verifyEgressIPFailOver(isIPv6 bool) {
	By("Creating egressIP test setup to verify fail-over procedure validity")

	if len(RDSCoreConfig.NodesCredentialsMap) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("BMC Details not specified")
		Skip("BMC Details not specified. Skipping...")
	}

	By("Getting node object")

	CreateEgressIPTestDeployment()

	By("Verify egressIP connectivity for the pods run in the namespace not assigned to the EgressIP")

	verifyEgressIPConnectivityBalancedTraffic(isIPv6)

	By("Execute graceful node reboot")

	egressIPUnderTest, nodeNameForReboot, err := getNodeForReboot(isIPv6)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to find a valid node for reboot: %v", err))

	egressIPMap, err := getEgressIPMap()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve egressIP map due to: %v", err))

	klog.V(100).Infof("Retrieved EgressIP map:\n%v\n", egressIPMap)

	klog.V(100).Infof("Retrieve node available for EgressIP assignment")

	var nodeNameForVerification string

	for _, nodeName := range []string{
		RDSCoreConfig.EgressIPNodeOne,
		RDSCoreConfig.EgressIPNodeTwo,
		RDSCoreConfig.EgressIPNodeThree} {
		_, nodeUsed := egressIPMap[nodeName]
		klog.V(100).Infof("Processing node: %q", nodeName)

		if !nodeUsed {
			klog.V(100).Infof("Node %q is not used by EgressIP", nodeName)

			nodeNameForVerification = nodeName

			break
		}
	}

	Expect(nodeNameForVerification).ToNot(Equal(""),
		fmt.Sprintf("Available node for the fail-over verification not found; %v", err))

	err = gracefulNodeReboot(nodeNameForReboot)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to reboot node %s: %v", nodeNameForReboot, err))

	By("Refreshing EgressIP configuration after node reboot")

	egressIPMap, err = getEgressIPMap()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve new EgressIP map due to: %v", err))

	klog.V(100).Infof("Refreshed EgressIP map:\n%v\n", egressIPMap)

	for _node, _ip := range egressIPMap {
		if _ip == egressIPUnderTest {
			klog.V(100).Infof("egressIP %s assigned to the node %q", egressIPUnderTest, _node)

			Expect(_node).ToNot(Equal(nodeNameForReboot),
				fmt.Sprintf("Failover procedure failure, egress IP %s was not assigned to the new node",
					egressIPUnderTest))

			break
		}
	}

	By("Verify egressIP connectivity after egressIP fail-over")

	verifyEgressIPConnectivityBalancedTraffic(isIPv6)
}

// EnsureInNodeReadiness create egressIP test setup to verify connectivity with the external server.
func EnsureInNodeReadiness(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\t*** Ensure nodes are uncordoned and Ready ***")

	if len(RDSCoreConfig.NodesCredentialsMap) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("BMC Details not specified")
		Skip("BMC Details not specified. Skipping...")
	}

	By("Getting list of all nodes")

	allNodes, err := nodes.List(APIClient, metav1.ListOptions{})

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error listing nodes in the cluster: %v", err))
	Expect(len(allNodes)).ToNot(Equal(0), "0 nodes found in the cluster")

	for _, _node := range allNodes {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing node %q", _node.Definition.Name)

		By(fmt.Sprintf("Checking node %q got into Ready", _node.Definition.Name))

		Eventually(func(ctx SpecContext) bool {
			currentNode, err := nodes.Pull(APIClient, _node.Definition.Name)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error pulling in node: %v", err)

				return false
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString {
					if condition.Status == rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

						return true
					}
				}
			}

			return false
		}).WithTimeout(25*time.Minute).WithPolling(15*time.Second).WithContext(ctx).Should(BeTrue(),
			"Node hasn't reached Ready state")

		if _node.Object.Spec.Unschedulable {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Uncordoning node %q", _node.Definition.Name)
			err = _node.Uncordon()
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to uncordon %q due to %v", _node.Definition.Name, err))

			time.Sleep(15 * time.Second)
		}
	}
}

// CreateEgressIPTestDeployment create egressIP test setup to verify connectivity with the external server.
func CreateEgressIPTestDeployment() {
	By("Creating the EgressIP test source deployment")

	err := cleanUpDeployments(APIClient)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to cleanup deployments: %v", err))

	klog.V(100).Infof("Creating the EgressIP assigned agnhost deployment in namespace %s for node %s and %s",
		RDSCoreConfig.EgressIPNamespaceOne, RDSCoreConfig.EgressIPNodeOne, RDSCoreConfig.EgressIPNodeTwo)

	deploySANameOne, err := createAgnhostRBAC(APIClient, RDSCoreConfig.EgressIPNamespaceOne)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create RBAC in namespace %s: %v", RDSCoreConfig.EgressIPNamespaceOne, err))

	for _, nodeToAssign := range []string{RDSCoreConfig.EgressIPNodeOne,
		RDSCoreConfig.EgressIPNodeTwo, RDSCoreConfig.NonEgressIPNode} {
		err = createAgnhostDeployment(
			APIClient,
			RDSCoreConfig.EgressIPNamespaceOne,
			deploySANameOne,
			nodeToAssign,
			egressIPPodLabelsMap)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create deployment for node %s in namespace %s: %v",
				nodeToAssign, RDSCoreConfig.EgressIPNamespaceOne, err))
	}

	klog.V(100).Infof("Creating the EgressIP assigned agnhost deployment in namespace %s for node %s",
		RDSCoreConfig.EgressIPNamespaceTwo, RDSCoreConfig.EgressIPNodeTwo)

	deploySANameTwo, err := createAgnhostRBAC(APIClient, RDSCoreConfig.EgressIPNamespaceTwo)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create RBAC in namespace %s: %v", RDSCoreConfig.EgressIPNamespaceTwo, err))

	err = createAgnhostDeployment(
		APIClient,
		RDSCoreConfig.EgressIPNamespaceTwo,
		deploySANameTwo,
		RDSCoreConfig.EgressIPNodeTwo,
		egressIPPodLabelsMap)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment for node %s in namespace %s: %v",
			RDSCoreConfig.EgressIPNodeTwo, RDSCoreConfig.EgressIPNamespaceTwo, err))

	klog.V(100).Infof("Creating the non EgressIP assigned agnhost deployment in namespace %s for node %s",
		RDSCoreConfig.EgressIPNamespaceOne, RDSCoreConfig.EgressIPNodeOne)

	nonEIPPodLabelsMap := map[string]string{
		strings.Split(nonEgressIPPodLabel, "=")[0]: strings.Split(nonEgressIPPodLabel, "=")[1],
	}

	err = createAgnhostDeployment(
		APIClient,
		RDSCoreConfig.EgressIPNamespaceOne,
		deploySANameOne,
		RDSCoreConfig.EgressIPNodeOne,
		nonEIPPodLabelsMap)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment for node %s in namespace %s: %v",
			RDSCoreConfig.EgressIPNodeOne, RDSCoreConfig.EgressIPNamespaceOne, err))
}

// VerifyEgressIPConnectivityBalancedTrafficIPv4 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPConnectivityBalancedTrafficIPv4() {
	verifyEgressIPConnectivityBalancedTraffic(false)
}

// VerifyEgressIPConnectivityBalancedTrafficIPv6 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPConnectivityBalancedTrafficIPv6() {
	verifyEgressIPConnectivityBalancedTraffic(true)
}

// VerifyEgressIPConnectivityThreeNodesIPv4 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPConnectivityThreeNodesIPv4() {
	verifyEgressIPConnectivityThreeNodes(false)
}

// VerifyEgressIPConnectivityThreeNodesIPv6 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPConnectivityThreeNodesIPv6() {
	verifyEgressIPConnectivityThreeNodes(true)
}

// VerifyEgressIPForPodWithWrongLabel verifies egress traffic applies only for pods with
// the correct pod label defined.
func VerifyEgressIPForPodWithWrongLabel() {
	By("Verifying no egressIP was used for the pod with the incorrect label assigned")

	expectedIPs, err := getEgressIPList()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve configured EgressIP addresses list from the egressIP %s: %v",
			RDSCoreConfig.EgressIPName, err))

	podObjectsPodObjects, err := pod.List(APIClient, RDSCoreConfig.EgressIPNamespaceOne, egressIPPodSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve pods list from namespace %s with label %s: %v",
			RDSCoreConfig.EgressIPNamespaceOne, RDSCoreConfig.EgressIPPodLabel, err))

	err = sendTrafficCheckIP(podObjectsPodObjects, false, expectedIPs)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Server response was note received: %v", err))

	podObjectsPodObjects, err = pod.List(APIClient, RDSCoreConfig.EgressIPNamespaceOne, nonEgressIPPodSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve pods list from namespace %s with label %v: %v",
			RDSCoreConfig.EgressIPNamespaceOne, nonEgressIPPodSelector, err))

	err = sendTrafficCheckIP(podObjectsPodObjects, false, expectedIPs)
	Expect(err).To(HaveOccurred(),
		fmt.Sprintf("Server response was received with the not correct egressIP address: %v", err))
}

// VerifyEgressIPForNamespaceWithWrongLabel verifies egress traffic applies only for the pods
// run in the namespace assigned to the EgressIP service.
func VerifyEgressIPForNamespaceWithWrongLabel() {
	klog.V(100).Infof("Create new namespace %s not referenced by EgressIP", nonEgressIPNamespace)

	_, err := namespace.NewBuilder(APIClient, nonEgressIPNamespace).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create namespace %s: %v", nonEgressIPNamespace, err))

	deploySAName, err := createAgnhostRBAC(APIClient, nonEgressIPNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create RBAC in namespace %s: %v", nonEgressIPNamespace, err))

	err = createAgnhostDeployment(
		APIClient,
		nonEgressIPNamespace,
		deploySAName,
		RDSCoreConfig.EgressIPNodeOne,
		egressIPPodLabelsMap)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment for node %s in namespace %s: %v",
			RDSCoreConfig.EgressIPNodeOne, nonEgressIPNamespace, err))

	By("Spawning the pods on the EgressIP assignable hosts")

	expectedIPs, err := getEgressIPList()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve configured EgressIP addresses list from the egressIP %s: %v",
			RDSCoreConfig.EgressIPName, err))

	podObjects, err := pod.List(APIClient, nonEgressIPNamespace, egressIPPodSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve pods list from namespace %s with label %s: %v",
			nonEgressIPNamespace, RDSCoreConfig.EgressIPPodLabel, err))

	err = sendTrafficCheckIP(podObjects, false, expectedIPs)
	Expect(err).To(HaveOccurred(),
		fmt.Sprintf("Server response was received with the not correct egressIP address: %v", err))
}

// VerifyEgressIPOneNamespaceThreeNodesBalancedEIPTrafficIPv4 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPOneNamespaceThreeNodesBalancedEIPTrafficIPv4() {
	By("Creating egressIP test setup: 7 pods run in the 3 different namespaces on 3 nodes")

	CreateEgressIPTestDeployment()

	By("Verify egressIP connectivity for the same namespace")

	for iter := 0; iter < 5; iter++ {
		VerifyEgressIPConnectivityBalancedTrafficIPv4()
	}
}

// VerifyEgressIPTwoNamespacesThreeNodesIPv4 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPTwoNamespacesThreeNodesIPv4() {
	By("Creating egressIP test setup to verify connectivity on the two namespaces")

	CreateEgressIPTestDeployment()

	By("Verify egressIP connectivity for the different namespaces")

	VerifyEgressIPConnectivityThreeNodesIPv4()
}

// VerifyEgressIPOneNamespaceThreeNodesBalancedEIPTrafficIPv6 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPOneNamespaceThreeNodesBalancedEIPTrafficIPv6() {
	By("Creating egressIP test setup: 7 pods run in the 3 different namespaces on 3 nodes")

	CreateEgressIPTestDeployment()

	By("Verify egressIP connectivity for the same namespace")

	for iter := 0; iter < 5; iter++ {
		VerifyEgressIPConnectivityBalancedTrafficIPv6()
	}
}

// VerifyEgressIPTwoNamespacesThreeNodesIPv6 verifies egress traffic works with egressIP
// applied for the external target.
func VerifyEgressIPTwoNamespacesThreeNodesIPv6() {
	By("Creating egressIP test setup to verify connectivity on the two namespaces")

	CreateEgressIPTestDeployment()

	By("Verify egressIP connectivity for the different namespaces")

	VerifyEgressIPConnectivityThreeNodesIPv6()
}

// VerifyEgressIPOneNamespaceOneNodeWrongPodLabel verifies egress traffic works with egressIP
// applied for the external target with the single namespace and wrong pod label.
func VerifyEgressIPOneNamespaceOneNodeWrongPodLabel() {
	By("Creating egressIP test setup to verify connectivity on the two namespaces")

	CreateEgressIPTestDeployment()

	By("Verify egressIP connectivity for the pod with the not correctly defined label")

	VerifyEgressIPForPodWithWrongLabel()
}

// VerifyEgressIPWrongNsLabel verifies egress traffic works with egressIP
// applied for the external target when two egressIP pods from the different namespaces run on the same node
// when one of the namespaces not assigned to the egressIP.
func VerifyEgressIPWrongNsLabel() {
	By("Creating egressIP test setup to verify connectivity on the two namespaces")

	CreateEgressIPTestDeployment()

	By("Verify egressIP connectivity for the pods run in the namespace not assigned to the EgressIP")

	VerifyEgressIPForNamespaceWithWrongLabel()
}

// VerifyEgressIPFailOverIPv4 verifies egressIP ipv4 address moving to the first available for assignment node
// after current node goes down, NotReady and egressIP traffic continues to use egressIP configured.
func VerifyEgressIPFailOverIPv4() {
	verifyEgressIPFailOver(false)
}

// VerifyEgressIPFailOverIPv6 verifies egressIP ipv4 address moving to the first available for assignment node
// after current node goes down, NotReady and egressIP traffic continues to use egressIP configured.
func VerifyEgressIPFailOverIPv6() {
	verifyEgressIPFailOver(true)
}
