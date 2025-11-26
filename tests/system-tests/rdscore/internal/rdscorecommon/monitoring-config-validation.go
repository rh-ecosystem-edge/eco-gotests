package rdscorecommon

// This test was written in part with AI assistance.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	monitoringConfigMapName      = "cluster-monitoring-config"
	monitoringNamespace          = "openshift-monitoring"
	monitoringConfigYAMLKey      = "config.yaml"
	remoteWriteTestPodName       = "prometheus-remote-write-test-server"
	remoteWriteTestServiceName   = "prometheus-remote-write-test-service"
	remoteWriteTestNamespace     = "openshift-monitoring"
	remoteWriteTestPort          = 8080
	remoteWriteTestContainerPort = 8080
	pythonHTTPServerImage        = "registry.access.redhat.com/ubi9/python-39:latest"
)

// HTTPStats represents the statistics from the HTTP server
type HTTPStats struct {
	Connections int64 `json:"connections"`
	Bytes       int64 `json:"bytes"`
}

// VerifyMonitoringConfigRemoteWrite verifies that the cluster monitoring configuration
// contains a remoteWrite endpoint under prometheusK8s in the config.yaml data.
// It also creates an HTTP server, adds a remoteWrite endpoint, and verifies connections.
func VerifyMonitoringConfigRemoteWrite() {
	var (
		httpServerPod  *pod.Builder
		cmBuilder      *configmap.Builder
		err            error
		ctx            SpecContext
		serviceURL     string
		initialStats   HTTPStats
		preUpdateStats HTTPStats
		finalStats     HTTPStats
	)

	// Step 1: Create namespace if it doesn't exist
	By(fmt.Sprintf("Ensuring namespace %q exists", remoteWriteTestNamespace))
	nsBuilder := namespace.NewBuilder(APIClient, remoteWriteTestNamespace)
	if !nsBuilder.Exists() {
		_, err = nsBuilder.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create namespace %q", remoteWriteTestNamespace))
	}

	// Step 2: Create HTTP server pod that tracks POST requests
	By("Creating HTTP server pod to receive remoteWrite requests")
	httpServerScript := fmt.Sprintf(`#!/usr/bin/env python3
import http.server
import socketserver
import json
import threading
from urllib.parse import urlparse

class Stats:
    def __init__(self):
        self.connections = 0
        self.bytes = 0
        self.lock = threading.Lock()

    def add_connection(self, bytes_received):
        with self.lock:
            self.connections += 1
            self.bytes += bytes_received

    def get_stats(self):
        with self.lock:
            return {"connections": self.connections, "bytes": self.bytes}

stats = Stats()

class RemoteWriteHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length)
        stats.add_connection(len(body))

        self.send_response(200)
        self.send_header('Content-type', 'application/json')
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}')

    def do_GET(self):
        if self.path == '/stats':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = json.dumps(stats.get_stats())
            self.wfile.write(response.encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass  # Suppress default logging

PORT = %d
with socketserver.TCPServer(("", PORT), RemoteWriteHandler) as httpd:
    httpd.serve_forever()
`, remoteWriteTestContainerPort)

	cPort := corev1.ContainerPort{
		ContainerPort: remoteWriteTestContainerPort,
		Protocol:      corev1.ProtocolTCP,
	}

	containerBuilder := pod.NewContainerBuilder(remoteWriteTestPodName, pythonHTTPServerImage,
		[]string{"python3", "-c", httpServerScript}).
		WithPorts([]corev1.ContainerPort{cPort})

	container, err := containerBuilder.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container configuration")

	httpServerPod = pod.NewBuilder(APIClient, remoteWriteTestPodName, remoteWriteTestNamespace, pythonHTTPServerImage).
		WithLabel("app", remoteWriteTestPodName).
		WithAdditionalContainer(container)

	Eventually(func() error {
		httpServerPod, err = httpServerPod.CreateAndWaitUntilRunning(2 * time.Minute)
		return err
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(3*time.Minute).Should(Succeed(),
		"Failed to create HTTP server pod")

	// Step 3: Create service for the HTTP server pod
	By("Creating service for HTTP server pod")
	svcPort, err := service.DefineServicePort(
		remoteWriteTestPort,
		remoteWriteTestContainerPort,
		corev1.ProtocolTCP)
	Expect(err).ToNot(HaveOccurred(), "Failed to define service port")

	_, err = service.NewBuilder(APIClient, remoteWriteTestServiceName, remoteWriteTestNamespace,
		map[string]string{"app": remoteWriteTestPodName}, *svcPort).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create service")

	// Register cleanup early so it runs even if test fails
	DeferCleanup(func() {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cleaning up test resources")
		if httpServerPod != nil {
			_, _ = httpServerPod.DeleteAndWait(2 * time.Minute)
		}
		svcBuilder, err := service.Pull(APIClient, remoteWriteTestServiceName, remoteWriteTestNamespace)
		if err == nil {
			_ = svcBuilder.Delete()
		}
	})

	// Step 4: Get service URL
	serviceURL = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/api/v1/write",
		remoteWriteTestServiceName, remoteWriteTestNamespace, remoteWriteTestPort)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("HTTP server service URL: %s", serviceURL)

	// Step 5: Get initial stats and verify rate of data is near 0 before ConfigMap update
	By("Getting initial stats from HTTP server (before ConfigMap update)")
	Eventually(func() error {
		initialStats, err = getHTTPStats(httpServerPod, remoteWriteTestPodName)
		return err
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(Succeed(),
		"Failed to get initial stats from HTTP server")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Initial stats: connections=%d, bytes=%d",
		initialStats.Connections, initialStats.Bytes)

	// Wait up to 10 minutes for the rate of connections and data to be near 0. This helps
	// stabilize the testing when run back-to-back in the same environment where it takes
	// a bit of time for prometheus to respond to the tear down of the prior config.
	By("Waiting for rate of connections and data to stabilize near 0 (up to 10 minutes)")
	checkInterval := 10 * time.Second
	requiredStableChecks := 3 // Number of consecutive checks with no change required

	var previousStats HTTPStats
	previousStats = initialStats
	stableCheckCount := 0

	Eventually(func() bool {
		// Wait for the check interval
		time.Sleep(checkInterval)

		currentStats, err := getHTTPStats(httpServerPod, remoteWriteTestPodName)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error getting stats during rate check: %v", err)
			stableCheckCount = 0 // Reset on error
			return false
		}

		// Check if stats values have changed
		connectionsChanged := currentStats.Connections != previousStats.Connections
		bytesChanged := currentStats.Bytes != previousStats.Bytes

		if connectionsChanged || bytesChanged {
			// Stats changed, reset stable check counter
			stableCheckCount = 0
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Stats changed: connections %d->%d (delta: %d), bytes %d->%d (delta: %d)",
				previousStats.Connections, currentStats.Connections, currentStats.Connections-previousStats.Connections,
				previousStats.Bytes, currentStats.Bytes, currentStats.Bytes-previousStats.Bytes)
		} else {
			// Stats are stable (no change)
			stableCheckCount++
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Stats stable (check %d/%d): connections=%d, bytes=%d (no change)",
				stableCheckCount, requiredStableChecks, currentStats.Connections, currentStats.Bytes)

			// If we've had enough consecutive stable checks, we've reached steady state
			if stableCheckCount >= requiredStableChecks {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Steady state reached: stats have been stable for %d consecutive checks (%.1f minutes)",
					stableCheckCount, float64(stableCheckCount)*checkInterval.Minutes())
				preUpdateStats = currentStats
				return true
			}
		}

		// Update for next iteration
		previousStats = currentStats

		return false
	}).WithContext(ctx).WithPolling(checkInterval).WithTimeout(10*time.Minute).Should(BeTrue(),
		"Rate of connections and data did not stabilize near 0 within 10 minutes")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pre-update stats (after rate stabilization): connections=%d, bytes=%d",
		preUpdateStats.Connections, preUpdateStats.Bytes)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Confirmed: Rate stabilized at 0 (no new connections or bytes) before ConfigMap update")

	// Step 6: Pull and update the ConfigMap
	By(fmt.Sprintf("Pulling ConfigMap %q from namespace %q",
		monitoringConfigMapName, monitoringNamespace))

	Eventually(func() bool {
		cmBuilder, err = configmap.Pull(APIClient, monitoringConfigMapName, monitoringNamespace)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error pulling ConfigMap %q from namespace %q: %v",
				monitoringConfigMapName, monitoringNamespace, err)
			return false
		}
		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to pull ConfigMap %q from namespace %q", monitoringConfigMapName, monitoringNamespace))

	By(fmt.Sprintf("Verifying ConfigMap %q contains key %q",
		monitoringConfigMapName, monitoringConfigYAMLKey))

	configYAML, keyExists := cmBuilder.Object.Data[monitoringConfigYAMLKey]
	if !keyExists {
		// If config.yaml doesn't exist, create it
		configYAML = "prometheusK8s:\n  remoteWrite: []\n"
		if cmBuilder.Object.Data == nil {
			cmBuilder.Object.Data = make(map[string]string)
		}
		cmBuilder.Object.Data[monitoringConfigYAMLKey] = configYAML
	}

	By("Parsing and updating config.yaml to add remoteWrite endpoint")

	var config map[string]interface{}
	err = yaml.Unmarshal([]byte(configYAML), &config)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse YAML from ConfigMap %q key %q", monitoringConfigMapName, monitoringConfigYAMLKey))

	// Ensure prometheusK8s section exists
	prometheusK8s, prometheusK8sExists := config["prometheusK8s"]
	if !prometheusK8sExists {
		config["prometheusK8s"] = make(map[interface{}]interface{})
		prometheusK8s = config["prometheusK8s"]
	}

	prometheusK8sMap, ok := prometheusK8s.(map[interface{}]interface{})
	Expect(ok).To(BeTrue(), "prometheusK8s is not a valid map structure")

	// Get or create remoteWrite array
	remoteWrite, remoteWriteExists := prometheusK8sMap["remoteWrite"]
	var remoteWriteSlice []interface{}
	if remoteWriteExists {
		remoteWriteSlice, ok = remoteWrite.([]interface{})
		if !ok {
			// If it's not a slice, convert it to a slice
			remoteWriteSlice = []interface{}{remoteWrite}
		}
	} else {
		remoteWriteSlice = []interface{}{}
	}

	// Add new remoteWrite endpoint
	newRemoteWriteEndpoint := map[interface{}]interface{}{
		"url": serviceURL,
	}
	remoteWriteSlice = append(remoteWriteSlice, newRemoteWriteEndpoint)
	prometheusK8sMap["remoteWrite"] = remoteWriteSlice

	// Marshal back to YAML
	updatedConfigYAML, err := yaml.Marshal(config)
	Expect(err).ToNot(HaveOccurred(), "Failed to marshal updated config to YAML")

	// Update ConfigMap
	cmBuilder.Object.Data[monitoringConfigYAMLKey] = string(updatedConfigYAML)

	By(fmt.Sprintf("Updating ConfigMap %q in namespace %q",
		monitoringConfigMapName, monitoringNamespace))

	Eventually(func() error {
		_, err = cmBuilder.Update()
		return err
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(Succeed(),
		fmt.Sprintf("Failed to update ConfigMap %q", monitoringConfigMapName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully updated ConfigMap with new remoteWrite endpoint")

	// Register cleanup for ConfigMap right after updating it
	DeferCleanup(func() {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cleaning up: Removing test remoteWrite endpoint from ConfigMap")
		cmBuilder, err := configmap.Pull(APIClient, monitoringConfigMapName, monitoringNamespace)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull ConfigMap for cleanup: %v", err)
			return
		}

		configYAML, exists := cmBuilder.Object.Data[monitoringConfigYAMLKey]
		if !exists {
			return
		}

		var config map[string]interface{}
		if err := yaml.Unmarshal([]byte(configYAML), &config); err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to parse config for cleanup: %v", err)
			return
		}

		prometheusK8s, exists := config["prometheusK8s"]
		if !exists {
			return
		}

		prometheusK8sMap, ok := prometheusK8s.(map[interface{}]interface{})
		if !ok {
			return
		}

		remoteWrite, exists := prometheusK8sMap["remoteWrite"]
		if !exists {
			return
		}

		remoteWriteSlice, ok := remoteWrite.([]interface{})
		if !ok {
			return
		}

		// Remove the test endpoint (the one with our service URL)
		filteredSlice := []interface{}{}
		for _, endpoint := range remoteWriteSlice {
			endpointMap, ok := endpoint.(map[interface{}]interface{})
			if !ok {
				continue
			}
			url, ok := endpointMap["url"].(string)
			if !ok || !strings.Contains(url, remoteWriteTestServiceName) {
				filteredSlice = append(filteredSlice, endpoint)
			}
		}

		if len(filteredSlice) != len(remoteWriteSlice) {
			prometheusK8sMap["remoteWrite"] = filteredSlice
			updatedConfigYAML, err := yaml.Marshal(config)
			if err == nil {
				cmBuilder.Object.Data[monitoringConfigYAMLKey] = string(updatedConfigYAML)
				_, err = cmBuilder.Update()
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to update ConfigMap during cleanup: %v", err)
				}
			}
		}
	})

	// Step 7: Wait for Prometheus to pick up the config and start sending data
	By("Waiting for Prometheus to start sending data to remoteWrite endpoint")
	time.Sleep(30 * time.Second) // Give Prometheus time to reload config and send data

	// Step 8: Get final stats and verify connections were made
	By("Getting final stats from HTTP server")
	Eventually(func() bool {
		finalStats, err = getHTTPStats(httpServerPod, remoteWriteTestPodName)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error getting stats: %v", err)
			return false
		}
		// Check if we received significantly more connections than before the update
		// Use preUpdateStats as baseline since that's right before the ConfigMap update
		return finalStats.Connections > preUpdateStats.Connections+50
	}).WithContext(ctx).WithPolling(10*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"Prometheus did not send data to remoteWrite endpoint")

	// Calculate increases for clear reporting
	connectionsIncreaseAfterUpdate := finalStats.Connections - preUpdateStats.Connections
	bytesIncreaseAfterUpdate := finalStats.Bytes - preUpdateStats.Bytes

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Final stats: connections=%d, bytes=%d",
		finalStats.Connections, finalStats.Bytes)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Bytes received after ConfigMap update: %d bytes (%.2f KB, %.2f MB)",
		bytesIncreaseAfterUpdate, float64(bytesIncreaseAfterUpdate)/1024, float64(bytesIncreaseAfterUpdate)/(1024*1024))
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Connections received after ConfigMap update: %d",
		connectionsIncreaseAfterUpdate)

	// Verify that connections and bytes increased significantly after ConfigMap update
	// Compare against preUpdateStats (right before update) to ensure the increase is from Prometheus
	Expect(connectionsIncreaseAfterUpdate).To(BeNumerically(">", 50),
		"Expected connections to increase by more than 50 after ConfigMap update (pre-update: %d, final: %d, increase: %d)",
		preUpdateStats.Connections, finalStats.Connections, connectionsIncreaseAfterUpdate)
	Expect(bytesIncreaseAfterUpdate).To(BeNumerically(">", (1024*100)),
		"Expected bytes to increase by more than 100KB after ConfigMap update (pre-update: %d bytes, final: %d bytes, increase: %d bytes = %.2f KB)",
		preUpdateStats.Bytes, finalStats.Bytes, bytesIncreaseAfterUpdate, float64(bytesIncreaseAfterUpdate)/1024)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully verified remoteWrite endpoint is receiving data")
}

// getHTTPStats queries the HTTP server pod for statistics
func getHTTPStats(podBuilder *pod.Builder, containerName string) (HTTPStats, error) {
	var stats HTTPStats

	// Execute curl command in the pod to get stats
	// First try with curl, if not available, use python
	statsURL := fmt.Sprintf("http://localhost:%d/stats", remoteWriteTestContainerPort)
	output, err := podBuilder.ExecCommand([]string{"curl", "-s", statsURL}, containerName)
	if err != nil {
		// Fallback to python if curl is not available
		pythonStatsCmd := fmt.Sprintf("import urllib.request; import json; print(json.dumps(json.loads(urllib.request.urlopen('%s').read().decode())))", statsURL)
		output, err = podBuilder.ExecCommand([]string{"python3", "-c", pythonStatsCmd},
			containerName)
		if err != nil {
			return stats, fmt.Errorf("failed to execute command to get stats: %w", err)
		}
	}

	// Parse JSON response
	err = json.Unmarshal([]byte(output.String()), &stats)
	if err != nil {
		return stats, fmt.Errorf("failed to parse stats JSON: %w", err)
	}

	return stats, nil
}
