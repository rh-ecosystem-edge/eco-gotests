package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {
	Context("DRA Ordered Upgrade", Label("dra", "dra-upgrade"), func() {
		nSpace := kmmparams.DRAUpgradeTestNamespace
		moduleName := "test-upgrade-dra"
		tempModuleName := "test-upgrade-dra-v2"
		kmodName := "upgmod"
		serviceAccountName := "upgrade-manager"
		imageV1 := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION-1.0.0",
			tsparams.LocalImageRegistry, nSpace, kmodName)
		imageV2 := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION-2.0.0",
			tsparams.LocalImageRegistry, nSpace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)

		versionLabel := fmt.Sprintf(kmmparams.ModuleVersionLabelTemplate, nSpace, moduleName)
		schedulePodLabel := fmt.Sprintf(kmmparams.SchedulePodVersionLabelTemplate, nSpace, moduleName)

		var v1DSName string

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
		})

		AfterAll(func() {
			By("Cleanup modules and CRB")

			err := await.CleanupModules(APIClient,
				[]string{moduleName, tempModuleName}, nSpace)
			Expect(err).ToNot(HaveOccurred(), "error cleaning up modules")

			crbName := fmt.Sprintf("%s-module-manager-rolebinding", kmodName)
			_ = APIClient.K8sClient.RbacV1().ClusterRoleBindings().Delete(
				context.TODO(), crbName, metav1.DeleteOptions{})

			By("Remove version labels from all workers")

			nodesBuilder, err := nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing nodes")

			for _, nodeBuilder := range nodesBuilder {
				nodeBuilder, _ = nodeBuilder.RemoveLabel(versionLabel, "v1").Update()
				nodeBuilder, _ = nodeBuilder.RemoveLabel(versionLabel, "v2").Update()
				nodeBuilder, _ = nodeBuilder.RemoveLabel(schedulePodLabel, "v1").Update()
				_, _ = nodeBuilder.RemoveLabel(schedulePodLabel, "v2").Update()
			}

			By("Delete Namespace")

			err = namespace.NewBuilder(APIClient, nSpace).DeleteAndWait(2 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
		})

		It("should deploy DRA with version-aware DaemonSet",
			reportxml.ID("89710"), Label("89710", "test_id:89710"), func() {
				By("Create Module with moduleLoader v1 and DRA")

				mlSpec := define.ModuleLoaderSpec(kmodName, imageV1, buildArgValue, serviceAccountName)

				containerSpec, ok := mlSpec["container"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "container spec should be a map")

				containerSpec["version"] = "v1"
				containerSpec["imagePullPolicy"] = "Always"

				module := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{
					"selector":     GeneralConfig.WorkerLabelMap,
					"moduleLoader": mlSpec,
					"dra":          define.DRASpec(serviceAccountName, []string{kmmparams.DRADeviceClassName}, nil),
				})

				err := APIClient.Create(context.TODO(), module)
				Expect(err).ToNot(HaveOccurred(), "error creating module")

				By("Await build pod to complete")

				err = await.BuildPodCompleted(APIClient, nSpace, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while building module")

				By("Set version label v1 on all workers")

				nodesBuilder, err := nodes.List(APIClient,
					metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
				Expect(err).ToNot(HaveOccurred(), "error listing nodes")

				for _, nodeBuilder := range nodesBuilder {
					_, err = nodeBuilder.WithNewLabel(versionLabel, "v1").Update()
					Expect(err).ToNot(HaveOccurred(), "error setting version label on node")
				}

				By("Await module deployment")

				err = await.ModuleDeployment(APIClient, moduleName, nSpace,
					5*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error waiting for module deployment")

				By("Await DRA deployment")

				err = await.DRADeployment(APIClient, moduleName, nSpace,
					3*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error waiting for DRA deployment")

				By("Verify DRA DaemonSet exists and record its name")

				draDaemonSet, err := get.DRADaemonSet(APIClient, moduleName, nSpace)
				Expect(err).ToNot(HaveOccurred(), "DRA DaemonSet should exist")

				v1DSName = draDaemonSet.Name
				klog.V(kmmparams.KmmLogLevel).Infof("v1 DRA DaemonSet: %s", v1DSName)

				By("Verify DRA DaemonSet has version-schedule-pod label")

				Expect(draDaemonSet.Labels).To(HaveKeyWithValue(schedulePodLabel, "v1"),
					"DRA DaemonSet should have schedule-pod version label v1")

				By("Verify DRA DaemonSet nodeSelector includes version-schedule-pod label")

				nodeSelector := draDaemonSet.Spec.Template.Spec.NodeSelector
				Expect(nodeSelector).To(HaveKeyWithValue(schedulePodLabel, "v1"),
					"DRA DaemonSet nodeSelector should include schedule-pod version label v1")

				By("Verify kernel module is loaded")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "kernel module should be loaded")

				By("Verify module node label is set")

				_, err = check.NodeLabel(APIClient, moduleName, nSpace, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "module node label should be set")

				By("Verify DRA node label is set")

				draLabelOK, err := check.DRANodeLabel(APIClient, moduleName, nSpace,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error checking DRA node label")
				Expect(draLabelOK).To(BeTrue(), "DRA node label should be set")
			})

		It("should create new DRA DaemonSet on version upgrade and GC old one",
			reportxml.ID("89710"), Label("89710", "test_id:89710"), func() {
				By("Pre-build v2 image via temporary module")

				tempMLSpec := define.ModuleLoaderSpec(kmodName, imageV2, buildArgValue, serviceAccountName)

				tempContainerSpec, ok := tempMLSpec["container"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "container spec should be a map")

				tempContainerSpec["version"] = "v2"
				tempContainerSpec["imagePullPolicy"] = "Always"

				tempModule := newUnstructuredModule(tempModuleName, nSpace, map[string]interface{}{
					"selector":     GeneralConfig.WorkerLabelMap,
					"moduleLoader": tempMLSpec,
				})

				By("Capture existing build pod names before creating temp module")

				existingPods, err := pod.List(APIClient, nSpace, metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "error listing existing pods")

				var existingBuildPodNames []string

				for _, p := range existingPods {
					if strings.Contains(p.Object.Name, "-build") {
						existingBuildPodNames = append(existingBuildPodNames, p.Object.Name)
					}
				}

				err = APIClient.Create(context.TODO(), tempModule)
				Expect(err).ToNot(HaveOccurred(), "error creating temp module")

				By("Await v2 build pod to complete")

				err = await.NewBuildPodCompleted(APIClient, nSpace, existingBuildPodNames, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error building v2 module")

				By("Delete temporary module")

				err = await.CleanupModules(APIClient, []string{tempModuleName}, nSpace)
				Expect(err).ToNot(HaveOccurred(), "error deleting temp module")

				By("Update original Module to version v2")

				patch := []map[string]interface{}{
					{
						"op":    "replace",
						"path":  "/spec/moduleLoader/container/version",
						"value": "v2",
					},
					{
						"op":    "replace",
						"path":  "/spec/moduleLoader/container/kernelMappings/0/containerImage",
						"value": imageV2,
					},
				}

				patchBytes, err := json.Marshal(patch)
				Expect(err).ToNot(HaveOccurred(), "error marshaling patch")

				patchModule := newUnstructuredModule(moduleName, nSpace, map[string]interface{}{})
				err = APIClient.Patch(context.TODO(), patchModule,
					runtimeClient.RawPatch(types.JSONPatchType, patchBytes))
				Expect(err).ToNot(HaveOccurred(), "error patching module to v2")

				By("Verify new DRA DaemonSet is created with v2 label")

				Eventually(func() bool {
					dsList, getErr := get.AllDRADaemonSets(APIClient, moduleName, nSpace)
					if getErr != nil {
						klog.V(kmmparams.KmmLogLevel).Infof("error listing DRA DaemonSets: %v", getErr)

						return false
					}

					for _, ds := range dsList {
						if ds.Labels[schedulePodLabel] == "v2" {
							klog.V(kmmparams.KmmLogLevel).Infof("Found v2 DRA DaemonSet: %s", ds.Name)

							return true
						}
					}

					return false
				}, 3*time.Minute, 5*time.Second).Should(BeTrue(),
					"new DRA DaemonSet with v2 label should be created")

				By("Upgrade nodes: set version label to v2 on all workers")

				nodesBuilder, err := nodes.List(APIClient,
					metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
				Expect(err).ToNot(HaveOccurred(), "error listing nodes")

				for _, nodeBuilder := range nodesBuilder {
					nodeBuilder, err = nodeBuilder.RemoveLabel(versionLabel, "v1").Update()
					Expect(err).ToNot(HaveOccurred(), "error removing version label v1 from node")

					_, err = nodeBuilder.WithNewLabel(versionLabel, "v2").Update()
					Expect(err).ToNot(HaveOccurred(), "error setting version label v2 on node")
				}

				By("Await module deployment at v2")

				err = await.ModuleDeployment(APIClient, moduleName, nSpace,
					5*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error waiting for v2 module deployment")

				By("Await DRA deployment at v2")

				err = await.DRADeployment(APIClient, moduleName, nSpace,
					3*time.Minute, GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error waiting for v2 DRA deployment")

				By("Verify old v1 DRA DaemonSet is garbage collected")

				Eventually(func() int {
					dsList, getErr := get.AllDRADaemonSets(APIClient, moduleName, nSpace)
					if getErr != nil {
						return -1
					}

					return len(dsList)
				}, 3*time.Minute, 5*time.Second).Should(Equal(1),
					"only one DRA DaemonSet should remain after GC")

				By("Verify remaining DRA DaemonSet has v2 label")

				draDaemonSet, err := get.DRADaemonSet(APIClient, moduleName, nSpace)
				Expect(err).ToNot(HaveOccurred(), "DRA DaemonSet should exist")
				Expect(draDaemonSet.Name).ToNot(Equal(v1DSName),
					"v2 DRA DaemonSet should have a different name than v1")
				Expect(draDaemonSet.Labels).To(HaveKeyWithValue(schedulePodLabel, "v2"),
					"remaining DRA DaemonSet should have version v2")

				By("Verify DRA DaemonSet nodeSelector has v2 version")

				nodeSelector := draDaemonSet.Spec.Template.Spec.NodeSelector
				Expect(nodeSelector).To(HaveKeyWithValue(schedulePodLabel, "v2"),
					"DRA DaemonSet nodeSelector should have schedule-pod version v2")

				By("Verify DRA DaemonSet is fully available")

				Expect(draDaemonSet.Status.DesiredNumberScheduled).To(
					BeNumerically(">", 0), "DRA DaemonSet should have desired pods")
				Expect(draDaemonSet.Status.NumberAvailable).To(
					Equal(draDaemonSet.Status.DesiredNumberScheduled),
					"all DRA DaemonSet pods should be available")

				By("Verify kernel module is still loaded")

				err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "kernel module should be loaded after upgrade")

				By("Verify DRA node label is still set")

				draLabelOK, err := check.DRANodeLabel(APIClient, moduleName, nSpace,
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error checking DRA node label after upgrade")
				Expect(draLabelOK).To(BeTrue(), "DRA node label should be set after upgrade")

				By("Verify module status.dra fields")

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
})
