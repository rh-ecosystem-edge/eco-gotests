package tests

import (
	"fmt"
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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/client-go/util/retry"
)

var _ = Describe("NFD", Ordered, func() {
	nfdConfig := nfdconfig.NewNfdConfig()

	var nfdInstaller *deploy.OperatorInstaller

	var nfdCRUtils *deploy.NFDCRUtils

	BeforeAll(func() {
		var options *nfdhelpers.NFDInstallConfigOptions

		if nfdConfig.CatalogSource != "" {
			options = &nfdhelpers.NFDInstallConfigOptions{
				CatalogSource: nfdhelpers.StringPtr(nfdConfig.CatalogSource),
			}
		}

		installConfig := nfdhelpers.GetDefaultNFDInstallConfig(APIClient, options)

		nfdInstaller = deploy.NewOperatorInstaller(installConfig)

		nfdCRUtils = deploy.NewNFDCRUtils(APIClient, installConfig.Namespace, nfdparams.NfdInstance)

	})

	Context("Node featues", Label("discovery-of-labels"), func() {
		var cpuFlags map[string][]string

		AfterAll(func() {

			err := nfdCRUtils.DeleteNFDCR()
			Expect(err).NotTo(HaveOccurred())

			By("Undeploy NFD instance")
			uninstallConfig := nfdhelpers.GetDefaultNFDUninstallConfig(
				APIClient,
				"nfd-operator-group",
				"nfd-subscription")
			nfdUninstaller := deploy.NewOperatorUninstaller(uninstallConfig)
			err = nfdUninstaller.Uninstall()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in Undeploy NFD %s", err))
		})

		BeforeAll(func() {
			By("Clear labels")
			err := nfddelete.NfdLabelsByKeys(APIClient, "nfd.node.kubernetes.io", "feature.node.kubernetes.io")
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in cleaning labels\n %s", err))

			By("Creating nfd")
			runNodeDiscoveryAndTestLabelExistence(nfdInstaller, nfdCRUtils, nfdConfig, true)

			labelExist, labelsError := wait.CheckLabel(APIClient, 15*time.Minute, "feature")
			if !labelExist || labelsError != nil {
				klog.Errorf("feature labels was not found in the given time error=%v", labelsError)
			}
		})

		It("Check pods state", reportxml.ID("54548"), func() {
			err := helpers.CheckPodStatus(APIClient)
			Expect(err).NotTo(HaveOccurred())

		})
		It("Check CPU feature labels", reportxml.ID("54222"), func() {
			skipIfConfigNotSet(nfdConfig)

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
			skipIfConfigNotSet(nfdConfig)
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
			skipIfConfigNotSet(nfdConfig)
			listOptions := metav1.ListOptions{
				AllowWatchBookmarks: false,
			}
			By("Check if NFD pod's log not contains in error messages")
			pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
			Expect(err).NotTo(HaveOccurred())
			for _, p := range pods {
				klog.V(ts.LogLevel).Infof("retrieve logs from %v", p.Object.Name)
				log, err := get.PodLogs(APIClient, hwaccelparams.NFDNamespace, p.Object.Name)
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
			skipIfConfigNotSet(nfdConfig)
			listOptions := metav1.ListOptions{
				AllowWatchBookmarks: false,
			}
			By("Check if NFD pods reset count equal to zero")
			pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
			Expect(err).NotTo(HaveOccurred())
			for _, p := range pods {
				klog.V(ts.LogLevel).Infof("retrieve reset count from %v.", p.Object.Name)
				resetCount, err := get.PodRestartCount(APIClient, hwaccelparams.NFDNamespace, p.Object.Name)
				Expect(err).NotTo(HaveOccurred(), "Error retrieving reset count.")
				klog.V(ts.LogLevel).Infof("Total resets %d.", resetCount)
				Expect(resetCount).To(Equal(int32(0)))

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

			err := nfdCRUtils.DeleteNFDCR()
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

			err = nfdCRUtils.PrintCr()
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

		})

		It("Verify Feature List contains only Whitelist", reportxml.ID("68300"), func() {
			skipIfConfigNotSet(nfdConfig)

			if nfdConfig.CPUFlagsHelperImage == "" {
				Skip("CPUFlagsHelperImage is not set.")
			}
			By("delete old instance")

			err := nfdCRUtils.DeleteNFDCR()
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

			err = nfdCRUtils.PrintCr()
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

func runNodeDiscoveryAndTestLabelExistence(
	nfdInstaller *deploy.OperatorInstaller,

	nfdCRUtils *deploy.NFDCRUtils,

	nfdConfig *nfdconfig.NfdConfig,
	enableTopology bool) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := get.PodStatus(APIClient, hwaccelparams.NFDNamespace)
		klog.Error(err)

		return err
	})

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in deploying %s", err))

	err = nfdInstaller.Install()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in deploying NFD operator: %s", err))

	ready, err := nfdInstaller.IsReady(15 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error checking NFD operator readiness: %s", err))
	Expect(ready).To(BeTrue(), "NFD operator is not ready")

	nfdCRConfig := deploy.NFDCRConfig{

		EnableTopology: enableTopology,
		Image:          nfdConfig.Image,
	}
	err = nfdCRUtils.DeployNFDCR(nfdCRConfig)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error deploying NFD CR: %s", err))

	crReady, err := nfdCRUtils.IsNFDCRReady(10 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error checking NFD CR readiness: %s", err))
	Expect(crReady).To(BeTrue(), "NFD CR is not ready")

	By("Check that pods are in running state")

	res, err := wait.ForPodsRunning(APIClient, 15*time.Minute, hwaccelparams.NFDNamespace)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(res).To(BeTrue())
	By("Check feature labels exists")
	Eventually(func() []string {
		nodesWithLabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
		Expect(err).ShouldNot(HaveOccurred())

		allNodeLabels := []string{}
		for _, labels := range nodesWithLabels {
			allNodeLabels = append(allNodeLabels, labels...)
		}

		return allNodeLabels
	}).WithTimeout(5 * time.Minute).ShouldNot(HaveLen(0))
}

func skipIfConfigNotSet(nfdConfig *nfdconfig.NfdConfig) {
	if nfdConfig.CatalogSource == "" {
		Skip("The catalog source is not set.")
	}
}
