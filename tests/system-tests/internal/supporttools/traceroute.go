package supporttools

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	scc "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/scc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	supportToolsDeployRBACName = "privileged-rdscore-supporttools"
	supportToolsDeploySAName   = "rdscore-supporttools-sa"
	supportToolsRBACRole       = "system:openshift:scc:privileged"
	tracerouteDeploymentName   = "traceroute"
)

// CreateTraceRouteDeployment creates support-tools deployment on the nodes specified in scheduleOnNodes.
func CreateTraceRouteDeployment(
	apiClient *clients.Settings,
	stNamespace,
	stDeploymentLabel,
	stImage string,
	scheduleOnNodes []string) (*deployment.Builder, error) {
	klog.V(100).Infof("Create support-tools namespace %s", stNamespace)

	err := ensureNamespaceExists(apiClient, stNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create support-tools namespace %s: %w", stNamespace, err)
	}

	klog.V(100).Infof("Adding SCC privileged to the support-tools namespace")

	err = scc.AddPrivilegedSCCtoDefaultSA(stNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to add SCC privileged to the supporttools namespace %s: %w",
			stNamespace, err)
	}

	stDeployment, err := createTraceRouteDeployment(
		apiClient,
		stImage,
		tracerouteDeploymentName,
		stNamespace,
		stDeploymentLabel,
		scheduleOnNodes,
	)
	if err != nil {
		klog.V(100).Infof("Failed to create traceroute deployment %s in namespace %s due to %v",
			tracerouteDeploymentName, stNamespace, err)

		return stDeployment, fmt.Errorf("failed to create traceroute deployment %s in namespace %s: %w",
			tracerouteDeploymentName, stNamespace, err)
	}

	klog.V(100).Infof("Creating deployment")

	stDeployment, err = stDeployment.CreateAndWaitUntilReady(5 * time.Minute)
	if err != nil {
		klog.V(100).Infof("Failed to create deployment %s in namespace %s: %v",
			tracerouteDeploymentName, stNamespace, err)

		return nil, fmt.Errorf("failed to create deployment %s in namespace %s: %w",
			tracerouteDeploymentName, stNamespace, err)
	}

	if stDeployment == nil {
		klog.V(100).Infof("deployment %s not found in namespace %s", tracerouteDeploymentName, stNamespace)

		return nil, fmt.Errorf("deployment %s not found in namespace %s", tracerouteDeploymentName, stNamespace)
	}

	return stDeployment, nil
}

func ensureNamespaceExists(apiClient *clients.Settings, nsName string) error {
	klog.V(100).Infof("Create namespace %q", nsName)

	createNs := namespace.NewBuilder(apiClient, nsName)

	if createNs.Exists() {
		err := createNs.Delete()
		if err != nil {
			klog.V(100).Infof("Failed to delete namespace %q: %v", nsName, err)

			return fmt.Errorf("failed to delete namespace %q: %w", nsName, err)
		}

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second,
			2*time.Minute,
			true,
			func(ctx context.Context) (bool, error) {
				if createNs.Exists() {
					klog.V(100).Infof("Error deleting namespace %q", nsName)

					return false, nil
				}

				klog.V(100).Infof("Deleted namespace %q", createNs.Definition.Name)

				return true, nil
			})
		if err != nil {
			klog.V(100).Infof("Failed to delete namespace %s due to %v", nsName, err)

			return fmt.Errorf("failed to delete supporttools namespace %q : %w", nsName, err)
		}
	}

	_, err := createNs.Create()
	if err != nil {
		klog.V(100).Infof("Error creating namespace %q: %v", nsName, err)

		return fmt.Errorf("failed to create namespace %q: %w", nsName, err)
	}

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second,
		3*time.Second,
		true,
		func(ctx context.Context) (bool, error) {
			if !createNs.Exists() {
				klog.V(100).Infof("Error creating namespace %q", nsName)

				return false, nil
			}

			klog.V(100).Infof("Created namespace %q", createNs.Definition.Name)

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("support-tools namespace %q not found created: %v", nsName, err)

		return fmt.Errorf("support-tools namespace %q not found created: %w", nsName, err)
	}

	return nil
}

