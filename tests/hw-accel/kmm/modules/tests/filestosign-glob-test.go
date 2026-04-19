package tests

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
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
	Context("Module", Label("filestosign-glob"), func() {
		var (
			nsName             = kmmparams.FilesToSignGlobTestNamespace
			moduleName         = "sign-glob"
			kmodName           = "kmm_ci_a"
			serviceAccountName = "sign-glob-sa"
			signerCN           = "cdvtest signing key"
		)

		BeforeAll(func() {
			By("Create Namespace")

			_, err := namespace.NewBuilder(APIClient, nsName).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Create signing-key-pub secret")

			signPub := get.SigningData("cert", kmmparams.SigningCertBase64)

			_, err = secret.NewBuilder(APIClient, "my-signing-key-pub",
				nsName, corev1.SecretTypeOpaque).WithData(signPub).Create()
			Expect(err).ToNot(HaveOccurred(), "failed creating signing pub secret")

			By("Create signing-key secret")

			signKey := get.SigningData("key", kmmparams.SigningKeyBase64)

			_, err = secret.NewBuilder(APIClient, "my-signing-key",
				nsName, corev1.SecretTypeOpaque).WithData(signKey).Create()
			Expect(err).ToNot(HaveOccurred(), "failed creating signing key secret")

			By("Create ServiceAccount")

			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, nsName).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")

			crb := define.ModuleCRB(*svcAccount, moduleName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")
		})

		AfterAll(func() {
			By("Delete Module if exists")

			_, _ = kmm.NewModuleBuilder(APIClient, moduleName, nsName).Delete()
			_ = await.ModuleObjectDeleted(APIClient, moduleName, nsName, time.Minute)

			By("Delete ClusterRoleBinding")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, nsName)
			svcAccount.Exists()

			crb := define.ModuleCRB(*svcAccount, moduleName)
			_ = crb.Delete()

			By("Delete Namespace")

			err := namespace.NewBuilder(APIClient, nsName).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")
		})

		Context("positive signing", Label("filestosign-glob-positive"), func() {
			var dockerfileConfigMap *configmap.Builder

			BeforeAll(func() {
				By("Create multi-ko Dockerfile ConfigMap")

				var err error

				configmapContents := define.MultiKoConfigMapContent()
				dockerfileConfigMap, err = configmap.
					NewBuilder(APIClient, "multi-ko-dockerfile", nsName).
					WithData(configmapContents).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating configmap")
			})

			AfterEach(func() {
				By("Delete Module")

				_, err := kmm.NewModuleBuilder(APIClient, moduleName, nsName).Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting module")

				By("Await module to be deleted")

				err = await.ModuleObjectDeleted(APIClient, moduleName, nsName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")
			})

			It("should sign module with explicit path", reportxml.ID("88314"), func() {
				imageTag := "explicit"
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION-%s",
					tsparams.LocalImageRegistry, nsName, moduleName, imageTag)
				filesToSign := []string{
					fmt.Sprintf("/opt/lib/modules/$KERNEL_FULL_VERSION/%s.ko", kmodName),
				}

				By("Create KernelMapping with explicit filesToSign path")

				kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
				kernelMapping.WithContainerImage(image).
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

				module := kmm.NewModuleBuilder(APIClient, moduleName, nsName).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(serviceAccountName)
				_, err = module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nsName, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nsName, 5*time.Minute,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check module is signed")

				err = check.ModuleSigned(APIClient, kmodName, signerCN, nsName, image)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is signed")
			})

			It("should sign all .ko files with *.ko glob", reportxml.ID("88315"), func() {
				imageTag := "wildcard"
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION-%s",
					tsparams.LocalImageRegistry, nsName, moduleName, imageTag)
				filesToSign := []string{
					"/opt/lib/modules/$KERNEL_FULL_VERSION/*.ko",
				}

				By("Create KernelMapping with *.ko glob filesToSign")

				kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
				kernelMapping.WithContainerImage(image).
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

				module := kmm.NewModuleBuilder(APIClient, moduleName, nsName).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(serviceAccountName)
				_, err = module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nsName, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nsName, 5*time.Minute,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check all three modules are signed")

				err = check.MultiModuleSigned(APIClient,
					[]string{"kmm_ci_a", "kmm_ci_b", "test_mod"},
					signerCN, nsName, image, "/opt")
				Expect(err).ToNot(HaveOccurred(), "error while checking all modules are signed")
			})

			It("should sign only matching files with ? glob", reportxml.ID("88316"), func() {
				imageTag := "qmark"
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION-%s",
					tsparams.LocalImageRegistry, nsName, moduleName, imageTag)
				filesToSign := []string{
					"/opt/lib/modules/$KERNEL_FULL_VERSION/kmm_ci_?.ko",
				}

				By("Create KernelMapping with ? glob filesToSign")

				kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
				kernelMapping.WithContainerImage(image).
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

				module := kmm.NewModuleBuilder(APIClient, moduleName, nsName).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(serviceAccountName)
				_, err = module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nsName, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nsName, 5*time.Minute,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check kmm_ci_a and kmm_ci_b are signed")

				err = check.MultiModuleSigned(APIClient,
					[]string{"kmm_ci_a", "kmm_ci_b"},
					signerCN, nsName, image, "/opt")
				Expect(err).ToNot(HaveOccurred(), "error while checking matched modules are signed")

				By("Check test_mod is NOT signed")

				err = check.ModuleNotSigned(APIClient, "test_mod", signerCN, nsName, image)
				Expect(err).ToNot(HaveOccurred(), "test_mod should not be signed by ? glob")
			})

			It("should sign only matching files with [ab] character class", reportxml.ID("88317"), func() {
				imageTag := "charrange"
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION-%s",
					tsparams.LocalImageRegistry, nsName, moduleName, imageTag)
				filesToSign := []string{
					"/opt/lib/modules/$KERNEL_FULL_VERSION/kmm_ci_[ab].ko",
				}

				By("Create KernelMapping with [ab] character class filesToSign")

				kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
				kernelMapping.WithContainerImage(image).
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

				module := kmm.NewModuleBuilder(APIClient, moduleName, nsName).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(serviceAccountName)
				_, err = module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nsName, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nsName, 5*time.Minute,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check kmm_ci_a and kmm_ci_b are signed")

				err = check.MultiModuleSigned(APIClient,
					[]string{"kmm_ci_a", "kmm_ci_b"},
					signerCN, nsName, image, "/opt")
				Expect(err).ToNot(HaveOccurred(), "error while checking matched modules are signed")

				By("Check test_mod is NOT signed")

				err = check.ModuleNotSigned(APIClient, "test_mod", signerCN, nsName, image)
				Expect(err).ToNot(HaveOccurred(), "test_mod should not be signed by [ab] glob")
			})
		})

		Context("custom dirName", Label("filestosign-glob-dirname"), func() {
			AfterEach(func() {
				By("Delete Module")

				_, err := kmm.NewModuleBuilder(APIClient, moduleName, nsName).Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting module")

				By("Await module to be deleted")

				err = await.ModuleObjectDeleted(APIClient, moduleName, nsName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")
			})

			It("should sign all .ko with glob under custom dirName", reportxml.ID("88320"), func() {
				imageTag := "customdir"
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION-%s",
					tsparams.LocalImageRegistry, nsName, moduleName, imageTag)
				filesToSign := []string{
					"/custom/lib/modules/$KERNEL_FULL_VERSION/*.ko",
				}

				By("Create custom-dir Dockerfile ConfigMap")

				customDirContents := define.MultiKoCustomDirConfigMapContent()
				customDirConfigMap, err := configmap.
					NewBuilder(APIClient, "custom-dir-dockerfile", nsName).
					WithData(customDirContents).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating custom dir configmap")

				By("Create KernelMapping with custom dirName and glob filesToSign")

				kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
				kernelMapping.WithContainerImage(image).
					WithBuildDockerCfgFile(customDirConfigMap.Object.Name).
					WithSign("my-signing-key-pub", "my-signing-key", filesToSign)
				kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create ModuleLoaderContainer with custom dirName")

				moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
				moduleLoaderContainer.WithKernelMapping(kerMapOne)
				moduleLoaderContainer.WithImagePullPolicy("Always")
				moduleLoaderContainer.WithModprobeSpec("/custom", "", nil, nil, nil, nil)
				moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")

				module := kmm.NewModuleBuilder(APIClient, moduleName, nsName).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(serviceAccountName)
				_, err = module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nsName, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nsName, 5*time.Minute,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check all modules are signed under custom dirName")

				err = check.MultiModuleSigned(APIClient,
					[]string{"kmm_ci_a", "kmm_ci_b"},
					signerCN, nsName, image, "/custom")
				Expect(err).ToNot(HaveOccurred(), "error while checking modules are signed under custom dir")
			})
		})
	})
})
