package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron DRA ResourceSlice Tests", Ordered,
	Label(params.Label, params.DRALabel), func() {
		Context("ResourceSlice verification", Label(tsparams.LabelSuite), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				if !neuronCfg.IsDRAConfigured() {
					Skip("DRA not configured - ECO_HWACCEL_NEURON_DRA_DRIVER_IMAGE not set")
				}
			})

			It("should have one ResourceSlice per Neuron node",
				reportxml.ID("90483"), func() {
					By("Counting ResourceSlices for neuron.aws.com")

					sliceList, err := APIClient.K8sClient.ResourceV1().ResourceSlices().List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					neuronSlices := 0

					for idx := range sliceList.Items {
						if sliceList.Items[idx].Spec.Driver == params.DRADriverName {
							neuronSlices++
						}
					}

					By("Counting Neuron nodes")

					neuronNodes, err := check.GetNeuronNodes(APIClient)
					Expect(err).ToNot(HaveOccurred())
					Expect(neuronNodes).ToNot(BeEmpty(), "At least one Neuron node must exist")

					Expect(neuronSlices).To(Equal(len(neuronNodes)),
						"ResourceSlice count should match Neuron node count")

					klog.V(params.NeuronLogLevel).Infof(
						"%d ResourceSlices for %d Neuron nodes", neuronSlices, len(neuronNodes))
				})

			It("should have ResourceSlice nodeName matching DRA driver pod node",
				reportxml.ID("90484"), func() {
					By("Collecting ResourceSlice nodeNames")

					sliceList, err := APIClient.K8sClient.ResourceV1().ResourceSlices().List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					sliceNodes := map[string]bool{}

					for idx := range sliceList.Items {
						if sliceList.Items[idx].Spec.Driver == params.DRADriverName {
							Expect(sliceList.Items[idx].Spec.NodeName).ToNot(BeNil(),
								"ResourceSlice should have a nodeName")

							sliceNodes[*sliceList.Items[idx].Spec.NodeName] = true
						}
					}

					Expect(sliceNodes).ToNot(BeEmpty(), "Should have at least one neuron ResourceSlice")

					By("Collecting DRA driver pod nodeNames")

					podList, err := APIClient.K8sClient.CoreV1().Pods(params.NeuronNamespace).List(
						context.TODO(), metav1.ListOptions{
							LabelSelector: fmt.Sprintf("%s=%s",
								params.DRADaemonSetLabelKey, params.DRADaemonSetLabelValue),
						})
					Expect(err).ToNot(HaveOccurred())

					podNodes := map[string]bool{}
					for idx := range podList.Items {
						podNodes[podList.Items[idx].Spec.NodeName] = true
					}

					By("Verifying 1:1 mapping")

					for nodeName := range sliceNodes {
						Expect(podNodes).To(HaveKey(nodeName),
							"ResourceSlice node %s should have a DRA driver pod", nodeName)
					}

					klog.V(params.NeuronLogLevel).Infof(
						"All %d ResourceSlice nodes match DRA pod nodes", len(sliceNodes))
				})

			It("should report at least one device per ResourceSlice",
				reportxml.ID("90485"), func() {
					sliceList, err := APIClient.K8sClient.ResourceV1().ResourceSlices().List(
						context.TODO(), metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred())

					neuronSliceCount := 0

					for idx := range sliceList.Items {
						slice := &sliceList.Items[idx]
						if slice.Spec.Driver != params.DRADriverName {
							continue
						}

						neuronSliceCount++

						nodeName := ""
						if slice.Spec.NodeName != nil {
							nodeName = *slice.Spec.NodeName
						}

						Expect(slice.Spec.Devices).ToNot(BeEmpty(),
							"ResourceSlice on node %s should have at least one device", nodeName)

						klog.V(params.NeuronLogLevel).Infof(
							"Node %s: %d devices", nodeName, len(slice.Spec.Devices))
					}

					Expect(neuronSliceCount).To(BeNumerically(">", 0),
						"Should have at least one neuron ResourceSlice")
				})
		})
	})
