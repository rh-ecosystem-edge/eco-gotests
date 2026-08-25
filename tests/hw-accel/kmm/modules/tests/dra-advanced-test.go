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
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {
	Context("DRA Advanced", Label("dra", "dra-advanced"), func() {
		BeforeEach(func() {
			if kmmparams.DRADriverImage == "" {
				Skip("ECO_HWACCEL_KMM_DRA_DRIVER_IMAGE_REPO is not set")
			}
		})

		Context("DeviceClass Lifecycle", Label("dra-deviceclass"), func() {
			nSpace := kmmparams.DRADeviceClassTestNamespace
			moduleName := "dc-lifecycle-test"
			serviceAccountName := "dc-manager"
			deviceClassA := "test-device-class-a"
			deviceClassB := "test-device-class-b"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

				By("Create ServiceAccount")

				svcAccount, err := serviceaccount.
					NewBuilder(APIClient, serviceAccountName, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

				By("Create ClusterRoleBinding")

				crbBuilder := define.ModuleCRB(*svcAccount, moduleName)
				_, err = crbBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

				By("Create Module with DRA and two deviceClasses")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
					"selector": GeneralConfig.WorkerLabelMap,
					"dra": map[string]interface{}{
						"driverName":         kmmparams.DRADriverName,
						"serviceAccountName": serviceAccountName,
						"container": map[string]interface{}{
							"image":   kmmparams.DRADriverImage,
							"command": []interface{}{"dra-example-kubeletplugin"},
							"env": []interface{}{
								map[string]interface{}{
									"name":  "DRIVER_NAME",
									"value": kmmparams.DRADriverName,
								},
							},
						},
						"deviceClasses": []interface{}{
							map[string]interface{}{"name": deviceClassA},
							map[string]interface{}{"name": deviceClassB},
						},
					},
				})

				err = APIClient.Create(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await DRA deployment")

				err = await.DRADeployment(APIClient, moduleName, nSpace,
					3*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error while waiting on DRA deployment")
			})

			AfterAll(func() {
				By("Delete Module")

				cleanupErr := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(cleanupErr).ToNot(HaveOccurred(), "error cleaning up modules")

				By("Await DeviceClass cleanup")

				Eventually(func() bool {
					_, errA := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), deviceClassA, metav1.GetOptions{})
					_, errB := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), deviceClassB, metav1.GetOptions{})
					_, errC := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), "test-device-class-c", metav1.GetOptions{})

					return apierrors.IsNotFound(errA) &&
						apierrors.IsNotFound(errB) &&
						apierrors.IsNotFound(errC)
				}, time.Minute, 5*time.Second).Should(BeTrue(),
					"DeviceClasses should be deleted after module cleanup")

				By("Delete Namespace")

				nsErr := namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(nsErr).ToNot(HaveOccurred(), "error deleting test namespace")
			})

			It("should create both DeviceClasses with ownership labels",
				reportxml.ID("89703"), func() {
					By("Verify DeviceClass A exists with ownership labels")

					dcA, err := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), deviceClassA, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass %s should exist", deviceClassA)
					Expect(dcA.Labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.name", moduleName))
					Expect(dcA.Labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.namespace", nSpace))

					By("Verify DeviceClass B exists with ownership labels")

					dcB, err := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
						context.TODO(), deviceClassB, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass %s should exist", deviceClassB)
					Expect(dcB.Labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.name", moduleName))
					Expect(dcB.Labels).To(HaveKeyWithValue(
						"kmm.node.kubernetes.io/module.namespace", nSpace))
				})

			It("should rename DeviceClass when Module spec changes",
				reportxml.ID("89703"), func() {
					By("Patch Module to rename DeviceClass B to C")

					patch := []map[string]interface{}{
						{
							"op":    "replace",
							"path":  "/spec/dra/deviceClasses/1/name",
							"value": "test-device-class-c",
						},
					}

					patchBytes, err := json.Marshal(patch)
					Expect(err).ToNot(HaveOccurred(), "error marshaling patch")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{})
					err = APIClient.Patch(context.TODO(), module,
						runtimeClient.RawPatch(types.JSONPatchType, patchBytes))
					Expect(err).ToNot(HaveOccurred(), "error patching module")

					By("Verify DeviceClass B is deleted")

					err = await.DeviceClassDeleted(APIClient, deviceClassB, time.Minute)
					Expect(err).ToNot(HaveOccurred(),
						"DeviceClass B should be deleted after rename")

					By("Verify DeviceClass C is created")

					Eventually(func() error {
						dcC, getErr := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
							context.TODO(), "test-device-class-c", metav1.GetOptions{})
						if getErr != nil {
							return getErr
						}

						if dcC.Labels["kmm.node.kubernetes.io/module.name"] != moduleName {
							return fmt.Errorf("DeviceClass C has wrong module.name label")
						}

						return nil
					}, time.Minute, 5*time.Second).Should(Succeed(),
						"DeviceClass C should be created with correct labels")
				})

			It("should recreate externally deleted DeviceClass",
				reportxml.ID("89703"), func() {
					By("Delete DeviceClass A externally")

					err := APIClient.K8sClient.ResourceV1().DeviceClasses().Delete(
						context.TODO(), deviceClassA, metav1.DeleteOptions{})
					Expect(err).ToNot(HaveOccurred(), "error deleting DeviceClass A")

					By("Verify controller recreates DeviceClass A")

					Eventually(func() error {
						dcA, getErr := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
							context.TODO(), deviceClassA, metav1.GetOptions{})
						if getErr != nil {
							return getErr
						}

						if dcA.Labels["kmm.node.kubernetes.io/module.name"] != moduleName {
							return fmt.Errorf("recreated DeviceClass A has wrong module.name label")
						}

						return nil
					}, time.Minute, 5*time.Second).Should(Succeed(),
						"DeviceClass A should be recreated by controller")
				})
		})

		Context("Optional Features", Label("dra-optional"), func() {
			nSpace := kmmparams.DRAOptionalFeaturesTestNamespace
			serviceAccountName := "opt-manager"
			moduleName := "dra-opt-test"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
			})

			AfterAll(func() {
				By("Delete any remaining modules in namespace")

				cleanupErr := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(cleanupErr).ToNot(HaveOccurred(), "error cleaning up modules")

				By("Delete Namespace")

				nsErr := namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(nsErr).ToNot(HaveOccurred(), "error deleting test namespace")
			})

			It("should run DRA init container, mount custom volume, and set automountServiceAccountToken",
				reportxml.ID("89709"), func() {
					By("Create ServiceAccount")

					svcAccount, err := serviceaccount.
						NewBuilder(APIClient, serviceAccountName, nSpace).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

					By("Create ClusterRoleBinding")

					crbBuilder := define.ModuleCRB(*svcAccount, moduleName)
					_, err = crbBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

					By("Create ConfigMap for volume test")

					_, err = configmap.NewBuilder(APIClient, "dra-config", nSpace).
						WithData(map[string]string{"config.yaml": "test: true"}).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating configmap")

					automountFalse := false

					By("Create Module with init container, custom volume, and automountServiceAccountToken: false")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"dra": map[string]interface{}{
							"driverName":                   kmmparams.DRADriverName,
							"serviceAccountName":           serviceAccountName,
							"automountServiceAccountToken": automountFalse,
							"container": map[string]interface{}{
								"image":   kmmparams.DRADriverImage,
								"command": []interface{}{"dra-example-kubeletplugin"},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DRIVER_NAME",
										"value": kmmparams.DRADriverName,
									},
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "custom-config",
										"mountPath": "/etc/dra-config",
										"readOnly":  true,
									},
								},
							},
							"initContainer": map[string]interface{}{
								"image":   "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
								"command": []interface{}{"sh", "-c", "echo init-done"},
							},
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "custom-config",
									"configMap": map[string]interface{}{
										"name": "dra-config",
									},
								},
							},
						},
					})

					err = APIClient.Create(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(), "error creating module")

					By("Wait for DRA DaemonSet")

					var daemonSet *appsv1.DaemonSet

					Eventually(func() error {
						ds, getErr := get.DRADaemonSet(APIClient, moduleName, nSpace)
						daemonSet = ds

						return getErr
					}, 2*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should be created")

					podSpec := daemonSet.Spec.Template.Spec

					By("Verify init container exists")

					Expect(podSpec.InitContainers).ToNot(BeEmpty(),
						"DRA DaemonSet should have init containers")

					initFound := false

					for _, initC := range podSpec.InitContainers {
						if initC.Name == "dra-init" {
							initFound = true

							break
						}
					}

					Expect(initFound).To(BeTrue(),
						"DRA DaemonSet should have dra-init container")

					By("Verify custom configmap volume is mounted")

					customVolumeFound := false

					for _, vol := range podSpec.Volumes {
						if vol.Name == "custom-config" && vol.ConfigMap != nil {
							Expect(vol.ConfigMap.Name).To(Equal("dra-config"),
								"custom-config volume should reference dra-config ConfigMap")

							customVolumeFound = true

							break
						}
					}

					Expect(customVolumeFound).To(BeTrue(),
						"DRA DaemonSet should have custom-config volume")

					By("Verify DRA container has custom-config VolumeMount")

					draContainer := get.DRAContainer(podSpec)
					Expect(draContainer).ToNot(BeNil(),
						"DRA DaemonSet should have a dra container")

					mountFound := false

					for _, vm := range draContainer.VolumeMounts {
						if vm.Name == "custom-config" {
							Expect(vm.MountPath).To(Equal("/etc/dra-config"),
								"custom-config mount path should be /etc/dra-config")
							Expect(vm.ReadOnly).To(BeTrue(),
								"custom-config mount should be read-only")

							mountFound = true

							break
						}
					}

					Expect(mountFound).To(BeTrue(),
						"DRA container should have custom-config VolumeMount")

					By("Verify automountServiceAccountToken is false")

					Expect(podSpec.AutomountServiceAccountToken).ToNot(BeNil(),
						"automountServiceAccountToken should be set")
					Expect(*podSpec.AutomountServiceAccountToken).To(BeFalse(),
						"automountServiceAccountToken should be false")
				})
		})

		Context("Pod Spec Tolerations", Label("dra-podspec"), func() {
			nSpace := kmmparams.DRATolerationTestNamespace
			moduleName := "dra-toleration-test"
			serviceAccountName := "tol-manager"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
			})

			AfterAll(func() {
				cleanupErr := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(cleanupErr).ToNot(HaveOccurred(), "error cleaning up modules")

				nsErr := namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(nsErr).ToNot(HaveOccurred(), "error deleting test namespace")
			})

			It("should propagate tolerations to DRA DaemonSet pods",
				reportxml.ID("89715"), func() {
					By("Create ServiceAccount")

					svcAccount, err := serviceaccount.
						NewBuilder(APIClient, serviceAccountName, nSpace).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

					By("Create ClusterRoleBinding")

					crbBuilder := define.ModuleCRB(*svcAccount, moduleName)
					_, err = crbBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

					By("Create Module with DRA and toleration")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"tolerations": []interface{}{
							map[string]interface{}{
								"key":      "dra-test",
								"value":    "edge-case",
								"operator": "Equal",
								"effect":   "NoSchedule",
							},
						},
						"dra": map[string]interface{}{
							"driverName":         kmmparams.DRADriverName,
							"serviceAccountName": serviceAccountName,
							"container": map[string]interface{}{
								"image":   kmmparams.DRADriverImage,
								"command": []interface{}{"dra-example-kubeletplugin"},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DRIVER_NAME",
										"value": kmmparams.DRADriverName,
									},
								},
							},
						},
					})

					err = APIClient.Create(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(), "error creating module")

					By("Wait for DRA DaemonSet and verify toleration")

					Eventually(func() error {
						daemonSet, getErr := get.DRADaemonSet(APIClient, moduleName, nSpace)
						if getErr != nil {
							return getErr
						}

						for _, tol := range daemonSet.Spec.Template.Spec.Tolerations {
							if tol.Key == "dra-test" && tol.Value == "edge-case" &&
								string(tol.Effect) == "NoSchedule" {
								return nil
							}
						}

						return fmt.Errorf("toleration dra-test=edge-case:NoSchedule not found")
					}, 2*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should have the expected toleration")
				})
		})

		Context("Pod Spec Priority", Label("dra-podspec"), func() {
			nSpace := kmmparams.DRAPriorityTestNamespace
			moduleName := "dra-priority-test"
			serviceAccountName := "pri-manager"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
			})

			AfterAll(func() {
				cleanupErr := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(cleanupErr).ToNot(HaveOccurred(), "error cleaning up modules")

				nsErr := namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(nsErr).ToNot(HaveOccurred(), "error deleting test namespace")
			})

			It("should set priorityClassName system-node-critical on DRA DaemonSet",
				reportxml.ID("89715"), func() {
					By("Create ServiceAccount")

					svcAccount, err := serviceaccount.
						NewBuilder(APIClient, serviceAccountName, nSpace).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

					By("Create ClusterRoleBinding")

					crbBuilder := define.ModuleCRB(*svcAccount, moduleName)
					_, err = crbBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

					By("Create Module with DRA")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"dra": map[string]interface{}{
							"driverName":         kmmparams.DRADriverName,
							"serviceAccountName": serviceAccountName,
							"container": map[string]interface{}{
								"image":   kmmparams.DRADriverImage,
								"command": []interface{}{"dra-example-kubeletplugin"},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DRIVER_NAME",
										"value": kmmparams.DRADriverName,
									},
								},
							},
						},
					})

					err = APIClient.Create(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(), "error creating module")

					By("Wait for DRA DaemonSet and verify priorityClassName")

					Eventually(func() error {
						daemonSet, getErr := get.DRADaemonSet(APIClient, moduleName, nSpace)
						if getErr != nil {
							return getErr
						}

						priority := daemonSet.Spec.Template.Spec.PriorityClassName
						if priority != "system-node-critical" {
							return fmt.Errorf(
								"DRA DaemonSet has priorityClassName %q, expected system-node-critical",
								priority)
						}

						return nil
					}, 2*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should have priorityClassName system-node-critical")
				})
		})

		Context("Edge Case No DeviceClasses", Label("dra-edge"), func() {
			nSpace := kmmparams.DRANoDeviceClassTestNamespace
			moduleName := "dra-no-dc-test"
			serviceAccountName := "nodc-manager"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
			})

			AfterAll(func() {
				cleanupErr := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(cleanupErr).ToNot(HaveOccurred(), "error cleaning up modules")

				nsErr := namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(nsErr).ToNot(HaveOccurred(), "error deleting test namespace")
			})

			It("should create DRA DaemonSet with no deviceClasses and report status correctly",
				reportxml.ID("89716"), func() {
					By("Create ServiceAccount")

					svcAccount, err := serviceaccount.
						NewBuilder(APIClient, serviceAccountName, nSpace).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

					By("Create ClusterRoleBinding")

					crbBuilder := define.ModuleCRB(*svcAccount, moduleName)
					_, err = crbBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

					By("Create Module with DRA but no deviceClasses")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"dra": map[string]interface{}{
							"driverName":         kmmparams.DRADriverName,
							"serviceAccountName": serviceAccountName,
							"container": map[string]interface{}{
								"image":   kmmparams.DRADriverImage,
								"command": []interface{}{"dra-example-kubeletplugin"},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DRIVER_NAME",
										"value": kmmparams.DRADriverName,
									},
								},
							},
						},
					})

					err = APIClient.Create(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(), "error creating module")

					By("Wait for DRA DaemonSet to be created")

					Eventually(func() error {
						_, getErr := get.DRADaemonSet(APIClient, moduleName, nSpace)

						return getErr
					}, 2*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should be created")

					By("Verify no DeviceClasses are created for this module")

					dcList, err := APIClient.K8sClient.ResourceV1().DeviceClasses().List(
						context.TODO(), metav1.ListOptions{
							LabelSelector: fmt.Sprintf(
								"kmm.node.kubernetes.io/module.name=%s,kmm.node.kubernetes.io/module.namespace=%s",
								moduleName, nSpace),
						})
					Expect(err).ToNot(HaveOccurred(), "error listing DeviceClasses")
					Expect(dcList.Items).To(BeEmpty(),
						"no DeviceClasses should exist for module without deviceClasses spec")

					By("Verify status.dra reports correctly")

					Eventually(func() bool {
						availNum, desiredNum, found, getErr := check.DRAModuleStatus(
							APIClient, moduleName, nSpace)
						if getErr != nil || !found {
							return false
						}

						return availNum > 0 && desiredNum > 0 && availNum == desiredNum
					}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
						"status.dra should report correct counts")
				})
		})

		Context("Edge Case ImagePullSecrets", Label("dra-edge"), func() {
			nSpace := kmmparams.DRAPullSecretTestNamespace
			moduleName := "dra-pullsecret-test"
			serviceAccountName := "pull-manager"
			secretName := "dra-pull-test-secret"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
			})

			AfterAll(func() {
				cleanupErr := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(cleanupErr).ToNot(HaveOccurred(), "error cleaning up modules")

				nsErr := namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(nsErr).ToNot(HaveOccurred(), "error deleting test namespace")
			})

			It("should propagate imagePullSecrets to DRA DaemonSet pod spec",
				reportxml.ID("89716"), func() {
					By("Create ServiceAccount")

					svcAccount, err := serviceaccount.
						NewBuilder(APIClient, serviceAccountName, nSpace).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

					By("Create ClusterRoleBinding")

					crbBuilder := define.ModuleCRB(*svcAccount, moduleName)
					_, err = crbBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

					By("Create docker-registry secret")

					dockerConfig := `{"auths":{"registry.example.com":{"auth":"dGVzdDp0ZXN0"}}}`

					_, err = APIClient.K8sClient.CoreV1().Secrets(nSpace).Create(
						context.TODO(), &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      secretName,
								Namespace: nSpace,
							},
							Type: corev1.SecretTypeDockerConfigJson,
							Data: map[string][]byte{
								corev1.DockerConfigJsonKey: []byte(dockerConfig),
							},
						},
						metav1.CreateOptions{})
					Expect(err).ToNot(HaveOccurred(), "error creating pull secret")

					By("Create Module with DRA and imagePullSecrets")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"imageRepoSecret": map[string]interface{}{
							"name": secretName,
						},
						"dra": map[string]interface{}{
							"driverName":         kmmparams.DRADriverName,
							"serviceAccountName": serviceAccountName,
							"container": map[string]interface{}{
								"image":   kmmparams.DRADriverImage,
								"command": []interface{}{"dra-example-kubeletplugin"},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DRIVER_NAME",
										"value": kmmparams.DRADriverName,
									},
								},
							},
						},
					})

					err = APIClient.Create(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(), "error creating module")

					By("Wait for DRA DaemonSet and verify imagePullSecrets")

					Eventually(func() error {
						daemonSet, getErr := get.DRADaemonSet(APIClient, moduleName, nSpace)
						if getErr != nil {
							return getErr
						}

						for _, ps := range daemonSet.Spec.Template.Spec.ImagePullSecrets {
							if ps.Name == secretName {
								return nil
							}
						}

						return fmt.Errorf("imagePullSecrets does not contain %s", secretName)
					}, 2*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should have imagePullSecrets")
				})
		})

		Context("Edge Case Remove Active DRA", Label("dra-edge"), func() {
			nSpace := kmmparams.DRARemoveActiveTestNamespace
			moduleName := "dra-remove-active-test"
			serviceAccountName := "rmact-manager"
			deviceClassName := "remove-active-class"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
			})

			AfterAll(func() {
				cleanupErr := await.CleanupModules(APIClient, []string{moduleName}, nSpace)
				Expect(cleanupErr).ToNot(HaveOccurred(), "error cleaning up modules")

				nsErr := namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
				Expect(nsErr).ToNot(HaveOccurred(), "error deleting test namespace")
			})

			It("should clean up DRA resources when removing spec.dra during active operation",
				reportxml.ID("89716"), func() {
					By("Create ServiceAccount")

					svcAccount, err := serviceaccount.
						NewBuilder(APIClient, serviceAccountName, nSpace).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

					By("Create ClusterRoleBinding")

					crbBuilder := define.ModuleCRB(*svcAccount, moduleName)
					_, err = crbBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

					By("Create Module with DRA and deviceClass")

					module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"dra": map[string]interface{}{
							"driverName":         kmmparams.DRADriverName,
							"serviceAccountName": serviceAccountName,
							"container": map[string]interface{}{
								"image":   kmmparams.DRADriverImage,
								"command": []interface{}{"dra-example-kubeletplugin"},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DRIVER_NAME",
										"value": kmmparams.DRADriverName,
									},
								},
							},
							"deviceClasses": []interface{}{
								map[string]interface{}{"name": deviceClassName},
							},
						},
					})

					err = APIClient.Create(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(), "error creating module")

					By("Wait for DRA DaemonSet to be running")

					Eventually(func() error {
						daemonSet, getErr := get.DRADaemonSet(APIClient, moduleName, nSpace)
						if getErr != nil {
							return getErr
						}

						if daemonSet.Status.NumberAvailable > 0 {
							return nil
						}

						return fmt.Errorf("DRA DaemonSet not yet available")
					}, 3*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should be running")

					By("Remove spec.dra while DRA is actively running")

					patch := []map[string]interface{}{
						{"op": "remove", "path": "/spec/dra"},
					}

					patchBytes, err := json.Marshal(patch)
					Expect(err).ToNot(HaveOccurred(), "error marshaling patch")

					moduleObj := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{})
					err = APIClient.Patch(context.TODO(), moduleObj,
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
				})
		})
	})
})
