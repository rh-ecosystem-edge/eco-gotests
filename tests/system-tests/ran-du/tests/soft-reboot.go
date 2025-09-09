package ran_du_system_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/reboot"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/sriov"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"SoftReboot",
	Ordered,
	ContinueOnFailure,
	Label("SoftReboot"), func() {
		BeforeAll(func() {
			By("Preparing workload")

			if namespace.NewBuilder(APIClient, RanDuTestConfig.TestWorkload.Namespace).Exists() {
				_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.DeleteShellCmd)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete workload")
			}

			if RanDuTestConfig.TestWorkload.CreateMethod == "shell" {
				By("Launching workload using shell method")
				_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.CreateShellCmd)
				Expect(err).ToNot(HaveOccurred(), "Failed to launch workload")
			}

			By("Waiting for deployment replicas to become ready")
			_, err := await.WaitUntilAllDeploymentsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
				randuparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for deployment to become ready")

			By("Waiting for statefulset replicas to become ready")
			_, err = await.WaitUntilAllStatefulSetsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
				randuparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for statefulsets to become ready")

		})
		It("Soft reboot nodes", reportxml.ID("42738"), Label("SoftReboot"), func() {
			By("Retrieve nodes list")
			nodeList, err := nodes.List(
				APIClient,
				metav1.ListOptions{},
			)
			Expect(err).ToNot(HaveOccurred(), "Error listing nodes.")

			By("Pull openshift-apiserver deployment spec")
			deploy, err := deployment.Pull(APIClient, "apiserver", "openshift-apiserver")
			Expect(err).ToNot(HaveOccurred(), "error while pulling openshift apiserver deployment")

			for r := 0; r < RanDuTestConfig.SoftRebootIterations; r++ {
				By("Soft rebooting cluster")
				fmt.Printf("Soft reboot iteration no. %d\n", r)
				for _, node := range nodeList {
					By("Reboot node")
					fmt.Printf("Reboot node %s", node.Definition.Name)
					err = reboot.SoftRebootNode(node.Definition.Name)
					Expect(err).ToNot(HaveOccurred(), "Error rebooting the nodes.")

					By("Wait for node to become unreachable")
					fmt.Printf("Wait for node %s to become unreachable", node.Definition.Name)
					err = await.WaitUntilNodeIsUnreachable(node.Definition.Name, 3*time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Node is still reachable: %s", err)

					By("Wait for the openshift apiserver deployment to be available")
					err = deploy.WaitUntilCondition("Available", 8*time.Minute)
					Expect(err).ToNot(HaveOccurred(), "openshift apiserver deployment has not recovered in time after reboot")

					By(fmt.Sprintf("Wait for %d minutes for the cluster resources to reconciliate their state",
						RanDuTestConfig.RebootRecoveryTime))
					time.Sleep(time.Duration(RanDuTestConfig.RebootRecoveryTime) * time.Minute)

					By("Remove any pods in UnexpectedAdmissionError state")
					listOptions := metav1.ListOptions{
						FieldSelector: "status.phase=Failed",
					}
					podsList, err := pod.List(APIClient, RanDuTestConfig.TestWorkload.Namespace, listOptions)
					Expect(err).ToNot(HaveOccurred(), "could not retrieve pod list")

					for _, failedPod := range podsList {
						if failedPod.Definition.Status.Reason == "UnexpectedAdmissionError" {
							_, err := failedPod.DeleteAndWait(60 * time.Second)
							Expect(err).ToNot(HaveOccurred(), "could not delete pod in UnexpectedAdmissionError state")
						}
					}

					By("Waiting for deployment replicas to become ready")
					_, err = await.WaitUntilAllDeploymentsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
						randuparams.DefaultTimeout)
					Expect(err).ToNot(HaveOccurred(), "error while waiting for deployment to become ready")

					By("Waiting for statefulset replicas to become ready")
					_, err = await.WaitUntilAllStatefulSetsReady(APIClient, RanDuTestConfig.TestWorkload.Namespace,
						randuparams.DefaultTimeout)
					Expect(err).ToNot(HaveOccurred(), "error while waiting for statefulsets to become ready")

					By("Retrieve pod list")
					podsList, err = pod.List(APIClient, RanDuTestConfig.TestWorkload.Namespace, metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "could not retrieve pod list")

					By("Retrieve sriov networks with vfio-pci driver")
					vfioNetworks, err := sriov.ListNetworksByDeviceType(APIClient, "vfio-pci")
					Expect(err).ToNot(HaveOccurred(), "error when retrieving sriov network using vfio-pci driver")

					By("Assert devices under /dev/vfio on pod are equal or more to the pods vfio-pci network attachments\n")
					for _, pod := range podsList {
						networkNames, err := sriov.ExtractNetworkNames(pod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"])
						Expect(err).ToNot(HaveOccurred(), "error when retrieving pod network attachments")

						podvfioDevices := 0

						for _, vfioNet := range vfioNetworks {
							for _, podNet := range networkNames {
								if strings.Contains(podNet, pod.Definition.Namespace+"/"+vfioNet) {
									podvfioDevices++
								}
							}
						}

						if podvfioDevices > 0 {
							fmt.Printf("Check /dev/vfio on pod %s\n", pod.Definition.Name)
							lscmd := []string{"ls", "--color=never", "/dev/vfio"}
							cmd, err := pod.ExecCommand(lscmd)
							Expect(err).ToNot(HaveOccurred(), "error when executing command on pod")

							// retry in case the command exec returns an empty string
							if len(cmd.String()) == 0 {
								cmd, err = pod.ExecCommand(lscmd)
								Expect(err).ToNot(HaveOccurred(), "error when executing command on pod")
							}

							vfioDevls := strings.Fields(strings.ReplaceAll(cmd.String(), "vfio", ""))
							Expect(len(vfioDevls)).To(BeNumerically(">=", podvfioDevices),
								"error: vfio devices inside pod( %s ) do not match pod %s attachments:", cmd.String(), pod.Definition.Name)
						}
					}

					if RanDuTestConfig.PtpEnabled {
						timeInterval := 3 * time.Minute
						time.Sleep(timeInterval)

						By("Check PTP status for the last 3 minutes")
						ptpOnSync, err := ptp.ValidatePTPStatus(APIClient, timeInterval)
						Expect(err).ToNot(HaveOccurred(), "PTP Error: %s", err)
						Expect(ptpOnSync).To(Equal(true))
					}
				}
			}
		})
		AfterAll(func() {
			By("Cleaning up test workload resources")
			_, err := shell.ExecuteCmd(RanDuTestConfig.TestWorkload.DeleteShellCmd)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete workload")
		})
	})
