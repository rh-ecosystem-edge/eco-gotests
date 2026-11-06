package tests

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/nhc-operator/internal/nhcparams"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var _ = Describe(
	"Sudden loss of a node",
	Ordered,
	ContinueOnFailure,
	Label(nhcparams.Label, nhcparams.LabelSuddenLoss), func() {
		var (
			targetNode       string
			bmcClient        *bmc.BMC
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

			if len(RHWAConfig.FailoverWorkers) == 0 {
				Skip("ECO_RHWA_NHC_FAILOVER_WORKERS not set")
			}

			hasHealthyFailover := false

			for _, failoverName := range RHWAConfig.FailoverWorkers {
				if failoverName == RHWAConfig.TargetWorker {
					continue
				}

				failoverNode, pullErr := nodes.Pull(APIClient, failoverName)
				if pullErr != nil {
					klog.Warningf("Failed to pull failover node %s: %v", failoverName, pullErr)

					continue
				}

				for _, condition := range failoverNode.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						hasHealthyFailover = true

						break
					}
				}

				if hasHealthyFailover {
					break
				}
			}

			if !hasHealthyFailover {
				Skip("No distinct healthy failover worker available")
			}

			if RHWAConfig.TargetWorkerBMC.Address == "" {
				Skip("ECO_RHWA_NHC_TARGET_WORKER_BMC address not set")
			}

			if RHWAConfig.TargetWorkerBMC.Username == "" || RHWAConfig.TargetWorkerBMC.Password == "" {
				Skip("ECO_RHWA_NHC_TARGET_WORKER_BMC username or password not set")
			}

			targetNode = RHWAConfig.TargetWorker

			By("Waiting for target worker node to be Ready")

			klog.Infof("Waiting up to %s for node %s to become Ready", nhcparams.NodeRecoveryTimeout, targetNode)

			targetNodeObj, err := nodes.Pull(APIClient, targetNode)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to pull node %s", targetNode))

			err = targetNodeObj.WaitUntilReady(nhcparams.NodeRecoveryTimeout)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Target worker node %s did not become Ready", targetNode))

			klog.Infof("Node %s is Ready", targetNode)

			By("Verifying NHC deployment is Ready")

			nhcDeployment, err := deployment.Pull(
				APIClient, nhcparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NHC deployment")
			Expect(nhcDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(),
				"NHC deployment is not Ready")

			By("Creating BMC client for target node")

			bmcClient = bmc.New(RHWAConfig.TargetWorkerBMC.Address).
				WithRedfishUser(RHWAConfig.TargetWorkerBMC.Username,
					RHWAConfig.TargetWorkerBMC.Password).
				WithRedfishTimeout(nhcparams.BMCTimeout)

			// Register all cleanup in BeforeAll so it runs after the last spec
			// in this Ordered container (LIFO order).

			DeferCleanup(func() {
				By("Powering the node back on via BMC")

				if bmcClient == nil {
					return
				}

				err := bmcClient.SystemPowerOn()
				if err != nil {
					klog.Warningf("Failed to power on node: %v", err)

					return
				}

				By("Waiting for node to become Ready")

				klog.Infof("Waiting up to %s for node %s to recover to Ready state",
					nhcparams.NodeRecoveryTimeout, targetNode)

				node, pullErr := nodes.Pull(APIClient, targetNode)
				if pullErr != nil {
					klog.Warningf("Failed to pull node %s during cleanup: %v", targetNode, pullErr)

					return
				}

				waitErr := node.WaitUntilReady(nhcparams.NodeRecoveryTimeout)
				if waitErr != nil {
					klog.Warningf("Node %s did not recover to Ready state: %v", targetNode, waitErr)
				} else {
					klog.Infof("Node %s recovered to Ready state", targetNode)
				}
			})

			DeferCleanup(func() {
				By("Restoring appworker label on nodes where it was temporarily removed")

				restoreLabelPatch := []byte(
					fmt.Sprintf(`{"metadata":{"labels":{%q:""}}}`, nhcparams.AppWorkerLabel))

				for _, workerName := range unlabeledWorkers {
					_, patchErr := APIClient.K8sClient.CoreV1().Nodes().Patch(
						context.TODO(), workerName, types.MergePatchType,
						restoreLabelPatch, metav1.PatchOptions{})
					if patchErr != nil {
						klog.Warningf("Failed to restore label on node %s: %v", workerName, patchErr)
					}
				}
			})

			DeferCleanup(func() {
				By("Removing appworker label from nodes labeled by the test")

				removeLabelPatch := []byte(
					fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, nhcparams.AppWorkerLabel))

				for _, workerName := range labeledWorkers {
					_, patchErr := APIClient.K8sClient.CoreV1().Nodes().Patch(
						context.TODO(), workerName, types.MergePatchType,
						removeLabelPatch, metav1.PatchOptions{})
					if patchErr != nil {
						klog.Warningf("Failed to remove label from node %s: %v", workerName, patchErr)
					}
				}
			})

			DeferCleanup(func() {
				By("Deleting test namespace")

				testNS := namespace.NewBuilder(APIClient, nhcparams.AppNamespace)

				if deleteErr := testNS.DeleteAndWait(nhcparams.DeletionTimeout); deleteErr != nil {
					klog.Warningf("Failed to delete test namespace: %v", deleteErr)
				}
			})
		})

		It("Step 1: Deploys stateful app on target node",
			reportxml.ID("00001"), func() {
				By("Ensuring only target node has appworker label")

				removeLabelPatch := []byte(
					fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, nhcparams.AppWorkerLabel))
				addLabelPatch := []byte(
					fmt.Sprintf(`{"metadata":{"labels":{%q:""}}}`, nhcparams.AppWorkerLabel))

				for _, workerName := range RHWAConfig.FailoverWorkers {
					var workerNode *nodes.Builder

					var pullErr error

					for attempt := 1; attempt <= 3; attempt++ {
						workerNode, pullErr = nodes.Pull(APIClient, workerName)
						if pullErr == nil {
							break
						}

						klog.Warningf("Failed to pull failover node %s (attempt %d/3): %v",
							workerName, attempt, pullErr)
						time.Sleep(nhcparams.PollingInterval)
					}

					if pullErr != nil {
						klog.Warningf("Giving up on failover node %s after 3 attempts", workerName)

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

				_, err = deploy.CreateAndWaitUntilReady(nhcparams.DeploymentTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create stateful app deployment")

				By("Verifying app pod is running on the target node")

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

				By("Labeling failover worker nodes")

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

		It("Step 2: Powers off the target node via BMC",
			reportxml.ID("00002"), func() {
				By("Sending power-off command via BMC Redfish")

				err := bmcClient.SystemPowerOff()
				Expect(err).ToNot(HaveOccurred(), "Failed to power off node via BMC")

				By("Waiting for node Ready condition to become Unknown (~40s expected)")

				klog.Infof("Waiting up to %s for node %s Ready condition to become Unknown",
					nhcparams.NodeReadyTimeout, targetNode)

				targetNodeObj, err := nodes.Pull(APIClient, targetNode)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to pull node %s", targetNode))

				err = targetNodeObj.WaitUntilNotReady(nhcparams.NodeReadyTimeout)
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Node %s Ready condition did not become NotReady/Unknown", targetNode))

				klog.Infof("Node %s Ready condition is now NotReady/Unknown", targetNode)
			})

		It("Step 3: Verifies NHC marks node unhealthy and creates SNR resource",
			reportxml.ID("00003"), func() {
				By("Getting NodeHealthCheck resource")

				nhcResource, err := APIClient.Resource(nhcparams.NhcGVR).Get(
					context.TODO(), nhcparams.NHCResourceName, metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to get NodeHealthCheck resource")

				By("Verifying minHealthy threshold is still satisfied")

				spec, specOk := nhcResource.Object["spec"].(map[string]any)
				Expect(specOk).To(BeTrue(), "NHC resource has no spec")

				status, statusOk := nhcResource.Object["status"].(map[string]any)
				Expect(statusOk).To(BeTrue(), "NHC resource has no status")

				minHealthyStr := fmt.Sprintf("%v", spec["minHealthy"])
				observedNodes := status["observedNodes"]
				healthyNodes := status["healthyNodes"]

				klog.Infof("NHC: minHealthy=%s, observedNodes=%v, healthyNodes=%v",
					minHealthyStr, observedNodes, healthyNodes)

				if pctStr, ok := strings.CutSuffix(minHealthyStr, "%"); ok {
					pct, parseErr := strconv.ParseFloat(pctStr, 64)
					Expect(parseErr).ToNot(HaveOccurred(), "Failed to parse minHealthy percentage")

					observed, observedErr := toFloat64(observedNodes)
					Expect(observedErr).ToNot(HaveOccurred(), "Failed to convert observedNodes to float64")

					healthy, healthyErr := toFloat64(healthyNodes)
					Expect(healthyErr).ToNot(HaveOccurred(), "Failed to convert healthyNodes to float64")

					minRequired := math.Ceil(observed * pct / 100)

					Expect(healthy).To(BeNumerically(">=", minRequired),
						fmt.Sprintf("minHealthy threshold not satisfied: %v/%v healthy, minimum %v required",
							healthy, observed, minRequired))
				}

				By("Waiting for NHC to mark node as unhealthy (~60s expected)")

				Eventually(func() bool {
					nhc, getErr := APIClient.Resource(nhcparams.NhcGVR).Get(
						context.TODO(), nhcparams.NHCResourceName, metav1.GetOptions{})
					if getErr != nil {
						klog.Infof("  polling NHC: error getting resource: %v", getErr)

						return false
					}

					nhcStatus, hasStatus := nhc.Object["status"].(map[string]any)
					if !hasStatus {
						klog.Infof("  polling NHC: no status yet")

						return false
					}

					healthy := nhcStatus["healthyNodes"]
					observed := nhcStatus["observedNodes"]

					unhealthyNodes, hasUnhealthy := nhcStatus["unhealthyNodes"].([]any)
					if !hasUnhealthy {
						klog.Infof("  polling NHC: healthy=%v/%v, no unhealthy nodes yet", healthy, observed)

						return false
					}

					var unhealthyNames []string

					for _, node := range unhealthyNodes {
						nodeMap, ok := node.(map[string]any)
						if !ok {
							continue
						}

						name := fmt.Sprintf("%v", nodeMap["name"])
						unhealthyNames = append(unhealthyNames, name)

						if name == targetNode {
							klog.Infof("  polling NHC: %s is now unhealthy", targetNode)

							return true
						}
					}

					klog.Infof("  polling NHC: healthy=%v/%v, unhealthy=%v (waiting for %s)",
						healthy, observed, unhealthyNames, targetNode)

					return false
				}).WithTimeout(nhcparams.NHCObserveTimeout).
					WithPolling(nhcparams.PollingInterval).
					Should(BeTrue(),
						fmt.Sprintf("NHC did not mark %s as unhealthy", targetNode))

				By("Waiting for SelfNodeRemediation resource to be created")

				var snrResourceName string

				Eventually(func() bool {
					snrList, listErr := APIClient.Resource(rhwaparams.SnrGVR).
						Namespace(rhwaparams.RhwaOperatorNs).
						List(context.TODO(), metav1.ListOptions{})
					if listErr != nil {
						klog.Infof("  polling SNR: error listing resources: %v", listErr)

						return false
					}

					klog.Infof("  polling SNR: found %d SelfNodeRemediation resources", len(snrList.Items))

					for _, snr := range snrList.Items {
						annotations := snr.GetAnnotations()
						if annotations["remediation.medik8s.io/node-name"] == targetNode {
							snrResourceName = snr.GetName()
							klog.Infof("  polling SNR: found %s for %s", snrResourceName, targetNode)

							return true
						}
					}

					klog.Infof("  polling SNR: none yet for %s", targetNode)

					return false
				}).WithTimeout(nhcparams.NHCObserveTimeout).
					WithPolling(nhcparams.PollingInterval).
					Should(BeTrue(),
						fmt.Sprintf("SelfNodeRemediation not created for %s", targetNode))

				By("Verifying remediationStrategy is OutOfServiceTaint")

				snrResource, err := APIClient.Resource(rhwaparams.SnrGVR).
					Namespace(rhwaparams.RhwaOperatorNs).
					Get(context.TODO(), snrResourceName, metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to get SelfNodeRemediation resource")

				snrSpec, snrSpecOk := snrResource.Object["spec"].(map[string]any)
				Expect(snrSpecOk).To(BeTrue(), "SNR resource has no spec")
				Expect(snrSpec["remediationStrategy"]).To(Equal("OutOfServiceTaint"),
					"Expected remediationStrategy=OutOfServiceTaint")
			})

		It("Step 4: Verifies SNR fences the node with out-of-service taint",
			reportxml.ID("00004"), func() {
				By("Waiting for SNR fencing to complete (~180s expected)")

				Eventually(func() bool {
					snrList, listErr := APIClient.Resource(rhwaparams.SnrGVR).
						Namespace(rhwaparams.RhwaOperatorNs).
						List(context.TODO(), metav1.ListOptions{})
					if listErr != nil {
						klog.Infof("  polling SNR fencing: error listing: %v", listErr)

						return false
					}

					for _, snr := range snrList.Items {
						annotations := snr.GetAnnotations()
						if annotations["remediation.medik8s.io/node-name"] != targetNode {
							continue
						}

						snrStatus, hasStatus := snr.Object["status"].(map[string]any)
						if !hasStatus {
							klog.Infof("  polling SNR fencing: no status yet on %s", snr.GetName())

							return false
						}

						phase := fmt.Sprintf("%v", snrStatus["phase"])
						lastErr := fmt.Sprintf("%v", snrStatus["lastError"])
						timeAssumedRebooted := fmt.Sprintf("%v", snrStatus["timeAssumedRebooted"])

						klog.Infof("  polling SNR fencing: phase=%s, lastError=%s, timeAssumedRebooted=%s",
							phase, lastErr, timeAssumedRebooted)

						if phase == "Fencing-Completed" || phase == "Fencing-CompletedSuccessfully" {
							return true
						}

						conditions, ok := snrStatus["conditions"].([]any)
						if ok {
							for _, cond := range conditions {
								condMap, ok := cond.(map[string]any)
								if !ok {
									continue
								}

								if fmt.Sprintf("%v", condMap["type"]) == "Succeeded" &&
									fmt.Sprintf("%v", condMap["status"]) == "True" {
									klog.Infof("  polling SNR fencing: Succeeded condition is True")

									return true
								}
							}
						}

						return false
					}

					klog.Infof("  polling SNR fencing: no SNR resource found for %s", targetNode)

					return false
				}).WithTimeout(nhcparams.SNRFenceTimeout).
					WithPolling(nhcparams.PollingInterval).
					Should(BeTrue(),
						fmt.Sprintf("SNR did not complete fencing for %s", targetNode))

				By("Verifying out-of-service taint was applied (via AddOutOfService event)")
				// Note: the out-of-service taint is transient — SNR removes it as part of completing
				// fencing. By the time the SNR phase reaches "Fencing-Completed", the taint has already
				// been removed. We verify the taint was applied by checking for the AddOutOfService event,
				// which is a permanent record that the taint was added during the remediation cycle.

				events, err := APIClient.CoreV1Interface.Events("default").List(
					context.TODO(), metav1.ListOptions{
						FieldSelector: fmt.Sprintf("involvedObject.name=%s,reason=AddOutOfService", targetNode),
					})
				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to list events for node %s", targetNode))
				Expect(events.Items).ToNot(BeEmpty(),
					fmt.Sprintf("No AddOutOfService event found for node %s — "+
						"out-of-service taint was never applied during remediation", targetNode))
			})

		It("Step 5: Verifies stateful app rescheduled to a healthy node",
			reportxml.ID("00005"), func() {
				var rescheduledNodeName string

				By("Waiting for app pod to be Ready on a different node")

				Eventually(func() bool {
					// Check deployment status for diagnostics.
					deploy, deployErr := deployment.Pull(
						APIClient, nhcparams.AppName, nhcparams.AppNamespace)
					if deployErr != nil {
						klog.Infof("  polling reschedule: error pulling deployment: %v", deployErr)
					} else {
						status := deploy.Object.Status
						klog.Infof("  polling reschedule: deployment replicas=%d ready=%d available=%d unavailable=%d",
							status.Replicas, status.ReadyReplicas, status.AvailableReplicas, status.UnavailableReplicas)
					}

					// List ALL pods in the namespace for visibility.
					allPods, allErr := pod.List(APIClient, nhcparams.AppNamespace, metav1.ListOptions{})
					if allErr != nil {
						klog.Infof("  polling reschedule: error listing all pods: %v", allErr)
					} else {
						klog.Infof("  polling reschedule: total pods in namespace: %d", len(allPods))

						for idx := range allPods {
							p := allPods[idx]
							klog.Infof("  polling reschedule:   pod %s phase=%s node=%s",
								p.Object.Name, p.Object.Status.Phase, p.Object.Spec.NodeName)

							if p.Object.Status.Phase == corev1.PodPending {
								for _, cond := range p.Object.Status.Conditions {
									if cond.Status != corev1.ConditionTrue {
										klog.Infof("  polling reschedule:     condition %s=%s: %s",
											cond.Type, cond.Status, cond.Message)
									}
								}
							}
						}
					}

					// Check PVC status.
					pvcObj, pvcErr := APIClient.K8sClient.CoreV1().PersistentVolumeClaims(nhcparams.AppNamespace).Get(
						context.TODO(), nhcparams.PVCName, metav1.GetOptions{})
					if pvcErr != nil {
						klog.Infof("  polling reschedule: error getting PVC: %v", pvcErr)
					} else {
						klog.Infof("  polling reschedule: PVC %s phase=%s volume=%s",
							pvcObj.Name, pvcObj.Status.Phase, pvcObj.Spec.VolumeName)
					}

					// Now check for the rescheduled app pod.
					appPods, listErr := pod.List(APIClient, nhcparams.AppNamespace, metav1.ListOptions{
						LabelSelector: fmt.Sprintf("%s=%s", nhcparams.AppLabelKey, nhcparams.AppLabelValue),
					})
					if listErr != nil {
						klog.Infof("  polling reschedule: error listing app pods: %v", listErr)

						return false
					}

					for idx := range appPods {
						appPod := appPods[idx]
						podReady := isPodReady(appPod)
						klog.Infof("  polling reschedule: app pod %s phase=%s ready=%v node=%s",
							appPod.Object.Name, appPod.Object.Status.Phase, podReady, appPod.Object.Spec.NodeName)

						if podReady && appPod.Object.Spec.NodeName != targetNode {
							rescheduledNodeName = appPod.Object.Spec.NodeName
							klog.Infof("  polling reschedule: app rescheduled and Ready on %s", rescheduledNodeName)

							return true
						}
					}

					klog.Infof("  polling reschedule: no Ready app pod on a different node yet (found %d app pods)",
						len(appPods))

					return false
				}).WithTimeout(nhcparams.RescheduleTimeout).
					WithPolling(nhcparams.PollingInterval).
					Should(BeTrue(), "App pod was not rescheduled to a healthy node")

				By("Verifying PVC is Bound")

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
				Expect(appPVC.Object.Status.Phase).To(Equal(corev1.ClaimBound),
					"PVC is not Bound")

				pvName := appPVC.Object.Spec.VolumeName
				klog.Infof("PVC %s is Bound to PV %s", nhcparams.PVCName, pvName)

				By("Verifying VolumeAttachment to new node")

				vaList, err := APIClient.StorageV1Interface.VolumeAttachments().List(
					context.TODO(), metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to list VolumeAttachments")

				foundOnNewNode := false
				foundOnOldNode := false
				hasVolumeAttachment := false

				for idx := range vaList.Items {
					attachment := &vaList.Items[idx]
					if attachment.Spec.Source.PersistentVolumeName == nil || *attachment.Spec.Source.PersistentVolumeName != pvName {
						continue
					}

					hasVolumeAttachment = true

					if attachment.Spec.NodeName == rescheduledNodeName && attachment.Status.Attached {
						foundOnNewNode = true
					}

					if attachment.Spec.NodeName == targetNode && attachment.Status.Attached {
						foundOnOldNode = true
					}
				}

				if hasVolumeAttachment {
					Expect(foundOnNewNode).To(BeTrue(),
						fmt.Sprintf("PV %s is not attached to rescheduled node %s", pvName, rescheduledNodeName))
					Expect(foundOnOldNode).To(BeFalse(),
						fmt.Sprintf("PV %s is still attached to shut-down node %s", pvName, targetNode))
				} else {
					klog.Infof("No VolumeAttachment found for PV %s — storage driver does not use CSI attachments (e.g. NFS)",
						pvName)
				}
			})
	})

func isPodReady(podBuilder *pod.Builder) bool {
	for _, condition := range podBuilder.Object.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

func toFloat64(val any) (float64, error) {
	switch value := val.(type) {
	case float64:
		return value, nil
	case int64:
		return float64(value), nil
	case string:
		return strconv.ParseFloat(value, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}
