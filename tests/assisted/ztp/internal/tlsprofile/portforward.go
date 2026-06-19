package tlsprofile

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
)

var portForwardStopFuncs = map[string]func(){}

func portForwardKey(endpoint Endpoint) string {
	return fmt.Sprintf("%s:%d", endpoint.ServiceName, endpoint.LocalPort)
}

// StartPortForward sets up a port-forward to the pod behind the endpoint and returns the local address.
func StartPortForward(client *clients.Settings, component *Component, endpoint Endpoint) string {
	key := portForwardKey(endpoint)
	StopPortForward(endpoint)

	targetPod := findPodForEndpoint(client, component, endpoint)

	addr, stop, err := targetPod.PortForward(endpoint.LocalPort, endpoint.RemotePort)
	Expect(err).ToNot(HaveOccurred(), "failed to port-forward to %s", endpoint.ServiceName)

	portForwardStopFuncs[key] = stop

	return addr
}

// StopPortForward closes the port-forward for the given endpoint if one is active.
func StopPortForward(endpoint Endpoint) {
	key := portForwardKey(endpoint)

	if stop, ok := portForwardStopFuncs[key]; ok {
		stop()
		delete(portForwardStopFuncs, key)
	}
}

// StopAllPortForwards closes all active port-forwards.
func StopAllPortForwards() {
	for key, stop := range portForwardStopFuncs {
		stop()
		delete(portForwardStopFuncs, key)
	}
}

func findPodForEndpoint(client *clients.Settings, component *Component, endpoint Endpoint) *pod.Builder {
	pods, err := component.ListPods(client, component.Namespace)
	Expect(err).ToNot(HaveOccurred(), "failed to list %s pods", component.Name)
	Expect(pods).ToNot(BeEmpty(), "no %s pods found", component.Name)

	if endpoint.DeploymentName == "" {
		return pods[0]
	}

	for _, p := range pods {
		if strings.Contains(p.Object.Name, endpoint.DeploymentName) {
			return p
		}
	}

	Fail(fmt.Sprintf("no %s pod found matching deployment %s for endpoint %s",
		component.Name, endpoint.DeploymentName, endpoint.ServiceName))

	return nil
}
