package tests

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/events"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Module", Label("build-sign"), func() {

		moduleName := kmmparams.ModuleBuildAndSignNamespace
		kmodName := "module-signing"
		serviceAccountName := "build-and-sign-sa"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.ModuleBuildAndSignNamespace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)
		filesToSign := []string{fmt.Sprintf("/opt/lib/modules/$KERNEL_FULL_VERSION/%s.ko", kmodName)}

		AfterAll(func() {
			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.ModuleBuildAndSignNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.ModuleBuildAndSignNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, kmmparams.ModuleBuildAndSignNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Delete preflightvalidationocp")
			_, err = kmm.NewPreflightValidationOCPBuilder(APIClient, kmmparams.PreflightName,
				kmmparams.ModuleBuildAndSignNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting preflightvalidationocp")

			By("Delete Namespace")
			err = namespace.NewBuilder(APIClient, kmmparams.ModuleBuildAndSignNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

		})

		It("should use build and sign a module", reportxml.ID("56252"), func() {

			By("Create Namespace")
			testNamespace, err := namespace.NewBuilder(APIClient, kmmparams.ModuleBuildAndSignNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Creating my-signing-key-pub")
			signKey := get.SigningData("cert", kmmparams.SigningCertBase64)

			_, err = secret.NewBuilder(APIClient, "my-signing-key-pub",
				kmmparams.ModuleBuildAndSignNamespace, corev1.SecretTypeOpaque).WithData(signKey).Create()
			Expect(err).ToNot(HaveOccurred(), "failed creating secret")

			By("Creating my-signing-key")
			signCert := get.SigningData("key", kmmparams.SigningKeyBase64)

			_, err = secret.NewBuilder(APIClient, "my-signing-key",
				kmmparams.ModuleBuildAndSignNamespace, corev1.SecretTypeOpaque).WithData(signCert).Create()
			Expect(err).ToNot(HaveOccurred(), "failed creating secret")

			By("Create ConfigMap")
			configmapContents := define.MultiStageConfigMapContent(kmodName)

			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")
			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.ModuleBuildAndSignNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			By("Create KernelMapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name).
				WithSign("my-signing-key-pub", "my-signing-key", filesToSign)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.ModuleBuildAndSignNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.ModuleBuildAndSignNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.ModuleBuildAndSignNamespace, 5*time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check module is signed")
			err = check.ModuleSigned(APIClient, kmodName, "cdvtest signing key",
				kmmparams.ModuleBuildAndSignNamespace, image)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is signed")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.ModuleBuildAndSignNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should generate event about build being created and completed", reportxml.ID("68110"), func() {
			By("Getting events from module's namespace")
			eventList, err := events.List(APIClient, kmmparams.ModuleBuildAndSignNamespace)
			Expect(err).ToNot(HaveOccurred(), "Fail to collect events")

			reasonBuildListLength := len(kmmparams.ReasonBuildList)
			foundEvents := 0
			for _, item := range kmmparams.ReasonBuildList {
				glog.V(kmmparams.KmmLogLevel).Infof("Checking %s is present in events", item)
				for _, event := range eventList {
					glog.V(kmmparams.KmmLogLevel).Infof("Checking event: %s", event.Object.Reason)
					if event.Object.Reason == item {
						glog.V(kmmparams.KmmLogLevel).Infof("Found %s in events", item)
						foundEvents++

						break
					}
				}
			}
			Expect(reasonBuildListLength).To(Equal(foundEvents), "Expected number of events not found")
		})

		It("should generate event about sign being created and completed", reportxml.ID("68108"), func() {
			By("Getting events from module's namespace")
			eventList, err := events.List(APIClient, kmmparams.ModuleBuildAndSignNamespace)
			Expect(err).ToNot(HaveOccurred(), "Fail to collect events")

			reasonSignListLength := len(kmmparams.ReasonSignList)
			foundEvents := 0
			for _, item := range kmmparams.ReasonSignList {
				glog.V(kmmparams.KmmLogLevel).Infof("Checking %s is present in events", item)
				for _, event := range eventList {
					if event.Object.Reason == item {
						glog.V(kmmparams.KmmLogLevel).Infof("Found %s in events", item)
						foundEvents++

						break
					}
				}
			}
			Expect(reasonSignListLength).To(Equal(foundEvents), "Expected number of events not found")
		})

		It("should be able to run preflightvalidation with no push", reportxml.ID("56329"), func() {
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
			pre, err := kmm.NewPreflightValidationOCPBuilder(APIClient, kmmparams.PreflightName,
				kmmparams.ModuleBuildAndSignNamespace).
				WithKernelVersion(kernelVersion).
				WithDtkImage(dtkImage).
				WithPushBuiltImage(false).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error while creating preflight")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.ModuleBuildAndSignNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "No build pod found or completed")

			By("Await preflightvalidationocp checks")
			err = await.PreflightStageDone(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.ModuleBuildAndSignNamespace, 3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "preflightvalidationocp did not complete")

			By("Get status of the preflightvalidationocp checks")
			status, _ := get.PreflightReason(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.ModuleBuildAndSignNamespace)
			Expect(strings.Contains(status, "Verification successful (build compiles)") ||
				strings.Contains(status, "verified image exists")).
				To(BeTrue(), "expected message not found")

			By("Delete preflight validation")
			_, err = pre.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting preflightvalidation")
		})

		It("should be able to run preflightvalidation and push to registry", reportxml.ID("56327"), func() {
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
				kmmparams.ModuleBuildAndSignNamespace).
				WithKernelVersion(kernelVersion).
				WithDtkImage(dtkImage).
				WithPushBuiltImage(true).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error while creating preflight")

			By("Await preflightvalidationocp checks")
			err = await.PreflightStageDone(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.ModuleBuildAndSignNamespace, 3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "preflightvalidationocp did not complete")

			By("Await build pod to complete build (if any)")
			err = await.BuildPodCompleted(APIClient, kmmparams.ModuleBuildAndSignNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "No build pod found or completed")

			By("Get status of the preflightvalidationocp checks")
			status, _ := get.PreflightReason(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.ModuleBuildAndSignNamespace)
			Expect(strings.Contains(status, "verified image exists")).
				To(BeTrue(), "expected message not found")

			By("Validate imagestream if using internal registry")
			err = check.ImageStreamExistsForModule(APIClient, kmmparams.ModuleBuildAndSignNamespace,
				moduleName, kmodName, kernelVersion)
			Expect(err).ToNot(HaveOccurred(), "imagestream validation failed")
		})
	})
})
