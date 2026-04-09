package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
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
	Context("DevicePlugin AutomountServiceAccountToken", Label("automount-satoken"), func() {
		var (
			moduleName         = kmmparams.AutomountSATokenTestNamespace
			kmodName           = "automountsa"
			serviceAccountName = "automountsa-manager"
			image              = fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
				tsparams.LocalImageRegistry, kmmparams.AutomountSATokenTestNamespace, kmodName)
			buildArgValue = fmt.Sprintf("%s.o", kmodName)

			testNamespace            *namespace.Builder
			dockerfileConfigMap      *configmap.Builder
			svcAccount               *serviceaccount.Builder
			crb                      rbac.ClusterRoleBindingBuilder
			moduleLoaderContainerCfg *v1beta1.ModuleLoaderContainerSpec
			devicePluginContainerCfg *v1beta1.DevicePluginContainerSpec
		)

		BeforeEach(func() {
			var err error

			By("Create Namespace")

			testNamespace, err = namespace.NewBuilder(APIClient, kmmparams.AutomountSATokenTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			By("Create ServiceAccount")

			svcAccount, err = serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.AutomountSATokenTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")

			crbBuilder := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crbBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			crb = crbBuilder

			configmapContents := define.MultiStageConfigMapContent(kmodName)

			By("Create ConfigMap")

			dockerfileConfigMap, err = configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

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
			moduleLoaderContainerCfg, err = moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create DevicePlugin container config")

			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			if ModulesConfig.DevicePluginImage == "" {
				Skip("ECO_HWACCEL_KMM_DEVICE_PLUGIN_IMAGE not configured. Skipping test.")
			}

			devicePluginImage := fmt.Sprintf(ModulesConfig.DevicePluginImage, arch)

			devicePlugin := kmm.NewDevicePluginContainerBuilder(devicePluginImage)
			devicePluginContainerCfg, err = devicePlugin.GetDevicePluginContainerConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating deviceplugincontainer")
		})

		AfterEach(func() {
			By("Delete Module")

			_, err := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting module")

			By("Await module to be deleted")

			err = await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Delete ConfigMap")

			err = dockerfileConfigMap.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting configmap")

			By("Delete ServiceAccount")

			err = svcAccount.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting serviceaccount")

			By("Delete ClusterRoleBinding")

			err = crb.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting clusterrolebinding")

			By("Delete Namespace")

			err = testNamespace.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")

			By("Wait for namespace to be fully deleted")
			Eventually(func() bool {
				_, err := namespace.Pull(APIClient, kmmparams.AutomountSATokenTestNamespace)

				return err != nil
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "namespace was not deleted in time")
		})

		It("should not mount SA token when automountServiceAccountToken is false",
			reportxml.ID("automount-satoken-false"), func() {
				By("Create Module with automountServiceAccountToken=false")

				module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(svcAccount.Object.Name)
				module = module.WithDevicePluginContainer(devicePluginContainerCfg).
					WithDevicePluginServiceAccount(svcAccount.Object.Name).
					WithDevicePluginAutomountServiceAccountToken(false)
				_, err := module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete build")

				err = await.BuildPodCompleted(APIClient, kmmparams.AutomountSATokenTestNamespace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Await device driver deployment")

				err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

				By("Verify device plugin pods have automountServiceAccountToken=false")

				devicePluginPods, err := get.DevicePluginPods(APIClient, moduleName,
					kmmparams.AutomountSATokenTestNamespace)
				Expect(err).ToNot(HaveOccurred(), "error getting device plugin pods")
				Expect(devicePluginPods).ToNot(BeEmpty(), "no device plugin pods found")

				for _, dpPod := range devicePluginPods {
					Expect(dpPod.Object.Spec.AutomountServiceAccountToken).ToNot(BeNil(),
						"automountServiceAccountToken should be set on pod %s", dpPod.Object.Name)
					Expect(*dpPod.Object.Spec.AutomountServiceAccountToken).To(BeFalse(),
						"automountServiceAccountToken should be false on pod %s", dpPod.Object.Name)
				}

				By("Verify no projected SA volume is mounted")

				for _, dpPod := range devicePluginPods {
					for _, vol := range dpPod.Object.Spec.Volumes {
						if strings.HasPrefix(vol.Name, "kube-api-access-") {
							Fail(fmt.Sprintf("unexpected projected SA volume %s found on pod %s",
								vol.Name, dpPod.Object.Name))
						}
					}
				}

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check label is set on all nodes")

				_, err = check.NodeLabel(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while checking node labels")
			})

		It("should mount SA token when automountServiceAccountToken is true",
			reportxml.ID("automount-satoken-true"), func() {
				By("Create Module with automountServiceAccountToken=true")

				module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(svcAccount.Object.Name)
				module = module.WithDevicePluginContainer(devicePluginContainerCfg).
					WithDevicePluginServiceAccount(svcAccount.Object.Name).
					WithDevicePluginAutomountServiceAccountToken(true)
				_, err := module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete build")

				err = await.BuildPodCompleted(APIClient, kmmparams.AutomountSATokenTestNamespace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Await device driver deployment")

				err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

				By("Verify device plugin pods have automountServiceAccountToken=true")

				devicePluginPods, err := get.DevicePluginPods(APIClient, moduleName,
					kmmparams.AutomountSATokenTestNamespace)
				Expect(err).ToNot(HaveOccurred(), "error getting device plugin pods")
				Expect(devicePluginPods).ToNot(BeEmpty(), "no device plugin pods found")

				for _, dpPod := range devicePluginPods {
					Expect(dpPod.Object.Spec.AutomountServiceAccountToken).ToNot(BeNil(),
						"automountServiceAccountToken should be set on pod %s", dpPod.Object.Name)
					Expect(*dpPod.Object.Spec.AutomountServiceAccountToken).To(BeTrue(),
						"automountServiceAccountToken should be true on pod %s", dpPod.Object.Name)
				}

				By("Verify projected SA volume is mounted")

				for _, dpPod := range devicePluginPods {
					hasProjectedVolume := false

					for _, vol := range dpPod.Object.Spec.Volumes {
						if strings.HasPrefix(vol.Name, "kube-api-access-") && vol.Projected != nil {
							hasProjectedVolume = true

							break
						}
					}

					Expect(hasProjectedVolume).To(BeTrue(),
						"expected projected SA volume on pod %s", dpPod.Object.Name)
				}

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check label is set on all nodes")

				_, err = check.NodeLabel(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while checking node labels")
			})

		It("should preserve default SA mount behavior when automountServiceAccountToken is unset",
			reportxml.ID("automount-satoken-default"), func() {
				By("Create Module without setting automountServiceAccountToken")

				module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(svcAccount.Object.Name)
				module = module.WithDevicePluginContainer(devicePluginContainerCfg).
					WithDevicePluginServiceAccount(svcAccount.Object.Name)
				_, err := module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete build")

				err = await.BuildPodCompleted(APIClient, kmmparams.AutomountSATokenTestNamespace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Await device driver deployment")

				err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

				By("Verify automountServiceAccountToken is not explicitly set on device plugin pods")

				devicePluginPods, err := get.DevicePluginPods(APIClient, moduleName,
					kmmparams.AutomountSATokenTestNamespace)
				Expect(err).ToNot(HaveOccurred(), "error getting device plugin pods")
				Expect(devicePluginPods).ToNot(BeEmpty(), "no device plugin pods found")

				for _, dpPod := range devicePluginPods {
					Expect(dpPod.Object.Spec.AutomountServiceAccountToken).To(BeNil(),
						"automountServiceAccountToken should not be set on pod %s", dpPod.Object.Name)
				}

				By("Verify projected SA volume is mounted (default K8s behavior)")

				for _, dpPod := range devicePluginPods {
					hasProjectedVolume := false

					for _, vol := range dpPod.Object.Spec.Volumes {
						if strings.HasPrefix(vol.Name, "kube-api-access-") && vol.Projected != nil {
							hasProjectedVolume = true

							break
						}
					}

					Expect(hasProjectedVolume).To(BeTrue(),
						"expected projected SA volume on pod %s (default behavior)", dpPod.Object.Name)
				}

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check label is set on all nodes")

				_, err = check.NodeLabel(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while checking node labels")
			})

		It("should allow custom volume at SA path when automountServiceAccountToken is false",
			reportxml.ID("automount-satoken-custom-vol"), func() {
				customSAConfigMapName := "custom-sa-data"

				By("Create custom SA data ConfigMap")

				customSAData := map[string]string{
					"ca.crt":    "-----BEGIN CERTIFICATE-----\nCUSTOM-CA-CERTIFICATE-DATA-FOR-TESTING\n-----END CERTIFICATE-----\n",
					"token":     "custom-token-value-for-testing",
					"namespace": kmmparams.AutomountSATokenTestNamespace,
				}

				_, err := configmap.NewBuilder(APIClient, customSAConfigMapName, kmmparams.AutomountSATokenTestNamespace).
					WithData(customSAData).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating custom SA configmap")

				By("Create DevicePlugin container with custom volume mount")

				arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {
					Skip("could not detect cluster architecture")
				}

				devicePluginImage := fmt.Sprintf(ModulesConfig.DevicePluginImage, arch)

				devicePluginWithMount := kmm.NewDevicePluginContainerBuilder(devicePluginImage)
				devicePluginWithMount.WithVolumeMount("/var/run/secrets/kubernetes.io/serviceaccount", "custom-sa")
				customDPCfg, err := devicePluginWithMount.GetDevicePluginContainerConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating deviceplugincontainer with volume mount")

				By("Create Module with automountServiceAccountToken=false and custom volume")

				module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(svcAccount.Object.Name)
				module = module.WithDevicePluginContainer(customDPCfg).
					WithDevicePluginServiceAccount(svcAccount.Object.Name).
					WithDevicePluginAutomountServiceAccountToken(false).
					WithDevicePluginVolume("custom-sa", customSAConfigMapName)
				_, err = module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete build")

				err = await.BuildPodCompleted(APIClient, kmmparams.AutomountSATokenTestNamespace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Await device driver deployment")

				err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

				By("Verify device plugin pods are running without volume conflicts")

				devicePluginPods, err := get.DevicePluginPods(APIClient, moduleName,
					kmmparams.AutomountSATokenTestNamespace)
				Expect(err).ToNot(HaveOccurred(), "error getting device plugin pods")
				Expect(devicePluginPods).ToNot(BeEmpty(), "no device plugin pods found")

				By("Verify automountServiceAccountToken is false")

				for _, dpPod := range devicePluginPods {
					Expect(dpPod.Object.Spec.AutomountServiceAccountToken).ToNot(BeNil(),
						"automountServiceAccountToken should be set on pod %s", dpPod.Object.Name)
					Expect(*dpPod.Object.Spec.AutomountServiceAccountToken).To(BeFalse(),
						"automountServiceAccountToken should be false on pod %s", dpPod.Object.Name)
				}

				By("Verify custom volume is mounted at the SA path with ConfigMap contents")

				for _, dpPod := range devicePluginPods {
					hasCustomVolume := false

					for _, vol := range dpPod.Object.Spec.Volumes {
						if vol.Name == "custom-sa" && vol.ConfigMap != nil &&
							vol.ConfigMap.Name == customSAConfigMapName {
							hasCustomVolume = true

							break
						}
					}

					Expect(hasCustomVolume).To(BeTrue(),
						"expected custom-sa volume on pod %s", dpPod.Object.Name)
				}

				By("Verify custom token content is accessible in the pod")

				dpPod := devicePluginPods[0]
				buff, err := dpPod.ExecCommand(
					[]string{"cat", "/var/run/secrets/kubernetes.io/serviceaccount/token"}, "device-plugin")
				Expect(err).ToNot(HaveOccurred(), "error reading custom token from pod")
				Expect(buff.String()).To(Equal("custom-token-value-for-testing"),
					"custom token content does not match expected value")

				By("Check module is loaded on node")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

				By("Check label is set on all nodes")

				_, err = check.NodeLabel(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while checking node labels")
			})

		It("should shadow projected SA token when custom volume is mounted at SA path without disabling automount",
			reportxml.ID("automount-satoken-conflict"), func() {
				customSAConfigMapName := "custom-sa-data"

				By("Create custom SA data ConfigMap")

				customSAData := map[string]string{
					"ca.crt":    "-----BEGIN CERTIFICATE-----\nCUSTOM-CA-CERTIFICATE-DATA-FOR-TESTING\n-----END CERTIFICATE-----\n",
					"token":     "custom-token-value-for-testing",
					"namespace": kmmparams.AutomountSATokenTestNamespace,
				}

				_, err := configmap.NewBuilder(APIClient, customSAConfigMapName, kmmparams.AutomountSATokenTestNamespace).
					WithData(customSAData).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating custom SA configmap")

				By("Create DevicePlugin container with custom volume mount at SA path")

				arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {
					Skip("could not detect cluster architecture")
				}

				devicePluginImage := fmt.Sprintf(ModulesConfig.DevicePluginImage, arch)

				devicePluginWithMount := kmm.NewDevicePluginContainerBuilder(devicePluginImage)
				devicePluginWithMount.WithVolumeMount("/var/run/secrets/kubernetes.io/serviceaccount", "custom-sa")
				customDPCfg, err := devicePluginWithMount.GetDevicePluginContainerConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating deviceplugincontainer with volume mount")

				By("Create Module WITHOUT setting automountServiceAccountToken but with custom volume at SA path")

				module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap)
				module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
					WithLoadServiceAccount(svcAccount.Object.Name)
				module = module.WithDevicePluginContainer(customDPCfg).
					WithDevicePluginServiceAccount(svcAccount.Object.Name).
					WithDevicePluginVolume("custom-sa", customSAConfigMapName)
				_, err = module.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete build")

				err = await.BuildPodCompleted(APIClient, kmmparams.AutomountSATokenTestNamespace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await driver container deployment")

				err = await.ModuleDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

				By("Await device driver deployment")

				err = await.DeviceDriverDeployment(APIClient, moduleName, kmmparams.AutomountSATokenTestNamespace,
					time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on device plugin deployment")

				By("Verify the custom volume shadows the projected SA token at the mount path")

				devicePluginPods, err := get.DevicePluginPods(APIClient, moduleName,
					kmmparams.AutomountSATokenTestNamespace)
				Expect(err).ToNot(HaveOccurred(), "error getting device plugin pods")
				Expect(devicePluginPods).ToNot(BeEmpty(), "no device plugin pods found")

				dpPod := devicePluginPods[0]
				buff, err := dpPod.ExecCommand(
					[]string{"cat", "/var/run/secrets/kubernetes.io/serviceaccount/token"}, "device-plugin")
				Expect(err).ToNot(HaveOccurred(), "error reading token from pod")

				Expect(buff.String()).To(Equal("custom-token-value-for-testing"),
					"custom volume should be served at the SA mount path when automount is not explicitly disabled")
			})
	})
})
