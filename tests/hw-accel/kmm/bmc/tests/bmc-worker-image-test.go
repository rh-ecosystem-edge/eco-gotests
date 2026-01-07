package tests

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/bmc/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

var _ = Describe("KMM-BMC", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity, tsparams.LabelSuite), func() {

	BeforeAll(func() {
		By("Check KMM version is 2.5 or higher")
		currentVersion, err := get.KmmOperatorVersion(APIClient)
		Expect(err).ToNot(HaveOccurred(), "failed to get KMM operator version")

		minVersion, err := version.NewVersion("2.5.0")
		Expect(err).ToNot(HaveOccurred(), "failed to parse minimum version")

		if currentVersion.LessThan(minVersion) {
			Skip("BootModuleConfig tests require KMM version 2.5.0 or higher")
		}

		klog.V(kmmparams.KmmLogLevel).Infof("KMM version %s >= 2.5.0, proceeding with BMC tests", currentVersion)
	})

	Context("BootModuleConfig", Label("bmc-worker-image"), func() {

		var (
			bmcBuilder *kmm.BootModuleConfigBuilder
		)

		AfterEach(func() {
			By("Delete BootModuleConfig if exists")
			if bmcBuilder != nil && bmcBuilder.Exists() {
				_, err := bmcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

				By("Wait for BootModuleConfig to be deleted")
				err = await.BootModuleConfigObjectDeleted(APIClient, tsparams.BMCTestName, tsparams.BMCTestNamespace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for bootmoduleconfig to be deleted")
			}

			By("Delete MachineConfig if exists")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)
			if err == nil && mcBuilder != nil {
				err = mcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")
			}
		})

		It("should create BMC with empty workerImage and use operator worker image", reportxml.ID("85553"), func() {
			By("Verify simple-kmod image exists for kernel version")
			_, err := check.ImageExists(
				APIClient,
				kmmparams.SimpleKmodImage,
				GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip(fmt.Sprintf("Simple-kmod image not available: %v", err))
			}

			By("Create BootModuleConfig with empty workerImage")
			bmcBuilder = kmm.NewBootModuleConfigBuilder(APIClient, tsparams.BMCTestName, tsparams.BMCTestNamespace).
				WithKernelModuleImage(kmmparams.SimpleKmodImage).
				WithKernelModuleName(kmmparams.SimpleKmodModuleName).
				WithMachineConfigName(tsparams.MachineConfigName).
				WithMachineConfigPoolName(kmmparams.DefaultWorkerMCPName)

			_, err = bmcBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating bootmoduleconfig")

			By("Wait for MachineConfig to be created")
			err = await.MachineConfigCreated(APIClient, tsparams.MachineConfigName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig was not created in time")

			By("Verify MachineConfig contains WORKER_IMAGE from operator")
			workerImageValue, err := get.MachineConfigEnvVar(APIClient, tsparams.MachineConfigName, "WORKER_IMAGE")
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should contain WORKER_IMAGE")
			Expect(workerImageValue).ToNot(BeEmpty(), "WORKER_IMAGE value should not be empty")
			Expect(workerImageValue).To(MatchRegexp(`^[a-zA-Z0-9].*`),
				"WORKER_IMAGE should be a valid image reference")

			By("Get a worker node for reboot")
			nodeList, err := nodes.List(APIClient, metav1.ListOptions{
				LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing worker nodes")
			Expect(len(nodeList)).To(BeNumerically(">", 0), "no worker nodes found")

			workerNode := nodeList[0]
			klog.V(kmmparams.KmmLogLevel).Infof("Using worker node: %s", workerNode.Object.Name)

			By("Wait for MCO to write new config to disk")
			err = await.NodeDesiredConfigChange(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MCO did not write new config for node in time")

			By("Reboot the worker node to apply BMC config")
			err = do.Reboot(APIClient, workerNode.Object.Name, kmmparams.KmmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to reboot node")

			By("Wait for node to come back up and be Ready")
			err = await.NodeConfigApplied(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "node did not become Ready after reboot")

			By("Wait for helper pod on rebooted node to be ready")
			_, err = await.ReadyHelperPod(APIClient, kmmparams.KmmOperatorNamespace,
				workerNode.Object.Name, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "helper pod not ready on node after reboot")

			By("Verify simple-kmod module is loaded on the node (BMC loads it during boot)")
			err = check.ModuleLoadedOnNode(APIClient, kmmparams.SimpleKmodModuleName, time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "module should be loaded by BMC during boot")

			By("Delete the BootModuleConfig")
			_, err = bmcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

			By("Verify MachineConfig is still present after BMC deletion")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should still exist after BMC deletion")
			Expect(mcBuilder.Exists()).To(BeTrue(), "MachineConfig should still exist")

			By("Delete the MachineConfig")
			err = mcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

			By("Verify MachineConfig is deleted")
			err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

			By("Wait for MachineConfigPool to start updating (nodes will reboot to remove module)")
			mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
			Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

			By("Wait for MachineConfigPool update to complete (all nodes rebooted)")
			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
		})
	})

	Context("BMC with inTreeModulesToRemove", Label("bmc-intree-remove"), func() {

		var (
			bmcBuilder *kmm.BootModuleConfigBuilder
			workerNode *nodes.Builder
		)

		AfterEach(func() {
			By("Delete BootModuleConfig if exists")
			if bmcBuilder != nil && bmcBuilder.Exists() {
				_, err := bmcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

				By("Wait for BootModuleConfig to be deleted")
				err = await.BootModuleConfigObjectDeleted(APIClient,
					tsparams.BMCInTreeRemoveName, tsparams.BMCTestNamespace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for bootmoduleconfig to be deleted")
			}

			By("Delete MachineConfig if exists")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigInTreeRemoveName)
			if err == nil && mcBuilder != nil {
				err = mcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

				By("Verify MachineConfig is deleted")
				err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigInTreeRemoveName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

				By("Wait for MachineConfigPool to start updating")
				mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
				Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

				err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
				Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

				By("Wait for MachineConfigPool update to complete")
				err = mcp.WaitForUpdate(30 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
			}
		})

		It("should remove in-tree module and load OOT module", reportxml.ID("85558"), func() {
			By("Get cluster architecture and determine in-tree module to remove")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error getting cluster architecture")

			inTreeModule := get.InTreeModuleToRemove(arch)
			klog.V(kmmparams.KmmLogLevel).Infof("Architecture: %s, in-tree module to remove: %s", arch, inTreeModule)

			By("Get a worker node for testing")
			nodeList, err := nodes.List(APIClient, metav1.ListOptions{
				LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing worker nodes")
			Expect(len(nodeList)).To(BeNumerically(">", 0), "no worker nodes found")

			workerNode = nodeList[0]
			klog.V(kmmparams.KmmLogLevel).Infof("Using worker node: %s", workerNode.Object.Name)

			By("Check if in-tree module exists on node")
			moduleExists, _ := check.ModuleExistsOnNode(APIClient, inTreeModule, workerNode.Object.Name)

			if !moduleExists {
				Skip(fmt.Sprintf("Module %s does not exist on node %s, skipping test",
					inTreeModule, workerNode.Object.Name))
			}

			klog.V(kmmparams.KmmLogLevel).Infof("Module %s exists on node %s", inTreeModule, workerNode.Object.Name)

			By("Verify simple-kmod image exists for kernel version")
			_, err = check.ImageExists(
				APIClient,
				kmmparams.SimpleKmodImage,
				GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip(fmt.Sprintf("Simple-kmod image not available: %v", err))
			}

			By("Create BMC with inTreeModulesToRemove")
			bmcBuilder = kmm.NewBootModuleConfigBuilder(APIClient,
				tsparams.BMCInTreeRemoveName, tsparams.BMCTestNamespace).
				WithKernelModuleImage(kmmparams.SimpleKmodImage).
				WithKernelModuleName(kmmparams.SimpleKmodModuleName).
				WithMachineConfigName(tsparams.MachineConfigInTreeRemoveName).
				WithMachineConfigPoolName(kmmparams.DefaultWorkerMCPName).
				WithInTreeModulesToRemove([]string{inTreeModule})

			_, err = bmcBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating bootmoduleconfig")

			By("Wait for MachineConfig to be created")
			err = await.MachineConfigCreated(APIClient, tsparams.MachineConfigInTreeRemoveName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig was not created in time")

			By("Verify MachineConfig contains IN_TREE_MODULES_TO_REMOVE")
			inTreeValue, err := get.MachineConfigEnvVar(APIClient,
				tsparams.MachineConfigInTreeRemoveName, "IN_TREE_MODULES_TO_REMOVE")
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should contain IN_TREE_MODULES_TO_REMOVE")
			Expect(inTreeValue).To(ContainSubstring(inTreeModule),
				"IN_TREE_MODULES_TO_REMOVE should contain the in-tree module")

			By("Wait for MCO to write new config to disk")
			err = await.NodeDesiredConfigChange(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MCO did not write new config for node in time")

			By("Reboot the worker node to apply BMC config")
			err = do.Reboot(APIClient, workerNode.Object.Name, kmmparams.KmmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to reboot node")

			By("Wait for node to come back up and be Ready")
			err = await.NodeConfigApplied(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "node did not become Ready after reboot")

			By("Wait for helper pod on rebooted node to be ready")
			_, err = await.ReadyHelperPod(APIClient, kmmparams.KmmOperatorNamespace,
				workerNode.Object.Name, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "helper pod not ready on node after reboot")

			By("Verify previously loaded in-tree module is NOT loaded after reboot")
			err = check.ModuleNotLoadedOnNode(APIClient, inTreeModule, time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "in-tree module should be removed by BMC")

			By("Verify simple-kmod module IS loaded after reboot")
			err = check.ModuleLoadedOnNode(APIClient, kmmparams.SimpleKmodModuleName, time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "simple-kmod module should be loaded by BMC")

			By("Delete the BootModuleConfig")
			_, err = bmcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

			By("Verify MachineConfig is still present after BMC deletion")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigInTreeRemoveName)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should still exist after BMC deletion")
			Expect(mcBuilder.Exists()).To(BeTrue(), "MachineConfig should still exist")

			By("Delete the MachineConfig")
			err = mcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

			By("Verify MachineConfig is deleted")
			err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigInTreeRemoveName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

			By("Wait for MachineConfigPool to start updating (nodes will reboot)")
			mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
			Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

			By("Wait for MachineConfigPool update to complete (all nodes rebooted)")
			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
		})
	})

	Context("BMC with firmwareFilesPath", Label("bmc-firmware"), func() {

		var (
			bmcBuilder      *kmm.BootModuleConfigBuilder
			workerNode      *nodes.Builder
			firmwareImage   string
			buildNsCreated  bool
			moduleCreated   bool
			svcAccountBuild *serviceaccount.Builder
		)

		AfterEach(func() {
			By("Delete BootModuleConfig if exists")
			if bmcBuilder != nil && bmcBuilder.Exists() {
				_, err := bmcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

				By("Wait for BootModuleConfig to be deleted")
				err = await.BootModuleConfigObjectDeleted(APIClient,
					tsparams.BMCFirmwareName, tsparams.BMCTestNamespace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for bootmoduleconfig to be deleted")
			}

			By("Delete MachineConfig if exists")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigFirmwareName)
			if err == nil && mcBuilder != nil {
				err = mcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

				By("Verify MachineConfig is deleted")
				err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigFirmwareName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

				By("Wait for MachineConfigPool to start updating")
				mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
				Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

				err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
				Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

				By("Wait for MachineConfigPool update to complete")
				err = mcp.WaitForUpdate(30 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
			}

			if moduleCreated {
				By("Delete firmware Module if exists")
				_, _ = kmm.NewModuleBuilder(APIClient,
					tsparams.FirmwareModuleName, tsparams.FirmwareBuildNamespace).Delete()
				_ = await.ModuleObjectDeleted(APIClient,
					tsparams.FirmwareModuleName, tsparams.FirmwareBuildNamespace, time.Minute)
			}

			if svcAccountBuild != nil && svcAccountBuild.Exists() {
				By("Delete firmware build ClusterRoleBinding")
				crb := define.ModuleCRB(*svcAccountBuild, tsparams.FirmwareModuleName)
				_ = crb.Delete()
			}

			if buildNsCreated {
				By("Delete firmware build namespace")
				_ = namespace.NewBuilder(APIClient, tsparams.FirmwareBuildNamespace).Delete()
			}
		})

		It("should create BMC with firmwareFilesPath and load firmware module", reportxml.ID("85566"), func() {
			By("Check if external registry credentials are configured")
			if ModulesConfig.PullSecret == "" || ModulesConfig.Registry == "" {
				Skip("ECO_HWACCEL_KMM_REGISTRY and ECO_HWACCEL_KMM_PULL_SECRET not configured")
			}

			klog.V(kmmparams.KmmLogLevel).Infof("Using external registry: %s", ModulesConfig.Registry)

			By("Create namespace for firmware image build")
			_, err := namespace.NewBuilder(APIClient, tsparams.FirmwareBuildNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating build namespace")
			buildNsCreated = true

			By("Create registry secret for pushing firmware image")
			secretContent := define.SecretContent(ModulesConfig.Registry, ModulesConfig.PullSecret)
			_, err = secret.NewBuilder(APIClient, tsparams.FirmwareSecretName,
				tsparams.FirmwareBuildNamespace, corev1.SecretTypeDockerConfigJson).
				WithData(secretContent).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating registry secret")

			By("Create ServiceAccount for firmware build")
			svcAccountBuild, err = serviceaccount.NewBuilder(APIClient,
				tsparams.FirmwareServiceAccountName, tsparams.FirmwareBuildNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding for firmware build")
			crb := define.ModuleCRB(*svcAccountBuild, tsparams.FirmwareModuleName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			By("Create ConfigMap with firmware module Dockerfile")
			configmapContents := define.SimpleKmodFirmwareConfigMapContents()
			dockerfileConfigMap, err := configmap.NewBuilder(APIClient,
				tsparams.FirmwareModuleName, tsparams.FirmwareBuildNamespace).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating dockerfile configmap")

			By("Create KernelMapping for firmware module build")
			firmwareImage = fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION",
				ModulesConfig.Registry, tsparams.FirmwareModuleName)

			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
			kernelMapping.WithContainerImage(firmwareImage).
				WithBuildArg("KMODVER", "0.0.1").
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer for firmware module (build only, no deployment)")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(tsparams.FirmwareModuleName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainer.WithVersion("first")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module CR to build and push firmware image")
			module := kmm.NewModuleBuilder(APIClient,
				tsparams.FirmwareModuleName, tsparams.FirmwareBuildNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithImageRepoSecret(tsparams.FirmwareSecretName).
				WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccountBuild.Object.Name)

			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating firmware module")
			moduleCreated = true

			By("Check if build is needed (give KMM time to create build pod)")
			time.Sleep(30 * time.Second)

			buildPods, err := pod.List(APIClient, tsparams.FirmwareBuildNamespace, metav1.ListOptions{
				LabelSelector: "kmm.node.kubernetes.io/build.pod=true",
			})
			Expect(err).ToNot(HaveOccurred(), "error listing build pods")

			if len(buildPods) > 0 {
				By("Wait for build pod to complete (image does not exist in registry)")
				err = await.BuildPodCompleted(APIClient, tsparams.FirmwareBuildNamespace, 10*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for firmware image build")
				klog.V(kmmparams.KmmLogLevel).Infof("Firmware image built: %s", firmwareImage)
			} else {
				klog.V(kmmparams.KmmLogLevel).Infof("No build pod created - image already exists: %s",
					firmwareImage)
			}

			By("Delete the Module CR (image remains in registry)")
			_, err = module.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting firmware module")
			moduleCreated = false

			err = await.ModuleObjectDeleted(APIClient,
				tsparams.FirmwareModuleName, tsparams.FirmwareBuildNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for module to be deleted")

			By("Get a worker node for testing")
			nodeList, err := nodes.List(APIClient, metav1.ListOptions{
				LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing worker nodes")
			Expect(len(nodeList)).To(BeNumerically(">", 0), "no worker nodes found")

			workerNode = nodeList[0]
			klog.V(kmmparams.KmmLogLevel).Infof("Using worker node: %s", workerNode.Object.Name)

			By("Create BMC with firmwareFilesPath")
			kernelVersion, err := get.KernelFullVersion(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error getting kernel version")

			actualFirmwareImage := fmt.Sprintf("%s/%s",
				ModulesConfig.Registry, tsparams.FirmwareModuleName)

			bmcBuilder = kmm.NewBootModuleConfigBuilder(APIClient,
				tsparams.BMCFirmwareName, tsparams.BMCTestNamespace).
				WithKernelModuleImage(actualFirmwareImage).
				WithKernelModuleName(tsparams.FirmwareModuleName).
				WithMachineConfigName(tsparams.MachineConfigFirmwareName).
				WithMachineConfigPoolName(kmmparams.DefaultWorkerMCPName).
				WithFirmwareFilesPath(tsparams.FirmwareFilesPath)

			_, err = bmcBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating bootmoduleconfig")

			klog.V(kmmparams.KmmLogLevel).Infof("BMC created with firmware image: %s, kernel: %s",
				actualFirmwareImage, kernelVersion)

			By("Wait for MachineConfig to be created")
			err = await.MachineConfigCreated(APIClient, tsparams.MachineConfigFirmwareName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig was not created in time")

			By("Verify MachineConfig contains FIRMWARE_FILES_PATH")
			firmwarePathValue, err := get.MachineConfigEnvVar(APIClient,
				tsparams.MachineConfigFirmwareName, "FIRMWARE_FILES_PATH")
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should contain FIRMWARE_FILES_PATH")
			Expect(firmwarePathValue).To(Equal(tsparams.FirmwareFilesPath),
				"FIRMWARE_FILES_PATH should match configured path")

			By("Wait for MCO to write new config to disk")
			err = await.NodeDesiredConfigChange(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MCO did not write new config for node in time")

			By("Reboot the worker node to apply BMC config")
			err = do.Reboot(APIClient, workerNode.Object.Name, kmmparams.KmmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to reboot node")

			By("Wait for node to come back up and be Ready")
			err = await.NodeConfigApplied(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "node did not become Ready after reboot")

			By("Wait for helper pod on rebooted node to be ready")
			_, err = await.ReadyHelperPod(APIClient, kmmparams.KmmOperatorNamespace,
				workerNode.Object.Name, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "helper pod not ready on node after reboot")

			By("Verify firmware module is loaded on the node")
			err = check.ModuleLoadedOnNode(APIClient,
				tsparams.FirmwareModuleName, time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "firmware module should be loaded by BMC")

			By("Verify dmesg contains firmware validation message")
			err = check.DmesgOnNode(APIClient, "ALL GOOD WITH FIRMWARE", time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "dmesg should contain firmware validation message")

			By("Delete the BootModuleConfig")
			_, err = bmcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

			By("Verify MachineConfig is still present after BMC deletion")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigFirmwareName)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should still exist after BMC deletion")
			Expect(mcBuilder.Exists()).To(BeTrue(), "MachineConfig should still exist")

			By("Delete the MachineConfig")
			err = mcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

			By("Verify MachineConfig is deleted")
			err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigFirmwareName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

			By("Wait for MachineConfigPool to start updating (nodes will reboot)")
			mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
			Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

			By("Wait for MachineConfigPool update to complete (all nodes rebooted)")
			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
		})
	})
})
