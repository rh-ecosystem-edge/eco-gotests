package tests

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	"k8s.io/klog/v2"

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

	Context("Module", Label("use-dtk"), func() {

		var testNamespace *namespace.Builder

		moduleName := kmmparams.UseDtkModuleTestNamespace
		kmodName := "use-dtk"
		serviceAccountName := "dtk-manager"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.UseDtkModuleTestNamespace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		BeforeAll(func() {

			By("Create Namespace")
			var err error
			testNamespace, err = namespace.NewBuilder(APIClient, kmmparams.UseDtkModuleTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

		})

		AfterAll(func() {

			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.UseDtkModuleTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.UseDtkModuleTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, kmmparams.UseDtkModuleTestNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")

			By("Delete preflightvalidationocp")
			_, err = kmm.NewPreflightValidationOCPBuilder(APIClient, kmmparams.PreflightName,
				kmmparams.UseDtkModuleTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting preflightvalidationocp")

			By("Delete Namespace")
			err = namespace.NewBuilder(APIClient, kmmparams.UseDtkModuleTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

		})

		It("should use DTK_AUTO parameter", reportxml.ID("54283"), func() {

			configmapContents := define.MultiStageConfigMapContent(kmodName)

			By("Create ConfigMap")
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")
			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.UseDtkModuleTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

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
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.UseDtkModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.UseDtkModuleTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.UseDtkModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.UseDtkModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should be able to modify a kmod in a module", reportxml.ID("53466"), func() {

			const newKmod = "kmm_ci_a"

			By("Getting the module")
			moduleBuilder, err := kmm.Pull(APIClient, moduleName, moduleName)
			Expect(err).ToNot(HaveOccurred(), "error getting the module")

			By("Modifying the kmod in the module and re-applying the module")
			moduleBuilder.Object.Spec.ModuleLoader.Container.Modprobe.ModuleName = newKmod
			_, err = moduleBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "error updating the module")

			By("Wait for worker pods to apply changes")
			time.Sleep(time.Minute)

			By("Await new driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.UseDtkModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check new kmod was loaded to node")
			err = check.ModuleLoaded(APIClient, newKmod, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

		})

		It("should fail to update an existing module with something that is wrong", reportxml.ID("62598"), func() {
			By("Getting the module")
			moduleBuilder, err := kmm.Pull(APIClient, moduleName, moduleName)
			Expect(err).ToNot(HaveOccurred(), "error getting the module")

			By("Create KernelMapping")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Update existing module with something wrong")
			_, err = moduleBuilder.WithModuleLoaderContainer(moduleLoaderContainerCfg).Update()
			klog.V(kmmparams.KmmLogLevel).Infof("webhook err: %s", err)
			Expect(err).To(HaveOccurred(), "error creating module")
			Expect(err.Error()).To(ContainSubstring("missing spec.moduleLoader.container.kernelMappings"))
			Expect(err.Error()).To(ContainSubstring(".containerImage"))
		})

		It("should be able to run preflightvalidation with no push to registry", reportxml.ID("56330"), func() {
			By("Detecting cluster architecture")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			By("Get kernel version for preflight")
			kernelVersion := get.PreflightKernel(arch, false)

			By("Get the DTK Image for preflight test")
			dtkImage := get.PreflightImage(arch)

			By("Wait for module to be fully deployed before creating preflight")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.UseDtkModuleTestNamespace, 2*time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for module deployment")

			By("Create preflightvalidationocp")
			pre, err := kmm.NewPreflightValidationOCPBuilder(APIClient, kmmparams.PreflightName,
				kmmparams.UseDtkModuleTestNamespace).
				WithKernelVersion(kernelVersion).
				WithDtkImage(dtkImage).
				WithPushBuiltImage(false).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error while creating preflight")

			By("Await preflight build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.UseDtkModuleTestNamespace, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "No build pod found or completed")

			By("Await preflightvalidationocp checks")
			err = await.PreflightStageDone(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.UseDtkModuleTestNamespace, 3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "preflightvalidationocp did not complete")

			By("Get status of the preflightvalidationocp checks")
			status, _ := get.PreflightReason(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.UseDtkModuleTestNamespace)
			Expect(strings.Contains(status, "Verification successful (build compiles)") ||
				strings.Contains(status, "verified image exists")).
				To(BeTrue(), "expected message not found")

			By("Validate imagestream tag is not created in internal registry")
			err = check.ImageStreamExistsForModule(APIClient, kmmparams.UseDtkModuleTestNamespace,
				moduleName, kmodName, kernelVersion)
			Expect(err).To(HaveOccurred(), "imagestream tag exists while it should not")

			By("Delete preflight validation")
			_, err = pre.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting preflightvalidation")

			By("Wait for resources cleanup")
			time.Sleep(time.Minute)
		})

		It("should be able to run preflightvalidation and push to registry", reportxml.ID("56328"), func() {
			By("Detecting cluster architecture")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			By("Get kernel version for preflight")
			kernelVersion := get.PreflightKernel(arch, false)

			By("Get the DTK Image for preflight test")
			dtkImage := get.PreflightImage(arch)

			By("Create preflightvalidationocp")
			_, err = kmm.NewPreflightValidationOCPBuilder(APIClient, kmmparams.PreflightName,
				kmmparams.UseDtkModuleTestNamespace).
				WithKernelVersion(kernelVersion).
				WithDtkImage(dtkImage).
				WithPushBuiltImage(true).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error while creating preflight")

			By("Await preflight build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.UseDtkModuleTestNamespace, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "No build pod found or completed")

			By("Await preflightvalidationocp checks")
			err = await.PreflightStageDone(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.UseDtkModuleTestNamespace, 3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "preflightvalidationocp did not complete")

			By("Get status of the preflightvalidationocp checks")
			status, _ := get.PreflightReason(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.UseDtkModuleTestNamespace)
			Expect(strings.Contains(status, "Verification successful (build compiles and image pushed)") ||
				strings.Contains(status, "verified image exists")).
				To(BeTrue(), "expected message not found")

			By("Validate new imagestream is created in internal registry")
			err = check.ImageStreamExistsForModule(APIClient, kmmparams.UseDtkModuleTestNamespace,
				moduleName, kmodName, kernelVersion)
			Expect(err).ToNot(HaveOccurred(), "imagestream validation failed")
		})
	})
})