// createTraceRouteDeployment creates a support-tools traceroute pod in namespace <namespace> on node <nodeName>.
//
//nolint:funlen
func createTraceRouteDeployment(
	apiClient *clients.Settings,
	stImage,
	stDeploymentName,
	stNamespace,
	stDeploymentLabel string,
	scheduleOnHosts []string) (*deployment.Builder, error) {
	klog.V(100).Infof("Creating the support-tools traceroute deployment with image %s", stImage)

	var err error

	klog.V(100).Infof("Checking support-tools deployment %q doesn't exist in namespace %s",
		stDeploymentName, stNamespace)

	err = apiobjectshelper.DeleteDeployment(apiClient, stDeploymentName, stNamespace)
	if err != nil {
		klog.V(100).Infof("failed to delete deployment %s from namespace %s due to %v",
			stDeploymentName, stNamespace, err)

		return nil, fmt.Errorf("failed to delete deployment %s from namespace %s due to %w",
			stDeploymentName, stNamespace, err)
	}

	klog.V(100).Infof("Sleeping 10 seconds")
	time.Sleep(10 * time.Second)

	klog.V(100).Infof("Removing ServiceAccount %s from namespace %s", stDeploymentName, stNamespace)

	err = apiobjectshelper.DeleteServiceAccount(apiClient, supportToolsDeploySAName, stNamespace)
	if err != nil {
		klog.V(100).Infof("failed to remove serviceAccount %q from namespace %q due to %v",
			supportToolsDeploySAName, stNamespace, err)

		return nil, fmt.Errorf("failed to remove serviceAccount %q from namespace %q due to %w",
			supportToolsDeploySAName, stNamespace, err)
	}

	klog.V(100).Infof("Creating ServiceAccount %s in namespace %s", stDeploymentName, stNamespace)

	err = apiobjectshelper.CreateServiceAccount(apiClient, supportToolsDeploySAName, stNamespace)
	if err != nil {
		klog.V(100).Infof("failed to create serviceAccount %q in namespace %q due to %v",
			supportToolsDeploySAName, stNamespace, err)

		return nil, fmt.Errorf("failed to create serviceAccount %q in namespace %q due to %w",
			supportToolsDeploySAName, stNamespace, err)
	}

	klog.V(100).Infof("Removing Cluster RBAC %s", supportToolsDeployRBACName)

	err = apiobjectshelper.DeleteClusterRBAC(apiClient, supportToolsDeployRBACName)
	if err != nil {
		klog.V(100).Infof("failed to delete supporttools RBAC %q due to %v",
			supportToolsDeployRBACName, err)

		return nil, fmt.Errorf("failed to delete supporttools RBAC %q due to %w",
			supportToolsDeployRBACName, err)
	}

	klog.V(100).Infof("Creating Cluster RBAC %s", supportToolsDeployRBACName)

	err = apiobjectshelper.CreateClusterRBAC(apiClient, supportToolsDeployRBACName, supportToolsRBACRole,
		supportToolsDeploySAName, stNamespace)
	if err != nil {
		klog.V(100).Infof("failed to create supporttools RBAC %q in namespace %s due to %v",
			supportToolsDeployRBACName, stNamespace, err)

		return nil, fmt.Errorf("failed to create supporttools RBAC %q in namespace %s due to %w",
			supportToolsDeployRBACName, stNamespace, err)
	}

	klog.V(100).Infof("Defining container configuration")

	deployContainer := defineTraceRouteContainer(stImage)

	klog.V(100).Infof("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	if err != nil {
		klog.V(100).Infof("failed to obtain container definition: %v", err)

		return nil, fmt.Errorf("failed to obtain container definition: %w", err)
	}

	klog.V(100).Infof("Defining deployment %s configuration", stDeploymentName)

	deployLabelsMap := map[string]string{
		strings.Split(stDeploymentLabel, "=")[0]: strings.Split(stDeploymentLabel, "=")[1]}

	trDeployment := defineDeployment(
		apiClient,
		deployContainerCfg,
		stDeploymentName,
		stNamespace,
		supportToolsDeploySAName,
		scheduleOnHosts,
		deployLabelsMap)

	klog.V(100).Infof("Creating deployment %s in namespace %s", stDeploymentName, stNamespace)

	trDeployment, err = trDeployment.CreateAndWaitUntilReady(5 * time.Minute)
	if err != nil {
		klog.V(100).Infof("failed to create deployment %s in namespace %s: %v",
			stDeploymentName, stNamespace, err)

		return nil, fmt.Errorf("failed to create deployment %s in namespace %s: %w",
			stDeploymentName, stNamespace, err)
	}

	if trDeployment == nil {
		klog.V(100).Infof("deployment %s not found in namespace %s", stDeploymentName, stNamespace)

		return nil, fmt.Errorf("deployment %s not found in namespace %s", stDeploymentName, stNamespace)
	}

	return trDeployment, err
}

