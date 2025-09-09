package exec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpucommon"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodCommand represents a command to be executed in a Kubernetes pod.
type PodCommand struct {
	apiClient     *clients.Settings
	name          string
	nsname        string
	image         string
	containerName string
	commad        []string
	resources     *ResourceSettings
	ActualPod     *pod.Builder

	executionMode ExecutionMode
	script        string

	// Node execution options
	nodeName    string
	privileged  bool
	hostNetwork bool
	hostPID     bool
	hostVolumes []HostVolumeMount
}

// ExecutionMode defines how the command should be executed.
type ExecutionMode int

const (
	// ShellCommand uses /bin/sh -c (existing behavior).
	ShellCommand ExecutionMode = iota
	// BashScript uses /bin/bash with script as argument.
	BashScript
	// DirectCommand executes command directly without shell.
	DirectCommand
)

// ResourceSettings contains CPU and memory resource requests and limits.
type ResourceSettings struct {
	Requests map[corev1.ResourceName]string
	Limits   map[corev1.ResourceName]string
}

// HostVolumeMount represents a host volume mount.
type HostVolumeMount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// NewPodCommand creates a new PodCommand with shell execution mode.
func NewPodCommand(
	apiClient *clients.Settings,
	name,
	nsname,
	image,
	containerName string,
	commad []string,
	requests, limits map[string]string) *PodCommand {
	resources := &ResourceSettings{
		Requests: convertToResourceNameMap(requests),
		Limits:   convertToResourceNameMap(limits),
	}

	return &PodCommand{
		apiClient:     apiClient,
		name:          name,
		nsname:        nsname,
		image:         image,
		containerName: containerName,
		commad:        commad,
		resources:     resources,
		executionMode: ShellCommand,
	}
}

// NewPodCommandWithBashScript creates a PodCommand that executes a bash script.
// This is better for complex scripts as it avoids shell escaping issues.
func NewPodCommandWithBashScript(
	apiClient *clients.Settings,
	name,
	nsname,
	image,
	containerName string,
	script string,
	requests, limits map[string]string) *PodCommand {
	resources := &ResourceSettings{
		Requests: convertToResourceNameMap(requests),
		Limits:   convertToResourceNameMap(limits),
	}

	return &PodCommand{
		apiClient:     apiClient,
		name:          name,
		nsname:        nsname,
		image:         image,
		containerName: containerName,
		script:        script,
		resources:     resources,
		executionMode: BashScript,
	}
}

// NewPodCommandDirect creates a PodCommand that executes commands directly without shell.
func NewPodCommandDirect(
	apiClient *clients.Settings,
	name,
	nsname,
	image,
	containerName string,
	command []string,
	requests, limits map[string]string) *PodCommand {
	resources := &ResourceSettings{
		Requests: convertToResourceNameMap(requests),
		Limits:   convertToResourceNameMap(limits),
	}

	return &PodCommand{
		apiClient:     apiClient,
		name:          name,
		nsname:        nsname,
		image:         image,
		containerName: containerName,
		commad:        command,
		resources:     resources,
		executionMode: DirectCommand,
	}
}

// Run executes the pod command based on the specified execution mode.
func (p *PodCommand) Run() error {
	podWorker := pod.NewBuilder(p.apiClient, p.name, p.nsname, p.image)

	container, err := p.getContainerConfig()
	if err != nil {
		glog.Errorf("Failed to get container configuration: %v", err)

		return err
	}

	podWorker.Definition.Spec.Containers = make([]corev1.Container, 0)
	podWorker.Definition.Spec.Containers = append(podWorker.Definition.Spec.Containers, *container)
	podWorker.Definition.Spec.RestartPolicy = corev1.RestartPolicyNever

	// Configure node placement
	if p.nodeName != "" {
		podWorker.Definition.Spec.NodeName = p.nodeName
	} else {
		if isSNO, err := amdgpucommon.IsSingleNodeOpenShift(p.apiClient); err == nil && isSNO {
			podWorker.Definition.Spec.NodeSelector = map[string]string{}
		} else {
			podWorker.Definition.Spec.NodeSelector = map[string]string{"node-role.kubernetes.io/worker": ""}
		}
	}

	// Configure host access
	if p.hostNetwork {
		podWorker.Definition.Spec.HostNetwork = true
	}

	if p.hostPID {
		podWorker.Definition.Spec.HostPID = true
	}

	// Configure host volumes
	if len(p.hostVolumes) > 0 {
		podWorker.Definition.Spec.Volumes = make([]corev1.Volume, 0)

		for i, hostVol := range p.hostVolumes {
			volumeName := fmt.Sprintf("host-vol-%d", i)
			hostPathType := corev1.HostPathDirectoryOrCreate

			podWorker.Definition.Spec.Volumes = append(podWorker.Definition.Spec.Volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: hostVol.HostPath,
						Type: &hostPathType,
					},
				},
			})

			// Add volume mount to container
			podWorker.Definition.Spec.Containers[0].VolumeMounts = append(
				podWorker.Definition.Spec.Containers[0].VolumeMounts,
				corev1.VolumeMount{
					Name:      volumeName,
					MountPath: hostVol.ContainerPath,
					ReadOnly:  hostVol.ReadOnly,
				},
			)
		}
	}

	// Set termination grace period for faster cleanup
	terminationGracePeriod := int64(1)
	podWorker.Definition.Spec.TerminationGracePeriodSeconds = &terminationGracePeriod

	p.ActualPod, err = podWorker.Create()

	if err != nil {
		glog.Errorf("Error creating pod: %s", err.Error())

		return err
	}

	return nil
}

