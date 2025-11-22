package iperf3workload

import (
	"bytes"
	"slices"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ipsec/internal/ipsecparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	retryDurationSecs = 360
	pollIntervalSecs  = 20
)

// CreateService Create a service for a workload.
// Return nil on success, otherwise return an error.
func CreateService(apiClient *clients.Settings,
	serviceName string,
	selectorLabels map[string]string,
	nodePort int32) (*service.Builder, error) {
	klog.V(ipsecparams.IpsecLogLevel).Infof("Creating Service %s with nodePort %d selectorLables %v",
		serviceName, nodePort, selectorLabels)

	svcPort, err := service.DefineServicePort(
		nodePort,
		nodePort,
		corev1.Protocol("TCP"))
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error defining ServicePort: %v", err)

		return nil, err
	}

	klog.V(ipsecparams.IpsecLogLevel).Infof("Creating Service Builder")

	// This service will be applied to deployments whose labels match the selectorLabels
	svcDemo := service.NewBuilder(apiClient,
		serviceName,
		ipsecparams.TestNamespaceName,
		selectorLabels,
		*svcPort)

	svcDemo = svcDemo.WithNodePort()
	svcDemo.Definition.Spec.Ports[0].NodePort = nodePort

	svcDemo, err = svcDemo.Create()
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error creating service: %v", err)

		return nil, err
	}

	klog.V(ipsecparams.IpsecLogLevel).Infof("Created service: %q in %q namespace",
		svcDemo.Definition.Name, svcDemo.Definition.Namespace)

	return svcDemo, nil
}

// DeleteService Deletes a service.
// Return nil on success, otherwise return an error.
func DeleteService(apiClient *clients.Settings, serviceName string) error {
	klog.V(ipsecparams.IpsecLogLevel).Infof("Deleting Service %q in %q namespace",
		serviceName, ipsecparams.TestNamespaceName)

	svcDemo, err := service.Pull(apiClient, serviceName, ipsecparams.TestNamespaceName)

	if err != nil && svcDemo == nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Service %q not found in %q namespace",
			serviceName, ipsecparams.TestNamespaceName)

		return err
	}

	err = svcDemo.Delete()
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error deleting service: %v", err)

		return err
	}

	klog.V(ipsecparams.IpsecLogLevel).Infof("Deleted service %q in %q namespace",
		serviceName, ipsecparams.TestNamespaceName)

	return nil
}

// CreateWorkload Create a workload with the iperf3 image, the iperf3 command will be
// launched from either LaunchIperf3Client() or LaunchIperf3Server().
// Return nil on success, otherwise return an error.
func CreateWorkload(apiClient *clients.Settings,
	deploymentName string,
	nodeName string,
	workloadLabels map[string]string,
	iperf3ToolImage string) (*deployment.Builder, error) {
	klog.V(ipsecparams.IpsecLogLevel).Infof("Creating Workload %s on node %s, image %s with labels %v",
		deploymentName,
		nodeName,
		iperf3ToolImage,
		workloadLabels)

	deployContainer := pod.NewContainerBuilder(deploymentName,
		iperf3ToolImage,
		ipsecparams.ContainerCmdSleep)

	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error getting container cfg: %s", err)

		return nil, err
	}

	// Services whose selectorLabels match this deployment labels will be applied
	// to this deployment.
	createDeploy := deployment.NewBuilder(apiClient,
		deploymentName,
		ipsecparams.TestNamespaceName,
		workloadLabels,
		*deployContainerCfg)
	createDeploy = createDeploy.WithNodeSelector(map[string]string{"kubernetes.io/hostname": nodeName})

	_, err = createDeploy.CreateAndWaitUntilReady(300 * time.Second)
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error deploying container: %s", err)

		return nil, err
	}

	return createDeploy, nil
}

