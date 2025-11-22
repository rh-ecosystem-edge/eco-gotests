package supporttools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	scc "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/scc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	logScanMaxTries = 3
)

// CreateTCPDumpDeployment creates tcpdump deployment on the nodes specified in scheduleOnNodes.
func CreateTCPDumpDeployment(
	apiClient *clients.Settings,
	tdNamespace,
	tdDeploymentLabel,
	tdImage,
	packetCaptureInterface,
	captureScript string,
	scheduleOnHosts []string) (*deployment.Builder, error) {
	klog.V(100).Infof("-------------------DEBUG: NODES NAMES: %q", scheduleOnHosts)

	stDeploymentName := "tcpdump"

	klog.V(100).Infof("Create tcpdump namespace %s", tdNamespace)

	err := ensureNamespaceExists(apiClient, tdNamespace)
	if err != nil {
		klog.V(100).Infof("Failed to create namespace %s: %v", tdNamespace, err)

		return nil, fmt.Errorf("failed to create namespace %s: %w", tdNamespace, err)
	}

	klog.V(100).Infof("Adding SCC privileged to the namespace %s", tdNamespace)

	err = scc.AddPrivilegedSCCtoDefaultSA(tdNamespace)
	if err != nil {
		klog.V(100).Infof("Failed to add SCC privileged to the tcpdump namespace %s: %v",
			tdNamespace, err)

		return nil, fmt.Errorf("failed to add SCC privileged to the tcpdump namespace %s: %w",
			tdNamespace, err)
	}

	stDeployment, err := createTCPDumpDeployment(
		apiClient,
		tdImage,
		stDeploymentName,
		tdNamespace,
		tdDeploymentLabel,
		packetCaptureInterface,
		captureScript,
		scheduleOnHosts,
	)
	if err != nil {
		klog.V(100).Infof("Failed to create tcpdump deployment %s in namespace %s due to %v",
			stDeploymentName, tdNamespace, err)

		return stDeployment, err
	}

	err = await.WaitUntilDeploymentReady(apiClient, stDeploymentName, tdNamespace, time.Minute)
	if err != nil {
		klog.V(100).Infof("deployment %s in namespace %s failed to reach ready state due to %v",
			stDeploymentName, tdNamespace, err)

		return nil, fmt.Errorf("deployment %s in namespace %s failed to reach ready state due to %w",
			stDeploymentName, tdNamespace, err)
	}

	return stDeployment, nil
}