func (p *PodCommand) getContainerConfig() (*corev1.Container, error) {
	var (
		containerBuilder *pod.ContainerBuilder
		container        *corev1.Container
		err              error
	)

	switch p.executionMode {
	case ShellCommand:
		containerBuilder = pod.NewContainerBuilder(p.containerName, p.image, []string{"/bin/sh", "-c"})
		container, err = containerBuilder.GetContainerCfg()

		if err != nil {
			return nil, fmt.Errorf("failed to create shell container: %w", err)
		}

		container.Args = []string{strings.Join(p.commad, " ")}

	case BashScript:
		containerBuilder = pod.NewContainerBuilder(p.containerName, p.image, []string{"/bin/bash"})
		container, err = containerBuilder.GetContainerCfg()

		if err != nil {
			return nil, fmt.Errorf("failed to create bash container: %w", err)
		}

		container.Command = []string{"/bin/bash"}
		container.Args = []string{"-c", p.script}

	case DirectCommand:
		if len(p.commad) == 0 {
			return nil, fmt.Errorf("no command specified for DirectCommand mode")
		}

		containerBuilder = pod.NewContainerBuilder(p.containerName, p.image, []string{p.commad[0]})
		container, err = containerBuilder.GetContainerCfg()

		if err != nil {
			return nil, fmt.Errorf("failed to create direct container: %w", err)
		}

		container.Command = []string{p.commad[0]}
		if len(p.commad) > 1 {
			container.Args = p.commad[1:]
		}

	default:
		return nil, fmt.Errorf("unknown execution mode: %v", p.executionMode)
	}

	// Apply common configuration
	container.Resources, err = p.resources.toResourceRequirements()
	if err != nil {
		return nil, fmt.Errorf("failed to set resources: %w", err)
	}

	// Configure security context
	if p.privileged {
		if container.SecurityContext == nil {
			container.SecurityContext = &corev1.SecurityContext{}
		}

		container.SecurityContext.Privileged = &p.privileged
	}

	return container, nil
}

func (rs *ResourceSettings) toResourceRequirements() (corev1.ResourceRequirements, error) {
	reqs := corev1.ResourceList{}
	lims := corev1.ResourceList{}

	for res, value := range rs.Requests {
		q, err := resource.ParseQuantity(value)
		if err != nil {
			return corev1.ResourceRequirements{}, fmt.Errorf("invalid request %s=%q: %w", res, value, err)
		}

		reqs[res] = q
	}

	for res, value := range rs.Limits {
		q, err := resource.ParseQuantity(value)
		if err != nil {
			return corev1.ResourceRequirements{}, fmt.Errorf("invalid limit %s=%q: %w", res, value, err)
		}

		lims[res] = q
	}

	return corev1.ResourceRequirements{Requests: reqs, Limits: lims}, nil
}

func convertToResourceNameMap(input map[string]string) map[corev1.ResourceName]string {
	result := make(map[corev1.ResourceName]string)
	for k, v := range input {
		result[corev1.ResourceName(k)] = v
	}

	return result
}

// WithPrivileged sets the privileged flag for the pod.
func (p *PodCommand) WithPrivileged(privileged bool) *PodCommand {
	p.privileged = privileged

	return p
}

