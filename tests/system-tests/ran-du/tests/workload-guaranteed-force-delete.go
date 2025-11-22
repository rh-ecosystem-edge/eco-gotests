package ran_du_system_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe(
	"WorkloadForceCleanup",
	Ordered,
	ContinueOnFailure,
	Label("WorkloadForceCleanup"), func() {
		BeforeAll(func() {
			By("Preparing workload")
			if namespace.NewBuilder(APIClient, RanDuTestConfig.TestWorkload.Namespace).Exists() {
				By("Deleting workload using shell method")
				_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.DeleteShellCmd)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete workload")
			}

			if RanDuTestConfig.TestWorkload.CreateMethod == randuparams.TestWorkloadShellLaunchMethod {
				By("Launching workload using shell method")
				_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.CreateShellCmd)
				Expect(err).ToNot(HaveOccurred(), "Failed to launch workload")
			}

			By("Waiting for deployment replicas to become ready")
			_, err := await.WaitUntilAllDeploymentsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
				randuparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for deployment to become ready")

			By("Waiting fo rstatefulset replicas to become ready")
			_, err = await.WaitUntilAllStatefulSetsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
				randuparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for statefulsets to become ready")

		})

		It("Assert all pods recover after force deletion", reportxml.ID("74462"), func() {
			for n := 0; n < 3; n++ {
				By("Force delete guaranteed pods")
				podList, err := pod.List(APIClient, RanDuTestConfig.TestWorkload.Namespace, metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to list pods")

				for _, runningPod := range podList {
					if runningPod.Object.Status.QOSClass == "Guaranteed" {
						klog.V(100).Infof("Force deleting guaranteed pod %s", runningPod.Object.Name)
						guaranteedPod, err := pod.Pull(APIClient, runningPod.Object.Name, RanDuTestConfig.TestWorkload.Namespace)
						Expect(err).ToNot(HaveOccurred(), "Failed to pull pod %s", runningPod.Object.Name)

						_, err = guaranteedPod.DeleteImmediate()
						Expect(err).ToNot(HaveOccurred(), "Failed to force delete pod %s", runningPod.Object.Name)
					}
				}

				By("Assert all pods recover after forced pod deletion")
				_, err = await.WaitUntilAllPodsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace, 2*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "pod not ready: %s", err)
			}

		})
		AfterAll(func() {
			By("Cleaning up test workload resources")
			_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.DeleteShellCmd)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete workload")
		})
	})
