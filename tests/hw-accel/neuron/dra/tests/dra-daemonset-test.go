package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/neuronconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("Neuron DRA DaemonSet Inspection Tests", Ordered,
	Label(params.Label, params.DRALabel), func() {
		Context("DRA DaemonSet spec verification", Label(tsparams.LabelSuite), func() {
			neuronCfg := neuronconfig.NewNeuronConfig()

			BeforeAll(func() {
				if !neuronCfg.IsDRAConfigured() {
					Skip("DRA not configured - ECO_HWACCEL_NEURON_DRA_DRIVER_IMAGE not set")
				}
			})

			It("should have correct container spec, env vars, and resource limits",
				reportxml.ID("90486"), func() {
					draDS, err := check.DRADaemonSet(APIClient)
					Expect(err).ToNot(HaveOccurred())

					containers := draDS.Spec.Template.Spec.Containers
					Expect(containers).ToNot(BeEmpty(), "DaemonSet should have at least one container")

					container := containers[0]

					By("Verifying container command")

					Expect(container.Command).To(ContainElement("k8s-neuron-dra-driver"),
						"Container should run k8s-neuron-dra-driver")

					By("Verifying container image")

					Expect(container.Image).To(Equal(neuronCfg.DRADriverImage),
						"Container image should match DRA driver image")

					By("Verifying env vars")

					envMap := map[string]string{}

					for _, env := range container.Env {
						if env.Value != "" {
							envMap[env.Name] = env.Value
						} else if env.ValueFrom != nil && env.ValueFrom.FieldRef != nil {
							envMap[env.Name] = env.ValueFrom.FieldRef.FieldPath
						}
					}

					Expect(envMap).To(HaveKeyWithValue("NODE_NAME", "spec.nodeName"))
					Expect(envMap).To(HaveKeyWithValue("POD_UID", "metadata.uid"))
					Expect(envMap).To(HaveKeyWithValue("CDI_ROOT", "/var/run/cdi"))
					Expect(envMap).To(HaveKeyWithValue("HEALTHCHECK_PORT", "51515"))

					By("Verifying resource limits")

					limits := container.Resources.Limits
					Expect(limits.Cpu().String()).To(Equal("20m"),
						"CPU limit should be 20m")
					Expect(limits.Memory().String()).To(Equal("256Mi"),
						"Memory limit should be 256Mi")

					klog.V(params.NeuronLogLevel).Info("Container spec, env vars, and resource limits verified")
				})

			It("should have correct volumes, hostNetwork, liveness probe, and privileged context",
				reportxml.ID("90487"), func() {
					draDS, err := check.DRADaemonSet(APIClient)
					Expect(err).ToNot(HaveOccurred())

					podSpec := draDS.Spec.Template.Spec

					By("Verifying hostNetwork")

					Expect(podSpec.HostNetwork).To(BeTrue(), "DRA DaemonSet should use hostNetwork")

					By("Verifying volumes")

					volumeNames := []string{}

					for _, vol := range podSpec.Volumes {
						volumeNames = append(volumeNames, vol.Name)
					}

					Expect(volumeNames).To(ContainElement("kubelet-plugins"))
					Expect(volumeNames).To(ContainElement("kubelet-plugins-registry"))
					Expect(volumeNames).To(ContainElement("cdi"))

					By("Verifying liveness probe")

					Expect(podSpec.Containers).ToNot(BeEmpty(), "DaemonSet should have at least one container")

					container := podSpec.Containers[0]
					Expect(container.LivenessProbe).ToNot(BeNil(), "Should have a liveness probe")
					Expect(container.LivenessProbe.GRPC).ToNot(BeNil(), "Liveness probe should be gRPC")
					Expect(container.LivenessProbe.GRPC.Port).To(Equal(int32(51515)),
						"gRPC probe port should be 51515")
					Expect(container.LivenessProbe.InitialDelaySeconds).To(Equal(int32(30)))
					Expect(container.LivenessProbe.PeriodSeconds).To(Equal(int32(10)))

					By("Verifying privileged security context")

					Expect(container.SecurityContext).ToNot(BeNil())
					Expect(container.SecurityContext.Privileged).ToNot(BeNil())
					Expect(*container.SecurityContext.Privileged).To(BeTrue(),
						"Container should run privileged")

					klog.V(params.NeuronLogLevel).Info("Volumes, hostNetwork, probe, and security context verified")
				})

			It("should have correct serviceAccount, priorityClass, and tolerations",
				reportxml.ID("90488"), func() {
					draDS, err := check.DRADaemonSet(APIClient)
					Expect(err).ToNot(HaveOccurred())

					podSpec := draDS.Spec.Template.Spec

					By("Verifying serviceAccountName")

					Expect(podSpec.ServiceAccountName).To(Equal("awslabs-gpu-operator-dra-driver"),
						"Should use DRA driver service account")

					By("Verifying priorityClassName")

					Expect(podSpec.PriorityClassName).To(Equal("system-node-critical"),
						"Should use system-node-critical priority")

					By("Verifying tolerations")

					foundUpgradeToleration := false
					foundUnschedulableToleration := false

					for _, tol := range podSpec.Tolerations {
						if tol.Key == "aws-neuron-driver-upgrade" && tol.Effect == "NoExecute" {
							foundUpgradeToleration = true
						}

						if tol.Key == "node.kubernetes.io/unschedulable" && tol.Effect == "NoSchedule" {
							foundUnschedulableToleration = true
						}
					}

					Expect(foundUpgradeToleration).To(BeTrue(),
						"Should tolerate aws-neuron-driver-upgrade:NoExecute")
					Expect(foundUnschedulableToleration).To(BeTrue(),
						"Should tolerate node.kubernetes.io/unschedulable:NoSchedule")

					klog.V(params.NeuronLogLevel).Info("ServiceAccount, priorityClass, and tolerations verified")
				})
		})
	})