// DeleteWorkload Delete a workload.
// Return nil on success, otherwise return an error.
func DeleteWorkload(apiClient *clients.Settings, deploymentName string, workloadLabels string) error {
	var (
		oldPods []*pod.Builder
		err     error
	)

	pullDeploy, _ := deployment.Pull(apiClient, deploymentName, ipsecparams.TestNamespaceName)
	if pullDeploy == nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Deployment %q not found in %q ns",
			deploymentName, ipsecparams.TestNamespaceName)
	}

	err = pullDeploy.Delete()
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Failed to delete deployment %q: %v",
			deploymentName, err)

		return err
	}

	totalPollTime := 0
	pollSuccess := false

	continueLooping := true
	for continueLooping {
		oldPods, err = pod.List(apiClient, ipsecparams.TestNamespaceName,
			metav1.ListOptions{LabelSelector: workloadLabels})
		if err == nil {
			pollSuccess = true
			continueLooping = false

			klog.V(ipsecparams.IpsecLogLevel).Infof("Found %d pods matching label %q ",
				len(oldPods), workloadLabels)
		} else {
			time.Sleep(pollIntervalSecs)

			totalPollTime += pollIntervalSecs
			if totalPollTime > retryDurationSecs {
				continueLooping = false
			}
		}
	}

	if !pollSuccess {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error listing pods in %q namespace",
			ipsecparams.TestNamespaceName)

		return err
	}

	if len(oldPods) == 0 {
		klog.V(ipsecparams.IpsecLogLevel).Infof("No pods matching label %q found in %q namespace",
			workloadLabels, ipsecparams.TestNamespaceName)
	}

	for _, _pod := range oldPods {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Deleting pod %q in %q namspace",
			_pod.Definition.Name, _pod.Definition.Namespace)

		_pod, err = _pod.DeleteAndWait(300 * time.Second)
		if err != nil {
			klog.V(ipsecparams.IpsecLogLevel).Infof("Failed to delete pod %q: %v",
				_pod.Definition.Name, err)

			return err
		}
	}

	return nil
}

// LaunchIperf3Command launches the iperf3 command in an already running workload
// Return nil on success, otherwise return an error.
func LaunchIperf3Command(apiClient *clients.Settings,
	deploymentName string,
	iperf3Command []string,
	containerLabels string) bool {
	// deployName       =>  deploymentName
	// deployNS         =>  ipsecparams.TestNamespaceName
	// deployLabel      =>  containerLabels
	// containerName    =>  deploymentName
	klog.V(ipsecparams.IpsecLogLevel).Infof("Check deployment %q exists in %q namespace",
		deploymentName, ipsecparams.TestNamespaceName)

	pullDeploy, _ := deployment.Pull(apiClient, deploymentName, ipsecparams.TestNamespaceName)

	if pullDeploy == nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Deployment %q not found in %q ns",
			deploymentName, ipsecparams.TestNamespaceName)
	}

	var (
		appPods []*pod.Builder
		err     error
		output  bytes.Buffer
	)

	klog.V(ipsecparams.IpsecLogLevel).Infof("Finding pod backed by deployment")

	totalPollTime := 0
	pollSuccess := false

	continueLooping := true
	for continueLooping {
		appPods, err = pod.List(apiClient,
			ipsecparams.TestNamespaceName,
			metav1.ListOptions{LabelSelector: containerLabels})
		if err == nil {
			pollSuccess = true
			continueLooping = false

			klog.V(ipsecparams.IpsecLogLevel).Infof("Found %d pods matching label %q",
				len(appPods), containerLabels)
		} else {
			time.Sleep(pollIntervalSecs)

			totalPollTime += pollIntervalSecs
			if totalPollTime > retryDurationSecs {
				continueLooping = false
			}
		}
	}

	if !pollSuccess {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Failed to find pods matching label %q",
			containerLabels)

		return false
	}

	for _, _pod := range appPods {
		cmdIperf3 := append(slices.Clone(ipsecparams.ContainerCmdBash), strings.Join(iperf3Command, " "))
		klog.V(ipsecparams.IpsecLogLevel).Infof("Running command %q from within a pod %q with labels %v",
			cmdIperf3, _pod.Definition.Name, _pod.Definition.ObjectMeta.Labels)

		output, err = _pod.ExecCommand(cmdIperf3, deploymentName)
		if err != nil {
			klog.V(ipsecparams.IpsecLogLevel).Infof(
				"Error running iperf3 lookup from within pod, output: [%s], err [%s]",
				output.String(), err)

			return false
		}

		klog.V(ipsecparams.IpsecLogLevel).Infof("Command's Output:\n%v\n", output.String())
	}

	return true
}
