package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {
	Context("DRA Lifecycle", Label("dra", "dra-lifecycle"), func() {
		BeforeEach(func() {
			if kmmparams.DRADriverImage == "" {
				Skip("ECO_HWACCEL_KMM_DRA_DRIVER_IMAGE_REPO is not set")
			}
		})

		Context("Happy Path", Label("dra-happy-path"), func() {
			nSpace := kmmparams.DRAHappyPathTestNamespace
			moduleName := "dra-test-module"
			kmodName := "dramod"
			serviceAccountName := "dra-manager"
			image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
				tsparams.LocalImageRegistry, nSpace, kmodName)
			buildArgValue := fmt.Sprintf("%s.o", kmodName)

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

				By("Create ServiceAccount")

				svcAccount, err := serviceaccount.
					NewBuilder(APIClient, serviceAccountName, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

				By("Create ClusterRoleBinding")

				crbBuilder := define.ModuleCRB(*svcAccount, kmodName)
				_, err = crbBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

				By("Create ConfigMap")

				configmapContents := define.MultiStageConfigMapContent(kmodName)
				_, err = configmap.NewBuilder(APIClient, kmodName, nSpace).
					WithData(configmapContents).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating configmap")

				By("Create Module with moduleLoader and DRA")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
					"selector":     GeneralConfig.WorkerLabelMap,
					"moduleLoader": define.ModuleLoaderSpec(kmodName, image, buildArgValue, serviceAccountName),
					"dra":          define.DRASpec(serviceAccountName, []string{kmmparams.DRADeviceClassName}, nil),
				})

				err = APIClient.Create(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nSpace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await module deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nSpace,
					2*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on module deployment")

				By("Await DRA deployment")

				err = await.DRADeployment(APIClient, moduleName, nSpace,
					3*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on DRA deployment")
			})

			AfterAll(func() {
				By("Cleanup modules and CRB")

				err := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(err).ToNot(HaveOccurred(), "error cleaning up modules")

				crbName := fmt.Sprintf("%s-module-manager-rolebinding", kmodName)
				_ = APIClient.K8sClient.RbacV1().ClusterRoleBindings().Delete(
					context.TODO(), crbName, metav1.DeleteOptions{})

				By("Delete Namespace")

				err = namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
			})

			It("should deploy DRA DaemonSet, DeviceClass, and set node labels",
				reportxml.ID("89695"), func() {
					By("Verify kernel module is loaded")

					err := check.ModuleLoaded(APIClient, kmodName, time.Minute)
					Expect(err).ToNot(HaveOccurred(), "kernel module should be loaded")

					By("Verify module node label is set")

					_, err = check.NodeLabel(APIClient, moduleName, nSpace,
						GeneralConfig.WorkerLabelMap)
					Expect(err).ToNot(HaveOccurred(), "module node label should be set")

					By("Verify DRA node label is set")

					draLabelOK, err := check.DRANodeLabel(APIClient, moduleName, nSpace,
						GeneralConfig.WorkerLabelMap)
					Expect(err).ToNot(HaveOccurred(), "error checking DRA node label")
					Expect(draLabelOK).To(BeTrue(), "DRA node label should be set")

					By("Verify DRA DaemonSet exists and is running")

					daemonSet, err := get.DRADaemonSet(APIClient, moduleName, nSpace)
					Expect(err).ToNot(HaveOccurred(), "DRA DaemonSet should exist")
					Expect(daemonSet.Status.DesiredNumberScheduled).To(
						BeNumerically(">", 0), "DRA DaemonSet should have desired pods")
					Expect(daemonSet.Status.NumberAvailable).To(
						Equal(daemonSet.Status.DesiredNumberScheduled),
						"all DRA DaemonSet pods should be available")

					By("Verify DeviceClass is created with ownership labels")

					deviceClass, err := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), kmmparams.DRADeviceClassName, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass %s should exist", kmmparams.DRADeviceClassName)
					Expect(deviceClass.Labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.name", moduleName))
					Expect(deviceClass.Labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.namespace", nSpace))

					By("Verify Module status.dra fields")

					availNum, desiredNum, found, err := check.DRAModuleStatus(
						APIClient, moduleName, nSpace)
					Expect(err).ToNot(HaveOccurred(), "error reading module status")
					Expect(found).To(BeTrue(), "status.dra should exist")
					Expect(availNum).To(BeNumerically(">", 0),
						"status.dra.availableNumber should be > 0")
					Expect(desiredNum).To(BeNumerically(">", 0),
						"status.dra.desiredNumber should be > 0")
					Expect(availNum).To(Equal(desiredNum),
						"status.dra available and desired should match")
				})
		})

		Context("Remove DRA", Label("dra-remove"), func() {
			nSpace := kmmparams.DRARemoveTestNamespace
			moduleName := "dra-remove-test"
			kmodName := "rmmod"
			serviceAccountName := "remove-manager"
			image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
				tsparams.LocalImageRegistry, nSpace, kmodName)
			buildArgValue := fmt.Sprintf("%s.o", kmodName)
			deviceClassName := "test-remove-class"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

				By("Create ServiceAccount")

				svcAccount, err := serviceaccount.
					NewBuilder(APIClient, serviceAccountName, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

				By("Create ClusterRoleBinding")

				crbBuilder := define.ModuleCRB(*svcAccount, kmodName)
				_, err = crbBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

				By("Create ConfigMap")

				configmapContents := define.MultiStageConfigMapContent(kmodName)
				_, err = configmap.NewBuilder(APIClient, kmodName, nSpace).
					WithData(configmapContents).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating configmap")

				By("Create Module with moduleLoader and DRA")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
					"selector":     GeneralConfig.WorkerLabelMap,
					"moduleLoader": define.ModuleLoaderSpec(kmodName, image, buildArgValue, serviceAccountName),
					"dra":          define.DRASpec(serviceAccountName, []string{deviceClassName}, nil),
				})

				err = APIClient.Create(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nSpace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Await module deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nSpace,
					2*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on module deployment")

				By("Await DRA deployment")

				err = await.DRADeployment(APIClient, moduleName, nSpace,
					3*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on DRA deployment")
			})

			AfterAll(func() {
				By("Cleanup modules and CRB")

				err := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(err).ToNot(HaveOccurred(), "error cleaning up modules")

				crbName := fmt.Sprintf("%s-module-manager-rolebinding", kmodName)
				_ = APIClient.K8sClient.RbacV1().ClusterRoleBindings().Delete(
					context.TODO(), crbName, metav1.DeleteOptions{})

				By("Delete Namespace")

				err = namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
			})

			It("should clean up DRA resources when spec.dra is removed",
				reportxml.ID("89704"), func() {
					By("Verify DRA DaemonSet is running before removal")

					_, err := get.DRADaemonSet(APIClient, moduleName, nSpace)
					Expect(err).ToNot(HaveOccurred(),
						"DRA DaemonSet should exist before removal")

					By("Verify DeviceClass exists before removal")

					_, err = APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), deviceClassName, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass should exist before removal")

					By("Remove spec.dra from Module")

					patch := []map[string]interface{}{
						{"op": "remove", "path": "/spec/dra"},
					}

					patchBytes, err := json.Marshal(patch)
					Expect(err).ToNot(HaveOccurred(), "error marshaling patch")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{})
					err = APIClient.Patch(context.TODO(), module,
						runtimeClient.RawPatch(types.JSONPatchType, patchBytes))
					Expect(err).ToNot(HaveOccurred(), "error patching module to remove DRA")

					By("Verify DRA DaemonSet is deleted")

					err = await.DRADaemonSetGone(APIClient, nSpace, 2*time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						"DRA DaemonSet should be deleted after removing spec.dra")

					By("Verify DeviceClass is deleted")

					err = await.DeviceClassDeleted(APIClient, deviceClassName, time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass should be deleted after removing spec.dra")

					By("Verify DRA node label is removed")

					Eventually(func() (bool, error) {
						return check.NoDRANodeLabel(APIClient, moduleName, nSpace,
							GeneralConfig.WorkerLabelMap)
					}, time.Minute, 5*time.Second).Should(BeTrue(),
						"DRA node label should be removed after removing spec.dra")

					By("Verify status.dra is cleared")

					Eventually(func() bool {
						availNum, desiredNum, found, getErr := check.DRAModuleStatus(
							APIClient, moduleName, nSpace)
						if getErr != nil {
							return false
						}

						return !found || (availNum == 0 && desiredNum == 0)
					}, time.Minute, 5*time.Second).Should(BeTrue(),
						"status.dra should be cleared after removing spec.dra")

					By("Verify module loader is still running")

					_, err = check.NodeLabel(APIClient, moduleName, nSpace,
						GeneralConfig.WorkerLabelMap)
					Expect(err).ToNot(HaveOccurred(),
						"module node label should still be set after removing DRA")
				})
		})

		Context("Preset Env and Probe", Label("dra-preset-env"), func() {
			nSpace := kmmparams.DRAPresetEnvTestNamespace
			moduleName := "dra-env-test"
			kmodName := "envmod"
			serviceAccountName := "env-manager"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

				By("Create ServiceAccount")

				svcAccount, err := serviceaccount.
					NewBuilder(APIClient, serviceAccountName, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

				By("Create ClusterRoleBinding")

				crbBuilder := define.ModuleCRB(*svcAccount, kmodName)
				_, err = crbBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

				By("Create Module with DRA and custom env var")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
					"selector": GeneralConfig.WorkerLabelMap,
					"dra": define.DRASpec(serviceAccountName, nil, []map[string]interface{}{
						{"name": "MY_CUSTOM_VAR", "value": "custom-value"},
					}),
				})

				err = APIClient.Create(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error creating module")
			})

			AfterAll(func() {
				By("Cleanup modules and CRB")

				err := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(err).ToNot(HaveOccurred(), "error cleaning up modules")

				crbName := fmt.Sprintf("%s-module-manager-rolebinding", kmodName)
				_ = APIClient.K8sClient.RbacV1().ClusterRoleBindings().Delete(
					context.TODO(), crbName, metav1.DeleteOptions{})

				By("Delete Namespace")

				err = namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
			})

			It("should have preset env vars, GRPC liveness probe, and hostNetwork",
				reportxml.ID("89705"), func() {
					By("Wait for DRA DaemonSet to be created")

					Eventually(func() error {
						_, getErr := get.DRADaemonSet(APIClient, moduleName, nSpace)

						return getErr
					}, 2*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should be created")

					daemonSet, err := get.DRADaemonSet(APIClient, moduleName, nSpace)
					Expect(err).ToNot(HaveOccurred(), "error getting DRA DaemonSet")

					podSpec := daemonSet.Spec.Template.Spec

					By("Verify hostNetwork is true")

					Expect(podSpec.HostNetwork).To(BeTrue(),
						"DRA DaemonSet should have hostNetwork: true")

					By("Verify preset env vars on DRA container")

					draContainer := get.DRAContainer(podSpec)
					Expect(draContainer).ToNot(BeNil(), "DRA container should exist")

					envNames := get.DRAContainerEnvNames(draContainer)
					for _, preset := range kmmparams.DRAPresetEnvNames {
						Expect(envNames).To(ContainElement(preset),
							"preset env var %s should be present", preset)
					}

					Expect(envNames).To(ContainElement("MY_CUSTOM_VAR"),
						"user-defined env var should be present")

					By("Verify user env var comes after preset env vars")

					customIdx := get.DRAContainerEnvIndex(draContainer, "MY_CUSTOM_VAR")
					for _, preset := range kmmparams.DRAPresetEnvNames {
						presetIdx := get.DRAContainerEnvIndex(draContainer, preset)
						Expect(customIdx).To(BeNumerically(">", presetIdx),
							"MY_CUSTOM_VAR should come after preset env var %s", preset)
					}

					By("Verify GRPC liveness probe")

					probe := draContainer.LivenessProbe
					Expect(probe).ToNot(BeNil(), "liveness probe should exist")
					Expect(probe.GRPC).ToNot(BeNil(), "liveness probe should be GRPC")
					Expect(probe.GRPC.Port).To(Equal(int32(51515)),
						"GRPC probe port should be 51515")
				})
		})
	})
})
