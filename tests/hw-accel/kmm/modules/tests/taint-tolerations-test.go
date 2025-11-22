package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Module", Label("tolerations"), func() {

		var testNamespace *namespace.Builder
		var node *nodes.Builder
		var kerMapOne *moduleV1Beta1.KernelMapping
		var svcAccount *serviceaccount.Builder

		moduleName := kmmparams.TolerationModuleTestNamespace
		kmodName := "use-toleration"
		serviceAccountName := "toleration-manager"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.TolerationModuleTestNamespace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		var taintNoSchedule []corev1.Taint
		taintNoSchedule = append(taintNoSchedule, corev1.Taint{
			Key:    kmmparams.TolerationNoScheduleKeyValue.Key,
			Value:  kmmparams.TolerationNoScheduleKeyValue.Value,
			Effect: kmmparams.TolerationNoScheduleKeyValue.Effect,
		})

		var taintNoExecute []corev1.Taint
		taintNoExecute = append(taintNoExecute, corev1.Taint{
			Key:    kmmparams.TolerationNoExecuteKeyValue.Key,
			Value:  kmmparams.TolerationNoExecuteKeyValue.Value,
			Effect: kmmparams.TolerationNoExecuteKeyValue.Effect,
		})

		var taintsWhileUpgrade []corev1.Taint
		taintsWhileUpgrade = append(taintsWhileUpgrade, corev1.Taint{
			Key:    kmmparams.TolerationNoExecuteK8sUnreachable.Key,
			Value:  kmmparams.TolerationNoExecuteK8sUnreachable.Value,
			Effect: kmmparams.TolerationNoExecuteK8sUnreachable.Effect,
		})

		taintsWhileUpgrade = append(taintsWhileUpgrade, corev1.Taint{
			Key:    kmmparams.TolerationNoScheduleK8sUnschedulable.Key,
			Value:  kmmparams.TolerationNoScheduleK8sUnschedulable.Value,
			Effect: kmmparams.TolerationNoScheduleK8sUnschedulable.Effect,
		})

		taintsWhileUpgrade = append(taintsWhileUpgrade, corev1.Taint{
			Key:    kmmparams.TolerationNoScheduleK8sUnreachable.Key,
			Value:  kmmparams.TolerationNoScheduleK8sUnreachable.Value,
			Effect: kmmparams.TolerationNoScheduleK8sUnreachable.Effect,
		})

		taintsWhileUpgrade = append(taintsWhileUpgrade, corev1.Taint{
			Key:    kmmparams.TolerationNoScheduleK8sDiskPressure.Key,
			Value:  kmmparams.TolerationNoScheduleK8sDiskPressure.Value,
			Effect: kmmparams.TolerationNoScheduleK8sDiskPressure.Effect,
		})

		BeforeAll(func() {
			By("Create Namespace")
			var err error
			testNamespace, err = namespace.NewBuilder(APIClient, kmmparams.TolerationModuleTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Checking nodes are not tainted already")
			nodeList, err := nodes.List(
				APIClient, metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error getting nodes")

			for _, node := range nodeList {
				if node.Object.Spec.Taints != nil {
					klog.V(kmmparams.KmmLogLevel).Infof("Node %s already tainted with %v o. Skipping test",
						node.Object.Name, node.Object.Spec.Taints)
					Skip(fmt.Sprintf("Node %s already tainted with %v. Skipping test",
						node.Object.Name, node.Object.Spec.Taints))
				}
			}

			By("Create ConfigMap")
			configmapContents := define.MultiStageConfigMapContent(kmodName)
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")
			svcAccount, err = serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.TolerationModuleTestNamespace).Create()
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
			kerMapOne, err = kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")
		})

		AfterEach(func() {
			By("Remove node taint")
			if node != nil {
				node.Definition.Spec.Taints = []corev1.Taint{}
				_, err := node.Update()
				Expect(err).ToNot(HaveOccurred(), "error while removing node taint")
			}

			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")
		})

		AfterAll(func() {
			By("Remove node taint")
			if node != nil {
				node.Definition.Spec.Taints = []corev1.Taint{}
				_, err := node.Update()
				Expect(err).ToNot(HaveOccurred(), "error while removing node taint")
			}

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, kmmparams.TolerationModuleTestNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			err := crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")

			By("Delete Namespace")
			err = namespace.NewBuilder(APIClient, kmmparams.TolerationModuleTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

		})

		It("should deploy module with NoSchedule toleration", reportxml.ID("79205"), func() {
			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name).
				WithToleration(
					kmmparams.TolerationNoScheduleKeyValue.Key,
					string(kmmparams.TolerationNoScheduleKeyValue.Operator),
					kmmparams.TolerationNoScheduleKeyValue.Value,
					string(kmmparams.TolerationNoScheduleKeyValue.Effect), nil)

			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.TolerationModuleTestNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Taint node with test taint")
			nodeList, err := nodes.List(
				APIClient, metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})

			if err != nil {
				Skip(fmt.Sprintf("Error listing worker nodes. Got error: '%v'", err))
			}

			node = nodeList[0]
			node.Definition.Spec.Taints = taintNoSchedule
			_, err = node.Update()
			Expect(err).ToNot(HaveOccurred(), "error while tainting node")

			By("Check node is still loaded on node")
			time.Sleep(30 * time.Second)
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should deploy module with NoExecute toleration", reportxml.ID("79206"), func() {
			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name).
				WithToleration(
					kmmparams.TolerationNoExecuteKeyValue.Key,
					string(kmmparams.TolerationNoExecuteKeyValue.Operator),
					kmmparams.TolerationNoExecuteKeyValue.Value,
					string(kmmparams.TolerationNoExecuteKeyValue.Effect), nil)

			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Taint node with test taint")
			node.Definition.Spec.Taints = taintNoExecute
			_, err = node.Update()
			Expect(err).ToNot(HaveOccurred(), "error while tainting node")

			By("Check node is still loaded on node")
			time.Sleep(30 * time.Second)
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should deploy module with device plugin and NoSchedule toleration", reportxml.ID("79997"), func() {
			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create DevicePlugin")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			if ModulesConfig.DevicePluginImage == "" {
				Skip("ECO_HWACCEL_KMM_DEVICE_PLUGIN_IMAGE not configured. Skipping test.")
			}

			devicePluginImage := fmt.Sprintf(ModulesConfig.DevicePluginImage, arch)

			devicePlugin := kmm.NewDevicePluginContainerBuilder(devicePluginImage)
			devicePluginContainerCfd, err := devicePlugin.GetDevicePluginContainerConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating deviceplugincontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithDevicePluginContainer(devicePluginContainerCfd).
				WithDevicePluginServiceAccount(svcAccount.Object.Name)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name).
				WithToleration(
					kmmparams.TolerationNoScheduleKeyValue.Key,
					string(kmmparams.TolerationNoScheduleKeyValue.Operator),
					kmmparams.TolerationNoScheduleKeyValue.Value,
					string(kmmparams.TolerationNoScheduleKeyValue.Effect), nil)

			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Await device driver deployment")
			err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Taint node with test taint")
			node.Definition.Spec.Taints = taintNoSchedule
			_, err = node.Update()
			Expect(err).ToNot(HaveOccurred(), "error while tainting node")

			By("Check node is still loaded on node")
			time.Sleep(30 * time.Second)
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should deploy module with device plugin and NoExecute toleration", reportxml.ID("79208"), func() {
			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create DevicePlugin")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			if ModulesConfig.DevicePluginImage == "" {
				Skip("ECO_HWACCEL_KMM_DEVICE_PLUGIN_IMAGE not configured. Skipping test.")
			}

			devicePluginImage := fmt.Sprintf(ModulesConfig.DevicePluginImage, arch)

			devicePlugin := kmm.NewDevicePluginContainerBuilder(devicePluginImage)
			devicePluginContainerCfd, err := devicePlugin.GetDevicePluginContainerConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating deviceplugincontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithDevicePluginContainer(devicePluginContainerCfd).
				WithDevicePluginServiceAccount(svcAccount.Object.Name)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name).
				WithToleration(
					kmmparams.TolerationNoExecuteKeyValue.Key,
					string(kmmparams.TolerationNoExecuteKeyValue.Operator),
					kmmparams.TolerationNoExecuteKeyValue.Value,
					string(kmmparams.TolerationNoExecuteKeyValue.Effect), nil)

			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Await device driver deployment")
			err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Taint node with test taint")
			node.Definition.Spec.Taints = taintNoExecute
			_, err = node.Update()
			Expect(err).ToNot(HaveOccurred(), "error while tainting node")

			By("Check node is still loaded on node")
			time.Sleep(30 * time.Second)
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})

		It("should handle taints used by cluster upgrade", reportxml.ID("79207"), func() {
			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name).
				WithToleration(
					kmmparams.TolerationNoExecuteK8sUnreachable.Key,
					string(kmmparams.TolerationNoExecuteK8sUnreachable.Operator),
					kmmparams.TolerationNoExecuteK8sUnreachable.Value,
					string(kmmparams.TolerationNoExecuteK8sUnreachable.Effect), nil)

			module = module.WithToleration(
				kmmparams.TolerationNoScheduleK8sUnreachable.Key,
				string(kmmparams.TolerationNoScheduleK8sUnreachable.Operator),
				kmmparams.TolerationNoScheduleK8sUnreachable.Value,
				string(kmmparams.TolerationNoScheduleK8sUnreachable.Effect), nil)

			module = module.WithToleration(
				kmmparams.TolerationNoScheduleK8sUnschedulable.Key,
				string(kmmparams.TolerationNoScheduleK8sUnschedulable.Operator),
				kmmparams.TolerationNoScheduleK8sUnschedulable.Value,
				string(kmmparams.TolerationNoScheduleK8sUnschedulable.Effect), nil)

			module = module.WithToleration(
				kmmparams.TolerationNoScheduleK8sDiskPressure.Key,
				string(kmmparams.TolerationNoScheduleK8sDiskPressure.Operator),
				kmmparams.TolerationNoScheduleK8sDiskPressure.Value,
				string(kmmparams.TolerationNoScheduleK8sDiskPressure.Effect), nil)

			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace, time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Taint node with upgrade taints")
			node.Definition.Spec.Taints = taintsWhileUpgrade
			_, err = node.Update()
			Expect(err).ToNot(HaveOccurred(), "error while tainting node")

			By("Check node is still loaded on node")
			time.Sleep(30 * time.Second)
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.TolerationModuleTestNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")
		})
	})
})
