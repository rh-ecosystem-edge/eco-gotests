package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/nhc-operator/internal/nhcparams"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var _ = Describe(
	"Planned reboot of a node during cluster upgrade",
	Ordered,
	ContinueOnFailure,
	Label(nhcparams.Label, nhcparams.LabelPlannedReboot), func() {
		var (
			targetNode       string
			initialVersion   string
			initialBootID    string
			upgradeImage     string
			labeledWorkers   []string
			unlabeledWorkers []string
		)

		BeforeAll(func() {
			By("Checking required configuration is provided")

			if RHWAConfig.TargetWorker == "" {
				Skip("ECO_RHWA_NHC_TARGET_WORKER not set")
			}

			if RHWAConfig.StorageClass == "" {
				Skip("ECO_RHWA_NHC_STORAGE_CLASS not set")
			}

			if RHWAConfig.AppImage == "" {
				Skip("ECO_RHWA_NHC_APP_IMAGE not set")
			}

			if RHWAConfig.UpgradeImage == "" {
				Skip("ECO_RHWA_NHC_UPGRADE_IMAGE not set")
			}

			if RHWAConfig.UpgradeChannel == "" {
				Skip("ECO_RHWA_NHC_UPGRADE_CHANNEL not set")
			}

			targetNode = RHWAConfig.TargetWorker

			By("Waiting for target worker node to be Ready")

			Eventually(func() bool {
				targetNodeObj, pullErr := nodes.Pull(APIClient, targetNode)
				if pullErr != nil {
					klog.Infof("  waiting for node %s: %v", targetNode, pullErr)

					return false
				}

				for _, condition := range targetNodeObj.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady {
						return condition.Status == corev1.ConditionTrue
					}
				}

				return false
			}).WithTimeout(nhcparams.NodeRecoveryTimeout).
				WithPolling(nhcparams.PollingInterval).
				Should(BeTrue(),
					fmt.Sprintf("Target worker node %s did not become Ready", targetNode))

			By("Verifying NHC deployment is Ready")

			nhcDeployment, err := deployment.Pull(
				APIClient, nhcparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NHC deployment")
			Expect(nhcDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(),
				"NHC deployment is not Ready")

			By("Verifying no stale SelfNodeRemediation resources exist")

			snrList, err := APIClient.Resource(rhwaparams.SnrGVR).
				Namespace(rhwaparams.RhwaOperatorNs).
				List(context.TODO(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to list SelfNodeRemediation resources")

			if len(snrList.Items) > 0 {
				var staleNames []string
				for _, snr := range snrList.Items {
					staleNames = append(staleNames, snr.GetName())
				}

				Skip(fmt.Sprintf("Stale SelfNodeRemediation resources found before upgrade: %v — "+
					"clean them up before running this test", staleNames))
			}

			By("Recording initial cluster version")

			cv, err := clusterversion.Pull(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterVersion")

			initialVersion = cv.Object.Status.Desired.Version
			upgradeImage = RHWAConfig.UpgradeImage

			klog.Infof("Current cluster version: %s", initialVersion)
			klog.Infof("Upgrade target image: %s", upgradeImage)

			By("Ensuring only target node has appworker label")

			removeLabelPatch := []byte(
				fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, nhcparams.AppWorkerLabel))
			addLabelPatch := []byte(
				fmt.Sprintf(`{"metadata":{"labels":{%q:""}}}`, nhcparams.AppWorkerLabel))

			if len(RHWAConfig.FailoverWorkers) > 0 {
				for _, workerName := range RHWAConfig.FailoverWorkers {
					workerNode, pullErr := nodes.Pull(APIClient, workerName)
					if pullErr != nil {
						continue
					}

					if _, exists := workerNode.Object.Labels[nhcparams.AppWorkerLabel]; exists {
						klog.Infof("Temporarily removing %s label from failover node %s before deployment",
							nhcparams.AppWorkerLabel, workerName)

						_, patchErr := APIClient.K8sClient.CoreV1().Nodes().Patch(
							context.TODO(), workerName, types.MergePatchType,
							removeLabelPatch, metav1.PatchOptions{})
						Expect(patchErr).ToNot(HaveOccurred(),
							fmt.Sprintf("Failed to remove label from node %s", workerName))

						unlabeledWorkers = append(unlabeledWorkers, workerName)
					}
				}
			}

			targetWorkerNode, err := nodes.Pull(APIClient, targetNode)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to pull node %s", targetNode))

			if _, exists := targetWorkerNode.Object.Labels[nhcparams.AppWorkerLabel]; !exists {
				_, patchErr := APIClient.K8sClient.CoreV1().Nodes().Patch(
					context.TODO(), targetNode, types.MergePatchType,
					addLabelPatch, metav1.PatchOptions{})
				Expect(patchErr).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to label node %s", targetNode))

				labeledWorkers = append(labeledWorkers, targetNode)
			}

			By("Creating test namespace")

			testNS := namespace.NewBuilder(APIClient, nhcparams.AppNamespace)

			_, err = testNS.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace")

			By("Creating PersistentVolumeClaim")

			pvcBuilder := storage.NewPVCBuilder(APIClient, nhcparams.PVCName, nhcparams.AppNamespace)

			pvcBuilder, err = pvcBuilder.WithPVCAccessMode("ReadWriteOnce")
			Expect(err).ToNot(HaveOccurred(), "Failed to set PVC access mode")

			pvcBuilder, err = pvcBuilder.WithPVCCapacity(nhcparams.PVCSize)
			Expect(err).ToNot(HaveOccurred(), "Failed to set PVC capacity")

			pvcBuilder, err = pvcBuilder.WithStorageClass(RHWAConfig.StorageClass)
			Expect(err).ToNot(HaveOccurred(), "Failed to set PVC storage class")

			_, err = pvcBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create PVC")

			By("Creating stateful app deployment")

			appLabels := map[string]string{nhcparams.AppLabelKey: nhcparams.AppLabelValue}

			containerCmd := []string{"/bin/sh", "-c",
				`echo "Starting stateful app on $(hostname)" > /data/heartbeat.log; ` +
					`while true; do echo "$(date -Iseconds) alive" >> /data/heartbeat.log; sleep 5; done`}

			container := pod.NewContainerBuilder(nhcparams.AppName, RHWAConfig.AppImage, containerCmd).
				WithVolumeMount(corev1.VolumeMount{
					Name:      nhcparams.PVCName,
					MountPath: "/data",
				})

			containerSpec, err := container.GetContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build container spec")

			containerSpec.SecurityContext = nil
			containerSpec.ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: &corev1.ExecAction{
						Command: []string{"cat", "/data/heartbeat.log"},
					},
				},
				InitialDelaySeconds: 5,
				PeriodSeconds:       5,
			}

			deploy := deployment.NewBuilder(
				APIClient, nhcparams.AppName, nhcparams.AppNamespace, appLabels, *containerSpec).
				WithNodeSelector(map[string]string{nhcparams.AppWorkerLabel: ""}).
				WithReplicas(int32(1)).
				WithVolume(corev1.Volume{
					Name: nhcparams.PVCName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: nhcparams.PVCName,
						},
					},
				})

			deploy.Definition.Spec.Strategy = appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			}

			_, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to create stateful app deployment")

			By("Recording which node the app pod is running on")

			appPods, err := pod.List(APIClient, nhcparams.AppNamespace, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", nhcparams.AppLabelKey, nhcparams.AppLabelValue),
				FieldSelector: "status.phase=Running",
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list app pods")
			Expect(appPods).To(HaveLen(1), "Expected exactly 1 running app pod")

			targetNode = appPods[0].Object.Spec.NodeName
			klog.Infof("Stateful app is running on node %s", targetNode)

			Expect(targetNode).To(Equal(RHWAConfig.TargetWorker),
				"App pod should be pinned to the configured target node")

			By("Recording target node boot ID before upgrade")

			targetNodeObj, err := nodes.Pull(APIClient, targetNode)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to pull node %s", targetNode))

			initialBootID = targetNodeObj.Object.Status.NodeInfo.BootID
			klog.Infof("Target node %s boot ID before upgrade: %s", targetNode, initialBootID)
			Expect(initialBootID).ToNot(BeEmpty(), "Node boot ID should not be empty")

			By("Labeling failover worker nodes (for post-upgrade app scheduling)")

			addLabelPatchFO := []byte(
				fmt.Sprintf(`{"metadata":{"labels":{%q:""}}}`, nhcparams.AppWorkerLabel))

			for _, workerName := range RHWAConfig.FailoverWorkers {
				if workerName == targetNode {
					continue
				}

				workerNode, pullErr := nodes.Pull(APIClient, workerName)
				Expect(pullErr).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to pull node %s", workerName))

				if _, exists := workerNode.Object.Labels[nhcparams.AppWorkerLabel]; exists {
					klog.Infof("Node %s already has label %s, skipping", workerName, nhcparams.AppWorkerLabel)

					continue
				}

				_, patchErr := APIClient.K8sClient.CoreV1().Nodes().Patch(
					context.TODO(), workerName, types.MergePatchType,
					addLabelPatchFO, metav1.PatchOptions{})
				Expect(patchErr).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to label node %s", workerName))

				labeledWorkers = append(labeledWorkers, workerName)
			}
		})

		AfterAll(func() {
			By("Deleting test namespace")

			testNS := namespace.NewBuilder(APIClient, nhcparams.AppNamespace)

			err := testNS.DeleteAndWait(5 * time.Minute)
			if err != nil {
				klog.Warningf("Failed to delete test namespace: %v", err)
			}

			By("Removing appworker label from nodes labeled by the test")

			removeLabelPatch := []byte(
				fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, nhcparams.AppWorkerLabel))
			addLabelPatch := []byte(
				fmt.Sprintf(`{"metadata":{"labels":{%q:""}}}`, nhcparams.AppWorkerLabel))

			for _, workerName := range labeledWorkers {
				_, patchErr := APIClient.K8sClient.CoreV1().Nodes().Patch(
					context.TODO(), workerName, types.MergePatchType,
					removeLabelPatch, metav1.PatchOptions{})
				if patchErr != nil {
					klog.Warningf("Failed to remove label from node %s: %v", workerName, patchErr)
				}
			}

			By("Restoring appworker label on nodes where it was temporarily removed")

			for _, workerName := range unlabeledWorkers {
				_, patchErr := APIClient.K8sClient.CoreV1().Nodes().Patch(
					context.TODO(), workerName, types.MergePatchType,
					addLabelPatch, metav1.PatchOptions{})
				if patchErr != nil {
					klog.Warningf("Failed to restore label on node %s: %v", workerName, patchErr)
				}
			}
		})

		It("Step 3: Verifies stateful app is running on target node", func() {
			By("Listing app pods")

			appPods, err := pod.List(APIClient, nhcparams.AppNamespace, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", nhcparams.AppLabelKey, nhcparams.AppLabelValue),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list app pods")

			runningPods := filterRunningPods(appPods)
			Expect(runningPods).To(HaveLen(1), "Expected exactly 1 running app pod")

			By("Verifying pod is on the target node")

			Expect(runningPods[0].Object.Spec.NodeName).To(Equal(targetNode),
				fmt.Sprintf("App pod should be on node %s", targetNode))
		})

		It("Step 4: Initiates cluster upgrade", func() {
			By("Pulling ClusterVersion")

			version, err := clusterversion.Pull(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterVersion")

			By(fmt.Sprintf("Patching ClusterVersion channel to %s", RHWAConfig.UpgradeChannel))

			version, err = version.WithDesiredUpdateChannel(RHWAConfig.UpgradeChannel).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to patch upgrade channel")

			By(fmt.Sprintf("Patching ClusterVersion with desired image %s", upgradeImage))

			version, err = version.WithDesiredUpdateImage(upgradeImage, true).Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to patch desired image")
			Expect(version.Object.Spec.DesiredUpdate.Image).To(Equal(upgradeImage),
				"Desired update image was not set correctly")

			By("Waiting for upgrade to start")

			err = version.WaitUntilUpdateIsStarted(nhcparams.UpgradeStartTimeout)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Upgrade did not start within %s", nhcparams.UpgradeStartTimeout))
			klog.Infof("Cluster upgrade has started")
		})

		It("Step 5: Verifies NHC does NOT remediate during upgrade", func() {
			snrCreated := false

			By("Polling for upgrade completion while checking for unwanted SNR resources")

			Eventually(func() bool {
				// Check if any SelfNodeRemediation resources exist (fail-fast).
				snrList, listErr := APIClient.Resource(rhwaparams.SnrGVR).
					Namespace(rhwaparams.RhwaOperatorNs).
					List(context.TODO(), metav1.ListOptions{})
				if listErr != nil {
					klog.Infof("  polling: error listing SNR resources: %v", listErr)

					return false
				}

				if len(snrList.Items) > 0 {
					for _, snr := range snrList.Items {
						annotations := snr.GetAnnotations()
						nodeName := annotations["remediation.medik8s.io/node-name"]
						klog.Errorf("  polling: UNEXPECTED SelfNodeRemediation %s for node %s",
							snr.GetName(), nodeName)
					}

					snrCreated = true

					return true
				}

				// Check NHC status for unhealthy nodes (informational warning).
				nhcResource, nhcErr := APIClient.Resource(nhcparams.NhcGVR).Get(
					context.TODO(), nhcparams.NHCResourceName, metav1.GetOptions{})
				if nhcErr == nil {
					nhcStatus, hasStatus := nhcResource.Object["status"].(map[string]any)
					if hasStatus {
						healthy := nhcStatus["healthyNodes"]
						observed := nhcStatus["observedNodes"]

						if unhealthyNodes, ok := nhcStatus["unhealthyNodes"].([]any); ok && len(unhealthyNodes) > 0 {
							var unhealthyNames []string

							for _, node := range unhealthyNodes {
								nodeMap, mapOk := node.(map[string]any)
								if !mapOk {
									continue
								}

								unhealthyNames = append(unhealthyNames, fmt.Sprintf("%v", nodeMap["name"]))
							}

							klog.Warningf("  polling: NHC reports unhealthy nodes: %v "+
								"(healthy=%v/%v) — should be transient during upgrade",
								unhealthyNames, healthy, observed)
						} else {
							klog.Infof("  polling: NHC healthy=%v/%v, no unhealthy nodes", healthy, observed)
						}
					}
				}

				// Check upgrade progress.
				clusterVer, cvErr := clusterversion.Pull(APIClient)
				if cvErr != nil {
					klog.Infof("  polling: error pulling ClusterVersion: %v", cvErr)

					return false
				}

				for _, history := range clusterVer.Object.Status.History {
					if history.Image == upgradeImage && history.State == configv1.CompletedUpdate {
						klog.Infof("  polling: upgrade completed")

						return true
					}
				}

				// Log current upgrade progress.
				progressing := "unknown"

				for _, cond := range clusterVer.Object.Status.Conditions {
					if cond.Type == configv1.OperatorProgressing {
						progressing = fmt.Sprintf("%s (%s)", cond.Status, cond.Message)

						break
					}
				}

				klog.Infof("  polling: upgrade in progress — version=%s, progressing=%s",
					clusterVer.Object.Status.Desired.Version, progressing)

				return false
			}).WithTimeout(nhcparams.UpgradeCompleteTimeout).
				WithPolling(nhcparams.UpgradePollingInterval).
				Should(BeTrue(), "Cluster upgrade did not complete in time")

			Expect(snrCreated).To(BeFalse(),
				"NHC created SelfNodeRemediation during upgrade — "+
					"should not remediate during planned reboot")
		})

		It("Step 6: Verifies NHC and SNR are clean post-upgrade", func() {
			By("Checking no SelfNodeRemediation resources exist")

			snrList, err := APIClient.Resource(rhwaparams.SnrGVR).
				Namespace(rhwaparams.RhwaOperatorNs).
				List(context.TODO(), metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to list SNR resources")
			Expect(snrList.Items).To(BeEmpty(),
				"SelfNodeRemediation resources should not exist after upgrade")

			By("Checking NHC status shows all nodes healthy")

			Eventually(func() bool {
				nhcResource, nhcErr := APIClient.Resource(nhcparams.NhcGVR).Get(
					context.TODO(), nhcparams.NHCResourceName, metav1.GetOptions{})
				if nhcErr != nil {
					klog.Infof("  polling NHC status: %v", nhcErr)

					return false
				}

				nhcStatus, hasStatus := nhcResource.Object["status"].(map[string]any)
				if !hasStatus {
					klog.Infof("  polling NHC status: no status yet")

					return false
				}

				healthy := nhcStatus["healthyNodes"]
				observed := nhcStatus["observedNodes"]

				unhealthyNodes, _ := nhcStatus["unhealthyNodes"].([]any)
				if len(unhealthyNodes) > 0 {
					klog.Infof("  polling NHC status: healthy=%v/%v, unhealthy=%d — waiting for reconciliation",
						healthy, observed, len(unhealthyNodes))

					return false
				}

				klog.Infof("NHC post-upgrade: healthy=%v/%v, no unhealthy nodes", healthy, observed)

				return true
			}).WithTimeout(nhcparams.NodeReadyTimeout).
				WithPolling(nhcparams.PollingInterval).
				Should(BeTrue(), "NHC still reports unhealthy nodes after upgrade")

			By("Checking no out-of-service taint on any node")

			nodeList, err := nodes.List(APIClient, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to list nodes")

			for _, node := range nodeList {
				for _, taint := range node.Object.Spec.Taints {
					Expect(taint.Key).ToNot(Equal(nhcparams.OutOfServiceTaintKey),
						fmt.Sprintf("Node %s has out-of-service taint", node.Object.Name))
				}
			}
		})

		It("Step 7: Verifies stateful app survived the upgrade", func() {
			By("Verifying cluster version changed")

			postUpgradeCV, err := clusterversion.Pull(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull ClusterVersion after upgrade")

			postUpgradeVersion := postUpgradeCV.Object.Status.Desired.Version
			klog.Infof("Post-upgrade cluster version: %s (was %s)", postUpgradeVersion, initialVersion)
			Expect(postUpgradeVersion).ToNot(Equal(initialVersion),
				"Cluster version should have changed after upgrade")

			By("Verifying target node rebooted during upgrade (boot ID changed)")

			Eventually(func() bool {
				targetNodeObj, pullErr := nodes.Pull(APIClient, targetNode)
				if pullErr != nil {
					klog.Infof("  waiting for node %s: %v", targetNode, pullErr)

					return false
				}

				for _, condition := range targetNodeObj.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady {
						return condition.Status == corev1.ConditionTrue
					}
				}

				return false
			}).WithTimeout(nhcparams.NodeRecoveryTimeout).
				WithPolling(nhcparams.PollingInterval).
				Should(BeTrue(),
					fmt.Sprintf("Target node %s did not become Ready after upgrade", targetNode))

			postUpgradeNode, err := nodes.Pull(APIClient, targetNode)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to pull node %s after upgrade", targetNode))

			postBootID := postUpgradeNode.Object.Status.NodeInfo.BootID
			klog.Infof("Target node %s boot ID after upgrade: %s (was %s)",
				targetNode, postBootID, initialBootID)
			Expect(postBootID).ToNot(Equal(initialBootID),
				fmt.Sprintf("Node %s boot ID unchanged — node did not reboot during upgrade", targetNode))

			By("Checking all cluster operators are available and not progressing")

			cosAvailable, err := clusteroperator.
				WaitForAllClusteroperatorsAvailable(APIClient, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(),
				"Error waiting for cluster operators to become available")
			Expect(cosAvailable).To(BeTrue(),
				"Some cluster operators are not available after upgrade")

			cosStoppedProgressing, err := clusteroperator.
				WaitForAllClusteroperatorsStopProgressing(APIClient, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(),
				"Error waiting for cluster operators to stop progressing")
			Expect(cosStoppedProgressing).To(BeTrue(),
				"Some cluster operators are still progressing after upgrade")

			By("Waiting for stateful app pod to be Running and Ready")

			Eventually(func() bool {
				appPods, listErr := pod.List(APIClient, nhcparams.AppNamespace, metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", nhcparams.AppLabelKey, nhcparams.AppLabelValue),
				})
				if listErr != nil {
					klog.Infof("  polling app: error listing pods: %v", listErr)

					return false
				}

				for idx := range appPods {
					appPod := appPods[idx]
					podReady := isPodReady(appPod)
					klog.Infof("  polling app: pod %s phase=%s ready=%v node=%s",
						appPod.Object.Name, appPod.Object.Status.Phase, podReady,
						appPod.Object.Spec.NodeName)

					if podReady {
						return true
					}
				}

				return false
			}).WithTimeout(nhcparams.NodeRecoveryTimeout).
				WithPolling(nhcparams.PollingInterval).
				Should(BeTrue(), "Stateful app pod is not Running and Ready after upgrade")

			By("Verifying PVC is still Bound")

			pvcList, err := storage.ListPVC(APIClient, nhcparams.AppNamespace, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to list PVCs")

			var appPVC *storage.PVCBuilder

			for idx := range pvcList {
				if pvcList[idx].Object.Name == nhcparams.PVCName {
					appPVC = pvcList[idx]

					break
				}
			}

			Expect(appPVC).ToNot(BeNil(), "PVC not found")
			Expect(appPVC.Object.Status.Phase).To(Equal(corev1.ClaimBound), "PVC is not Bound")
			klog.Infof("PVC %s is still Bound after upgrade", nhcparams.PVCName)

			By("Verifying app pod location")

			appPods, err := pod.List(APIClient, nhcparams.AppNamespace, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", nhcparams.AppLabelKey, nhcparams.AppLabelValue),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to list app pods")

			runningPods := filterRunningPods(appPods)
			Expect(runningPods).To(HaveLen(1), "Expected exactly 1 running app pod")

			podNode := runningPods[0].Object.Spec.NodeName
			klog.Infof("App pod is running on node %s (was on %s before upgrade)", podNode, targetNode)
		})
	})
