package tests

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/mcm/internal/tsparams"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM-Hub", Ordered, Label(tsparams.LabelSuite), func() {

	Context("MCM", Label("hub-intree-remove"), func() {

		moduleName := "intree"
		secretName := "registry-secret"
		plainImage := fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION-%v-tree",
			ModulesConfig.Registry, "multi-build", time.Now().Unix())
		buildArgValue := fmt.Sprintf("%s.o", moduleName)
		kmodToRemove := "ice"

		BeforeAll(func() {
			if ModulesConfig.SpokeClusterName == "" || ModulesConfig.SpokeKubeConfig == "" {
				Skip("Skipping test. No Spoke environment variables defined.")
			}

			if ModulesConfig.Registry == "" || ModulesConfig.PullSecret == "" {
				Skip("Skipping test. No Registry or PullSecret environment variables defined.")
			}
		})

		AfterAll(func() {
			By("Delete ManagedClusterModule")
			_, err := kmm.NewManagedClusterModuleBuilder(APIClient, moduleName, kmmparams.KmmHubOperatorNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting managedclustermodule")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(ModulesConfig.SpokeAPIClient, moduleName, kmmparams.KmmOperatorNamespace,
				time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for module to be deleted on spoke")

			By("Delete Hub Secret")
			err = secret.NewBuilder(APIClient, secretName,
				kmmparams.KmmHubOperatorNamespace, corev1.SecretTypeDockerConfigJson).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting hub registry secret")

			By("Delete Spoke Secret")
			err = secret.NewBuilder(ModulesConfig.SpokeAPIClient, secretName,
				kmmparams.KmmOperatorNamespace, corev1.SecretTypeDockerConfigJson).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting spoke registry secret")

		})

		It("should replace in-tree module", reportxml.ID("62744"), func() {

			By("Creating registry secret on Hub")
			secretContent := define.SecretContent(ModulesConfig.Registry, ModulesConfig.PullSecret)

			_, err := secret.NewBuilder(APIClient, secretName,
				kmmparams.KmmHubOperatorNamespace, corev1.SecretTypeDockerConfigJson).WithData(secretContent).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating secret on hub")

			By("Creating registry secret on Spoke")
			_, err = secret.NewBuilder(ModulesConfig.SpokeAPIClient, secretName,
				kmmparams.KmmOperatorNamespace, corev1.SecretTypeDockerConfigJson).WithData(secretContent).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating secret on spoke")

			By("Obtain DTK image from the Spoke")
			dtkImage, err := get.DTKImage(ModulesConfig.SpokeAPIClient)
			Expect(err).ToNot(HaveOccurred(), "Could not get spoke's DTK image.")

			By("Create ConfigMap")
			configmapContents := define.UserDtkMultiStateConfigMapContents(moduleName, dtkImage)
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, moduleName, kmmparams.KmmHubOperatorNamespace).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Making sure in-tree-module is loaded on spoke")
			err = check.IntreeModuleLoaded(ModulesConfig.SpokeAPIClient, kmodToRemove, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while loading in-tree module")

			By("Check in-tree-module is loaded on node")
			err = check.ModuleLoaded(ModulesConfig.SpokeAPIClient, kmodToRemove, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the in-tree module is loaded")

			By("Create KernelMapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
			kernelMapping.WithContainerImage(plainImage).
				WithBuildArg("MY_MODULE", buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name).
				WithBuildImageRegistryTLS(true, true).
				RegistryTLS(true, true).
				WithInTreeModuleToRemove(kmodToRemove)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(moduleName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Build Module Spec")
			moduleSpec, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.KmmOperatorNamespace).
				WithNodeSelector(GeneralConfig.ControlPlaneLabelMap).
				WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithImageRepoSecret(secretName).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error creating module spec")

			By("Create ManagedClusterModule")
			selector := map[string]string{"name": ModulesConfig.SpokeClusterName}
			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, moduleName, kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(selector).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error creating managedclustermodule")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.KmmHubOperatorNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment on Spoke")
			err = await.ModuleDeployment(ModulesConfig.SpokeAPIClient, moduleName, kmmparams.KmmOperatorNamespace,
				time.Minute, GeneralConfig.ControlPlaneLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(ModulesConfig.SpokeAPIClient, moduleName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check in-tree module is removed on node")
			err = check.ModuleLoaded(ModulesConfig.SpokeAPIClient, kmodToRemove, 20*time.Second)
			Expect(err).To(HaveOccurred(), "error while checking the in-tree-module was removed")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(ModulesConfig.SpokeAPIClient, moduleName, kmmparams.KmmOperatorNamespace,
				GeneralConfig.ControlPlaneLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking label on all nodes")
		})
	})
})
