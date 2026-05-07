package tests

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/imagestream"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite), func() {
	Context("Module", Label("rebuild-trigger"), func() {
		moduleName := "rt-basic"
		kmodName := "rt-basic"
		serviceAccountName := "rt-basic-manager"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.RebuildTriggerBasicNamespace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		AfterAll(func() {
			By("Delete Module")

			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.RebuildTriggerBasicNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")

			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.RebuildTriggerBasicNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName,
				kmmparams.RebuildTriggerBasicNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")

			crb := define.ModuleCRB(*svcAccount, kmodName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting clusterrolebinding")

			By("Delete Namespace")

			err = namespace.NewBuilder(APIClient, kmmparams.RebuildTriggerBasicNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
		})

		It("should rebuild after trigger when image deleted", reportxml.ID("87950"), func() {
			By("Create Namespace")

			testNamespace, err := namespace.NewBuilder(APIClient, kmmparams.RebuildTriggerBasicNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Create ConfigMap")

			configmapContents := define.MultiStageConfigMapContent(kmodName)
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")

			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.RebuildTriggerBasicNamespace).Create()
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

			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.RebuildTriggerBasicNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")

			err = await.BuildPodCompleted(APIClient, kmmparams.RebuildTriggerBasicNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")

			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.RebuildTriggerBasicNamespace,
				5*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")

			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Record existing build pod names before trigger")

			existingPods, err := pod.List(APIClient, kmmparams.RebuildTriggerBasicNamespace, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "error listing pods")

			oldBuildPods := []string{}

			for _, existingPod := range existingPods {
				if strings.Contains(existingPod.Object.Name, "-build") {
					oldBuildPods = append(oldBuildPods, existingPod.Object.Name)
				}
			}

			By("Delete imagestream to simulate lost image")

			imgStream, err := imagestream.Pull(APIClient, kmodName, kmmparams.RebuildTriggerBasicNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling imagestream")

			err = imgStream.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting imagestream")

			By("Set imageRebuildTriggerGeneration to trigger rebuild")

			moduleBuilder, err := kmm.Pull(APIClient, moduleName, kmmparams.RebuildTriggerBasicNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling module")

			moduleBuilder.WithImageRebuildTriggerGeneration(1)
			_, err = moduleBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "error updating module with trigger")

			By("Await new build pod to complete (excluding old build pods)")

			err = await.NewBuildPodCompleted(APIClient, kmmparams.RebuildTriggerBasicNamespace,
				oldBuildPods, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for rebuild")

			By("Await driver container deployment")

			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.RebuildTriggerBasicNamespace,
				5*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment after rebuild")

			By("Check module is loaded on node after rebuild")

			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded after rebuild")
		})

		It("should only re-verify when trigger set and image exists", reportxml.ID("87958"), func() {
			By("Record existing build pod names")

			existingPods, err := pod.List(APIClient, kmmparams.RebuildTriggerBasicNamespace, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "error listing pods")

			existingBuildPods := []string{}

			for _, existingPod := range existingPods {
				if strings.Contains(existingPod.Object.Name, "-build") {
					existingBuildPods = append(existingBuildPods, existingPod.Object.Name)
				}
			}

			klog.V(kmmparams.KmmLogLevel).Infof("Existing build pods before trigger: %v", existingBuildPods)

			By("Set imageRebuildTriggerGeneration to 2 (image exists, should only verify)")

			moduleBuilder, err := kmm.Pull(APIClient, moduleName, kmmparams.RebuildTriggerBasicNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling module")

			moduleBuilder.WithImageRebuildTriggerGeneration(2)
			_, err = moduleBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "error updating module with trigger")

			By("Wait for controller to process trigger")

			time.Sleep(30 * time.Second)

			By("Verify NO new build pod was created")

			currentPods, err := pod.List(APIClient, kmmparams.RebuildTriggerBasicNamespace, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "error listing pods")

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
						fmt.Sprintf("unexpected new build pod found: %s", currentPod.Object.Name))
				}
			}

			By("Check module is still loaded on node")

			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})
	})

	Context("Module", Label("rebuild-trigger-noop"), func() {
		moduleName := "rt-noop"
		kmodName := "rt-noop"
		serviceAccountName := "rt-noop-manager"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.RebuildTriggerNoopNamespace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		AfterAll(func() {
			By("Delete Module")

			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.RebuildTriggerNoopNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")

			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.RebuildTriggerNoopNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName,
				kmmparams.RebuildTriggerNoopNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")

			crb := define.ModuleCRB(*svcAccount, kmodName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting clusterrolebinding")

			By("Delete Namespace")

			err = namespace.NewBuilder(APIClient, kmmparams.RebuildTriggerNoopNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
		})

		It("same trigger value should cause no action", reportxml.ID("87951"), func() {
			By("Create Namespace")

			testNamespace, err := namespace.NewBuilder(APIClient, kmmparams.RebuildTriggerNoopNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Create ConfigMap")

			configmapContents := define.MultiStageConfigMapContent(kmodName)
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")

			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.RebuildTriggerNoopNamespace).Create()
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

			By("Create Module with imageRebuildTriggerGeneration=1")

			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.RebuildTriggerNoopNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithImageRebuildTriggerGeneration(1)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")

			err = await.BuildPodCompleted(APIClient, kmmparams.RebuildTriggerNoopNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")

			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.RebuildTriggerNoopNamespace,
				5*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")

			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Patch module with same trigger value (should be no-op)")

			moduleBuilder, err := kmm.Pull(APIClient, moduleName, kmmparams.RebuildTriggerNoopNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling module")

			moduleBuilder.WithImageRebuildTriggerGeneration(1)
			_, err = moduleBuilder.Update()
			Expect(err).ToNot(HaveOccurred(), "error updating module with same trigger value")

			By("Wait 30 seconds for controller to process")

			time.Sleep(30 * time.Second)

			By("Verify no new pull pod was created")

			pods, err := pod.List(APIClient, kmmparams.RebuildTriggerNoopNamespace, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "error listing pods")

			for _, nsPod := range pods {
				if strings.Contains(nsPod.Object.Name, "-pull") {
					Fail(fmt.Sprintf("unexpected pull pod found: %s", nsPod.Object.Name))
				}
			}

			By("Check module is still loaded on node")

			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})
	})
})
