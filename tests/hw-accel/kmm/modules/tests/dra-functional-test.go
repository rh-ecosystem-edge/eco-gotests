package tests

import (
	"context"
	"fmt"
	"strings"
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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {
	Context("DRA Functional", Label("dra", "dra-functional"), func() {
		Context("Backward Compatibility", Label("dra-compat"), func() {
			nSpace := kmmparams.DRABackwardCompatTestNamespace
			moduleName := "compat-test-module"
			kmodName := "compat"
			serviceAccountName := "compat-manager"
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

				By("Create Module with moduleLoader and devicePlugin (no DRA)")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
					"selector": GeneralConfig.WorkerLabelMap,
					"moduleLoader": map[string]interface{}{
						"container": map[string]interface{}{
							"modprobe": map[string]interface{}{
								"moduleName": kmodName,
							},
							"kernelMappings": []interface{}{
								map[string]interface{}{
									"regexp":         "^.+$",
									"containerImage": image,
									"build": map[string]interface{}{
										"buildArgs": []interface{}{
											map[string]interface{}{
												"name":  kmmparams.BuildArgName,
												"value": buildArgValue,
											},
										},
										"dockerfileConfigMap": map[string]interface{}{
											"name": kmodName,
										},
									},
								},
							},
						},
						"serviceAccountName": serviceAccountName,
					},
					"devicePlugin": map[string]interface{}{
						"container": map[string]interface{}{
							"image":   "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							"command": []interface{}{"sleep", "3600"},
						},
					},
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
			})

			AfterAll(func() {
				By("Delete Module")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{})
				err := APIClient.Delete(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error deleting module")

				By("Await module deletion")

				err = await.ModuleObjectDeleted(APIClient, moduleName, nSpace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for module deletion")

				By("Delete ClusterRoleBinding")

				crbName := fmt.Sprintf("%s-module-manager-rolebinding", kmodName)
				err = APIClient.K8sClient.RbacV1().ClusterRoleBindings().Delete(
					context.TODO(), crbName, metav1.DeleteOptions{})
				Expect(err).ToNot(HaveOccurred(), "error deleting clusterrolebinding")

				By("Delete Namespace")

				err = namespace.NewBuilder(APIClient, nSpace).Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting namespace")

				By("Wait for namespace deletion")

				Eventually(func() bool {
					_, pullErr := namespace.Pull(APIClient, nSpace)

					return pullErr != nil
				}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
					"namespace was not deleted in time")
			})

			It("should not create DRA resources when using devicePlugin",
				reportxml.ID("89708"), func() {
					By("Verify module is loaded")

					err := check.ModuleLoaded(APIClient, kmodName, time.Minute)
					Expect(err).ToNot(HaveOccurred(), "module should be loaded")

					By("Verify module node label is set")

					_, err = check.NodeLabel(APIClient, moduleName, nSpace,
						GeneralConfig.WorkerLabelMap)
					Expect(err).ToNot(HaveOccurred(), "module node label should be set")

					By("Verify NO DRA node label exists")

					noDRA, err := check.NoDRANodeLabel(APIClient, moduleName, nSpace,
						GeneralConfig.WorkerLabelMap)
					Expect(err).ToNot(HaveOccurred(), "error checking DRA node label")
					Expect(noDRA).To(BeTrue(),
						"DRA node label should NOT be set for devicePlugin module")

					By("Verify no DRA DaemonSets exist")

					dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(nSpace).List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "error listing DaemonSets")

					for _, daemonSet := range dsList.Items {
						Expect(daemonSet.Name).ToNot(HavePrefix(moduleName+"-dra-"),
							"no DRA DaemonSet should exist for devicePlugin module")
					}
				})
		})

		Context("In-tree Driver Mode", Label("dra-intree"), func() {
			nSpace := kmmparams.DRAInTreeTestNamespace
			moduleName := "intree-dra-module"

			BeforeAll(func() {
				By("Create Namespace")

				_, err := namespace.NewBuilder(APIClient, nSpace).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

				By("Create Module with only spec.dra (no moduleLoader)")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
					"selector": GeneralConfig.WorkerLabelMap,
					"dra": map[string]interface{}{
						"driverName": kmmparams.DRADriverName,
						"container": map[string]interface{}{
							"image": kmmparams.DRADriverImage,
						},
					},
				})

				err = APIClient.Create(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error creating module")
			})

			AfterAll(func() {
				By("Delete Module")

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{})
				err := APIClient.Delete(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error deleting module")

				By("Await module deletion")

				err = await.ModuleObjectDeleted(APIClient, moduleName, nSpace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for module deletion")

				By("Delete Namespace")

				err = namespace.NewBuilder(APIClient, nSpace).Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting namespace")

				By("Wait for namespace deletion")

				Eventually(func() bool {
					_, pullErr := namespace.Pull(APIClient, nSpace)

					return pullErr != nil
				}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
					"namespace was not deleted in time")
			})

			It("should use spec.selector for DRA DaemonSet nodeSelector",
				reportxml.ID("89707"), func() {
					By("Wait for DRA DaemonSet to be created and verify nodeSelector")

					Eventually(func() error {
						dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(nSpace).List(
							context.TODO(), metav1.ListOptions{})
						if err != nil {
							return fmt.Errorf("error listing DaemonSets: %w", err)
						}

						for _, daemonSet := range dsList.Items {
							if !strings.HasPrefix(daemonSet.Name, moduleName+"-dra-") {
								continue
							}

							nodeSelector := daemonSet.Spec.Template.Spec.NodeSelector
							readyLabel := fmt.Sprintf(kmmparams.ModuleNodeLabelTemplate,
								nSpace, moduleName)

							if _, has := nodeSelector[readyLabel]; has {
								return fmt.Errorf(
									"DRA DaemonSet %s uses module-ready label in nodeSelector",
									daemonSet.Name)
							}

							for k, v := range GeneralConfig.WorkerLabelMap {
								if nodeSelector[k] != v {
									return fmt.Errorf(
										"DRA DaemonSet %s missing spec.selector key %s=%s in nodeSelector",
										daemonSet.Name, k, v)
								}
							}

							return nil
						}

						return fmt.Errorf("DRA DaemonSet not found yet")
					}, 2*time.Minute, 5*time.Second).Should(Succeed(),
						"DRA DaemonSet should use spec.selector as nodeSelector in in-tree mode")
				})
		})
	})
})
