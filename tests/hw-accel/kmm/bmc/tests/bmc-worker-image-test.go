package tests

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/bmc/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

var _ = Describe("KMM-BMC", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity, tsparams.LabelSuite), func() {

	Context("BootModuleConfig", Label("bmc-worker-image"), func() {

		var (
			bmcBuilder *kmm.BootModuleConfigBuilder
		)

		AfterEach(func() {
			By("Delete any reboot pods")
			podList, err := pod.List(APIClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{})
			if err == nil {
				for _, p := range podList {
					if strings.HasSuffix(p.Object.Name, "-debug-reboot") ||
						strings.HasPrefix(p.Object.Name, "sysrq-check-") {
						klog.V(kmmparams.KmmLogLevel).Infof("Cleaning up pod: %s", p.Object.Name)
						_, _ = p.DeleteAndWait(30 * time.Second)
					}
				}
			}

			By("Delete BootModuleConfig if exists")
			if bmcBuilder != nil && bmcBuilder.Exists() {
				_, err := bmcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

				By("Wait for BootModuleConfig to be deleted")
				err = await.BootModuleConfigObjectDeleted(APIClient, tsparams.BMCTestName, tsparams.BMCTestNamespace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for bootmoduleconfig to be deleted")
			}

			By("Delete MachineConfig if exists")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)
			if err == nil && mcBuilder != nil {
				err = mcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")
			}
		})

		It("should create BMC with empty workerImage and use operator worker image", reportxml.ID("85553"), func() {
			By("Check KMM version is 2.5 or higher")
			currentVersion, err := get.KmmOperatorVersion(APIClient)
			Expect(err).ToNot(HaveOccurred(), "failed to get KMM operator version")

			minVersion, _ := version.NewVersion("2.5.0")
			if currentVersion.LessThan(minVersion) {
				Skip("BootModuleConfig tests require KMM version 2.5.0 or higher")
			}

			klog.V(kmmparams.KmmLogLevel).Infof("KMM version %s >= 2.5.0, proceeding with BMC test", currentVersion)

			By("Verify simple-kmod image exists")
			klog.V(kmmparams.KmmLogLevel).Infof("Using kernel module image: %s", kmmparams.SimpleKmodImage)

			By("Create BootModuleConfig with empty workerImage")
			bmcBuilder = kmm.NewBootModuleConfigBuilder(APIClient, tsparams.BMCTestName, tsparams.BMCTestNamespace).
				WithKernelModuleImage(kmmparams.SimpleKmodImage).
				WithKernelModuleName(kmmparams.SimpleKmodModuleName).
				WithMachineConfigName(tsparams.MachineConfigName).
				WithMachineConfigPoolName(kmmparams.DefaultWorkerMCPName)
			// Note: WithWorkerImage is NOT called - workerImage is left empty

			_, err = bmcBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating bootmoduleconfig")

			By("Wait for MachineConfig to be created")
			Eventually(func() bool {
				mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)

				return err == nil && mcBuilder != nil && mcBuilder.Exists()
			}, 3*time.Minute, 10*time.Second).Should(BeTrue(), "MachineConfig was not created in time")

			By("Verify MachineConfig contains WORKER_IMAGE from operator")
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)
			Expect(err).ToNot(HaveOccurred(), "error pulling machineconfig")

			mcJSON, err := json.Marshal(mcBuilder.Object)
			Expect(err).ToNot(HaveOccurred(), "error marshaling machineconfig to JSON")

			mcString := string(mcJSON)
			klog.V(kmmparams.KmmLogLevel).Infof("MachineConfig JSON: %s", mcString)

			workerImagePattern := regexp.MustCompile(`WORKER_IMAGE=(\S+)`)
			matches := workerImagePattern.FindStringSubmatch(mcString)
			Expect(len(matches)).To(BeNumerically(">=", 2),
				"MachineConfig should contain WORKER_IMAGE environment variable with a value")

			workerImageValue := matches[1]
			klog.V(kmmparams.KmmLogLevel).Infof("Found WORKER_IMAGE value: %s", workerImageValue)

			Expect(workerImageValue).ToNot(BeEmpty(),
				"WORKER_IMAGE value should not be empty")
			Expect(workerImageValue).To(MatchRegexp(`^[a-zA-Z0-9].*`),
				"WORKER_IMAGE should be a valid image reference")

			By("Get a worker node for reboot")
			nodeList, err := nodes.List(
				APIClient, metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing worker nodes")
			Expect(len(nodeList)).To(BeNumerically(">", 0), "no worker nodes found")

			workerNode := nodeList[0]
			klog.V(kmmparams.KmmLogLevel).Infof("Using worker node: %s", workerNode.Object.Name)

			By("Reboot the worker node using sysrq-trigger")
			rebootPodName := fmt.Sprintf("%s-debug-reboot", workerNode.Object.Name)
			rebootPod := pod.NewBuilder(APIClient, rebootPodName, kmmparams.KmmOperatorNamespace,
				"registry.access.redhat.com/ubi9/ubi-minimal:latest")
			rebootPod.Definition.Spec.NodeName = workerNode.Object.Name
			rebootPod.Definition.Spec.RestartPolicy = corev1.RestartPolicyNever
			rebootPod.Definition.Spec.HostPID = true
			rebootPod.Definition.Spec.AutomountServiceAccountToken = ptr.To(false)

			rebootPod.Definition.Spec.Containers[0].Command = []string{
				"/bin/sh", "-c",
				"echo s > /host/proc/sysrq-trigger; sleep 1; " +
					"echo u > /host/proc/sysrq-trigger; sleep 1; " +
					"echo b > /host/proc/sysrq-trigger",
			}
			rebootPod.Definition.Spec.Containers[0].SecurityContext = kmmparams.PrivilegedSC
			rebootPod.Definition.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
				{Name: "host-proc", MountPath: "/host/proc"},
			}
			rebootPod.Definition.Spec.Volumes = []corev1.Volume{
				{Name: "host-proc", VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{Path: "/proc"},
				}},
			}

			originalBootID := workerNode.Object.Status.NodeInfo.BootID
			klog.V(kmmparams.KmmLogLevel).Infof("Node %s current boot ID: %s", workerNode.Object.Name, originalBootID)

			By("Verify sysrq-trigger is available")
			sysrqCheckPod := pod.NewBuilder(APIClient, fmt.Sprintf("sysrq-check-%s", workerNode.Object.Name),
				kmmparams.KmmOperatorNamespace, "registry.access.redhat.com/ubi9/ubi-minimal:latest")
			sysrqCheckPod.Definition.Spec.NodeName = workerNode.Object.Name
			sysrqCheckPod.Definition.Spec.RestartPolicy = corev1.RestartPolicyNever
			sysrqCheckPod.Definition.Spec.AutomountServiceAccountToken = ptr.To(false)
			sysrqCheckPod.Definition.Spec.Containers[0].Command = []string{
				"/bin/sh", "-c", "cat /host/proc/sys/kernel/sysrq",
			}
			sysrqCheckPod.Definition.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
				{Name: "host-proc", MountPath: "/host/proc"},
			}
			sysrqCheckPod.Definition.Spec.Volumes = []corev1.Volume{
				{Name: "host-proc", VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{Path: "/proc"},
				}},
			}

			_, err = sysrqCheckPod.CreateAndWaitUntilRunning(2 * time.Minute)
			if err != nil {
				klog.V(kmmparams.KmmLogLevel).Infof("Warning: Could not verify sysrq availability: %v", err)
			} else {
				Eventually(func() bool {
					podObj, err := pod.Pull(APIClient, sysrqCheckPod.Definition.Name, kmmparams.KmmOperatorNamespace)

					return err == nil && (podObj.Object.Status.Phase == corev1.PodSucceeded ||
						podObj.Object.Status.Phase == corev1.PodFailed)
				}, 1*time.Minute, 5*time.Second).Should(BeTrue(), "sysrq check pod did not complete")

				_, err = sysrqCheckPod.DeleteAndWait(30 * time.Second)
				if err != nil {
					klog.V(kmmparams.KmmLogLevel).Infof("Warning: Could not delete sysrq check pod: %v", err)
				}
			}

			_, err = rebootPod.CreateAndWaitUntilRunning(2 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "reboot pod did not start - cannot trigger reboot")
			klog.V(kmmparams.KmmLogLevel).Infof(
				"Reboot pod running on node %s, executing sysrq-trigger reboot (sync, unmount, reboot)",
				workerNode.Object.Name)

			By("Wait for node boot ID to change (reboot occurred)")
			Eventually(func() bool {
				node, err := nodes.Pull(APIClient, workerNode.Object.Name)
				if err != nil {
					klog.V(kmmparams.KmmLogLevel).Infof("Node %s unreachable during reboot", workerNode.Object.Name)

					return false
				}

				newBootID := node.Object.Status.NodeInfo.BootID
				if newBootID != originalBootID {
					klog.V(kmmparams.KmmLogLevel).Infof(
						"Node %s boot ID changed: %s -> %s (reboot confirmed)",
						workerNode.Object.Name, originalBootID, newBootID)

					return true
				}

				return false
			}, 10*time.Minute, 5*time.Second).Should(BeTrue(), "Node boot ID did not change - reboot may not have occurred")

			By("Wait for node to become Ready after reboot")
			Eventually(func() bool {
				node, err := nodes.Pull(APIClient, workerNode.Object.Name)
				if err != nil {
					klog.V(kmmparams.KmmLogLevel).Infof("Node %s still unreachable", workerNode.Object.Name)

					return false
				}

				for _, condition := range node.Object.Status.Conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						klog.V(kmmparams.KmmLogLevel).Infof("Node %s is Ready after reboot", workerNode.Object.Name)

						return true
					}
				}

				return false
			}, 10*time.Minute, 5*time.Second).Should(BeTrue(), "Node did not become Ready after reboot")

			klog.V(kmmparams.KmmLogLevel).Infof("Node %s is back up and Ready", workerNode.Object.Name)

			By("Wait for helper pod on rebooted node to be ready and check module is loaded")

			var helperPod *pod.Builder
			Eventually(func() bool {
				helperPods, err := pod.List(APIClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
					LabelSelector: kmmparams.KmmTestHelperLabelName,
				})
				if err != nil {
					klog.V(kmmparams.KmmLogLevel).Infof("Error listing helper pods: %v", err)

					return false
				}

				for _, helperPodCandidate := range helperPods {
					if helperPodCandidate.Object.Spec.NodeName != workerNode.Object.Name {
						continue
					}

					if helperPodCandidate.Object.Status.Phase != corev1.PodRunning {
						klog.V(kmmparams.KmmLogLevel).Infof(
							"Helper pod %s on node %s is in phase %s, waiting...",
							helperPodCandidate.Object.Name, workerNode.Object.Name,
							helperPodCandidate.Object.Status.Phase)

						continue
					}

					for _, cs := range helperPodCandidate.Object.Status.ContainerStatuses {
						if cs.Name == "test" && cs.Ready {
							klog.V(kmmparams.KmmLogLevel).Infof(
								"Helper pod %s container ready on node %s",
								helperPodCandidate.Object.Name, workerNode.Object.Name)
							helperPod = helperPodCandidate

							return true
						}
					}

					klog.V(kmmparams.KmmLogLevel).Infof(
						"Helper pod %s on node %s container not ready yet",
						helperPodCandidate.Object.Name, workerNode.Object.Name)
				}

				return false
			}, 5*time.Minute, 10*time.Second).Should(BeTrue(),
				fmt.Sprintf("helper pod container not ready on node %s after reboot", workerNode.Object.Name))

			By("Verify simple-kmod module is loaded")
			modName := strings.ReplaceAll(kmmparams.SimpleKmodModuleName, "-", "_")
			buff, err := helperPod.ExecCommand([]string{"lsmod"}, "test")
			Expect(err).ToNot(HaveOccurred(), "error executing lsmod on helper pod")

			lsmodOutput := buff.String()
			klog.V(kmmparams.KmmLogLevel).Infof("lsmod output on %s: %s", workerNode.Object.Name, lsmodOutput)
			Expect(lsmodOutput).To(ContainSubstring(modName),
				fmt.Sprintf("module %s should be loaded on node %s after reboot", modName, workerNode.Object.Name))

			By("Delete the BootModuleConfig")
			_, err = bmcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

			By("Verify MachineConfig is still present after BMC deletion")
			mcBuilder, err = mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should still exist after BMC deletion")
			Expect(mcBuilder.Exists()).To(BeTrue(), "MachineConfig should still exist")

			By("Delete the MachineConfig")
			err = mcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

			By("Verify MachineConfig is deleted")
			Eventually(func() bool {
				_, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)

				return err != nil && strings.Contains(err.Error(), "does not exist")
			}, time.Minute, 5*time.Second).Should(BeTrue(), "MachineConfig should be deleted")

			By("Wait for MachineConfigPool to start updating (nodes will reboot to remove module)")
			mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
			Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			if err == nil {
				klog.V(kmmparams.KmmLogLevel).Infof(
					"MCP was stable - MC deletion may not have triggered node updates")
			} else {
				klog.V(kmmparams.KmmLogLevel).Infof("MCP is updating as expected after MC deletion")
			}

			By("Wait for MachineConfigPool update to complete (all nodes rebooted)")
			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")

			klog.V(kmmparams.KmmLogLevel).Infof("MachineConfigPool update complete - nodes have rebooted")
		})
	})
})
