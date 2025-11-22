package tests

import (
	"encoding/json"
	"fmt"

	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("KMM", Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	const simpleKmodName = "simple-kmod"

	Context("Module", Ordered, Label("simple-kmod"), func() {

		moduleName := simpleKmodName
		kmodName := simpleKmodName
		localNsName := kmmparams.SimpleKmodModuleTestNamespace
		serviceAccountName := "simple-kmod-manager"
		secretName := "test-build-secret"
		image := fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION-%v",
			ModulesConfig.Registry, moduleName, time.Now().Unix())
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		var module *kmm.ModuleBuilder
		var svcAccount *serviceaccount.Builder
		var originalSecretMap map[string]map[string]interface{}
		var secretMap map[string]map[string]interface{}

		BeforeAll(func() {
			if ModulesConfig.PullSecret == "" || ModulesConfig.Registry == "" {
				Skip("No external registry secret found in environment, Skipping test")
			}

			By("Create Namespace")
			_, err := namespace.NewBuilder(APIClient, localNsName).Create()
			Expect(err).NotTo(HaveOccurred(), "error creating test namespace")

			By("Creating registry secret")
			secretContent := define.SecretContent(ModulesConfig.Registry, ModulesConfig.PullSecret)
			_, err = secret.NewBuilder(APIClient, secretName,
				localNsName, corev1.SecretTypeDockerConfigJson).WithData(secretContent).Create()
			Expect(err).ToNot(HaveOccurred(), "failed creating secret")

			By("Get cluster's global pull-secret")
			globalSecret, err := cluster.GetOCPPullSecret(APIClient)
			Expect(err).ToNot(HaveOccurred(), "error fetching cluster's pull-secret")

			err = json.Unmarshal(globalSecret.Object.Data[".dockerconfigjson"], &secretMap)
			Expect(err).ToNot(HaveOccurred(), "error unmarshal pull-secret")
			err = json.Unmarshal(globalSecret.Object.Data[".dockerconfigjson"], &originalSecretMap)
			Expect(err).ToNot(HaveOccurred(), "error unmarshal pull-secret")

		})
		It("should build and push image to quay", reportxml.ID("53584"), func() {

			By("Create configmap")
			configmapContent := define.SimpleKmodConfigMapContents()

			dockerfileConfigMap, err := configmap.NewBuilder(APIClient, kmodName, localNsName).
				WithData(configmapContent).Create()

			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create service account")
			svcAccount, err = serviceaccount.NewBuilder(APIClient, serviceAccountName, localNsName).Create()

			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create clusterrolebinding")
			crb := define.ModuleCRB(*svcAccount, moduleName)

			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			By("Create kernel mapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create Module LoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(moduleName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create module")
			module = kmm.NewModuleBuilder(APIClient, moduleName, localNsName).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)

			module = module.WithImageRepoSecret(secretName)

			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, localNsName, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, localNsName, 5*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, kmodName, localNsName, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

		})

		It("should delete simple-kmod module", reportxml.ID("53413"), func() {
			By("Deleting the module")
			_, err := module.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting the module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, kmodName, localNsName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Await pods deletion")
			err = await.ModuleUndeployed(APIClient, localNsName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting pods to be deleted")

			By("Check labels are removed on all nodes")
			_, err = check.NodeLabel(APIClient, kmodName, localNsName, GeneralConfig.WorkerLabelMap)
			Expect(err).To(HaveOccurred(), "error while checking the module is loaded")

		})

		It("should generate events on nodes when module is loaded", reportxml.ID("68106"), func() {
			By("Waiting events to be flushed")
			time.Sleep(30 * time.Second)

			By("Getting events from 'default' namespace")
			eventList, err := events.List(APIClient, "default")
			Expect(err).ToNot(HaveOccurred(), "Fail to collect events")

			totalNodes, _ := get.NumberOfNodesForSelector(APIClient, GeneralConfig.WorkerLabelMap)

			foundModuleLoadedEvents := 0
			foundModuleUnloadedEvents := 0

			messageModuleLoaded := get.ModuleLoadedMessage(localNsName, moduleName)
			messageModuleUnloaded := get.ModuleUnloadedMessage(localNsName, moduleName)

			for _, event := range eventList {
				klog.V(kmmparams.KmmLogLevel).Infof("Reason: %s, Message: %s", event.Object.Reason, event.Object.Message)
				if event.Object.Reason == kmmparams.ReasonModuleLoaded &&
					event.Object.Message == messageModuleLoaded {
					foundModuleLoadedEvents++
					klog.V(kmmparams.KmmLogLevel).Infof("ModuleLoaded events: %d", foundModuleLoadedEvents)
				}
				if event.Object.Reason == kmmparams.ReasonModuleUnloaded &&
					event.Object.Message == messageModuleUnloaded {
					foundModuleUnloadedEvents++
					klog.V(kmmparams.KmmLogLevel).Infof("ModuleUnloaded events: %d", foundModuleUnloadedEvents)
				}
			}
			Expect(foundModuleLoadedEvents).To(Equal(totalNodes), "ModuleLoaded events do not match")
			Expect(foundModuleUnloadedEvents).To(Equal(totalNodes), "ModuleUnloaded events do not match")
		})

	})

	// Separate context to isolate flaky event test (68106) in first context.
	// If 68106 fails, these tests will still run. Reuses namespace/secrets.
	Context("Module - Additional Tests", Ordered, Label("simple-kmod"), func() {

		moduleName := simpleKmodName
		kmodName := simpleKmodName
		localNsName := kmmparams.SimpleKmodModuleTestNamespace
		serviceAccountName := "simple-kmod-manager"
		secretName := "test-build-secret"
		image := fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION-%v",
			ModulesConfig.Registry, moduleName, time.Now().Unix())
		imageNotUniq := fmt.Sprintf("%s/%s:$KERNEL_FULL_VERSION",
			ModulesConfig.Registry, moduleName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		var module *kmm.ModuleBuilder
		var svcAccount *serviceaccount.Builder
		var originalSecretMap map[string]map[string]interface{}
		var secretMap map[string]map[string]interface{}

		BeforeAll(func() {
			if ModulesConfig.PullSecret == "" || ModulesConfig.Registry == "" {
				Skip("No external registry secret found in environment, Skipping test")
			}

			By("Get existing service account from Context 1")
			var err error
			svcAccount, err = serviceaccount.Pull(APIClient, serviceAccountName, localNsName)
			Expect(err).ToNot(HaveOccurred(), "service account should exist from Context 1")

			By("Get cluster's global pull-secret")
			globalSecret, err := cluster.GetOCPPullSecret(APIClient)
			Expect(err).ToNot(HaveOccurred(), "error fetching cluster's pull-secret")

			err = json.Unmarshal(globalSecret.Object.Data[".dockerconfigjson"], &secretMap)
			Expect(err).ToNot(HaveOccurred(), "error unmarshal pull-secret")
			err = json.Unmarshal(globalSecret.Object.Data[".dockerconfigjson"], &originalSecretMap)
			Expect(err).ToNot(HaveOccurred(), "error unmarshal pull-secret")

		})

		It("should deploy prebuild image", reportxml.ID("53395"), func() {

			By("Create kernel mapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image)

			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create Module LoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(moduleName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create module")
			module = kmm.NewModuleBuilder(APIClient, moduleName, localNsName).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)

			module = module.WithImageRepoSecret(secretName)

			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, localNsName, 3*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, kmodName, localNsName, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should delete simple-kmod module", reportxml.ID("53413"), func() {
			By("Deleting the module")
			_, err := module.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting the module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, kmodName, localNsName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Await pods deletion")
			err = await.ModuleUndeployed(APIClient, localNsName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting pods to be deleted")

			By("Check labels are removed on all nodes")
			_, err = check.NodeLabel(APIClient, kmodName, localNsName, GeneralConfig.WorkerLabelMap)
			Expect(err).To(HaveOccurred(), "error while checking the module is loaded")

		})

		It("should deploy prebuild image with global secret", reportxml.ID("71694"), func() {

			By("Update global pull-secret")
			if secretMap["auths"][ModulesConfig.Registry] == nil {
				secretMap["auths"][ModulesConfig.Registry] = map[string]string{
					"auth":  ModulesConfig.PullSecret,
					"email": "",
				}

				ps, err := json.Marshal(secretMap)
				Expect(err).ToNot(HaveOccurred(), "error encoding pull secret")
				secretContents := map[string][]byte{".dockerconfigjson": ps}

				pullSecret, _ := secret.Pull(APIClient, "pull-secret", "openshift-config")
				_, err = pullSecret.WithData(secretContents).Update()
				Expect(err).ToNot(HaveOccurred(), "error updating global pull secret")
			}

			By("Create kernel mapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image)

			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create Module LoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(moduleName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create module")
			module = kmm.NewModuleBuilder(APIClient, moduleName, localNsName).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)

			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, localNsName, 3*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, kmodName, localNsName, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should build image without loading it", func() {

			By("Create configmap")
			configmapContent := define.SimpleKmodConfigMapContents()

			dockerfileConfigMap, err := configmap.NewBuilder(APIClient, kmodName, localNsName).
				WithData(configmapContent).Create()

			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create service account")
			svcAccount, err = serviceaccount.NewBuilder(APIClient, serviceAccountName, localNsName).Create()

			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create clusterrolebinding")
			crb := define.ModuleCRB(*svcAccount, moduleName)

			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			By("Create kernel mapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(imageNotUniq).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create Module LoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(moduleName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainer.WithVersion("first")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create module")
			module = kmm.NewModuleBuilder(APIClient, "build", localNsName).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)

			module = module.WithImageRepoSecret(secretName)

			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			_ = await.BuildPodCompleted(APIClient, localNsName, 5*time.Minute)

			_, err = module.Delete()
			Expect(err).ToNot(HaveOccurred(), "error while building module")
		})

		AfterAll(func() {
			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, kmodName, localNsName).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, kmodName, localNsName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, moduleName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting clusterrolebinding")

			By("Delete Namespace")
			err = namespace.NewBuilder(APIClient, moduleName).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")

			By("Restore original global pull-secret")
			if originalSecretMap["auths"][ModulesConfig.Registry] == nil {
				pullSecret, err := secret.Pull(APIClient, "pull-secret", "openshift-config")
				Expect(err).ToNot(HaveOccurred(), "error pulling global pull secret")
				ps, err := json.Marshal(originalSecretMap)
				Expect(err).ToNot(HaveOccurred(), "error encoding pull-secret")

				origSecretContents := map[string][]byte{".dockerconfigjson": ps}
				_, err = pullSecret.WithData(origSecretContents).Update()
				Expect(err).ToNot(HaveOccurred(), "error restoring global pull secret")
			}
		})

	})
})
