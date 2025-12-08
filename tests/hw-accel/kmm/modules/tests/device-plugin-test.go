package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Module", Label("devplug", "kmm-short", "redeploy"), func() {

		var (
			moduleName         = kmmparams.DevicePluginTestNamespace
			kmodName           = "devplug"
			serviceAccountName = "devplug-manager"
			image              = fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
				tsparams.LocalImageRegistry, kmmparams.DevicePluginTestNamespace, kmodName)
			buildArgValue = fmt.Sprintf("%s.o", kmodName)

			testNamespace            *namespace.Builder
			dockerfileConfigMap      *configmap.Builder
			svcAccount               *serviceaccount.Builder
			crb                      rbac.ClusterRoleBindingBuilder
			moduleLoaderContainerCfg *v1beta1.ModuleLoaderContainerSpec
			devicePluginContainerCfd *v1beta1.DevicePluginContainerSpec
		)

		BeforeEach(func() {
			var err error

			By("Create Namespace")
			testNamespace, err = namespace.NewBuilder(APIClient, kmmparams.DevicePluginTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Create ServiceAccount")
			svcAccount, err = serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.DevicePluginTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")
			crbBuilder := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crbBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")
			crb = crbBuilder

			configmapContents := define.MultiStageConfigMapContent(kmodName)

			By("Create ConfigMap")
			dockerfileConfigMap, err = configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create KernelMapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
			kernelMapping.WithContainerImage(image).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err = moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create DevicePlugin")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			if ModulesConfig.DevicePluginImage == "" {
				Skip("ECO_HWACCEL_KMM_DEVICE_PLUGIN_IMAGE not configured. Skipping test.")
			}

			devicePluginImage := fmt.Sprintf(ModulesConfig.DevicePluginImage, arch)

			devicePlugin := kmm.NewDevicePluginContainerBuilder(devicePluginImage)
			devicePluginContainerCfd, err = devicePlugin.GetDevicePluginContainerConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating deviceplugincontainer")
		})

		AfterEach(func() {
			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.DevicePluginTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.DevicePluginTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Delete ConfigMap")
			err = dockerfileConfigMap.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting configmap")

			By("Delete ServiceAccount")
			err = svcAccount.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting serviceaccount")

			By("Delete ClusterRoleBinding")
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting clusterrolebinding")

			By("Delete Namespace")
			err = testNamespace.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")

			By("Wait for namespace to be fully deleted")
			Eventually(func() bool {
				_, err := namespace.Pull(APIClient, kmmparams.DevicePluginTestNamespace)
				return err != nil // namespace is deleted when Pull fails
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "namespace was not deleted in time")
		})

		It("should deploy module with a device plugin", reportxml.ID("53678"), func() {
			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.DevicePluginTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			module = module.WithDevicePluginContainer(devicePluginContainerCfd).
				WithDevicePluginServiceAccount(svcAccount.Object.Name)
			_, err := module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.DevicePluginTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.DevicePluginTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Await device driver deployment")
			err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.DevicePluginTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.DevicePluginTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should redeploy module with a device plugin", reportxml.ID("82674"), func() {
			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.DevicePluginTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			module = module.WithDevicePluginContainer(devicePluginContainerCfd).
				WithDevicePluginServiceAccount(svcAccount.Object.Name)
			_, err := module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.DevicePluginTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.DevicePluginTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Await device driver deployment")
			err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.DevicePluginTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.DevicePluginTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking node labels")
		})
	})

})
