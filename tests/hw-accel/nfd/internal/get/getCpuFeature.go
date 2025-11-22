package get

import (
	"context"
	"fmt"

	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdhelpersparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
)

// CPUInfo retrieves cpu info of worker node.
func CPUInfo(apiClient *clients.Settings, name, nsname, containerName, image string) map[string]string {
	workerNodesNames, err := getWorkerNodes(apiClient)
	if err != nil {
		klog.V(nfdparams.LogLevel).Infof("Error getting worker nodes: %v\n", err)

		return nil
	}

	nodeCPUFlagsMap := make(map[string]string)

	for _, nodeName := range workerNodesNames {
		podWorker := pod.NewBuilder(apiClient, nodeName+name, nsname, image)
		containerBuilder := pod.NewContainerBuilder(containerName, image, []string{"./cpucheck"})

		container, err := containerBuilder.GetContainerCfg()
		if err != nil {
			klog.V(nfdparams.LogLevel).Infof("Failed to define the default container settings %v", err)
		}

		podWorker.Definition.Spec.Containers = make([]corev1.Container, 0)
		podWorker.Definition.Spec.Containers = append(podWorker.Definition.Spec.Containers, *container)
		podWorker.Definition.Spec.RestartPolicy = corev1.RestartPolicyNever
		podWorker.Definition.Spec.NodeName = strings.ReplaceAll(nodeName, ".", "")
		podWorker.Definition.Spec.NodeSelector = map[string]string{"node-role.kubernetes.io/worker": ""}

		_, err = podWorker.Create()
		if err != nil {
			klog.V(nfdparams.LogLevel).Infof(
				"Error creating pod: %s", err.Error())

			return nil
		}

		err = podWorker.WaitUntilInStatus(corev1.PodSucceeded, 5*time.Minute)
		if err != nil {
			klog.V(nfdparams.LogLevel).Infof(
				"Error in waiting for pod: %s", err.Error())

			return nil
		}

		con := corev1.PodLogOptions{
			Container: containerName,
		}

		podLogs := apiClient.Pods(nsname).GetLogs(podWorker.Definition.Name, &con).Do(context.TODO())

		_, err = podWorker.DeleteAndWait(5 * time.Minute)
		if err != nil {
			klog.V(nfdparams.LogLevel).Infof(
				"Error creating pod: %s", err.Error())

			return nil
		}

		err = podLogs.Error()
		if err != nil {
			klog.V(nfdparams.LogLevel).Infof(
				"Failed to retrieve logs: %s", err.Error())

			return nil
		}

		body, err := podLogs.Raw()
		if err != nil {
			klog.V(nfdparams.LogLevel).Infof("Failed to retrieve logs %v", err)
		}

		nodeCPUFlagsMap[nodeName] = fmt.Sprint(string(body))
	}

	return nodeCPUFlagsMap
}

// CPUFlags returns cpu flags list.
func CPUFlags(apiClient *clients.Settings, nsName string, workerImage string) map[string][]string {
	nodeCPUFlagmap := CPUInfo(apiClient,
		nfdhelpersparams.PodName,
		nsName,
		nfdhelpersparams.ContainerName,
		workerImage)

	nodeCPUFlagsMap := make(map[string][]string)

	for nodeName, nodeLabels := range nodeCPUFlagmap {
		startIndex := strings.Index(nodeLabels, "Features")
		newlineIndex := strings.Index(nodeLabels[startIndex:], "\n")

		flags := strings.Split(strings.ReplaceAll(nodeLabels[startIndex:startIndex+newlineIndex], "Features: ", ""), ",")
		nodeCPUFlagsMap[nodeName] = flags
	}

	return nodeCPUFlagsMap
}

func getWorkerNodes(clientset *clients.Settings) ([]string, error) {
	labelSelector := "node-role.kubernetes.io/worker="

	// Create ListOptions with the label selector
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	workerNodes, err := nodes.List(clientset, listOptions)
	if err != nil {
		return nil, err
	}

	var workerNodeNames []string
	for _, node := range workerNodes {
		workerNodeNames = append(workerNodeNames, node.Object.Name)
	}

	return workerNodeNames, nil
}