// createTCPDumpDeployment creates a tcpdump deployment in namespace <namespace> on node <nodeName>.
//
//nolint:funlen
func createTCPDumpDeployment(
	apiClient *clients.Settings,
	stImage,
	stDeploymentName,
	stNamespace,
	stDeploymentLabel,
	packetCaptureInterface,
	captureScript string,
	scheduleOnHosts []string) (*deployment.Builder, error) {
	klog.V(100).Infof("Creating the tcpdump deployment with image %s", stImage)

	var err error

	klog.V(100).Infof("Checking tcpdump deployment %q in namespace %s doesn't exist",
		stDeploymentName, stNamespace)

	err = apiobjectshelper.DeleteDeployment(apiClient, stDeploymentName, stNamespace)
	if err != nil {
		klog.V(100).Infof("Failed to delete deployment %s from namespace %s; %v",
			stDeploymentName, stNamespace, err)

		return nil, fmt.Errorf("failed to delete deployment %s from namespace %s; %w",
			stDeploymentName, stNamespace, err)
	}

	klog.V(100).Infof("Sleeping 10 seconds")
	time.Sleep(10 * time.Second)

	klog.V(100).Infof("Removing ServiceAccount %s from namespace %s", supportToolsDeploySAName, stNamespace)

	err = apiobjectshelper.DeleteServiceAccount(apiClient, supportToolsDeploySAName, stNamespace)
	if err != nil {
		klog.V(100).Infof("Failed to remove serviceAccount %q from namespace %q; %v",
			supportToolsDeploySAName, stNamespace, err)

		return nil, fmt.Errorf("failed to remove serviceAccount %q from namespace %q; %w",
			supportToolsDeploySAName, stNamespace, err)
	}

	klog.V(100).Infof("Creating ServiceAccount %s in namespace %s", supportToolsDeploySAName, stNamespace)

	err = apiobjectshelper.CreateServiceAccount(apiClient, supportToolsDeploySAName, stNamespace)
	if err != nil {
		klog.V(100).Infof("Failed to create serviceAccount %q in namespace %q; %v",
			supportToolsDeploySAName, stNamespace, err)

		return nil, fmt.Errorf("failed to create serviceAccount %q in namespace %q; %w",
			supportToolsDeploySAName, stNamespace, err)
	}

	klog.V(100).Infof("Removing Cluster RBAC %q", supportToolsDeployRBACName)

	err = apiobjectshelper.DeleteClusterRBAC(apiClient, supportToolsDeployRBACName)
	if err != nil {
		klog.V(100).Infof("Failed to delete supporttools RBAC %q; %v", supportToolsDeployRBACName, err)

		return nil, fmt.Errorf("failed to delete supporttools RBAC %q; %w", supportToolsDeployRBACName, err)
	}

	klog.V(100).Infof("Creating Cluster RBAC %q", supportToolsDeployRBACName)

	err = apiobjectshelper.CreateClusterRBAC(apiClient, supportToolsDeployRBACName, supportToolsRBACRole,
		supportToolsDeploySAName, stNamespace)
	if err != nil {
		klog.V(100).Infof("failed to create supporttools RBAC %q in namespace %s; %v",
			supportToolsDeployRBACName, stNamespace, err)

		return nil, fmt.Errorf("failed to create supporttools RBAC %q in namespace %s; %w",
			supportToolsDeployRBACName, stNamespace, err)
	}

	klog.V(100).Infof("Defining container configuration")

	deployContainer := defineTCPDumpContainer(
		stImage,
		packetCaptureInterface,
		captureScript)

	klog.V(100).Infof("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	if err != nil {
		klog.V(100).Infof("Failed to obtain container definition: %v", err)

		return nil, fmt.Errorf("failed to obtain container definition: %w", err)
	}

	klog.V(100).Infof("Defining deployment %q configuration in namespace %s",
		stDeploymentName, stNamespace)

	deployLabelsMap := map[string]string{
		strings.Split(stDeploymentLabel, "=")[0]: strings.Split(stDeploymentLabel, "=")[1]}

	stDeployment := defineDeployment(
		apiClient,
		deployContainerCfg,
		stDeploymentName,
		stNamespace,
		supportToolsDeploySAName,
		scheduleOnHosts,
		deployLabelsMap)

	klog.V(100).Infof("Creating deployment")

	klog.V(100).Infof("-------------------DEBUG: NODES NAMES: %q", scheduleOnHosts)

	stDeployment, err = stDeployment.CreateAndWaitUntilReady(5 * time.Minute)
	if err != nil {
		klog.V(100).Infof("Failed to create deployment %s in namespace %s: %v",
			stDeploymentName, stNamespace, err)

		return nil, fmt.Errorf("failed to create deployment %s in namespace %s: %w",
			stDeploymentName, stNamespace, err)
	}

	if stDeployment == nil {
		klog.V(100).Infof("Failed to create deployment %s in namespace %s; %v",
			stDeploymentName, stDeploymentName, err)

		return nil, fmt.Errorf("failed to create deployment %s in namespace %s; %w",
			stDeploymentName, stDeploymentName, err)
	}

	return stDeployment, nil
}

func defineTCPDumpContainer(
	cImage,
	packetCaptureInterface,
	captureScript string) *pod.ContainerBuilder {
	cName := "tcpdump"

	cCmd := []string{
		"/bin/bash",
		"-c",
		fmt.Sprintf(captureScript, packetCaptureInterface),
	}

	klog.V(100).Infof("-------------------DEBUG: Packet Capture Interface Name: %q", packetCaptureInterface)

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
			Add: []corev1.Capability{
				"SETFCAP",
				"CAP_NET_RAW",
				"CAP_NET_ADMIN",
			},
		},
	}

	klog.V(100).Infof("Setting SecurityContext")

	deployContainer = deployContainer.WithSecurityContext(securityContext)

	klog.V(100).Infof("Dropping ALL security capability")

	deployContainer = deployContainer.WithDropSecurityCapabilities([]string{"ALL"}, true)

	klog.V(100).Infof("Enable TTY and Stdin; needed for immediate log propagation")

	deployContainer = deployContainer.WithTTY(true).WithStdin(true)

	klog.V(100).Infof("%q container's  definition:\n%#v", cName, deployContainer)

	return deployContainer
}

