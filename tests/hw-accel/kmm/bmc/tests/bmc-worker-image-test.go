package tests

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/bmc/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

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

			_, err := check.ImageExists(
				APIClient,
				kmmparams.SimpleKmodImage,
				GeneralConfig.WorkerLabelMap)
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

			nodeList, err := nodes.List(APIClient, metav1.ListOptions{
				LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing worker nodes")
			Expect(len(nodeList)).To(BeNumerically(">", 0), "no worker nodes found")

			workerNode := nodeList[0]
			klog.V(kmmparams.KmmLogLevel).Infof("Using worker node: %s", workerNode.Object.Name)

			By("Wait for MCO to write new config to disk")

			err = await.NodeDesiredConfigChange(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MCO did not write new config for node in time")

			By("Reboot the worker node to apply BMC config")

			err = do.Reboot(APIClient, workerNode.Object.Name, kmmparams.KmmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to reboot node")

			By("Wait for node to come back up and be Ready")

			err = await.NodeConfigApplied(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "node did not become Ready after reboot")

			By("Wait for helper pod on rebooted node to be ready")

			_, err = await.ReadyHelperPod(APIClient, kmmparams.KmmOperatorNamespace,
				workerNode.Object.Name, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "helper pod not ready on node after reboot")

			By("Verify simple-kmod module is loaded on the node (BMC loads it during boot)")

			err = check.ModuleLoadedOnNode(APIClient, kmmparams.SimpleKmodModuleName, time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "module should be loaded by BMC during boot")

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

			err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

			By("Wait for MachineConfigPool to start updating (nodes will reboot to remove module)")

			mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
			Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

			By("Wait for MachineConfigPool update to complete (all nodes rebooted)")

			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
		})
	})

	Context("BMC with inTreeModulesToRemove", Label("bmc-intree-remove"), func() {
		var (
			bmcBuilder *kmm.BootModuleConfigBuilder
			workerNode *nodes.Builder
		)

		AfterEach(func() {
			By("Delete BootModuleConfig if exists")

			if bmcBuilder != nil && bmcBuilder.Exists() {
				_, err := bmcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

				By("Wait for BootModuleConfig to be deleted")

				err = await.BootModuleConfigObjectDeleted(APIClient,
					tsparams.BMCInTreeRemoveName, tsparams.BMCTestNamespace, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for bootmoduleconfig to be deleted")
			}

			By("Delete MachineConfig if exists")

			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigInTreeRemoveName)
			if err == nil && mcBuilder != nil {
				err = mcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

				By("Verify MachineConfig is deleted")

				err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigInTreeRemoveName, time.Minute)
				Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

				By("Wait for MachineConfigPool to start updating")

				mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
				Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

				err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
				Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

				By("Wait for MachineConfigPool update to complete")

				err = mcp.WaitForUpdate(30 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
			}
		})

		It("should remove in-tree module and load OOT module", reportxml.ID("85558"), func() {
			By("Get cluster architecture and determine in-tree module to remove")

			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error getting cluster architecture")

			inTreeModule := get.InTreeModuleToRemove(arch)
			klog.V(kmmparams.KmmLogLevel).Infof("Architecture: %s, in-tree module to remove: %s", arch, inTreeModule)

			By("Get a worker node for testing")

			nodeList, err := nodes.List(APIClient, metav1.ListOptions{
				LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "error listing worker nodes")
			Expect(len(nodeList)).To(BeNumerically(">", 0), "no worker nodes found")

			workerNode = nodeList[0]
			klog.V(kmmparams.KmmLogLevel).Infof("Using worker node: %s", workerNode.Object.Name)

			By("Check if in-tree module exists on node")

			moduleExists, _ := check.ModuleExistsOnNode(APIClient, inTreeModule, workerNode.Object.Name)

			if !moduleExists {
				Skip(fmt.Sprintf("Module %s does not exist on node %s, skipping test",
					inTreeModule, workerNode.Object.Name))
			}

			klog.V(kmmparams.KmmLogLevel).Infof("Module %s exists on node %s", inTreeModule, workerNode.Object.Name)

			By("Verify simple-kmod image exists for kernel version")

			_, err = check.ImageExists(
				APIClient,
				kmmparams.SimpleKmodImage,
				GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip(fmt.Sprintf("Simple-kmod image not available: %v", err))
			}

			By("Create BMC with inTreeModulesToRemove")

			bmcBuilder = kmm.NewBootModuleConfigBuilder(APIClient,
				tsparams.BMCInTreeRemoveName, tsparams.BMCTestNamespace).
				WithKernelModuleImage(kmmparams.SimpleKmodImage).
				WithKernelModuleName(kmmparams.SimpleKmodModuleName).
				WithMachineConfigName(tsparams.MachineConfigInTreeRemoveName).
				WithMachineConfigPoolName(kmmparams.DefaultWorkerMCPName).
				WithInTreeModulesToRemove([]string{inTreeModule})

			_, err = bmcBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating bootmoduleconfig")

			By("Wait for MachineConfig to be created")

			err = await.MachineConfigCreated(APIClient, tsparams.MachineConfigInTreeRemoveName, 3*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig was not created in time")

			By("Verify MachineConfig contains IN_TREE_MODULES_TO_REMOVE")

			inTreeValue, err := get.MachineConfigEnvVar(APIClient,
				tsparams.MachineConfigInTreeRemoveName, "IN_TREE_MODULES_TO_REMOVE")
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should contain IN_TREE_MODULES_TO_REMOVE")
			Expect(inTreeValue).To(ContainSubstring(inTreeModule),
				"IN_TREE_MODULES_TO_REMOVE should contain the in-tree module")

			By("Wait for MCO to write new config to disk")

			err = await.NodeDesiredConfigChange(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MCO did not write new config for node in time")

			By("Reboot the worker node to apply BMC config")

			err = do.Reboot(APIClient, workerNode.Object.Name, kmmparams.KmmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed to reboot node")

			By("Wait for node to come back up and be Ready")

			err = await.NodeConfigApplied(APIClient, workerNode.Object.Name, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "node did not become Ready after reboot")

			By("Wait for helper pod on rebooted node to be ready")

			_, err = await.ReadyHelperPod(APIClient, kmmparams.KmmOperatorNamespace,
				workerNode.Object.Name, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "helper pod not ready on node after reboot")

			By("Verify previously loaded in-tree module is NOT loaded after reboot")

			err = check.ModuleNotLoadedOnNode(APIClient, inTreeModule, time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "in-tree module should be removed by BMC")

			By("Verify simple-kmod module IS loaded after reboot")

			err = check.ModuleLoadedOnNode(APIClient, kmmparams.SimpleKmodModuleName, time.Minute, workerNode.Object.Name)
			Expect(err).ToNot(HaveOccurred(), "simple-kmod module should be loaded by BMC")

			By("Delete the BootModuleConfig")

			_, err = bmcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting bootmoduleconfig")

			By("Verify MachineConfig is still present after BMC deletion")

			mcBuilder, err := mco.PullMachineConfig(APIClient, tsparams.MachineConfigInTreeRemoveName)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should still exist after BMC deletion")
			Expect(mcBuilder.Exists()).To(BeTrue(), "MachineConfig should still exist")

			By("Delete the MachineConfig")

			err = mcBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting machineconfig")

			By("Verify MachineConfig is deleted")

			err = await.MachineConfigDeleted(APIClient, tsparams.MachineConfigInTreeRemoveName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "MachineConfig should be deleted")

			By("Wait for MachineConfigPool to start updating (nodes will reboot)")

			mcp, err := mco.Pull(APIClient, kmmparams.DefaultWorkerMCPName)
			Expect(err).ToNot(HaveOccurred(), "error pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			Expect(err).To(HaveOccurred(), "the MachineConfig deletion did not trigger a MCP update")

			By("Wait for MachineConfigPool update to complete (all nodes rebooted)")

			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for machineconfigpool to finish updating")
		})
	})
})
