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

var _ = Describe("KMM-BMC-Firmware", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity, tsparams.LabelSuite),
	func() {

		Context("BMC with firmwareFilesPath", Label("bmc-firmware"), func() {

			var (
				bmcBuilder       *kmm.BootModuleConfigBuilder
				workerNode       *nodes.Builder
				buildNsCreated   bool
				svcAccountBuild  *serviceaccount.Builder
				firmwareImageURL string
			)

			BeforeAll(func() {
				By("Check KMM version is 2.5 or higher")
				currentVersion, err := get.KmmOperatorVersion(APIClient)
				Expect(err).ToNot(HaveOccurred(), "failed to get KMM operator version")

				minVersion, err := version.NewVersion("2.5.0")
				Expect(err).ToNot(HaveOccurred(), "failed to parse minimum version")

				if currentVersion.LessThan(minVersion) {
					Skip("BootModuleConfig tests require KMM version 2.5.0 or higher")
				}

				klog.V(kmmparams.KmmLogLevel).Infof("KMM version %s >= 2.5.0, proceeding with BMC firmware tests",
					currentVersion)

				By("Check if external registry credentials are configured")
				if ModulesConfig.PullSecret == "" || ModulesConfig.Registry == "" {
					Skip("ECO_HWACCEL_KMM_REGISTRY and ECO_HWACCEL_KMM_PULL_SECRET not configured")
				}

				klog.V(kmmparams.KmmLogLevel).Infof("Using external registry: %s", ModulesConfig.Registry)

				By("Build firmware image URL")
				firmwareImageURL = fmt.Sprintf("%s/%s",
					ModulesConfig.Registry, tsparams.FirmwareModuleName)

				By("Check if firmware image already exists")
				_, err = check.ImageExists(APIClient, firmwareImageURL, GeneralConfig.WorkerLabelMap)

				if err == nil {
					klog.V(kmmparams.KmmLogLevel).Infof("Firmware image already exists: %s", firmwareImageURL)
					return
				}

				klog.V(kmmparams.KmmLogLevel).Infof("Firmware image not found, building: %s", firmwareImageURL)

				By("Create namespace for firmware image build")
				_, err = namespace.NewBuilder(APIClient, tsparams.FirmwareBuildNamespace).Create()
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
				firmwareImageWithTag := fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION",
					ModulesConfig.Registry, tsparams.FirmwareModuleName)

				kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
				kernelMapping.WithContainerImage(firmwareImageWithTag).
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

				By("Wait for firmware image to be available")
				err = await.ModuleDeployment(APIClient, tsparams.FirmwareModuleName,
					tsparams.FirmwareBuildNamespace, 10*time.Minute, GeneralConfig.WorkerLabelMap)
				if err != nil {
					klog.V(kmmparams.KmmLogLevel).Infof("Module deployment wait returned: %v (may be expected)", err)
				}

				By("Delete the Module CR (image remains in registry)")
				_, err = module.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting firmware module")

				err = await.ModuleObjectDeleted(APIClient,
					tsparams.FirmwareModuleName, tsparams.FirmwareBuildNamespace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for module to be deleted")

				klog.V(kmmparams.KmmLogLevel).Infof("Firmware image built and available: %s", firmwareImageURL)
			})

			AfterAll(func() {
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

				bmcBuilder = kmm.NewBootModuleConfigBuilder(APIClient,
					tsparams.BMCFirmwareName, tsparams.BMCTestNamespace).
					WithKernelModuleImage(firmwareImageURL).
					WithKernelModuleName(tsparams.FirmwareModuleName).
					WithMachineConfigName(tsparams.MachineConfigFirmwareName).
					WithMachineConfigPoolName(kmmparams.DefaultWorkerMCPName).
					WithFirmwareFilesPath(tsparams.FirmwareFilesPath)

				_, err = bmcBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating bootmoduleconfig")

				klog.V(kmmparams.KmmLogLevel).Infof("BMC created with firmware image: %s, kernel: %s",
					firmwareImageURL, kernelVersion)

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
			})
		})
	})