// sendProbesAndCheckOutput sends traceroute requests and makes sure that the expected string was seen in the output.
func sendProbesAndCheckOutput(
	trPod *pod.Builder,
	targetIP,
	targetPort,
	searchString string) (bool, error) {
	klog.V(100).Infof("Sending requests to the IP %s port %s from the pod %s/%s",
		targetIP, targetPort, trPod.Definition.Namespace, trPod.Definition.Name)

	cmdToRun := []string{"/bin/bash", "-c", fmt.Sprintf("traceroute -p %s %s", targetPort, targetIP)}

	var output bytes.Buffer

	var err error

	timeout := time.Minute

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			output, err = trPod.ExecCommand(cmdToRun, trPod.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(100).Infof("query failed. Request: %s, Output: %q, Error: %v",
					targetIP, output, err)

				return false, nil
			}

			klog.V(100).Infof("Successfully executed command from within a pod %q: %v",
				trPod.Object.Name, cmdToRun)
			klog.V(100).Infof("Command's output:\n\t%v", output.String())

			klog.V(100).Infof("Make sure that search string %s was seen in response %q",
				searchString, output.String())

			if output.String() == "" {
				return false, nil
			}

			if !strings.Contains(output.String(), searchString) {
				return false, nil
			}

			klog.V(100).Infof("Expected string %s was found in the command's output:\n\t%v",
				searchString, output.String())

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("expected string %s not found in traceroute output: %q; %v",
			searchString, output.String(), err)

		return false, fmt.Errorf("expected string %s not found in traceroute output: %q; %w",
			searchString, output.String(), err)
	}

	return true, nil
}

// SendTrafficFindExpectedString sends requests to the specific destination and makes sure that
// expected string was seen in command output.
func SendTrafficFindExpectedString(
	trPodObj *pod.Builder,
	targetIP,
	targetPort,
	searchString string) error {
	timeout := 3 * time.Minute

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		3*time.Second,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			result, err := sendProbesAndCheckOutput(
				trPodObj,
				targetIP,
				targetPort,
				searchString)

			if err == nil && result {
				return true, nil
			}

			return false, nil
		})
	if err != nil {
		klog.V(100).Infof("expected string was not found in the traceroute output; %v", err)

		return fmt.Errorf("expected string was not found in the traceroute output; %w", err)
	}

	return nil
}

func defineTraceRouteContainer(cImage string) *pod.ContainerBuilder {
	cName := "traceroute"

	cCmd := []string{"/bin/bash", "-c", "sleep INF"}

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

func defineDeployment(
	apiClient *clients.Settings,
	containerConfig *corev1.Container,
	deployName, deployNs, saName string,
	scheduleOnHosts []string,
	deployLabels map[string]string) *deployment.Builder {
	klog.V(100).Infof("Defining deployment %q in %q ns", deployName, deployNs)

	klog.V(100).Infof("-------------------DEBUG: NODES NAMES: %q", scheduleOnHosts)

	nodeAffinity := corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   scheduleOnHosts,
							},
						},
					},
				},
			},
		},
	}

	trDeployment := deployment.NewBuilder(apiClient, deployName, deployNs, deployLabels, *containerConfig)

	klog.V(100).Infof("Assigning ServiceAccount %q to the deployment", saName)

	trDeployment = trDeployment.WithServiceAccountName(saName)

	klog.V(100).Infof("Setting Replicas count")

	replicasCnt := len(scheduleOnHosts)

	trDeployment = trDeployment.WithReplicas(int32(replicasCnt))

	trDeployment = trDeployment.WithHostNetwork(true).WithAffinity(&nodeAffinity)

	return trDeployment
}
