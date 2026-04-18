package tests

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/mcm/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM-Hub", Ordered, Label(tsparams.LabelSuite), func() {
	Context("MCM", Label("hub-rebuild-trigger-verify"), func() {
		moduleName := "mcm-rt-verify"
		secretName := "registry-secret-verify"
		buildArgValue := fmt.Sprintf("%s.o", moduleName)
		plainImage := fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION-%v",
			ModulesConfig.Registry, moduleName, time.Now().Unix())

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

			_, err := kmm.NewManagedClusterModuleBuilder(APIClient, moduleName,
				kmmparams.KmmHubOperatorNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting managedclustermodule")

			By("Await module to be deleted on spoke")

			err = await.ModuleObjectDeleted(ModulesConfig.SpokeAPIClient, moduleName,
				kmmparams.KmmOperatorNamespace, time.Minute)
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

		It("MCM trigger should only re-verify when image exists", reportxml.ID("87954"), func() {
			By("Creating registry secret on Hub")

			secretContent := define.SecretContent(ModulesConfig.Registry, ModulesConfig.PullSecret)
			_, err := secret.NewBuilder(APIClient, secretName,
				kmmparams.KmmHubOperatorNamespace, corev1.SecretTypeDockerConfigJson).
				WithData(secretContent).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating secret on hub")

			By("Creating registry secret on Spoke")

			_, err = secret.NewBuilder(ModulesConfig.SpokeAPIClient, secretName,
				kmmparams.KmmOperatorNamespace, corev1.SecretTypeDockerConfigJson).
				WithData(secretContent).Create()
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

			By("Create KernelMapping")

			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
			kernelMapping.WithContainerImage(plainImage).
				WithBuildArg("MY_MODULE", buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name).
				WithBuildImageRegistryTLS(true, true).
				RegistryTLS(true, true)
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
			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, moduleName,
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(selector).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error creating managedclustermodule")

			By("Await build pod to complete on hub")

			err = await.BuildPodCompleted(APIClient, kmmparams.KmmHubOperatorNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module on hub")

			By("Await driver container deployment on spoke")

			err = await.ModuleDeployment(ModulesConfig.SpokeAPIClient, moduleName,
				kmmparams.KmmOperatorNamespace, 5*time.Minute, GeneralConfig.ControlPlaneLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment on spoke")

			By("Check label is set on spoke nodes")

			_, err = check.NodeLabel(ModulesConfig.SpokeAPIClient, moduleName,
				kmmparams.KmmOperatorNamespace, GeneralConfig.ControlPlaneLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking label on spoke nodes")

			By("Record existing build pod names on hub")

			existingPods, err := pod.List(APIClient, kmmparams.KmmHubOperatorNamespace, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "error listing hub pods")

			existingBuildPods := []string{}

			for _, existingPod := range existingPods {
				if strings.Contains(existingPod.Object.Name, "-build") {
					existingBuildPods = append(existingBuildPods, existingPod.Object.Name)
				}
			}

			klog.V(kmmparams.KmmLogLevel).Infof("Existing build pods on hub before trigger: %v",
				existingBuildPods)

			By("Set imageRebuildTriggerGeneration to 1 on MCM (image exists, should only verify)")

			mcmBuilder, err := kmm.PullManagedClusterModule(APIClient, moduleName,
				kmmparams.KmmHubOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling MCM")

			mcmBuilder.WithImageRebuildTriggerGeneration(1)
			_, err = mcmBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "error updating MCM with trigger")

			By("Wait for controller to process trigger")

			time.Sleep(30 * time.Second)

			By("Verify NO new build pod was created on hub")

			currentPods, err := pod.List(APIClient, kmmparams.KmmHubOperatorNamespace, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "error listing hub pods")

			for _, currentPod := range currentPods {
				if strings.Contains(currentPod.Object.Name, "-build") {
					found := false

					for _, existing := range existingBuildPods {
						if currentPod.Object.Name == existing {
							found = true

							break
						}
					}

					Expect(found).To(BeTrue(),
						fmt.Sprintf("unexpected new build pod on hub: %s", currentPod.Object.Name))
				}
			}

			By("Check label is still set on spoke nodes")

			_, err = check.NodeLabel(ModulesConfig.SpokeAPIClient, moduleName,
				kmmparams.KmmOperatorNamespace, GeneralConfig.ControlPlaneLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking label on spoke nodes")
		})
	})

	Context("MCM", Label("hub-rebuild-trigger-noop"), func() {
		moduleName := "mcm-rt-noop"
		secretName := "registry-secret-noop"
		buildArgValue := fmt.Sprintf("%s.o", moduleName)
		plainImage := fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION-%v",
			ModulesConfig.Registry, moduleName, time.Now().Unix())

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

			_, err := kmm.NewManagedClusterModuleBuilder(APIClient, moduleName,
				kmmparams.KmmHubOperatorNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting managedclustermodule")

			By("Await module to be deleted on spoke")

			err = await.ModuleObjectDeleted(ModulesConfig.SpokeAPIClient, moduleName,
				kmmparams.KmmOperatorNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for module to be deleted on spoke")

			By("Delete Hub Secret")

			_ = secret.NewBuilder(APIClient, secretName,
				kmmparams.KmmHubOperatorNamespace, corev1.SecretTypeDockerConfigJson).Delete()

			By("Delete Spoke Secret")

			_ = secret.NewBuilder(ModulesConfig.SpokeAPIClient, secretName,
				kmmparams.KmmOperatorNamespace, corev1.SecretTypeDockerConfigJson).Delete()
		})

		It("same MCM trigger value should cause no spoke action", reportxml.ID("87955"), func() {
			By("Creating registry secret on Hub")

			secretContent := define.SecretContent(ModulesConfig.Registry, ModulesConfig.PullSecret)
			_, err := secret.NewBuilder(APIClient, secretName,
				kmmparams.KmmHubOperatorNamespace, corev1.SecretTypeDockerConfigJson).
				WithData(secretContent).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating secret on hub")

			By("Creating registry secret on Spoke")

			_, err = secret.NewBuilder(ModulesConfig.SpokeAPIClient, secretName,
				kmmparams.KmmOperatorNamespace, corev1.SecretTypeDockerConfigJson).
				WithData(secretContent).Create()
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

			By("Create KernelMapping")

			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
			kernelMapping.WithContainerImage(plainImage).
				WithBuildArg("MY_MODULE", buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name).
				WithBuildImageRegistryTLS(true, true).
				RegistryTLS(true, true)
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

			By("Create ManagedClusterModule with imageRebuildTriggerGeneration=1")

			selector := map[string]string{"name": ModulesConfig.SpokeClusterName}
			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, moduleName,
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(selector).
				WithImageRebuildTriggerGeneration(1).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error creating managedclustermodule")

			By("Await build pod to complete on hub")

			err = await.BuildPodCompleted(APIClient, kmmparams.KmmHubOperatorNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module on hub")

			By("Await driver container deployment on spoke")

			err = await.ModuleDeployment(ModulesConfig.SpokeAPIClient, moduleName,
				kmmparams.KmmOperatorNamespace, 5*time.Minute, GeneralConfig.ControlPlaneLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment on spoke")

			By("Patch MCM with same trigger value (should be no-op)")

			mcmBuilder, err := kmm.PullManagedClusterModule(APIClient, moduleName,
				kmmparams.KmmHubOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling MCM")

			mcmBuilder.WithImageRebuildTriggerGeneration(1)
			_, err = mcmBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "error updating MCM with same trigger value")

			By("Wait 30 seconds for controller to process")

			time.Sleep(30 * time.Second)

			By("Verify no new pull pod on spoke")

			pods, err := pod.List(ModulesConfig.SpokeAPIClient, kmmparams.KmmOperatorNamespace,
				metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "error listing spoke pods")

			for _, nsPod := range pods {
				if strings.Contains(nsPod.Object.Name, "-pull") {
					Fail(fmt.Sprintf("unexpected pull pod found on spoke: %s", nsPod.Object.Name))
				}
			}

			By("Check label is still set on spoke nodes")

			_, err = check.NodeLabel(ModulesConfig.SpokeAPIClient, moduleName,
				kmmparams.KmmOperatorNamespace, GeneralConfig.ControlPlaneLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking label on spoke nodes")
		})
	})
})
