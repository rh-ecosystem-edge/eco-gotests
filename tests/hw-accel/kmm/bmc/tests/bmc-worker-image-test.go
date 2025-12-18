package tests

import (
	"fmt"
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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/reboot"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

func findReadyHelperPodOnNode(namespace, nodeName, containerName string) (*pod.Builder, error) {
	helperPods, err := pod.List(APIClient, namespace, metav1.ListOptions{
		LabelSelector: kmmparams.KmmTestHelperLabelName,
	})
	if err != nil {
		return nil, err
	}

	for _, helperPodCandidate := range helperPods {
		if helperPodCandidate.Object.Spec.NodeName != nodeName {
			continue
		}

		if helperPodCandidate.Object.Status.Phase != corev1.PodRunning {
			klog.V(kmmparams.KmmLogLevel).Infof(
				"Helper pod %s on node %s is in phase %s, waiting...",
				helperPodCandidate.Object.Name, nodeName, helperPodCandidate.Object.Status.Phase)

			continue
		}

		for _, cs := range helperPodCandidate.Object.Status.ContainerStatuses {
			if cs.Name == containerName && cs.Ready {
				klog.V(kmmparams.KmmLogLevel).Infof(
					"Helper pod %s container ready on node %s",
					helperPodCandidate.Object.Name, nodeName)

				return helperPodCandidate, nil
			}
		}

		klog.V(kmmparams.KmmLogLevel).Infof(
			"Helper pod %s on node %s container not ready yet",
			helperPodCandidate.Object.Name, nodeName)
	}

	return nil, fmt.Errorf("no ready helper pod found on node %s", nodeName)
}

var _ = Describe("KMM-BMC", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity, tsparams.LabelSuite), func() {

	BeforeAll(func() {
		By("Check KMM version is 2.5 or higher")
		currentVersion, err := get.KmmOperatorVersion(APIClient)
		Expect(err).ToNot(HaveOccurred(), "failed to get KMM operator version")

		minVersion, err := version.NewVersion("2.5.0")
		Expect(err).ToNot(HaveOccurred(), "failed to parse minimum version")

		if currentVersion.LessThan(minVersion) {
			Skip("BootModuleConfig tests require KMM version 2.5.0 or higher")
		}

		klog.V(kmmparams.KmmLogLevel).Infof("KMM version %s >= 2.5.0, proceeding with BMC tests", currentVersion)
	})

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
			By("Verify simple-kmod image exists for kernel version")
			_, err := get.KernelModuleImageExists(
				APIClient,
				kmmparams.SimpleKmodImage,
				GeneralConfig.WorkerLabelMap,
				kmmparams.KmmOperatorNamespace)
			if err != nil {
				Skip(fmt.Sprintf("Simple-kmod image not available: %v", err))
			}

			By("Create BootModuleConfig with empty workerImage")
			bmcBuilder = kmm.NewBootModuleConfigBuilder(APIClient, tsparams.BMCTestName, tsparams.BMCTestNamespace).
				WithKernelModuleImage(kmmparams.SimpleKmodImage).
				WithKernelModuleName(kmmparams.SimpleKmodModuleName).
				WithMachineConfigName(tsparams.MachineConfigName).
				WithMachineConfigPoolName(kmmparams.DefaultWorkerMCPName)

			_, err = bmcBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating bootmoduleconfig")

			By("Wait for MachineConfig to be created")
			err = await.MachineConfigCreated(APIClient, tsparams.MachineConfigName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig was not created in time")

			By("Verify MachineConfig contains WORKER_IMAGE from operator")
			workerImageValue, err := get.MachineConfigEnvVar(APIClient, tsparams.MachineConfigName, "WORKER_IMAGE")
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should contain WORKER_IMAGE")
			Expect(workerImageValue).ToNot(BeEmpty(), "WORKER_IMAGE value should not be empty")
			Expect(workerImageValue).To(MatchRegexp(`^[a-zA-Z0-9].*`),
				"WORKER_IMAGE should be a valid image reference")

			By("Get a worker node for reboot")
			nodeList, err := nodes.List(
				APIClient, metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing worker nodes")
			Expect(len(nodeList)).To(BeNumerically(">", 0), "no worker nodes found")

			workerNode := nodeList[0]
			klog.V(kmmparams.KmmLogLevel).Infof("Using worker node: %s", workerNode.Object.Name)

			By("Wait for MCO to render new config for the node")
			err = await.NodeDesiredConfigChange(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MCO did not render new config for node in time")

			By("Reboot the worker node")
			err = reboot.PerformReboot(APIClient, workerNode.Object.Name, kmmparams.KmmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to reboot node")

			klog.V(kmmparams.KmmLogLevel).Infof("Node %s is back up and Ready", workerNode.Object.Name)

			By("Wait for node to apply new config (currentConfig == desiredConfig)")
			err = await.NodeConfigApplied(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "node did not apply new config after reboot")

			By("Wait for helper pod on rebooted node to be ready and check module is loaded")
			var helperPod *pod.Builder
			Eventually(func() bool {
				foundPod, err := findReadyHelperPodOnNode(
					kmmparams.KmmOperatorNamespace, workerNode.Object.Name, "test")
				if err != nil {
					return false
				}
				helperPod = foundPod

				return true
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
			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigName)
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
