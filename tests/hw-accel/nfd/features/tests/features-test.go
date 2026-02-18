package tests

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/machine"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/helpers"
	ts "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfddelete"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("NFD", Ordered, func() {
	nfdConfig := nfdconfig.NewNfdConfig()

	Context("Node featues", Label("discovery-of-labels"), func() {
		var cpuFlags map[string][]string

		BeforeAll(func() {
			By("Verifying NFD is ready (installed by suite)")
			By("Check that pods are in running state")
			res, err := wait.ForPodsRunning(APIClient, 3*time.Minute, hwaccelparams.NFDNamespace)
			Expect(err).ShouldNot(HaveOccurred(), "NFD pods should be running")
			Expect(res).To(BeTrue(), "NFD pods should be running")

			By("Waiting for feature labels to appear")
			Eventually(func() bool {
				labelExist, labelsError := wait.CheckLabel(APIClient, 1*time.Minute, "feature")
				if labelsError != nil {
					klog.V(ts.LogLevel).Infof("Checking for labels, error: %v", labelsError)
				}

				return labelExist
			}).WithTimeout(5*time.Minute).WithPolling(10*time.Second).Should(BeTrue(),
				"Feature labels should be present after NFD is running")
		})

		It("Check pods state", reportxml.ID("54548"), func() {
			err := helpers.CheckPodStatus(APIClient)
			Expect(err).NotTo(HaveOccurred())

		})
		It("Check CPU feature labels", reportxml.ID("54222"), func() {
			// Skip check removed - NFD is already running from BeforeSuite
			if nfdConfig.CPUFlagsHelperImage == "" {
				Skip("CPUFlagsHelperImage is not set.")
			}
			cpuFlags = get.CPUFlags(APIClient, hwaccelparams.NFDNamespace, nfdConfig.CPUFlagsHelperImage)
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)

			Expect(err).NotTo(HaveOccurred())

			By("Check if features exists")

			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels, cpuFlags[nodeName], nil, nodeName)
				Expect(err).NotTo(HaveOccurred())
			}

		})

		It("Check Kernel config", reportxml.ID("54471"), func() {
			// Skip check removed - NFD is already running from BeforeSuite
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			By("Check if custom label topology is exist")
			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels, ts.KernelConfig, nil, nodeName)
				Expect(err).NotTo(HaveOccurred())
			}

		})

		It("Check topology", reportxml.ID("54491"), func() {
			Skip("configuration issue")
			skipIfConfigNotSet(nfdConfig)
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			By("Check if NFD labeling of the kernel config flags")
			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels, ts.Topology, nil, nodeName)
				Expect(err).NotTo(HaveOccurred())
			}

		})
		It("Check Logs", reportxml.ID("54549"), func() {
			errorKeywords := []string{"error", "exception", "failed"}
			// Skip check removed - NFD is already running from BeforeSuite
			listOptions := metav1.ListOptions{
				AllowWatchBookmarks: false,
			}
			By("Check if NFD pod's log not contains in error messages")
			pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
			Expect(err).NotTo(HaveOccurred())
			for _, nfdPod := range pods {
				klog.V(ts.LogLevel).Infof("retrieve logs from %v", nfdPod.Object.Name)
				log, err := get.PodLogs(APIClient, hwaccelparams.NFDNamespace, nfdPod.Object.Name)
				Expect(err).NotTo(HaveOccurred(), "Error retrieving pod logs.")
				Expect(len(log)).NotTo(Equal(0))
				for _, errorKeyword := range errorKeywords {

					logLines := strings.Split(log, "\n")
					for _, line := range logLines {
						if strings.Contains(errorKeyword, line) {
							klog.Errorf("error found in log: %v", line)
						}
					}

				}

			}

		})

		It("Check Restart Count", reportxml.ID("54538"), func() {
			// Check that pods are stable (not restarting unexpectedly)
			// Note: This test verifies pods don't restart during observation period
			// It accounts for controlled restarts from resilience tests
			listOptions := metav1.ListOptions{
				AllowWatchBookmarks: false,
			}

			By("Recording initial restart counts")
			pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
			Expect(err).NotTo(HaveOccurred())

			initialRestartCounts := make(map[string]int32)
			for _, nfdPod := range pods {
				resetCount, err := get.PodRestartCount(APIClient, hwaccelparams.NFDNamespace, nfdPod.Object.Name)
				Expect(err).NotTo(HaveOccurred(), "Error retrieving reset count.")
				initialRestartCounts[nfdPod.Object.Name] = resetCount
				klog.V(ts.LogLevel).Infof("Pod %v initial restart count: %d", nfdPod.Object.Name, resetCount)
			}

			By("Waiting 30 seconds to verify pod stability")
			time.Sleep(30 * time.Second)

			By("Verifying restart counts have not increased (pods are stable)")
			pods, err = pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
			Expect(err).NotTo(HaveOccurred())

			for _, nfdPod := range pods {
				currentCount, err := get.PodRestartCount(APIClient, hwaccelparams.NFDNamespace, nfdPod.Object.Name)
				Expect(err).NotTo(HaveOccurred(), "Error retrieving reset count.")

				initialCount := initialRestartCounts[nfdPod.Object.Name]
				klog.V(ts.LogLevel).Infof("Pod %v: initial=%d, current=%d", nfdPod.Object.Name, initialCount, currentCount)

				Expect(currentCount).To(Equal(initialCount),
					"Pod %s restart count increased unexpectedly from %d to %d",
					nfdPod.Object.Name, initialCount, currentCount)
			}
		})

		It("Check if NUMA detected ", reportxml.ID("54408"), func() {
			Skip("configuration issue")
			skipIfConfigNotSet(nfdConfig)
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())
			By("Check if NFD labeling nodes with custom NUMA labels")
			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels, ts.NUMA, nil, nodeName)
				Expect(err).NotTo(HaveOccurred())
			}

		})

		It("Verify Feature List not contains items from Blacklist ", reportxml.ID("68298"), func() {
			skipIfConfigNotSet(nfdConfig)
			By("delete old instance")

			err := ts.SharedNFDCRUtils.DeleteNFDCR()
			Expect(err).NotTo(HaveOccurred())

			err = nfddelete.NfdLabelsByKeys(APIClient, "nfd.node.kubernetes.io", "feature.node.kubernetes.io")
			Expect(err).NotTo(HaveOccurred())

			By("waiting for new image")
			set.CPUConfigLabels(APIClient,
				[]string{"BMI2"},
				nil,
				true,
				hwaccelparams.NFDNamespace,
				nfdConfig.Image)

			labelExist, labelsError := wait.CheckLabel(APIClient, 15*time.Minute, "feature")
			if !labelExist || labelsError != nil {
				klog.Errorf("feature labels was not found in the given time error=%v", labelsError)
			}

			err = ts.SharedNFDCRUtils.PrintCr()
			if err != nil {
				klog.Errorf("error in printing NFD CR: %v", err)
			}

			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			klog.V(ts.LogLevel).Infof("Received nodelabel: %v", nodelabels)
			Expect(err).NotTo(HaveOccurred())
			By("Check if features exists")
			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels, []string{"BMI2"}, nil, nodeName)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Restoring original NFD CR configuration")
			err = ts.SharedNFDCRUtils.DeleteNFDCR()
			Expect(err).NotTo(HaveOccurred())

			err = nfddelete.NfdLabelsByKeys(APIClient, "nfd.node.kubernetes.io", "feature.node.kubernetes.io")
			Expect(err).NotTo(HaveOccurred())

			// Recreate original CR without blacklist
			crConfig := deploy.NFDCRConfig{
				Image:          nfdConfig.Image,
				EnableTopology: true,
			}
			err = ts.SharedNFDCRUtils.DeployNFDCR(crConfig)
			Expect(err).NotTo(HaveOccurred())

			// Wait for CR to be ready again
			crReady, err := ts.SharedNFDCRUtils.IsNFDCRReady(5 * time.Minute)
			Expect(err).NotTo(HaveOccurred())
			Expect(crReady).To(BeTrue(), "NFD CR should be restored to original state")

		})

		It("Verify Feature List contains only Whitelist", reportxml.ID("68300"), func() {
			skipIfConfigNotSet(nfdConfig)

			if nfdConfig.CPUFlagsHelperImage == "" {
				Skip("CPUFlagsHelperImage is not set.")
			}
			By("delete old instance")

			err := ts.SharedNFDCRUtils.DeleteNFDCR()
			Expect(err).NotTo(HaveOccurred())

			err = nfddelete.NfdLabelsByKeys(APIClient, "nfd.node.kubernetes.io", "feature.node.kubernetes.io")
			Expect(err).NotTo(HaveOccurred())

			By("waiting for new image")
			set.CPUConfigLabels(APIClient,
				nil,
				[]string{"BMI2"},
				true,
				hwaccelparams.NFDNamespace,
				nfdConfig.Image)

			labelExist, labelsError := wait.CheckLabel(APIClient, time.Minute*15, "feature")
			if !labelExist || labelsError != nil {
				klog.Errorf("feature labels was not found in the given time error=%v", labelsError)
			}

			err = ts.SharedNFDCRUtils.PrintCr()
			if err != nil {
				klog.Errorf("error in printing NFD CR: %v", err)
			}

			cpuFlags = get.CPUFlags(APIClient, hwaccelparams.NFDNamespace, nfdConfig.CPUFlagsHelperImage)
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())
			By("Check if features exists")
			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels, []string{"BMI2"}, cpuFlags[nodeName], nodeName)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Restoring original NFD CR configuration")
			err = ts.SharedNFDCRUtils.DeleteNFDCR()
			Expect(err).NotTo(HaveOccurred())

			err = nfddelete.NfdLabelsByKeys(APIClient, "nfd.node.kubernetes.io", "feature.node.kubernetes.io")
			Expect(err).NotTo(HaveOccurred())

			// Recreate original CR without whitelist
			crConfig := deploy.NFDCRConfig{
				Image:          nfdConfig.Image,
				EnableTopology: true,
			}
			err = ts.SharedNFDCRUtils.DeployNFDCR(crConfig)
			Expect(err).NotTo(HaveOccurred())

			// Wait for CR to be ready again
			crReady, err := ts.SharedNFDCRUtils.IsNFDCRReady(5 * time.Minute)
			Expect(err).NotTo(HaveOccurred())
			Expect(crReady).To(BeTrue(), "NFD CR should be restored to original state")

		})

		It("Add day2 workers", reportxml.ID("54539"), func() {
			skipIfConfigNotSet(nfdConfig)
			if !nfdConfig.AwsTest {
				Skip("This test works only on AWS cluster." +
					"Set ECO_HWACCEL_NFD_AWS_TESTS=true when running NFD tests against AWS cluster. ")
			}

			if nfdConfig.CPUFlagsHelperImage == "" {
				Skip("CPUFlagsHelperImage is not set.")
			}
			By("Creating machine set")
			msBuilder := machine.NewSetBuilderFromCopy(APIClient, ts.MachineSetNamespace, ts.InstanceType,
				ts.WorkerMachineSetLabel, ts.Replicas)
			Expect(msBuilder).NotTo(BeNil(), "Failed to Initialize MachineSetBuilder from copy")

			By("Create the new MachineSet")
			createdMsBuilder, err := msBuilder.Create()

			Expect(err).ToNot(HaveOccurred(), "error creating a machineset: %v", err)

			pulledMachineSetBuilder, err := machine.PullSet(APIClient,
				createdMsBuilder.Definition.ObjectMeta.Name,
				ts.MachineSetNamespace)

			Expect(err).ToNot(HaveOccurred(), "error pulling machineset: %v", err)

			By("Wait on machineset to be ready")

			err = machine.WaitForMachineSetReady(APIClient, createdMsBuilder.Definition.ObjectMeta.Name,
				ts.MachineSetNamespace, 15*time.Minute)

			Expect(err).ToNot(HaveOccurred(),
				"Failed to detect at least one replica of MachineSet %s in Ready state during 15 min polling interval: %v",
				pulledMachineSetBuilder.Definition.ObjectMeta.Name,
				err)

			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)

			Expect(err).NotTo(HaveOccurred())

			By("check node readiness")

			isNodeReady, err := wait.ForNodeReadiness(APIClient, 10*time.Minute, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(isNodeReady).To(BeTrue(), "the new node is not ready for use")

			By("Check if features exists")
			cpuFlags = get.CPUFlags(APIClient, hwaccelparams.NFDNamespace, nfdConfig.CPUFlagsHelperImage)
			for nodeName := range nodelabels {
				klog.V(ts.LogLevel).Infof("checking labels in %v", nodeName)
				err = helpers.CheckLabelsExist(nodelabels, cpuFlags[nodeName], nil, nodeName)
				Expect(err).NotTo(HaveOccurred())
			}
			defer func() {
				err := pulledMachineSetBuilder.Delete()
				Expect(err).ToNot(HaveOccurred())
			}()

		})

	})
})

func skipIfConfigNotSet(nfdConfig *nfdconfig.NfdConfig) {
	if nfdConfig.CatalogSource == "" {
		Skip("The catalog source is not set.")
	}
}
