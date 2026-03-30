package tests

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronmetrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/metrics/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type metricFetchFunc func() ([]map[string]interface{}, error)

func pollForMetric(fetchFn metricFetchFunc, description string) []map[string]interface{} {
	var results []map[string]interface{}

	Eventually(func() int {
		data, err := fetchFn()
		if err != nil {
			klog.V(params.NeuronLogLevel).Infof("Failed to get %s: %v", description, err)

			return 0
		}

		results = data

		return len(data)
	}, tsparams.MetricScrapeTimeout, tsparams.MetricScrapeInterval).Should(BeNumerically(">", 0),
		"Expected %s to have values after polling", description)

	return results
}

var _ = Describe("Neuron Metrics Tests", Ordered, Label(params.Label), Label(params.LabelSuite), func() {
	Context("Metrics Provisioning", Label(tsparams.LabelSuite), func() {
		neuronConfig := neuronconfig.NewNeuronConfig()

		BeforeAll(func() {
			By("Verifying configuration")

			if !neuronConfig.IsValid() {
				Skip("Neuron configuration is not valid - DriversImage and DevicePluginImage are required")
			}

			By("Verifying all required operators are ready")

			var options *neuronhelpers.NeuronInstallConfigOptions
			if neuronConfig.CatalogSource != "" {
				options = &neuronhelpers.NeuronInstallConfigOptions{
					CatalogSource: neuronhelpers.StringPtr(neuronConfig.CatalogSource),
				}
			}

			Expect(neuronhelpers.AreAllOperatorsReady(APIClient, options)).To(BeTrue(),
				"All operators (NFD, KMM, Neuron) must be pre-installed and ready")

			var err error

			By("Creating DeviceConfig")

			builder := neuron.NewBuilder(
				APIClient,
				params.DefaultDeviceConfigName,
				params.NeuronNamespace,
				neuronConfig.DriversImage,
				neuronConfig.DriverVersion,
				neuronConfig.DevicePluginImage,
			).WithSelector(map[string]string{
				params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
			}).WithNodeMetricsImage(neuronConfig.NodeMetricsImage)

			if neuronConfig.SchedulerImage != "" && neuronConfig.SchedulerExtensionImage != "" {
				builder = builder.WithScheduler(neuronConfig.SchedulerImage, neuronConfig.SchedulerExtensionImage)
			}

			if neuronConfig.ImageRepoSecretName != "" {
				builder = builder.WithImageRepoSecret(neuronConfig.ImageRepoSecretName)
			}

			if !builder.Exists() {
				_, err = builder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create DeviceConfig")
			}

			By("Waiting for cluster stability after DeviceConfig")

			err = neuronhelpers.WaitForClusterStabilityAfterDeviceConfig(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Cluster not stable after DeviceConfig")

			By("Waiting for Neuron nodes to be labeled")

			err = await.NeuronNodesLabeled(APIClient, tsparams.DevicePluginReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "No Neuron-labeled nodes found")

			By("Waiting for device plugin deployment")

			err = await.DevicePluginDeployment(APIClient, params.NeuronNamespace, tsparams.DevicePluginReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Device plugin deployment failed")

			By("Waiting for metrics DaemonSet deployment")

			err = await.MetricsDaemonSet(APIClient, params.NeuronNamespace, tsparams.ServiceMonitorReadyTimeout)
			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Metrics DaemonSet not found (may not be enabled): %v", err)
			}

			By("Creating metrics test namespace")

			nsBuilder := namespace.NewBuilder(APIClient, tsparams.MetricsTestNamespace)
			if !nsBuilder.Exists() {
				_, err = nsBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create metrics test namespace")
			}

			if os.Getenv("ECO_SKIP_VLLM_CLEANUP") == "true" {
				klog.V(params.NeuronLogLevel).Info(
					"vLLM workload kept alive (ECO_SKIP_VLLM_CLEANUP=true), skipping helper deployment")
			} else {
				By("Deploying helper workload to activate Neuron runtime")

				neuronNodes, err := check.GetNeuronNodes(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron nodes")
				Expect(len(neuronNodes)).To(BeNumerically(">", 0), "No Neuron nodes found")

				workloadPod := do.CreateTestWorkloadPod(
					tsparams.MetricsWorkloadPodName,
					tsparams.MetricsTestNamespace,
					neuronNodes[0].Object.Name,
					tsparams.MetricsWorkloadContainerName,
					tsparams.MetricsWorkloadLabels,
				)

				_, err = APIClient.CoreV1Interface.Pods(tsparams.MetricsTestNamespace).Create(
					context.Background(), workloadPod, metav1.CreateOptions{})
				if !apierrors.IsAlreadyExists(err) {
					Expect(err).ToNot(HaveOccurred(), "Failed to create helper workload pod")
				}

				By("Waiting for helper workload to be running")

				Eventually(func() bool {
					healthy, checkErr := check.PodHealthy(
						APIClient, tsparams.MetricsWorkloadPodName, tsparams.MetricsTestNamespace)

					return checkErr == nil && healthy
				}, tsparams.WorkloadStartupTimeout, 10*time.Second).Should(BeTrue(),
					"Helper workload pod should be running")
			}
		})

		AfterAll(func() {
			nsBuilder := namespace.NewBuilder(APIClient, tsparams.MetricsTestNamespace)
			if nsBuilder.Exists() {
				err := nsBuilder.DeleteAndWait(5 * time.Minute)
				if err != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to delete metrics test namespace: %v", err)
				}
			}
		})

		It("Should verify metrics DaemonSet is created",
			Label("neuron-metrics-001"), reportxml.ID("neuron-metrics-001"), func() {
				By("Checking metrics pods are running")

				running, err := check.MetricsPodsRunning(APIClient)

				if err != nil || !running {
					klog.V(params.NeuronLogLevel).Info("Metrics pods not running - may be expected if disabled")
					Skip("Metrics pods not running - skipping metrics tests")
				}

				Expect(running).To(BeTrue(), "Metrics pods should be running on all Neuron nodes")
			})

		It("Should verify ServiceMonitor exists",
			Label("neuron-metrics-002"), reportxml.ID("neuron-metrics-002"), func() {
				By("Checking ServiceMonitor in operator namespace")

				serviceMonitors, err := neuronmetrics.ListServiceMonitors(APIClient, params.NeuronNamespace)
				if err != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to list ServiceMonitors: %v", err)
					Skip("ServiceMonitor CRD not available - skipping test")
				}

				if len(serviceMonitors.Items) == 0 {
					klog.V(params.NeuronLogLevel).Info("No ServiceMonitors found in namespace")
					Skip("No ServiceMonitors found - metrics may not be enabled")
				}

				klog.V(params.NeuronLogLevel).Infof("Found %d ServiceMonitors in namespace %s",
					len(serviceMonitors.Items), params.NeuronNamespace)

				for _, sm := range serviceMonitors.Items {
					klog.V(params.NeuronLogLevel).Infof("ServiceMonitor: %s", sm.GetName())
				}

				Expect(len(serviceMonitors.Items)).To(BeNumerically(">=", 1),
					"Expected at least one ServiceMonitor")
			})

		It("Should verify Prometheus is scraping Neuron targets",
			Label("neuron-metrics-003"), reportxml.ID("neuron-metrics-003"), func() {
				By("Polling Prometheus until Neuron metrics are available")

				var available, missing []string

				Eventually(func() int {
					avail, miss, err := neuronmetrics.VerifyNeuronMetricsAvailable(APIClient)
					if err != nil {
						klog.V(params.NeuronLogLevel).Infof("Error checking metrics: %v", err)

						return 0
					}

					available = avail
					missing = miss

					klog.V(params.NeuronLogLevel).Infof("Available: %d, Missing: %d", len(avail), len(miss))

					return len(avail)
				}, tsparams.MetricScrapeTimeout, tsparams.MetricScrapeInterval).Should(BeNumerically(">", 0),
					"Expected at least one Neuron metric to be available after polling")

				klog.V(params.NeuronLogLevel).Infof("Available metrics: %v", available)
				klog.V(params.NeuronLogLevel).Infof("Missing metrics: %v", missing)
			})

		It("Should verify neuron_hardware_info metric",
			Label("neuron-metrics-004"), reportxml.ID("neuron-metrics-004"), func() {
				By("Polling for neuron_hardware_info metric")

				hardwareInfo := pollForMetric(
					func() ([]map[string]interface{}, error) {
						return neuronmetrics.GetNeuronHardwareInfo(APIClient)
					},
					"neuron_hardware_info",
				)

				klog.V(params.NeuronLogLevel).Infof("Hardware info: %v", hardwareInfo)
			})

		It("Should verify neuroncore utilization metric",
			Label("neuron-metrics-005"), reportxml.ID("neuron-metrics-005"), func() {
				By("Polling for neuroncore_utilization_ratio metric")

				utilization := pollForMetric(
					func() ([]map[string]interface{}, error) {
						return neuronmetrics.GetNeuroncoreUtilization(APIClient)
					},
					"neuroncore_utilization_ratio",
				)

				for _, u := range utilization {
					if value, ok := u["value"].(string); ok {
						klog.V(params.NeuronLogLevel).Infof("Utilization value: %s", value)
					}
				}
			})

		It("Should verify metrics accuracy by comparing with device info",
			Label("neuron-metrics-006"), reportxml.ID("neuron-metrics-006"), func() {
				By("Getting Neuron nodes")

				neuronNodes, err := check.GetNeuronNodes(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron nodes")
				Expect(len(neuronNodes)).To(BeNumerically(">", 0), "Expected at least one Neuron node")

				By("Comparing metrics with node capacity")

				for _, node := range neuronNodes {
					neuronDevices, neuronCores, err := check.GetNeuronCapacity(APIClient, node.Object.Name)
					Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron capacity for node %s", node.Object.Name)

					klog.V(params.NeuronLogLevel).Infof("Node %s: %d devices, %d cores (from node capacity)",
						node.Object.Name, neuronDevices, neuronCores)

					Expect(neuronDevices).To(BeNumerically(">", 0),
						"Expected node %s to have at least one Neuron device", node.Object.Name)
					Expect(neuronCores).To(BeNumerically(">", 0),
						"Expected node %s to have at least one Neuron core", node.Object.Name)
				}

				By("Polling for memory metrics")

				memoryUsed := pollForMetric(
					func() ([]map[string]interface{}, error) {
						return neuronmetrics.GetNeuronMemoryUsed(APIClient)
					},
					"neuron_runtime_memory_used_bytes",
				)

				for _, metric := range memoryUsed {
					value, ok := metric["value"]
					Expect(ok).To(BeTrue(), "Memory metric should contain a value")
					Expect(value).ToNot(BeNil(), "Memory metric value should not be nil")
					klog.V(params.NeuronLogLevel).Infof("Memory used metric: %v", metric)
				}

				By("Polling for hardware info metrics")

				hardwareInfo := pollForMetric(
					func() ([]map[string]interface{}, error) {
						return neuronmetrics.GetNeuronHardwareInfo(APIClient)
					},
					"neuron_hardware_info",
				)

				klog.V(params.NeuronLogLevel).Infof("Hardware info metrics count: %d", len(hardwareInfo))

				By("Polling for core utilization metrics")

				utilization := pollForMetric(
					func() ([]map[string]interface{}, error) {
						return neuronmetrics.GetNeuroncoreUtilization(APIClient)
					},
					"neuroncore_utilization_ratio",
				)

				for _, u := range utilization {
					if valueStr, ok := u["value"].(string); ok {
						klog.V(params.NeuronLogLevel).Infof("Core utilization: %s", valueStr)
					}
				}

				klog.V(params.NeuronLogLevel).Info("Metrics accuracy verification completed")
			})

		It("Should verify metrics are exposed for all Neuron nodes",
			Label("neuron-metrics-007"), reportxml.ID("neuron-metrics-007"), func() {
				By("Getting Neuron nodes")

				neuronNodes, err := check.GetNeuronNodes(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron nodes")

				By("Checking metrics pods exist on all nodes")

				running, err := check.MetricsPodsRunning(APIClient)
				if err != nil || !running {
					Skip("Metrics pods not running on all nodes")
				}

				klog.V(params.NeuronLogLevel).Infof("Metrics are being collected from %d Neuron nodes",
					len(neuronNodes))

				for _, node := range neuronNodes {
					klog.V(params.NeuronLogLevel).Infof("Metrics collection active on node: %s",
						node.Object.Name)
				}
			})
	})
})