// WithHostNetwork sets the host network flag for the pod.
func (p *PodCommand) WithHostNetwork(hostNetwork bool) *PodCommand {
	p.hostNetwork = hostNetwork

	return p
}

// WithHostPID sets the host PID flag for the pod.
func (p *PodCommand) WithHostPID(hostPID bool) *PodCommand {
	p.hostPID = hostPID

	return p
}

// WithNodeName sets the target node for the pod.
func (p *PodCommand) WithNodeName(nodeName string) *PodCommand {
	p.nodeName = nodeName

	return p
}

// WithHostVolume adds a host volume mount to the pod.
func (p *PodCommand) WithHostVolume(hostPath, containerPath string, readOnly bool) *PodCommand {
	p.hostVolumes = append(p.hostVolumes, HostVolumeMount{
		HostPath:      hostPath,
		ContainerPath: containerPath,
		ReadOnly:      readOnly,
	})

	return p
}

// Execute runs the pod command and returns the output.
func (p *PodCommand) Execute() (string, error) {
	err := p.Run()
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %w", err)
	}

	// Wait for pod to be running
	err = p.ActualPod.WaitUntilRunning(2 * time.Minute)
	if err != nil {
		return "", fmt.Errorf("pod failed to start: %w", err)
	}

	// Execute the command
	output, err := p.ActualPod.ExecCommand([]string{"sh", "-c", strings.Join(p.commad, " ")})
	if err != nil {
		return output.String(), fmt.Errorf("command execution failed: %w", err)
	}

	return strings.TrimSpace(output.String()), nil
}

// ExecuteAndCleanup runs the pod command, returns output, and cleans up the pod.
func (p *PodCommand) ExecuteAndCleanup(timeout time.Duration) (string, error) {
	err := p.Run()
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %w", err)
	}

	// Ensure cleanup happens even if execution fails
	defer func() {
		if cleanupErr := p.Cleanup(timeout); cleanupErr != nil {
			glog.Errorf("Failed to cleanup pod %s: %v", p.name, cleanupErr)
		}
	}()

	// Wait for pod completion (for one-shot pods) - wait until it's no longer running
	err = p.ActualPod.WaitUntilCondition(corev1.PodReady, timeout)
	if err != nil {
		glog.V(90).Infof("Pod %s may have failed, retrieving logs", p.name)
	}

	// Alternative approach: poll for completion status
	completionErr := p.waitForPodCompletion(timeout)

	// Get the logs regardless of success/failure
	output, logErr := p.GetPodLogs()
	if logErr != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", logErr)
	}

	if completionErr != nil {
		return output, fmt.Errorf("pod execution failed: %w", completionErr)
	}

	return strings.TrimSpace(output), nil
}

// Cleanup removes the pod and waits for deletion.
func (p *PodCommand) Cleanup(timeout time.Duration) error {
	if p.ActualPod == nil {
		return nil
	}

	_, err := p.ActualPod.DeleteAndWait(timeout)

	return err
}

// GetPodLogs retrieves logs from the pod.
func (p *PodCommand) GetPodLogs() (string, error) {
	if p.ActualPod == nil {
		return "", fmt.Errorf("pod not created yet")
	}

	// Use the simplified approach similar to original implementation
	logs, err := p.apiClient.CoreV1Interface.Pods(p.nsname).GetLogs(
		p.name, &corev1.PodLogOptions{}).DoRaw(context.TODO())
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}

	return strings.TrimSpace(string(logs)), nil
}

// waitForPodCompletion waits for a pod to complete (succeed or fail).
func (p *PodCommand) waitForPodCompletion(timeout time.Duration) error {
	if p.ActualPod == nil {
		return fmt.Errorf("pod not created yet")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			podStatus, err := p.apiClient.CoreV1Interface.Pods(p.nsname).Get(
				ctx, p.name, metav1.GetOptions{})
			if err != nil {
				time.Sleep(5 * time.Second)

				continue
			}

			if podStatus.Status.Phase == corev1.PodSucceeded || podStatus.Status.Phase == corev1.PodFailed {
				if podStatus.Status.Phase == corev1.PodFailed {
					return fmt.Errorf("pod failed with phase: %s", podStatus.Status.Phase)
				}

				return nil
			}

			time.Sleep(5 * time.Second)
		}
	}
}
