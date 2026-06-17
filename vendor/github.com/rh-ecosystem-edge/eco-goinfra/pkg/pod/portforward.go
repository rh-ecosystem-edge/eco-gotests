package pod

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog/v2"
)

const defaultPortForwardReadyTimeout = 60 * time.Second

// PortForward establishes a port-forward to the pod and returns the local address (e.g. "localhost:8443")
// and a stop function. Callers must invoke the stop function to close the port-forward when done.
func (builder *Builder) PortForward(localPort, remotePort int) (string, func(), error) {
	if valid, err := builder.validate(); !valid {
		return "", nil, err
	}

	if !builder.Exists() {
		return "", nil, fmt.Errorf("pod object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	klog.V(100).Infof("Setting up port-forward %d:%d to pod %s in namespace %s",
		localPort, remotePort, builder.Object.Name, builder.Object.Namespace)

	restConfig := builder.apiClient.Config

	req := builder.apiClient.CoreV1Interface.RESTClient().
		Post().
		Namespace(builder.Object.Namespace).
		Resource("pods").
		Name(builder.Object.Name).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SPDY round-tripper: %w", err)
	}

	spdyDialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	wsDialer, err := portforward.NewSPDYOverWebsocketDialer(req.URL(), restConfig)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create WebSocket dialer: %w", err)
	}

	dialer := portforward.NewFallbackDialer(wsDialer, spdyDialer, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})

	forwarder, err := portforward.New(dialer,
		[]string{fmt.Sprintf("%d:%d", localPort, remotePort)},
		stopChan, readyChan, nil, nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create port-forwarder: %w", err)
	}

	errChan := make(chan error, 1)

	go func() {
		errChan <- forwarder.ForwardPorts()
	}()

	select {
	case <-readyChan:
	case err := <-errChan:
		return "", nil, fmt.Errorf("port-forward failed: %w", err)
	case <-time.After(defaultPortForwardReadyTimeout):
		close(stopChan)

		return "", nil, fmt.Errorf("port-forward to pod %s did not become ready in %s",
			builder.Object.Name, defaultPortForwardReadyTimeout)
	}

	ports, err := forwarder.GetPorts()
	if err != nil {
		close(stopChan)

		return "", nil, fmt.Errorf("failed to get forwarded ports: %w", err)
	}

	var once sync.Once

	stop := func() {
		once.Do(func() { close(stopChan) })
	}

	return fmt.Sprintf("localhost:%d", ports[0].Local), stop, nil
}
