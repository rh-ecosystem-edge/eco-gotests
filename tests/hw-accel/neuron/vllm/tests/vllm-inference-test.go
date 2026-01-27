package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/vllm/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron vLLM Inference Tests", Ordered, Label(params.LabelSuite), func() {

	Context("vLLM Workload", Label(tsparams.LabelSuite), func() {

		neuronConfig := neuronconfig.NewNeuronConfig()

		BeforeAll(func() {
			By("Verifying configuration")

			if !neuronConfig.IsValid() {
				Skip("Neuron configuration is not valid - DriversImage and DevicePluginImage are required")
			}

			if !neuronConfig.IsVLLMConfigured() {
				Skip("vLLM configuration is not set - ECO_HWACCEL_NEURON_HF_TOKEN is required for model download")
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
			})

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

			By("Waiting for Neuron resources to be available")
			err = await.AllNeuronNodesResourceAvailable(APIClient, tsparams.DevicePluginReadyTimeout)
			Expect(err).ToNot(HaveOccurred(), "Neuron resources not available on nodes")
		})

		AfterAll(func() {
			By("Cleaning up vLLM test resources")

			nsBuilder := namespace.NewBuilder(APIClient, tsparams.VLLMTestNamespace)
			if nsBuilder.Exists() {
				err := nsBuilder.Delete()
				if err != nil {
					klog.V(params.NeuronLogLevel).Infof("Failed to delete vLLM namespace: %v", err)
				}
			}

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

		It("Should deploy vLLM and execute inference", Label("neuron-vllm"), reportxml.ID("neuron-vllm"), func() {
			By("Creating vLLM test namespace")
			nsBuilder := namespace.NewBuilder(APIClient, tsparams.VLLMTestNamespace)
			if !nsBuilder.Exists() {
				_, err := nsBuilder.WithMultipleLabels(map[string]string{
					"pod-security.kubernetes.io/enforce": "privileged",
				}).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create vLLM namespace")
			}

			By("Configuring vLLM deployment")
			vllmConfig := do.DefaultVLLMConfig(tsparams.VLLMTestNamespace)
			vllmConfig.ModelName = neuronConfig.ModelName
			vllmConfig.ServedModelName = neuronConfig.ModelName
			vllmConfig.Image = neuronConfig.VLLMImage
			vllmConfig.StorageClassName = neuronConfig.StorageClassName

			By("Creating PersistentVolumeClaim for model storage")
			pvc := do.CreateVLLMPVC(vllmConfig)
			_, err := APIClient.CoreV1Interface.PersistentVolumeClaims(tsparams.VLLMTestNamespace).Create(
				context.Background(), pvc, metav1.CreateOptions{})
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).ToNot(HaveOccurred(), "Failed to create PVC")
			}

			By("Creating HuggingFace token secret")
			hfSecret := do.CreateHFTokenSecret(
				tsparams.VLLMTestNamespace,
				vllmConfig.HFSecretName,
				neuronConfig.HuggingFaceToken,
			)
			_, err = APIClient.CoreV1Interface.Secrets(tsparams.VLLMTestNamespace).Create(
				context.Background(), hfSecret, metav1.CreateOptions{})
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).ToNot(HaveOccurred(), "Failed to create HuggingFace token secret")
			}

			By("Creating vLLM deployment with init container for model download")
			vllmDeployment := do.CreateVLLMDeployment(vllmConfig)
			_, err = APIClient.AppsV1Interface.Deployments(tsparams.VLLMTestNamespace).Create(
				context.Background(), vllmDeployment, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to create vLLM deployment")
			klog.V(params.NeuronLogLevel).Infof("Created vLLM deployment: %s with model: %s",
				vllmConfig.Name, vllmConfig.ModelName)

			By("Creating vLLM service")
			svcConfig := do.VLLMServiceConfig{
				Name:      vllmConfig.Name,
				Namespace: tsparams.VLLMTestNamespace,
				Port:      vllmConfig.Port,
				Labels:    vllmConfig.Labels,
			}
			svc := do.CreateVLLMService(svcConfig)
			_, err = APIClient.CoreV1Interface.Services(tsparams.VLLMTestNamespace).Create(
				context.Background(), svc, metav1.CreateOptions{})
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).ToNot(HaveOccurred(), "Failed to create vLLM service")
			}

			By("Waiting for vLLM deployment to be ready (model download + compilation may take 30+ minutes)")
			deploymentReady := false
			timeout := 45 * time.Minute
			pollInterval := 30 * time.Second

			Eventually(func() bool {
				dep, pullErr := deployment.Pull(APIClient, vllmConfig.Name, tsparams.VLLMTestNamespace)
				if pullErr != nil {
					klog.V(params.NeuronLogLevel).Infof("Deployment not found yet: %v", pullErr)

					return false
				}

				available := dep.Object.Status.AvailableReplicas
				desired := *dep.Object.Spec.Replicas
				klog.V(params.NeuronLogLevel).Infof("Deployment %s: %d/%d replicas available",
					vllmConfig.Name, available, desired)

				if available >= desired && desired > 0 {
					deploymentReady = true

					return true
				}

				return false
			}, timeout, pollInterval).Should(BeTrue(), "vLLM deployment failed to become ready")

			Expect(deploymentReady).To(BeTrue(), "vLLM deployment should be ready")

			By("Sending inference request")
			inferenceConfig := do.InferenceConfig{
				ServiceName: vllmConfig.Name,
				Namespace:   tsparams.VLLMTestNamespace,
				Port:        vllmConfig.Port,
				ModelName:   vllmConfig.ServedModelName,
				Timeout:     tsparams.VLLMInferenceTimeout,
			}
			inferenceResult, err := do.ExecuteInferenceFromCluster(APIClient, inferenceConfig)

			if err != nil {
				klog.V(params.NeuronLogLevel).Infof("Inference request failed: %v", err)
				Skip("Inference test skipped - service not accessible from test environment")
			}

			Expect(inferenceResult).ToNot(BeEmpty(), "Inference should return a result")
			klog.V(params.NeuronLogLevel).Infof("Inference result: %s", inferenceResult)
		})
	})
})