// SendTrafficCheckTCPDumpLogs is a wrapper function to reduce code duplication when probing for target IPs.
func SendTrafficCheckTCPDumpLogs(
	apiClient *clients.Settings,
	proberPodObj *pod.Builder,
	tcpdumpPodNamespace,
	tcpdumpPodSelectorLabel,
	targetHost,
	targetPort string) error {
	timeout := 3 * time.Minute

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		3*time.Second,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			klog.V(100).Infof("Verifying that the expected outbound request can be seen in the tcpdump logs")

			result, err := sendProbeAndCheckTCPDumpLogs(
				apiClient,
				proberPodObj,
				tcpdumpPodNamespace,
				tcpdumpPodSelectorLabel,
				targetHost,
				targetPort)

			if err == nil && result {
				return true, nil
			}

			return false, nil
		})
	if err != nil {
		klog.V(100).Infof("Expected request was not found in the packet sniffer logs; %v", err)

		return fmt.Errorf("expected request was not found in the packet sniffer logs; %w", err)
	}

	return nil
}

// sendProbeAndCheckTCPDumpLogs sends requests from the prober pod.
func sendProbeAndCheckTCPDumpLogs(
	apiClient *clients.Settings,
	proberPod *pod.Builder,
	tcpdumpPodNamespace,
	tcpdumpPodSelectorLabel,
	targetHost,
	targetPort string) (bool, error) {
	timeStart := time.Now()

	klog.V(100).Infof("Sending request with a unique search string from "+
		"prober pod %s/%s", proberPod.Definition.Namespace, proberPod.Definition.Name)

	searchString, err := sendProbeToHostPort(
		proberPod,
		targetHost,
		targetPort)
	if err != nil {
		klog.V(100).Infof("Failed to send probe from the pod %s in namespace %s to the host %s:%s due to %v",
			proberPod.Definition.Name, proberPod.Definition.Namespace, targetHost, targetPort, err)

		return false, fmt.Errorf("failed to send probe from the pod %s in namespace %s to the host %s:%s due to %w",
			proberPod.Definition.Name, proberPod.Definition.Namespace, targetHost, targetPort, err)
	}

	klog.V(100).Infof("request sent from the prober pod %s/%s",
		proberPod.Definition.Namespace, proberPod.Definition.Name)

	var errMsg error

	for i := 0; i < logScanMaxTries; i++ {
		klog.V(100).Infof("Making sure that request with the search string %s were seen (try %d of %d)",
			searchString, i+1, logScanMaxTries)

		numberFound, _, err := ScanTCPDumpPodLogs(
			apiClient,
			proberPod,
			tcpdumpPodNamespace,
			tcpdumpPodSelectorLabel,
			searchString,
			timeStart)
		if err != nil {
			errMsg = err
		} else if numberFound >= 1 {
			klog.V(100).Infof("The searching string %s was found in log %d times", searchString, numberFound)

			return true, nil
		}
	}

	return false, errMsg
}

