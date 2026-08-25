package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/neuron"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron DRA Cleanup Tests", Ordered,
	Label(params.Label, params.DRALabel, "dra-cleanup"), func() {
		Context("DeviceConfig deletion cleanup", Label(tsparams.LabelSuite), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				if !neuronCfg.IsDRAConfigured() {
					Skip("DRA not configured - ECO_HWACCEL_NEURON_DRA_DRIVER_IMAGE not set")
				}

				By("Verifying DRA DeviceConfig exists before cleanup")

				dcBuilder, err := neuron.Pull(
					APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)
				Expect(err).ToNot(HaveOccurred(), "DRA DeviceConfig must exist")
				Expect(dcBuilder.Definition.Spec.DRADriverImage).ToNot(BeEmpty(),
					"DeviceConfig must be in DRA mode")

				By("Deleting DeviceConfig")

				_, err = dcBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete DeviceConfig")

				By("Waiting for DeviceConfig to be fully deleted")

				Eventually(func() bool {
					_, pullErr := neuron.Pull(
						APIClient, params.DefaultDeviceConfigName, params.NeuronNamespace)

					return pullErr != nil
				}, 5*time.Minute, 5*time.Second).Should(BeTrue(),
					"DeviceConfig should be deleted")

				klog.V(params.NeuronLogLevel).Info("DeviceConfig deleted, verifying cleanup")
			})

			It("should remove DRA DaemonSet after DeviceConfig deletion",
				reportxml.ID("90480"), func() {
					By("Waiting for DRA DaemonSets to be removed")

					Eventually(func() int {
						dsList, err := APIClient.K8sClient.AppsV1().DaemonSets(
							params.NeuronNamespace).List(
							context.TODO(), metav1.ListOptions{
								LabelSelector: params.DRADaemonSetLabelKey + "=" + params.DRADaemonSetLabelValue,
							})
						if err != nil {
							return -1
						}

						return len(dsList.Items)
					}, 3*time.Minute, 5*time.Second).Should(Equal(0),
						"All DRA DaemonSets should be removed")

					klog.V(params.NeuronLogLevel).Info("DRA DaemonSets removed")
				})

			It("should remove ResourceSlices after DRA DaemonSet deletion",
				reportxml.ID("90481"), func() {
					By("Waiting for ResourceSlices to be cleaned up")

					Eventually(func() int {
						sliceList, err := APIClient.K8sClient.ResourceV1().ResourceSlices().List(
							context.TODO(), metav1.ListOptions{})
						if err != nil {
							return -1
						}

						count := 0

						for idx := range sliceList.Items {
							if sliceList.Items[idx].Spec.Driver == params.DRADriverName {
								count++
							}
						}

						return count
					}, 3*time.Minute, 5*time.Second).Should(Equal(0),
						"All ResourceSlices for %s should be removed", params.DRADriverName)

					klog.V(params.NeuronLogLevel).Info("ResourceSlices cleaned up")
				})

			It("should remove DeviceClass after DeviceConfig deletion",
				reportxml.ID("90482"), func() {
					By("Waiting for DeviceClass to be removed")

					Eventually(func() bool {
						_, err := APIClient.K8sClient.ResourceV1().DeviceClasses().Get(
							context.TODO(), params.DRADefaultDeviceClassName, metav1.GetOptions{})

						return err != nil
					}, 3*time.Minute, 5*time.Second).Should(BeTrue(),
						"DeviceClass %s should be removed", params.DRADefaultDeviceClassName)

					klog.V(params.NeuronLogLevel).Infof(
						"DeviceClass %s removed", params.DRADefaultDeviceClassName)
				})
		})
	})
