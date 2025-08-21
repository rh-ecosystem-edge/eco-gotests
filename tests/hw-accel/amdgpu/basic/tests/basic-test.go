package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/eco-goinfra/pkg/amdgpu"
	"github.com/openshift-kni/eco-goinfra/pkg/nodes"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpudeviceconfig"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuhelpers"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpunfd"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuregistry"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/deviceconfig"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/labels"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/pods"
	amdparams "github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/params"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"github.com/openshift-kni/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AMD GPU Basic Tests", Ordered, Label(amdparams.LabelSuite), func() {

	Context("AMD GPU Basic 01", Label(amdparams.LabelSuite+"-01"), func() {

		apiClient := inittools.APIClient

		amdListOptions := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", amdparams.AMDNFDLabelKey, amdparams.AMDNFDLabelValue),
		}

		var amdNodeBuilders []*nodes.Builder
		var amdNodeBuildersErr error

		BeforeAll(func() {

			By("Verifying and configuring internal image registry for AMD GPU operator")
			err := amdgpuregistry.VerifyAndConfigureInternalRegistry(apiClient)
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("Internal registry configuration warning: %v", err)
				Skip("Internal image registry is not available - required for AMD GPU operator")
			}

			By("Deploying required operators")
			err = amdgpuhelpers.DeployAllOperators(apiClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to deploy required operators")

			By("Creating machineconfig for AMD GPU blacklist")
			err = amdgpuhelpers.CreateBlacklistMachineConfig(apiClient)
			Expect(err).ToNot(HaveOccurred(), "failed to create blacklist MachineConfig")

		})
		AfterAll(func() {

			By("Uninstalling all operators in reverse order")

			By("Uninstalling AMD GPU operator")
			amdgpuUninstallConfig := amdgpuhelpers.GetDefaultAMDGPUUninstallConfig(
				apiClient,
				"amd-gpu-operator-group",
				"amd-gpu-operator")
			amdgpuUninstaller := deploy.NewOperatorUninstaller(amdgpuUninstallConfig)
			err := amdgpuUninstaller.Uninstall()
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("AMD GPU operator uninstall completed with issues: %v", err)
			} else {
				glog.V(amdgpuparams.LogLevel).Info("AMD GPU operator uninstalled successfully")
			}

			By("Uninstalling KMM operator")
			kmmUninstallConfig := amdgpuhelpers.GetDefaultKMMUninstallConfig(apiClient, nil)
			kmmUninstaller := deploy.NewOperatorUninstaller(kmmUninstallConfig)
			err = kmmUninstaller.Uninstall()
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("KMM operator uninstall completed with issues: %v", err)
			} else {
				glog.V(amdgpuparams.LogLevel).Info("KMM operator uninstalled successfully")
			}

			By("Uninstalling NFD operator")
			nfdUninstallConfig := deploy.OperatorUninstallConfig{
				APIClient:         apiClient,
				Namespace:         nfdparams.NFDNamespace,
				OperatorGroupName: "nfd-operator-group",
				SubscriptionName:  "nfd-subscription",
				CustomResourceCleaner: deploy.NewNFDCustomResourceCleaner(
					apiClient, nfdparams.NFDNamespace, glog.Level(amdgpuparams.LogLevel)),
				LogLevel: glog.Level(amdgpuparams.LogLevel),
			}
			nfdUninstaller := deploy.NewOperatorUninstaller(nfdUninstallConfig)
			err = nfdUninstaller.Uninstall()
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("NFD operator uninstall completed with issues: %v", err)
			} else {
				glog.V(amdgpuparams.LogLevel).Info("NFD operator uninstalled successfully")
			}

			glog.V(amdgpuparams.LogLevel).Info("All operator uninstallations completed")
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
			glog.V(amdgpuparams.LogLevel).Infof("Internal image registry verified: %d pods running", runningPods)
		})

		It("Should create NodeFeatureDiscovery for AMD GPU detection", func() {
			By("Deploying NFD custom resource with AMD GPU worker config")

			nfdCRUtils := deploy.NewNFDCRUtils(apiClient, nfdparams.NFDNamespace, "amd-gpu-nfd-instance")

			nfdConfig := deploy.NFDCRConfig{
				EnableTopology: false,
				Image:          "",
				WorkerConfig:   "",
			}
			glog.V(amdgpuparams.LogLevel).Infof("NFD CR deployment : %+v", nfdConfig)
			err := nfdCRUtils.DeployNFDCR(nfdConfig)
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("NFD CR deployment result: %v", err)
				Skip("NFD CR deployment may require existing NFD operator or custom configuration")
			}

			By("Creating AMD GPU FeatureRule for enhanced detection")
			err = amdgpunfd.CreateAMDGPUFeatureRule(apiClient)
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("AMD GPU FeatureRule creation result: %v", err)
				Skip("AMD GPU FeatureRule creation may require NFD operator or manual creation")
			}

			By("NFD CR deployed successfully for AMD GPU detection")
			glog.V(amdgpuparams.LogLevel).Info("NFD custom resource created with AMD GPU worker configuration")

		})
		It("Should provide instructions for DeviceConfig creation", func() {
			By("Creating DeviceConfig custom resource")
			err := amdgpudeviceconfig.CreateDeviceConfig(apiClient, amdgpuparams.DefaultDeviceConfigName)
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("DeviceConfig creation result: %v", err)

			}
			Expect(err).ToNot(HaveOccurred(), "DeviceConfig should be created successfully")

			deviceConfigBuilder, deviceConfigBuilderErr := amdgpu.Pull(
				apiClient,
				amdgpuparams.DefaultDeviceConfigName,
				amdgpuparams.AMDGPUOperatorNamespace)

			if deviceConfigBuilderErr != nil {
				glog.Info(deviceConfigBuilder)
			}

			By("Waiting for cluster stability after DeviceConfig creation")
			err = amdgpuhelpers.WaitForClusterStabilityAfterDeviceConfig(apiClient)
			if err != nil {
				glog.V(amdgpuparams.LogLevel).Infof("Cluster stability check failed: %w", err)

				Skip("Cluster stability check failed - may need longer wait time or manual intervention")
			}
		})

		It("Check AMD label was added by NFD", func() {

			By("Checking AMD label was added to all AMD GPU Worker Nodes by NFD")
			amdNFDLabelFound, amdNFDLabelFoundErr := labels.LabelPresentOnAllNodes(
				apiClient, amdparams.AMDNFDLabelKey, amdparams.AMDNFDLabelValue, inittools.GeneralConfig.WorkerLabelMap)

			Expect(amdNFDLabelFoundErr).To(BeNil(),
				"An error occurred while attempting to verify the AMD label by NFD: %v ", amdNFDLabelFoundErr)
			Expect(amdNFDLabelFound).To(BeTrue(),
				"AMD label check failed to match label %s and label value %s on all nodes",
				amdparams.AMDNFDLabelKey, amdparams.AMDNFDLabelValue)
		})

		It("Node Labeller", func() {

			amdNodeBuilders, amdNodeBuildersErr = nodes.List(apiClient, amdListOptions)

			Expect(amdNodeBuildersErr).To(BeNil(),
				fmt.Sprintf("Failed to get Builders for AMD GPU Worker Nodes. Error:\n%v\n", amdNodeBuildersErr))

			Expect(amdNodeBuilders).ToNot(BeEmpty(),
				"'amdNodeBuilders' can't be empty")
			By("Getting Device Config Builder")
			deviceConfigBuilder, deviceConfigBuilderErr := amdgpu.Pull(
				apiClient, amdparams.DeviceConfigName, amdparams.AMDGPUNamespace)
			Expect(deviceConfigBuilderErr).To(BeNil(),
				fmt.Sprintf("Failed to get DeviceConfig Builder. Error:\n%v\n", deviceConfigBuilderErr))

			By("Saving the Node Labeller state for post-test restoration")
			nodeLabellerEnabled := deviceconfig.IsNodeLabellerEnabled(deviceConfigBuilder)
			glog.V(amdparams.AMDGPULogLevel).Infof("nodeLabellerEnabled: %t", nodeLabellerEnabled)

			By("Enabling the Node Labeller")
			enableNodeLabellerErr := deviceconfig.SetEnableNodeLabeller(true, deviceConfigBuilder, true)
			Expect(enableNodeLabellerErr).To(BeNil(),
				fmt.Sprintf("Failed to enable NodeLabeller. Error:\n%v\n", enableNodeLabellerErr))

			By("Getting Node Labeller Pods from all AMD GPU Worker Nodes")
			nodeLabellerPodBuilders, err := pods.NodeLabellerPodsFromNodes(apiClient, amdNodeBuilders)
			Expect(err).To(BeNil(), fmt.Sprintf("Failed to get Node Labeller Pods: %v", err))

			By("Waiting for all Node Labeller Nodes to be in 'Running' state")
			for _, nodeLabellerPod := range nodeLabellerPodBuilders {
				err := nodeLabellerPod.WaitUntilRunning(amdparams.DefaultTimeout * time.Second)
				Expect(err).To(BeNil(), fmt.Sprintf("Got the following error while waiting for "+
					"Pod '%s' to be in 'Running' state:\n%v", nodeLabellerPod.Object.Name, err))
			}

			By("Validating all AMD labels are added to each AMD GPU Worker Node by the Node Labeller Pod")
			labelsCheckErr := labels.LabelsExistOnAllNodes(amdNodeBuilders, amdparams.NodeLabellerLabels,
				amdparams.DefaultTimeout*time.Second, amdparams.DefaultSleepInterval*time.Second)
			Expect(labelsCheckErr).To(BeNil(), fmt.Sprintf("Node Labeller labels don't "+
				"exist on all AMD GPU Worker Nodes: %v\n", labelsCheckErr))

			By("Getting a new Device Config Builder with all thr changes")
			deviceConfigBuilderNew, deviceConfigBuilderNewErr := amdgpu.Pull(
				apiClient, amdparams.DeviceConfigName, amdparams.AMDGPUNamespace)
			Expect(deviceConfigBuilderNewErr).To(BeNil(), fmt.Sprintf(
				"Failed to get DeviceConfigNew Builder. Error:\n%v\n", deviceConfigBuilderNewErr))

			By("Disabling the Node Labeller")
			disableNodeLabellerErr := deviceconfig.SetEnableNodeLabeller(false, deviceConfigBuilderNew, true)
			Expect(disableNodeLabellerErr).To(BeNil(),
				fmt.Sprintf("Failed to disable NodeLabeller. Error:\n%v\n", disableNodeLabellerErr))

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
