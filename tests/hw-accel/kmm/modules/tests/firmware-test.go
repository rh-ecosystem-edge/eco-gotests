package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Module", Label("firmware"), func() {

		moduleName := kmmparams.FirmwareTestNamespace
		kmodName := "simple-kmod-firmware"
		serviceAccountName := "firmware-manager"
		image := fmt.Sprintf("%s/$MOD_NAMESPACE/$MOD_NAME:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry)

		AfterEach(func() {
			By("Delete Module")
			_, _ = kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.FirmwareTestNamespace).Delete()

			By("Await module to be deleted")
			err := await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.FirmwareTestNamespace, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, kmmparams.FirmwareTestNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_ = crb.Delete()

			By("Delete Namespace")
			_ = namespace.NewBuilder(APIClient, kmmparams.FirmwareTestNamespace).Delete()

		})

		It("should properly build a module with firmware support", reportxml.ID("56675"), func() {

			By("Create Namespace")
			testNamespace, err := namespace.NewBuilder(APIClient, kmmparams.FirmwareTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			configmapContents := define.SimpleKmodFirmwareConfigMapContents()

			By("Create ConfigMap")
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")
			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.FirmwareTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			By("Create KernelMapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image).
				WithBuildArg("KMODVER", "0.0.1").
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithModprobeSpec("/opt", "/firmware", []string{}, []string{}, []string{}, []string{})
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")

			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.FirmwareTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.FirmwareTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.FirmwareTestNamespace, 3*time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check dmesg contains module message")
			err = check.Dmesg(APIClient, "ALL GOOD WITH FIRMWARE", time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking dmesg contents")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.FirmwareTestNamespace, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

		})
	})
})
