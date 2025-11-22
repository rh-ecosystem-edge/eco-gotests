package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Operator images", Label("clamav"), func() {
		relatedImgMap := map[string]string{}
		scanner := "scanner"
		image := fmt.Sprintf("%s/%s/%s:latest",
			tsparams.LocalImageRegistry, kmmparams.ScannerTestNamespace, scanner)
		var containerImg []string
		var dockerfileConfigMap *configmap.Builder

		BeforeAll(func() {
			By("Create Namespace")
			_, err := namespace.NewBuilder(APIClient, kmmparams.ScannerTestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating namespace")

			By("Create Configmap")
			configmMapContents := define.KmmScannerConfigMapContents()
			dockerfileConfigMap, err = configmap.NewBuilder(APIClient, scanner, kmmparams.ScannerTestNamespace).
				WithData(configmMapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")
		})

		AfterAll(func() {
			By("Delete Module")
			_, err := kmm.NewModuleBuilder(APIClient, scanner, kmmparams.ScannerTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting scanner module")

			By("Await module to be deleted")
			err = await.ModuleObjectDeleted(APIClient, scanner, kmmparams.ScannerTestNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			By("Delete namespace")
			err = namespace.NewBuilder(APIClient, kmmparams.ScannerTestNamespace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
		})

		It("should pass malware testing", reportxml.ID("68147"), func() {
			By("Obtain KMM images for test")
			pods, _ := pod.List(APIClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
				FieldSelector: "status.phase=Running",
			})

			for _, pod := range pods {
				for _, container := range pod.Object.Spec.Containers {
					containerImg = append(containerImg, container.Image)
					for _, env := range container.Env {
						if strings.Contains(env.Name, kmmparams.RelImgSign) {
							relatedImgMap[kmmparams.RelImgSign] = env.Value
						}
						if strings.Contains(env.Name, kmmparams.RelImgWorker) {
							relatedImgMap[kmmparams.RelImgWorker] = env.Value
						}
						if strings.Contains(env.Name, kmmparams.RelImgMustGather) {
							relatedImgMap[kmmparams.RelImgMustGather] = env.Value
						}
					}
				}
			}

			// pre 4.14 there is no worker image. Swapping it with operator image
			_, ok := relatedImgMap[kmmparams.RelImgWorker]
			if !ok {
				klog.V(kmmparams.KmmLogLevel).Infof("No %s found. Swapping it with operator image %s",
					kmmparams.RelImgWorker, containerImg[0])
				relatedImgMap[kmmparams.RelImgWorker] = containerImg[0]
			}

			klog.V(kmmparams.KmmLogLevel).Infof("related: %s", relatedImgMap)
			klog.V(kmmparams.KmmLogLevel).Infof("container: %s", containerImg)

			By("Create kernel mapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image).
				WithBuildArg("OPERATOR_IMAGE", containerImg[0]).
				// starting KMM 2.1 RBAC image was removed.
				// starting KMM 2.1 webhook images was added.
				WithBuildArg("RBAC_IMAGE", containerImg[1]).
				WithBuildArg(kmmparams.RelImgSign, relatedImgMap[kmmparams.RelImgSign]).
				WithBuildArg(kmmparams.RelImgWorker, relatedImgMap[kmmparams.RelImgWorker]).
				WithBuildArg(kmmparams.RelImgMustGather, relatedImgMap[kmmparams.RelImgMustGather]).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMalOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create moduleloader")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(scanner)
			moduleLoaderContainer.WithKernelMapping(kerMalOne)
			moduleLoaderContainer.WithVersion("never")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "Error creating moduleloadercontainer")

			By("Create module")
			module := kmm.NewModuleBuilder(APIClient, scanner, kmmparams.ScannerTestNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating scanner")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.ScannerTestNamespace, 20*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building scanner")

			By("Run pod from the scanner image")
			scannerPod := pod.NewBuilder(APIClient, "scan-checker", kmmparams.ScannerTestNamespace, image)
			_, err = scannerPod.CreateAndWaitUntilRunning(time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error running scanner image")

			By("Checking operator image scan")
			buff, err := scannerPod.ExecCommand([]string{"tail", "-11", "operator.log"})
			Expect(err).ToNot(HaveOccurred(), "error verifying operator log")
			opContents := buff.String()
			klog.V(kmmparams.KmmLogLevel).Info(opContents)
			Expect(strings.Contains(opContents, "Infected files: 0")).To(BeTrue(), "infected files not zero")

			By("Checking must-gather image scan")
			buff, err = scannerPod.ExecCommand([]string{"tail", "-11", "must-gather.log"})
			Expect(err).ToNot(HaveOccurred(), "error verifying must-gather log")
			opContents = buff.String()
			klog.V(kmmparams.KmmLogLevel).Info(opContents)
			Expect(strings.Contains(opContents, "Infected files: 0")).To(BeTrue(), "infected files not zero")

			By("Checking sign image scan")
			buff, err = scannerPod.ExecCommand([]string{"tail", "-11", "sign.log"})
			Expect(err).ToNot(HaveOccurred(), "error verifying sign log")
			opContents = buff.String()
			klog.V(kmmparams.KmmLogLevel).Info(opContents)
			Expect(strings.Contains(opContents, "Infected files: 0")).To(BeTrue(), "infected files not zero")

			By("Checking worker image scan")
			buff, err = scannerPod.ExecCommand([]string{"tail", "-11", "worker.log"})
			Expect(err).ToNot(HaveOccurred(), "error verifying worker log")
			opContents = buff.String()
			klog.V(kmmparams.KmmLogLevel).Info(opContents)
			Expect(strings.Contains(opContents, "Infected files: 0")).To(BeTrue(), "infected files not zero")

			By("Checking rbac/webhook image scan")
			buff, err = scannerPod.ExecCommand([]string{"tail", "-11", "rbac.log"})
			Expect(err).ToNot(HaveOccurred(), "error verifying rbac log")
			opContents = buff.String()
			klog.V(kmmparams.KmmLogLevel).Info(opContents)
			Expect(strings.Contains(opContents, "Infected files: 0")).To(BeTrue(), "infected files not zero")

		})
	})
})
