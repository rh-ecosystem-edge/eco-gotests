package tlsprofile

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
)

// FindPod returns the first pod whose name contains deploymentName.
func FindPod(client *clients.Settings, component *Component, deploymentName string) *pod.Builder {
	pods, err := component.ListPods(client, component.Namespace)
	Expect(err).ToNot(HaveOccurred(), "failed to list %s pods", component.Name)

	for _, p := range pods {
		if strings.Contains(p.Object.Name, deploymentName) {
			return p
		}
	}

	Fail(fmt.Sprintf("no %s pod found matching %s", component.Name, deploymentName))

	return nil
}

// WaitPodsReady waits until the expected number of healthy pods is reached.
func WaitPodsReady(client *clients.Settings, component *Component) {
	Eventually(func() int {
		pods, err := component.ListPods(client, component.Namespace)
		if err != nil {
			return 0
		}

		ready := 0

		for _, p := range pods {
			if p.IsHealthy() {
				ready++
			}
		}

		return ready
	}).WithTimeout(component.PodReadyTimeout).WithPolling(10*time.Second).
		Should(BeNumerically(">=", component.ExpectedHealthyPods),
			"%s pods should be ready", component.Name)
}

// RestartPods deletes all component pods and waits for them to come back ready.
func RestartPods(client *clients.Settings, component *Component) {
	pods, err := component.ListPods(client, component.Namespace)
	Expect(err).ToNot(HaveOccurred(), "failed to list %s pods", component.Name)

	for _, p := range pods {
		podName := p.Object.Name
		_, err := p.DeleteAndWait(component.PodReadyTimeout)
		Expect(err).ToNot(HaveOccurred(), "failed to delete pod %s", podName)
	}

	WaitPodsReady(client, component)
}

// WaitPodsRestarted waits for the component to automatically restart after a TLS profile change.
func WaitPodsRestarted(client *clients.Settings, component *Component) {
	switch component.RestartMode {
	case RestartModeContainerRestart:
		waitContainerRestart(client, component)
	case RestartModePodReplacement:
		waitPodReplacement(client, component)
	}

	WaitPodsReady(client, component)
}

func waitContainerRestart(client *clients.Settings, component *Component) {
	restartCounts := make(map[string]int32)

	for _, d := range component.Deployments {
		restartCounts[d.Name] = GetContainerRestartCount(client, component, d)
	}

	for _, d := range component.Deployments {
		deploy := d

		Eventually(func() int32 {
			return GetContainerRestartCount(client, component, deploy)
		}).WithTimeout(component.AutoRestartTimeout).WithPolling(5*time.Second).
			Should(BeNumerically(">", restartCounts[deploy.Name]),
				"%s %s should have restarted", component.Name, deploy.Name)
	}
}

func waitPodReplacement(client *clients.Settings, component *Component) {
	pods, err := component.ListPods(client, component.Namespace)
	Expect(err).ToNot(HaveOccurred(), "failed to list %s pods", component.Name)

	oldNames := make(map[string]bool)

	for _, p := range pods {
		oldNames[p.Object.Name] = true
	}

	Eventually(func() bool {
		currentPods, err := component.ListPods(client, component.Namespace)
		if err != nil {
			return false
		}

		for _, p := range currentPods {
			if oldNames[p.Object.Name] {
				return false
			}
		}

		return len(currentPods) >= component.ExpectedHealthyPods
	}).WithTimeout(component.AutoRestartTimeout).WithPolling(5*time.Second).
		Should(BeTrue(), "%s pods should have been replaced", component.Name)
}

// GetContainerRestartCount returns the restart count of the container for the given deployment.
func GetContainerRestartCount(client *clients.Settings, component *Component, deploy Deployment) int32 {
	p := FindPod(client, component, deploy.Name)

	for _, cs := range p.Object.Status.ContainerStatuses {
		if cs.Name == deploy.ContainerName {
			return cs.RestartCount
		}
	}

	return -1
}

// AssertControllerLogsContain asserts that the deployment's container logs contain the given pattern.
func AssertControllerLogsContain(client *clients.Settings, component *Component,
	deploy Deployment, pattern string) {
	Eventually(func() string {
		p := FindPod(client, component, deploy.Name)

		logs, err := p.GetFullLog(deploy.ContainerName)
		if err != nil {
			return ""
		}

		return logs
	}).WithTimeout(30*time.Second).WithPolling(5*time.Second).
		Should(ContainSubstring(pattern),
			fmt.Sprintf("%s %s logs should contain %q", component.Name, deploy.Name, pattern))
}
