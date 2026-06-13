package tlsprofile

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

var portForwardStopChans = map[string]chan struct{}{}

func portForwardKey(endpoint Endpoint) string {
	return fmt.Sprintf("%s:%d", endpoint.ServiceName, endpoint.LocalPort)
}

// StartPortForward sets up a port-forward to the pod behind the endpoint and returns the local address.
func StartPortForward(client *clients.Settings, component *Component, endpoint Endpoint) string {
	key := portForwardKey(endpoint)
	StopPortForward(endpoint)

	targetPod := findPodForEndpoint(client, component, endpoint)

	restConfig := client.Config
	apiURL, err := url.Parse(restConfig.Host)
	Expect(err).ToNot(HaveOccurred(), "failed to parse API server URL")

	apiURL.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward",
		component.Namespace, targetPod.Object.Name)

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	Expect(err).ToNot(HaveOccurred(), "failed to create SPDY round-tripper")

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, apiURL)

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})

	forwarder, err := portforward.New(dialer,
		[]string{fmt.Sprintf("%d:%d", endpoint.LocalPort, endpoint.RemotePort)},
		stopChan, readyChan, nil, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to create port-forwarder for %s", endpoint.ServiceName)

	go func() {
		_ = forwarder.ForwardPorts()
	}()

	select {
	case <-readyChan:
	case <-time.After(60 * time.Second):
		close(stopChan)
		Fail(fmt.Sprintf("port-forward to %s did not become ready in 60s", endpoint.ServiceName))
	}

	portForwardStopChans[key] = stopChan

	return fmt.Sprintf("localhost:%d", endpoint.LocalPort)
}

// StopPortForward closes the port-forward for the given endpoint if one is active.
func StopPortForward(endpoint Endpoint) {
	key := portForwardKey(endpoint)

	if stopChan, ok := portForwardStopChans[key]; ok {
		close(stopChan)
		delete(portForwardStopChans, key)
	}
}

// StopAllPortForwards closes all active port-forwards.
func StopAllPortForwards() {
	for key, stopChan := range portForwardStopChans {
		close(stopChan)
		delete(portForwardStopChans, key)
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
