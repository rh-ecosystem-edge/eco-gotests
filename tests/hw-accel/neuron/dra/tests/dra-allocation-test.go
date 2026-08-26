package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/do"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	draAllocTestNS   = "neuron-dra-alloc-test"
	draClaimTemplate = "neuron-claim"
	draAllocTimeout  = 5 * time.Minute
)

var _ = Describe("Neuron DRA Allocation Tests", Ordered,
	Label(params.Label, params.DRALabel), func() {
		Context("DRA device allocation", Label(tsparams.LabelSuite), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				if !neuronCfg.IsDRAConfigured() {
					Skip("DRA not configured - ECO_HWACCEL_NEURON_DRA_DRIVER_IMAGE not set")
				}

				By("Verifying DRA DeviceConfig exists")

				dcBuilder, err := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
				Expect(err).ToNot(HaveOccurred(), "DRA DeviceConfig must exist")
				Expect(dcBuilder.Definition.Spec.DRADriverImage).ToNot(BeEmpty(),
					"DeviceConfig must be in DRA mode")

				By("Verifying Neuron nodes exist")

				exists, err := check.NeuronNodesExist(APIClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue(), "At least one Neuron node must exist")

				By("Creating test namespace and ResourceClaimTemplate")

				nsBuilder := namespace.NewBuilder(APIClient, draAllocTestNS)
				if !nsBuilder.Exists() {
					_, err = nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred())
				}

				rctBuilder := resource.NewResourceClaimTemplateBuilder(
					APIClient, draClaimTemplate, draAllocTestNS).
					WithDeviceRequest("neuron-device", params.DRADefaultDeviceClassName, 1)
				Expect(rctBuilder).ToNot(BeNil())

				if !rctBuilder.Exists() {
					_, err = rctBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create ResourceClaimTemplate")
				}
			})

			AfterAll(func() {
				By("Cleaning up test namespace")

				nsBuilder := namespace.NewBuilder(APIClient, draAllocTestNS)
				if nsBuilder.Exists() {
					err := nsBuilder.DeleteAndWait(5 * time.Minute)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("should allocate a Neuron device to a pod via ResourceClaim",
				reportxml.ID("90398"), func() {
					By("Creating DRA consumer pod and waiting for Running")

					err := do.CreateDRAConsumerPodAndWait(
						APIClient, "dra-consumer", draAllocTestNS, draClaimTemplate, draAllocTimeout)
					Expect(err).ToNot(HaveOccurred(), "DRA consumer pod should be Running")
				})

			It("should have /dev/neuron device visible in the allocated pod",
				reportxml.ID("90399"), func() {
					By("Verifying Neuron devices inside the container")

					hasDevices, err := check.PodHasNeuronDevices(APIClient, "dra-consumer", draAllocTestNS)
					Expect(err).ToNot(HaveOccurred())
					Expect(hasDevices).To(BeTrue(), "Pod should have /dev/neuron* device visible")
				})

			It("should NOT expose Neuron devices to a pod without ResourceClaim",
				reportxml.ID("90400"), func() {
					By("Creating a sleep pod without ResourceClaim on a Neuron node")

					sleepPod := do.NewSleepPod(APIClient, "no-claim-pod", draAllocTestNS).
						WithNodeSelector(map[string]string{
							params.NeuronNFDLabelKey: params.NeuronNFDLabelValue,
						})
					_, err := sleepPod.Create()
					Expect(err).ToNot(HaveOccurred())

					err = await.PodRunning(APIClient, "no-claim-pod", draAllocTestNS, draAllocTimeout)
					Expect(err).ToNot(HaveOccurred())

					By("Verifying no Neuron devices are visible")

					hasDevices, err := check.PodHasNeuronDevices(APIClient, "no-claim-pod", draAllocTestNS)
					Expect(err).ToNot(HaveOccurred())
					Expect(hasDevices).To(BeFalse(),
						"Pod without ResourceClaim should NOT have Neuron devices")
				})

			It("should keep pod Pending when all Neuron devices are allocated",
				reportxml.ID("90401"), func() {
					By("Cleaning up pods from previous tests")

					err := do.DeletePodsIfExist(APIClient, draAllocTestNS,
						[]string{"dra-consumer", "no-claim-pod"})
					Expect(err).ToNot(HaveOccurred())

					By("Exhausting all devices on the smallest node")

					targetNode, deviceCount, err := check.SmallestDRANode(APIClient)
					Expect(err).ToNot(HaveOccurred())

					err = do.ExhaustDRADevicesOnNode(
						APIClient, draAllocTestNS, draClaimTemplate,
						targetNode, deviceCount, draAllocTimeout)
					Expect(err).ToNot(HaveOccurred())

					By("Creating one more pod and verifying it stays Pending")

					pendingPod := do.NewDRAConsumerPod(
						APIClient, "dra-pending", draAllocTestNS, draClaimTemplate).
						WithNodeSelector(map[string]string{"kubernetes.io/hostname": targetNode})
					_, err = pendingPod.Create()
					Expect(err).ToNot(HaveOccurred())

					Consistently(func(g Gomega) corev1.PodPhase {
						refreshed, pullErr := pod.Pull(APIClient, "dra-pending", draAllocTestNS)
						g.Expect(pullErr).ToNot(HaveOccurred(), "Failed to pull pending pod")

						return refreshed.Object.Status.Phase
					}, 30*time.Second, 5*time.Second).Should(Equal(corev1.PodPending),
						"Pod should stay Pending when all devices are allocated")

					klog.V(params.NeuronLogLevel).Info("Confirmed: pod stays Pending with no available devices")
				})
		})
	})
