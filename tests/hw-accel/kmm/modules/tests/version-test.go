package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {
	Context("Module", Label("versions"), func() {

		moduleName := "modver"
		temporaryModuleName := "modver2"
		kmodName := "modver"
		serviceAccountName := "modver-manager"
		buildArgValue := fmt.Sprintf("%s.o", kmodName)
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.VersionModuleTestNamespace, kmodName)
		imageV1 := fmt.Sprintf("%s-1.0.0", image)
		imageV2 := fmt.Sprintf("%s-2.0.0", image)
		versionLabel := fmt.Sprintf("kmm.node.kubernetes.io/version-module-loader.%s.%s",
			kmmparams.VersionModuleTestNamespace, moduleName)

		var dockerfileConfigMap *configmap.Builder
		var svcAccount *serviceaccount.Builder
		var firstNode *nodes.Builder

		BeforeAll(func() {
			By("Create Namespace")
			_, err := namespace.NewBuilder(APIClient, kmmparams.VersionModuleTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred())

			configMapContents := define.MultiStageConfigMapContent(kmodName)

			By("Create ConfigMap")
			dockerfileConfigMap, err = configmap.NewBuilder(APIClient, kmodName, kmmparams.VersionModuleTestNamespace).
				WithData(configMapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")
			svcAccount, err = serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.VersionModuleTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating service account")

			By("Create ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

		})

		AfterAll(func() {

			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.VersionModuleTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Await module deletion")
			err = await.ModuleUndeployed(APIClient, kmmparams.VersionModuleTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting pods to be deleted")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.VersionModuleTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Delete Namespace")
			err = namespace.NewBuilder(APIClient, kmmparams.VersionModuleTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Remove existing node label")
			firstNode, err = firstNode.RemoveLabel(versionLabel, "first").Update()
			Expect(err).ToNot(HaveOccurred(), "error removing node label")

			firstNode, err = firstNode.RemoveLabel(versionLabel, "second").Update()
			Expect(err).ToNot(HaveOccurred(), "error removing node label")

			nodesBuilder, err := nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error getting nodes")

			for _, nodeBuilder := range nodesBuilder {
				_, err = nodeBuilder.RemoveLabel(versionLabel, "second").Update()
				Expect(err).ToNot(HaveOccurred(), "error setting node label")
			}
		})

		It("should be able to use a version", reportxml.ID("63112"), func() {

			By("Create KernelMapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(imageV1).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainer.WithVersion("first")
			moduleLoaderContainer.WithModprobeSpec("", "", []string{"myversion=1.0.0"}, nil, nil, nil)
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.VersionModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.VersionModuleTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Make sure the module is not loaded")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.VersionModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).To(HaveOccurred(), "module should not be loaded")

			By("Set version label on the first worker")
			nodesBuilder, err := nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error getting nodes")

			firstNode = nodesBuilder[0]
			firstNode, err = firstNode.WithNewLabel(versionLabel, "first").Update()
			Expect(err).ToNot(HaveOccurred(), "error setting node label")

			By("Check that the module is deployed on just one node")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.VersionModuleTestNamespace, 5*time.Minute,
				firstNode.Object.Labels)
			Expect(err).ToNot(HaveOccurred(), "error while checking module is deployed")
		})

		It("should upgrade from a version to another", reportxml.ID("63111"), func() {

			By("Create a new image by temporary creating a module")

			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")
			kernelMapping.WithContainerImage(imageV2).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer for temporary module")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainer.WithVersion("second")
			moduleLoaderContainer.WithModprobeSpec("", "", []string{"myversion=2.0.0"}, nil, nil, nil)
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create temporary Module")
			tempModule := kmm.NewModuleBuilder(APIClient, temporaryModuleName, kmmparams.VersionModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			tempModule = tempModule.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = tempModule.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.VersionModuleTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Deleting temporary module")
			_, err = tempModule.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")

			By("Update existing module with new version and params")
			module, err := kmm.Pull(APIClient, kmodName, kmmparams.VersionModuleTestNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling existing module")

			module.Definition.Spec.ModuleLoader.Container.Version = "second"
			module.Definition.Spec.ModuleLoader.Container.ContainerImage = imageV2
			module.Definition.Spec.ModuleLoader.Container.Modprobe.Parameters = []string{"myversion=2.0.0.upgraded"}
			_, err = module.Update()
			Expect(err).ToNot(HaveOccurred(), "error update module")

			By("Remove existing node label")
			firstNode, err = firstNode.RemoveLabel(versionLabel, "first").Update()
			Expect(err).ToNot(HaveOccurred(), "error removing node label")

			By("Check that the module is undeployed on just one node")
			err = await.ModuleUndeployed(APIClient, kmmparams.VersionModuleTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking module is undeployed")

			By("Set version label on all workers")
			nodesBuilder, err := nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error getting nodes")

			for _, nodeBuilder := range nodesBuilder {
				_, err = nodeBuilder.WithNewLabel(versionLabel, "second").Update()
				Expect(err).ToNot(HaveOccurred(), "error setting node label")
			}

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check dmesg contains module message")
			err = check.Dmesg(APIClient, "2.0.0.upgraded", time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking dmesg contents")

			By("Waiting 10 seconds for labels to be be properly set")
			time.Sleep(10 * time.Second)

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.VersionModuleTestNamespace, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})
	})
})
