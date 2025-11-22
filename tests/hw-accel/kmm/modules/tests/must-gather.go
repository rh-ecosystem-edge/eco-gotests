package tests

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Module", Label("must-gather"), func() {

		It("Check must-gather functionality", reportxml.ID("53653"), func() {
			By("Print Pod Name")
			pods, _ := pod.List(APIClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{
				FieldSelector: "status.phase=Running",
			})
			var mustGatherImage string
			for _, pod := range pods {
				for _, container := range pod.Object.Spec.Containers {
					for _, env := range container.Env {

						if strings.Contains(env.Name, kmmparams.RelImgMustGather) {
							mustGatherImage = env.Value
							klog.V(kmmparams.KmmLogLevel).Infof("%s: %s\n", kmmparams.RelImgMustGather, mustGatherImage)
						}

					}
				}
			}
			// Create must-gather pod
			By("Creating must-gather pod")
			mgPod, err := pod.NewBuilder(APIClient, "must-gather-pod", kmmparams.KmmOperatorNamespace, mustGatherImage).
				CreateAndWaitUntilRunning(300 * time.Second)
			Expect(err).ToNot(HaveOccurred(), "error creating must-gather pod")
			cmdToExec := []string{"ls", "-l", "/usr/bin/gather"}

			klog.V(90).Infof("Exec cmd %v on pod %s", cmdToExec, mgPod.Definition.Name)

			buf, err := mgPod.ExecCommand(cmdToExec)
			Expect(err).ToNot(HaveOccurred(), "gather binary not found")

			klog.V(90).Infof("gather binary found: %s", buf.String())

			By("Deleting must-gather pod")
			_, err2 := pod.NewBuilder(APIClient, "must-gather-pod", kmmparams.KmmOperatorNamespace, mustGatherImage).
				DeleteAndWait(300 * time.Second)
			Expect(err2).ToNot(HaveOccurred(), "error deleting must-gather pod")
		})

	})
})
