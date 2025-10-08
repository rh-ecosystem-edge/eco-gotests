package tests

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Module", Label("in-tree-replace"), func() {

		moduleName := kmmparams.InTreeReplacementNamespace
		kmodName := "replace"
		serviceAccountName := "replace-manager"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.InTreeReplacementNamespace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)
		var kmodToRemove string

		BeforeAll(func() {
			By("Get architecture ")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			kmodToRemove = "ice"
			if arch == "s390x" {
				kmodToRemove = "qeth"
			}
		})

		AfterAll(func() {
			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.InTreeReplacementNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.InTreeReplacementNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, kmmparams.InTreeReplacementNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Delete Namespace")
			err = namespace.NewBuilder(APIClient, kmmparams.InTreeReplacementNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
		})

		It("should replace in-tree module", reportxml.ID("62745"), func() {

			By("Create Namespace")
			_, err := namespace.NewBuilder(APIClient, kmmparams.InTreeReplacementNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			configmapContents := define.LocalMultiStageConfigMapContent(kmodName)

			By("Create ConfigMap")
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, kmmparams.InTreeReplacementNamespace).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")
			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.InTreeReplacementNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			By("Create KernelMapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name).WithInTreeModuleToRemove(kmodToRemove)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Making sure in-tree-module is loaded")
			err = check.IntreeModuleLoaded(APIClient, kmodToRemove, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while loading in-tree module")

			By("Check in-tree-module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodToRemove, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the in-tree module is loaded")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.InTreeReplacementNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.InTreeReplacementNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.InTreeReplacementNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check in-tree module is removed on node")
			err = check.ModuleLoaded(APIClient, kmodToRemove, 20*time.Second)
			Expect(err).To(HaveOccurred(), "error while checking the in-tree-module was removed")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.InTreeReplacementNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

	})
})
