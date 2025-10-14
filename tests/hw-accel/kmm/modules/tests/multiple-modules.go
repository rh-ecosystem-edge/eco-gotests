package tests

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
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

	Context("Module", Label("multiple"), func() {

		var nSpace = kmmparams.MultipleModuleTestNamespace
		kmodName := "multiplemodules"
		buildArgValue := fmt.Sprintf("%s.o", kmodName)
		serviceAccountName := "multiple-sa"

		BeforeAll(func() {

			By("Create Namespace")
			_, err := namespace.NewBuilder(APIClient, nSpace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
		})

		AfterAll(func() {
			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, kmodName, nSpace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, kmodName, nSpace, 1*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Delete Namespace")
			err = namespace.NewBuilder(APIClient, nSpace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, nSpace)
			svcAccount.Exists()

			By("Delete clusterrolebinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting clusterrolebinding ")
		})

		Context("Modprobe", Label("multiple"), func() {

			It("should fail if any of the modules is not present", reportxml.ID("62743"), func() {
				configmapContents := define.LocalMultiStageConfigMapContent(kmodName)

				By("Create ConfigMap")
				dockerFileConfigMap, err := configmap.
					NewBuilder(APIClient, kmodName, nSpace).
					WithData(configmapContents).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating configmap")

				By("Create ServiceAccount")
				svcAccount, err := serviceaccount.
					NewBuilder(APIClient, serviceAccountName, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

				By("Create ClusterRoleBinding")
				crb := define.ModuleCRB(*svcAccount, kmodName)
				_, err = crb.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

				By("Create KernelMapping")
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					tsparams.LocalImageRegistry, nSpace, kmodName)
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
					WithContainerImage(image).
					WithBuildArg(kmmparams.BuildArgName, buildArgValue).
					WithBuildDockerCfgFile(dockerFileConfigMap.Object.Name).
					BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create moduleLoader container")
				moduleLoader, err := kmm.NewModLoaderContainerBuilder(kmodName).
					WithModprobeSpec("", "", nil, nil, nil, []string{kmodName, "kmm-ci-a"}).
					WithKernelMapping(kernelMapping).
					WithImagePullPolicy("Always").
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, kmodName, nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoader).
					WithLoadServiceAccount(svcAccount.Object.Name).
					Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete build")
				err = await.BuildPodCompleted(APIClient, nSpace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")
				err = await.ModuleDeployment(APIClient, kmodName, nSpace, 3*time.Minute,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Check module is loaded on node")
				err = check.ModuleLoaded(APIClient, kmodName, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check module is loaded on node")
				err = check.ModuleLoaded(APIClient, "kmm-ci-a", 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			})
		})
	})
})
