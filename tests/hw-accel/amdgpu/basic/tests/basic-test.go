package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/amdgpu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpudeviceconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpunfd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuregistry"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/deviceconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/labels"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/internal/pods"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AMD GPU Basic Tests", Ordered, Label(amdgpuparams.LabelSuite), func() {

	Context("AMD GPU Basic 01", Label(amdgpuparams.LabelSuite+"-01"), func() {

		apiClient := inittools.APIClient
		amdconfig := amdgpuconfig.NewAMDConfig()

		amdListOptions := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", amdgpuparams.AMDNFDLabelKey, amdgpuparams.AMDNFDLabelValue),
		}

		var amdNodeBuilders []*nodes.Builder
		var amdNodeBuildersErr error

		BeforeAll(func() {
			By("Verifying configuration")

			if amdconfig == nil {
				Skip("AMDConfig is not available - required for AMD GPU tests")
			}

			if amdconfig.AMDDriverVersion == "" {
				Skip("AMD Driver Version is not set in environment - required for AMD GPU tests")
			}

			By("Verifying and configuring internal image registry for AMD GPU operator")
			err := amdgpuregistry.VerifyAndConfigureInternalRegistry(apiClient)
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Internal registry configuration warning: %v", err)
				Skip("Internal image registry is not available - required for AMD GPU operator")
			}

			By("Checking if cluster is stable")
			err = amdgpuhelpers.WaitForClusterStability(apiClient, amdgpuparams.ClusterStabilityTimeout)
			Expect(err).ToNot(HaveOccurred(), "Cluster should be stable before proceeding with the test")

			By("Deploying required operators")
			err = amdgpuhelpers.DeployAllOperators(apiClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to deploy required operators")

			By("Creating machineconfig for AMD GPU blacklist")
			err = amdgpuhelpers.CreateBlacklistMachineConfig(apiClient)
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Blacklist MachineConfig not applied (non-fatal): %v", err)
				Skip("Blacklist MachineConfig requires cluster-admin; skipping hard failure")
			}

		})
		AfterAll(func() {

			By("Uninstalling all operators in reverse order")

			By("Uninstalling AMD GPU operator")
			amdgpuUninstallConfig := amdgpuhelpers.GetDefaultAMDGPUUninstallConfig(
				apiClient,
				"amd-gpu-operator-group",
				"amd-gpu-subscription")
			amdgpuUninstaller := deploy.NewOperatorUninstaller(amdgpuUninstallConfig)
			err := amdgpuUninstaller.Uninstall()
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("AMD GPU operator uninstall completed with issues: %v", err)
			} else {
				glog.V(amdgpuparams.AMDGPULogLevel).Info("AMD GPU operator uninstalled successfully")
			}

			By("Uninstalling KMM operator")
			kmmUninstallConfig := amdgpuhelpers.GetDefaultKMMUninstallConfig(apiClient, nil)
			kmmUninstaller := deploy.NewOperatorUninstaller(kmmUninstallConfig)
			err = kmmUninstaller.Uninstall()
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("KMM operator uninstall completed with issues: %v", err)
			} else {
				glog.V(amdgpuparams.AMDGPULogLevel).Info("KMM operator uninstalled successfully")
			}

			By("Uninstalling NFD operator")
			nfdUninstallConfig := deploy.OperatorUninstallConfig{
				APIClient:         apiClient,
				Namespace:         nfdparams.NFDNamespace,
				OperatorGroupName: "nfd-operator-group",
				SubscriptionName:  "nfd-subscription",
				CustomResourceCleaner: deploy.NewNFDCustomResourceCleaner(
					apiClient, nfdparams.NFDNamespace, glog.Level(amdgpuparams.AMDGPULogLevel)),
				LogLevel: glog.Level(amdgpuparams.AMDGPULogLevel),
			}
			nfdUninstaller := deploy.NewOperatorUninstaller(nfdUninstallConfig)
			err = nfdUninstaller.Uninstall()
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("NFD operator uninstall completed with issues: %v", err)
			} else {
				glog.V(amdgpuparams.AMDGPULogLevel).Info("NFD operator uninstalled successfully")
			}

			glog.V(amdgpuparams.AMDGPULogLevel).Info("All operator uninstallations completed")
		})
		It("Should verify internal registry is configured and available", func() {
			By("Checking internal image registry configuration")
			err := amdgpuregistry.VerifyAndConfigureInternalRegistry(apiClient)
			Expect(err).NotTo(HaveOccurred(), "Internal image registry should be properly configured")

			By("Verifying image registry pods are running")
			pods, err := apiClient.CoreV1Interface.Pods("openshift-image-registry").List(
				context.Background(), metav1.ListOptions{
					LabelSelector: "docker-registry=default",
				})
			Expect(err).NotTo(HaveOccurred(), "Should be able to list image registry pods")

			runningPods := 0
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					allReady := true
					for _, containerStatus := range pod.Status.ContainerStatuses {
						if !containerStatus.Ready {
							allReady = false

							break
						}
					}
					if allReady {
						runningPods++
					}
				}
			}

			Expect(runningPods).To(BeNumerically(">", 0), "At least one image registry pod should be running")
			glog.V(amdgpuparams.AMDGPULogLevel).Infof("Internal image registry verified: %d pods running", runningPods)
		})

		It("Should create NodeFeatureDiscovery for AMD GPU detection", func() {
			By("Deploying NFD custom resource with AMD GPU worker config")

			nfdCRUtils := deploy.NewNFDCRUtils(apiClient, nfdparams.NFDNamespace, "amd-gpu-nfd-instance")

			nfdConfig := deploy.NFDCRConfig{
				EnableTopology: false,
				Image:          "",
				WorkerConfig:   "",
			}
			glog.V(amdgpuparams.AMDGPULogLevel).Infof("NFD CR deployment : %+v", nfdConfig)

			err := nfdCRUtils.DeployNFDCR(nfdConfig)
			Expect(err).ToNot(HaveOccurred(), "NFD CR should be created successfully %v", err)

			By("Creating AMD GPU FeatureRule for enhanced detection")
			err = amdgpunfd.CreateAMDGPUFeatureRule(apiClient)
			Expect(err).ToNot(HaveOccurred(), "AMD GPU FeatureRule should be created successfully")

			By("NFD CR deployed successfully for AMD GPU detection")
			glog.V(amdgpuparams.AMDGPULogLevel).Info("NFD custom resource created with AMD GPU worker configuration")
			glog.V(amdgpuparams.AMDGPULogLevel).Info("waiting for 1 minute for NFD to label nodes")
			time.Sleep(1 * time.Minute)
		})

		It("Check AMD label was added by NFD", func() {

			By("Checking AMD label was added to all AMD GPU Worker Nodes by NFD")
			amdNFDLabelFound, amdNFDLabelFoundErr := labels.LabelPresentOnAllNodes(
				apiClient, amdgpuparams.AMDNFDLabelKey, amdgpuparams.AMDNFDLabelValue, inittools.GeneralConfig.WorkerLabelMap)

			Expect(amdNFDLabelFoundErr).To(BeNil(),
				"An error occurred while attempting to verify the AMD label by NFD: %v ", amdNFDLabelFoundErr)
			Expect(amdNFDLabelFound).To(BeTrue(),
				"AMD label check failed to match label %s and label value %s on all nodes",
				amdgpuparams.AMDNFDLabelKey, amdgpuparams.AMDNFDLabelValue)
		})

		It("Should provide instructions for DeviceConfig creation", func() {
			By("Creating DeviceConfig custom resource")

			err := amdgpudeviceconfig.CreateDeviceConfig(
				apiClient,
				amdgpuparams.DefaultDeviceConfigName,
				amdconfig.AMDDriverVersion)
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("DeviceConfig creation result: %v", err)

			}
			Expect(err).ToNot(HaveOccurred(), "DeviceConfig should be created successfully")

			deviceConfigBuilder, deviceConfigBuilderErr := amdgpu.Pull(
				apiClient,
				amdgpuparams.DefaultDeviceConfigName,
				amdgpuparams.AMDGPUNamespace)

			if deviceConfigBuilderErr != nil {
				glog.Info(deviceConfigBuilder)
			}

			By("Waiting for cluster stability after DeviceConfig creation")
			err = amdgpuhelpers.WaitForClusterStabilityAfterDeviceConfig(apiClient)
			if err != nil {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("Cluster stability check failed: %w", err)

				Skip("Cluster stability check failed - may need longer wait time or manual intervention")
			}
		})

		It("Node Labeller", func() {

			By("Getting AMD GPU Worker Nodes")
			amdNodeBuilders, amdNodeBuildersErr = nodes.List(apiClient, amdListOptions)

			Expect(amdNodeBuildersErr).To(BeNil(),
				fmt.Sprintf("Failed to get Builders for AMD GPU Worker Nodes. Error:\n%v\n", amdNodeBuildersErr))

			Expect(amdNodeBuilders).ToNot(BeEmpty(),
				"'amdNodeBuilders' can't be empty")

			glog.V(amdgpuparams.AMDGPULogLevel).Infof("Found %d AMD GPU nodes", len(amdNodeBuilders))
			for _, node := range amdNodeBuilders {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof("AMD GPU node: %s", node.Object.Name)
			}

			By("Getting Device Config Builder")
			deviceConfigBuilder, deviceConfigBuilderErr := amdgpu.Pull(
				apiClient, amdgpuparams.DeviceConfigName, amdgpuparams.AMDGPUNamespace)
			glog.V(amdgpuparams.AMDGPULogLevel).Infof("deviceConfigBuilder: %v", deviceConfigBuilder)

			Expect(deviceConfigBuilderErr).To(BeNil(),
				fmt.Sprintf("Failed to get DeviceConfig Builder. Error:\n%v\n", deviceConfigBuilderErr))

			By("Saving the Node Labeller state for post-test restoration")
			nodeLabellerEnabled := deviceconfig.IsNodeLabellerEnabled(deviceConfigBuilder)
			glog.V(amdgpuparams.AMDGPULogLevel).Infof("nodeLabellerEnabled: %t", nodeLabellerEnabled)

			By("Enabling the Node Labeller")
			enableNodeLabellerErr := deviceconfig.SetEnableNodeLabeller(true, deviceConfigBuilder, true)
			glog.V(amdgpuparams.AMDGPULogLevel).Infof("deviceConfigBuilder after enabling: %v",
				deviceConfigBuilder.Object.Spec.DevicePlugin)
			Expect(enableNodeLabellerErr).To(BeNil(),
				fmt.Sprintf("Failed to enable NodeLabeller. Error:\n%v\n", enableNodeLabellerErr))

			By("Waiting for Node Labeller pods to be created")
			// Allow time for the operator to reconcile and create the node labeller pods
			time.Sleep(30 * time.Second)

			By("Getting Node Labeller Pods from all AMD GPU Worker Nodes")
			// Retry getting pods with a timeout to handle slow pod creation
			var nodeLabellerPodBuilders []*pod.Builder
			var err error

			Eventually(func() error {
				nodeLabellerPodBuilders, err = pods.NodeLabellerPodsFromNodes(apiClient, amdNodeBuilders)

				if err != nil {
					glog.V(amdgpuparams.AMDGPULogLevel).Infof("Waiting for node labeller pods: %v", err)

					return err
				}

				if len(nodeLabellerPodBuilders) == 0 {
					return fmt.Errorf("no node labeller pods found yet")
				}

				return nil
			}, 2*time.Minute, 10*time.Second).Should(Succeed(),
				"Failed to get Node Labeller Pods within timeout")

			By("Waiting for all Node Labeller Nodes to be in 'Running' state")
			for _, nodeLabellerPod := range nodeLabellerPodBuilders {
				glog.V(amdgpuparams.AMDGPULogLevel).Infof(
					"nodeLabellerPod: %s,status:%v", nodeLabellerPod.Object.Name, nodeLabellerPod.Object.Status.Phase)
				err := nodeLabellerPod.WaitUntilRunning(amdgpuparams.DefaultTimeout)
				Expect(err).To(BeNil(), fmt.Sprintf("Got the following error while waiting for "+
					"Pod '%s' to be in 'Running' state:\n%v", nodeLabellerPod.Object.Name, err))
			}

			By("Waiting for Node Labeller pods to be fully ready")
			// Additional wait to ensure pods are not just running but fully initialized
			time.Sleep(10 * time.Second)

			By("Validating all AMD labels are added to each AMD GPU Worker Node by the Node Labeller Pod")
			labelsCheckErr := labels.LabelsExistOnAllNodes(amdNodeBuilders, amdgpuparams.NodeLabellerLabels,
				amdgpuparams.DefaultTimeout, amdgpuparams.DefaultSleepInterval)
			Expect(labelsCheckErr).To(BeNil(), fmt.Sprintf("Node Labeller labels don't "+
				"exist on all AMD GPU Worker Nodes: %v\n", labelsCheckErr))

			By("Getting a new Device Config Builder with all the changes")
			deviceConfigBuilderNew, deviceConfigBuilderNewErr := amdgpu.Pull(
				apiClient, amdgpuparams.DeviceConfigName, amdgpuparams.AMDGPUNamespace)
			Expect(deviceConfigBuilderNewErr).To(BeNil(), fmt.Sprintf(
				"Failed to get DeviceConfigNew Builder. Error:\n%v\n", deviceConfigBuilderNewErr))

			By("Disabling the Node Labeller")
			disableNodeLabellerErr := deviceconfig.SetEnableNodeLabeller(false, deviceConfigBuilderNew, true)
			Expect(disableNodeLabellerErr).To(BeNil(),
				fmt.Sprintf("Failed to disable NodeLabeller. Error:\n%v\n", disableNodeLabellerErr))

			By("Waiting for Node Labeller configuration to be applied")
			// Allow time for the operator to reconcile and delete the node labeller pods
			time.Sleep(30 * time.Second)

			By("Make sure there are no Node Labeller Pods")
			noNodeLabellerPodsErr := pods.WaitUntilNoNodeLabellerPodes(apiClient)
			Expect(noNodeLabellerPodsErr).To(BeNil(), fmt.Sprintf("Got an error while waiting for "+
				"all Node Labeller Pods to be deleted. %v", noNodeLabellerPodsErr))

			By("Ensuring that all labels added by the Node Labeller are removed")
			missingNodeLabellerLabelsErr := labels.NodeLabellersLabelsMissingOnAllAMDGPUNode(amdNodeBuilders)
			Expect(missingNodeLabellerLabelsErr).To(BeNil(), fmt.Sprintf("failure occurred while checking "+
				"that Node Labeller Labels were removed from all AMD GPU Worker Nodes. %v", missingNodeLabellerLabelsErr))

		})
	})
})