// ScanTCPDumpPodLogs iterates over the pods logs and searches for searchString and then counts the occurrences.
//
//nolint:funlen
func ScanTCPDumpPodLogs(
	apiClient *clients.Settings,
	proberPod *pod.Builder,
	tcpdumpPodNamespace,
	tcpdumpPodSelectorLabel,
	searchString string,
	timeSpan time.Time) (int, string, error) {
	tcpdumpPodObjects, err := pod.List(apiClient, tcpdumpPodNamespace,
		metav1.ListOptions{LabelSelector: tcpdumpPodSelectorLabel})
	if err != nil {
		klog.V(100).Infof("failed to list pod objects: %v", err)

		return 0, "", fmt.Errorf("failed to list pod objects: %w", err)
	}

	matchesFound := 0

	var tcpdumpPodObj *pod.Builder

	for _, podObj := range tcpdumpPodObjects {
		if podObj.Object.Spec.NodeName == proberPod.Object.Spec.NodeName {
			klog.V(100).Infof("tcpdump pod: %s", podObj.Definition.Name)
			tcpdumpPodObj = podObj

			break
		}
	}

	if tcpdumpPodObj == nil {
		klog.V(100).Infof("failed to find a tcpdump pod on the %s node", proberPod.Object.Spec.NodeName)

		return 0, "", fmt.Errorf("tcpdump pod not found on the %s node", proberPod.Object.Spec.NodeName)
	}

	var tcpdumpLog string

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second,
		time.Second*5,
		true,
		func(ctx context.Context) (bool, error) {
			logStartTimestamp := time.Since(timeSpan)
			klog.V(100).Infof("\tTime duration is %s", logStartTimestamp)

			if logStartTimestamp.Abs().Seconds() < 1 {
				logStartTimestamp, err = time.ParseDuration("1s")
				if err != nil {
					klog.V(100).Infof("failed to parse the time duration: %v", err)

					return false, nil
				}
			}

			tcpdumpLog, err = tcpdumpPodObj.GetLog(logStartTimestamp, tcpdumpPodObj.Definition.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("Failed to get tcpdumpLog from pod %q, log %s: %v",
					tcpdumpPodObj.Definition.Name, tcpdumpLog, err)

				return false, nil
			}

			if len(tcpdumpLog) == 0 {
				klog.V(100).Infof("tcpdumpLog from pod %q is empty, log %s: %v",
					tcpdumpPodObj.Definition.Name, tcpdumpLog, err)

				return false, nil
			}

			klog.V(100).Infof("debug log for %s: -%s-", tcpdumpPodObj.Definition.Name, tcpdumpLog)
			klog.V(100).Infof("len(tcpdumpLog) = %d", len(tcpdumpLog))

			buf := new(bytes.Buffer)

			_, err = buf.WriteString(tcpdumpLog)
			if err != nil {
				klog.V(100).Infof("error in copying info from pod tcpdumpLog to buffer: %v", err)

				return false, nil
			}

			scanner := bufio.NewScanner(buf)

			// reset value for the current iteration
			matchesFound = 0

			for scanner.Scan() {
				logLine := scanner.Text()
				if strings.Contains(logLine, searchString) {
					klog.V(100).Infof("Found match in log line: %s", logLine)
					logLineExploded := strings.Fields(logLine)

					if len(logLineExploded) != 2 {
						klog.V(100).Infof("Unexpected logline content: %s", logLine)

						return false, nil
					}

					matchesFound++
				}
			}

			if matchesFound != 0 {
				klog.V(100).Infof("Found %d matches for %q", matchesFound, searchString)

				return true, nil
			}

			klog.V(100).Infof("No matches for %q found", searchString)

			return false, nil
		})
	if err != nil {
		klog.V(100).Infof("Error processing logs: %v", err)

		return matchesFound, tcpdumpLog, fmt.Errorf("no matches found in tcpdump log: %w", err)
	}

	return matchesFound, tcpdumpLog, nil
}

// sendProbeToHostPort runs curl against a http://targetHost:targetPort.
// Returns the "targetHost:targetPort" string that will be used for the requests search.
func sendProbeToHostPort(
	proberPod *pod.Builder,
	targetHost,
	targetPort string) (string, error) {
	if proberPod == nil {
		return "", fmt.Errorf("nil pointer to proberPod was provided in sendProbeToHostPort")
	}

	targetURL := fmt.Sprintf("%s:%s", targetHost, targetPort)
	request := fmt.Sprintf("http://%s", targetURL)

	cmdToRun := []string{"/bin/bash", "-c", fmt.Sprintf("curl --max-time 10 -s %s", request)}

	var errorMsg error

	timeout := time.Minute * 3

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			output, err := proberPod.ExecCommand(cmdToRun, proberPod.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("query failed. Request: %s, Output: %q, Error: %v", request, output, err)

				errorMsg = fmt.Errorf("query failed. Request: %s, Output: %q, Error: %w", request, output, err)

				return false, nil
			}

			klog.V(100).Infof("Successfully executed command from within a pod %q: %v",
				proberPod.Object.Name, cmdToRun)
			klog.V(100).Infof("Command's output:\n\t%v", output.String())

			errorMsg = nil

			return true, nil
		})
	if err != nil {
		return "", fmt.Errorf("failed to execute the command '%s' during timeout %v; %w", cmdToRun, timeout, err)
	}

	if errorMsg != nil {
		return "", errorMsg
	}

	return targetURL, nil
}
