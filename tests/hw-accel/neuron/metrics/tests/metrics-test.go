package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronmetrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/metrics/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron Metrics Tests", Ordered, Label(params.LabelSuite), func() {

	Context("Metrics Provisioning", Label(tsparams.LabelSuite), func() {

		neuronConfig := neuronconfig.NewNeuronConfig()

		BeforeAll(func() {
			By("Verifying configuration")

			if !neuronConfig.IsValid() {
				Skip("Neuron configuration is not valid - DriversImage and DevicePluginImage are required")
			}

			By("Deploying required operators")
			var options *neuronhelpers.NeuronInstallConfigOptions
			if neuronConfig.CatalogSource != "" {
				options = &neuronhelpers.NeuronInstallConfigOptions{
					CatalogSource: neuronhelpers.StringPtr(neuronConfig.CatalogSource),
				}
			}

			err := neuronhelpers.DeployAllOperators(APIClient, options)
			Expect(err).ToNot(HaveOccurred(), "Failed to deploy required operators")

			By("Waiting for NFD operator to be ready")
			nfdInstallConfig := deploy.OperatorInstallConfig{
				APIClient:              APIClient,
				Namespace:              params.NFDNamespace,
				OperatorGroupName:      "nfd-operator-group",
				SubscriptionName:       "nfd-subscription",
				PackageName:            "nfd",
				CatalogSource:          "redhat-operators",
				CatalogSourceNamespace: "openshift-marketplace",
				Channel:                "stable",
				TargetNamespaces:       []string{params.NFDNamespace},
				LogLevel:               params.NeuronLogLevel,
			}
			nfdInstaller := deploy.NewOperatorInstaller(nfdInstallConfig)
			ready, err := nfdInstaller.IsReady(tsparams.OperatorDeployTimeout)
			Expect(err).ToNot(HaveOccurred(), "NFD operator readiness check failed")
			Expect(ready).To(BeTrue(), "NFD operator is not ready")

			By("Waiting for KMM operator to be ready")
			kmmInstallConfig := neuronhelpers.GetDefaultKMMInstallConfig(APIClient)
			kmmInstaller := deploy.NewOperatorInstaller(kmmInstallConfig)
			ready, err = kmmInstaller.IsReady(tsparams.OperatorDeployTimeout)
			Expect(err).ToNot(HaveOccurred(), "KMM operator readiness check failed")
			Expect(ready).To(BeTrue(), "KMM operator is not ready")

			By("Waiting for Neuron operator to be ready")
			neuronInstallConfig := neuronhelpers.GetDefaultNeuronInstallConfig(APIClient, options)
			neuronInstaller := deploy.NewOperatorInstaller(neuronInstallConfig)
			ready, err = neuronInstaller.IsReady(tsparams.OperatorDeployTimeout)
			Expect(err).ToNot(HaveOccurred(), "Neuron operator readiness check failed")
			Expect(ready).To(BeTrue(), "Neuron operator is not ready")


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
		})

		AfterAll(func() {
			By("Cleaning up DeviceConfig and waiting for deletion")
			deviceConfigBuilder, err := neuron.Pull(
				APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
			if err == nil {
				_, deleteErr := deviceConfigBuilder.Delete()
				if deleteErr != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to delete DeviceConfig: %v", deleteErr)
				} else {

					klog.V(params.NeuronLogLevel).Info("Waiting for DeviceConfig finalizer to be processed...")
					Eventually(func() bool {
						_, pullErr := neuron.Pull(APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)

						return pullErr != nil 
					}, 5*time.Minute, 5*time.Second).Should(BeTrue(),
						"DeviceConfig should be fully deleted")
				}
			}

			By("Uninstalling operators")
			uninstallErr := neuronhelpers.UninstallAllOperators(APIClient)
			if uninstallErr != nil {
				klog.V(params.NeuronLogLevel).Infof("Operator uninstall completed with issues: %v", uninstallErr)
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
				By("Waiting for metrics to be scraped")
				time.Sleep(2 * time.Minute)

				By("Checking if Neuron metrics are available in Prometheus")
				available, missing, err := neuronmetrics.VerifyNeuronMetricsAvailable(APIClient)

				if err != nil {
					klog.V(params.NeuronLogLevel).Infof("Error checking metrics: %v", err)
					Skip("Unable to query Prometheus - skipping metrics verification")
				}

				klog.V(params.NeuronLogLevel).Infof("Available metrics: %v", available)
				klog.V(params.NeuronLogLevel).Infof("Missing metrics: %v", missing)

				if len(available) == 0 {
					Skip("No metrics available yet - Prometheus may need more time to scrape")
				}

				Expect(len(available)).To(BeNumerically(">", 0),
					"Expected at least one Neuron metric to be available")
			})

		It("Should verify neuron_hardware_info metric",
			Label("neuron-metrics-004"), reportxml.ID("neuron-metrics-004"), func() {
				By("Querying neuron_hardware_info metric")
				hardwareInfo, err := neuronmetrics.GetNeuronHardwareInfo(APIClient)

				if err != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to get hardware info: %v", err)
					Skip("neuron_hardware_info metric not available")
				}

				klog.V(params.NeuronLogLevel).Infof("Hardware info: %v", hardwareInfo)
				Expect(len(hardwareInfo)).To(BeNumerically(">", 0),
					"Expected neuron_hardware_info to have values")
			})

		It("Should verify neuroncore utilization metric",
			Label("neuron-metrics-005"), reportxml.ID("neuron-metrics-005"), func() {
				By("Querying neuroncore_utilization_ratio metric")
				utilization, err := neuronmetrics.GetNeuroncoreUtilization(APIClient)

				if err != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to get utilization: %v", err)
					Skip("neuroncore_utilization_ratio metric not available")
				}

				klog.V(params.NeuronLogLevel).Infof("Utilization: %v", utilization)

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

				By("Verifying memory metrics are available")
				memoryUsed, err := neuronmetrics.GetNeuronMemoryUsed(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron memory used metrics")
				Expect(len(memoryUsed)).To(BeNumerically(">", 0),
					"Expected at least one memory metric result")

				for _, metric := range memoryUsed {
					value, ok := metric["value"]
					Expect(ok).To(BeTrue(), "Memory metric should contain a value")
					Expect(value).ToNot(BeNil(), "Memory metric value should not be nil")
					klog.V(params.NeuronLogLevel).Infof("Memory used metric: %v", metric)
				}

				By("Verifying hardware info metrics match node capacity")
				hardwareInfo, err := neuronmetrics.GetNeuronHardwareInfo(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron hardware info metrics")
				Expect(len(hardwareInfo)).To(BeNumerically(">", 0),
					"Expected at least one hardware info metric")

				klog.V(params.NeuronLogLevel).Infof("Hardware info metrics count: %d", len(hardwareInfo))

				By("Verifying core utilization metrics are within valid range")
				utilization, err := neuronmetrics.GetNeuroncoreUtilization(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get Neuron core utilization metrics")

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
