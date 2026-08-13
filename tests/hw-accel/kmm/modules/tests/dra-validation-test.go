package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {
	Context("DRA Validation", Label("dra", "dra-validation"), func() {
		nSpace := kmmparams.DRAValidationTestNamespace
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, nSpace, "dra-kmod")

		BeforeAll(func() {
			By("Create Namespace")

			_, err := namespace.NewBuilder(APIClient, nSpace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
		})

		AfterAll(func() {
			By("Delete Namespace")

			err := namespace.NewBuilder(APIClient, nSpace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")
		})

		Context("Mutual Exclusion", Label("dra-validation"), func() {
			It("should reject Module with both dra and devicePlugin",
				reportxml.ID("89701"), func() {
					By("Create Module with both spec.dra and spec.devicePlugin")

					module := newUnstructuredModule("test-mutual-exclusion", nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"moduleLoader": map[string]interface{}{
							"container": map[string]interface{}{
								"modprobe": map[string]interface{}{
									"moduleName": "simple-kmod",
								},
								"kernelMappings": []interface{}{
									map[string]interface{}{
										"regexp":         "^.+$",
										"containerImage": image,
									},
								},
							},
						},
						"devicePlugin": map[string]interface{}{
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
						},
						"dra": map[string]interface{}{
							"driverName": "gpu.example.com",
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
						},
					})

					err := APIClient.Create(context.TODO(), module)
					Expect(err).To(HaveOccurred(), "module with both dra and devicePlugin should be rejected")
					Expect(err.Error()).To(
						ContainSubstring("spec.dra and spec.devicePlugin are mutually exclusive"))
				})
		})

		Context("Webhook Validation", Label("dra-validation"), func() {
			It("should reject Module with empty driverName",
				reportxml.ID("89702"), func() {
					By("Create Module with empty spec.dra.driverName")

					module := newUnstructuredModule("test-empty-driver", nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"moduleLoader": map[string]interface{}{
							"container": map[string]interface{}{
								"modprobe": map[string]interface{}{
									"moduleName": "simple-kmod",
								},
								"kernelMappings": []interface{}{
									map[string]interface{}{
										"regexp":         "^.+$",
										"containerImage": image,
									},
								},
							},
						},
						"dra": map[string]interface{}{
							"driverName": "",
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
						},
					})

					err := APIClient.Create(context.TODO(), module)
					Expect(err).To(HaveOccurred(), "module with empty driverName should be rejected")
					Expect(err.Error()).To(ContainSubstring("driverName"))
				})

			It("should reject Module with invalid driverName format",
				reportxml.ID("89702"), func() {
					By("Create Module with non-DNS driverName")

					module := newUnstructuredModule("test-invalid-driver", nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"moduleLoader": map[string]interface{}{
							"container": map[string]interface{}{
								"modprobe": map[string]interface{}{
									"moduleName": "simple-kmod",
								},
								"kernelMappings": []interface{}{
									map[string]interface{}{
										"regexp":         "^.+$",
										"containerImage": image,
									},
								},
							},
						},
						"dra": map[string]interface{}{
							"driverName": "INVALID DRIVER NAME WITH SPACES",
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
						},
					})

					err := APIClient.Create(context.TODO(), module)
					Expect(err).To(HaveOccurred(), "module with invalid driverName should be rejected")
					Expect(err.Error()).To(ContainSubstring("not a valid DNS subdomain"))
				})

			It("should reject Module with duplicate deviceClass names",
				reportxml.ID("89702"), func() {
					By("Create Module with duplicate deviceClass names")

					module := newUnstructuredModule("test-dup-deviceclass", nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"moduleLoader": map[string]interface{}{
							"container": map[string]interface{}{
								"modprobe": map[string]interface{}{
									"moduleName": "simple-kmod",
								},
								"kernelMappings": []interface{}{
									map[string]interface{}{
										"regexp":         "^.+$",
										"containerImage": image,
									},
								},
							},
						},
						"dra": map[string]interface{}{
							"driverName": "gpu.example.com",
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
							"deviceClasses": []interface{}{
								map[string]interface{}{"name": "my-class"},
								map[string]interface{}{"name": "my-class"},
							},
						},
					})

					err := APIClient.Create(context.TODO(), module)
					Expect(err).To(HaveOccurred(), "module with duplicate deviceClass names should be rejected")
					Expect(err.Error()).To(ContainSubstring("duplicate"))
				})
		})

		Context("Volume Validation", Label("dra-validation"), func() {
			It("should accept Module with valid DRA hostPath volumes",
				reportxml.ID("89706"), func() {
					By("Create Module with valid hostPath prefixes (/dev, /sys, /var, /opt, /run)")

					module := newUnstructuredModule("test-valid-volumes", nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"moduleLoader": map[string]interface{}{
							"container": map[string]interface{}{
								"modprobe": map[string]interface{}{
									"moduleName": "simple-kmod",
								},
								"kernelMappings": []interface{}{
									map[string]interface{}{
										"regexp":         "^.+$",
										"containerImage": image,
									},
								},
							},
						},
						"dra": map[string]interface{}{
							"driverName": "gpu.example.com",
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "dev-vol",
									"hostPath": map[string]interface{}{
										"path": "/dev/neuron0",
									},
								},
								map[string]interface{}{
									"name": "sys-vol",
									"hostPath": map[string]interface{}{
										"path": "/sys/devices",
									},
								},
								map[string]interface{}{
									"name": "var-vol",
									"hostPath": map[string]interface{}{
										"path": "/var/run/custom",
									},
								},
								map[string]interface{}{
									"name": "opt-vol",
									"hostPath": map[string]interface{}{
										"path": "/opt/drivers",
									},
								},
								map[string]interface{}{
									"name": "run-vol",
									"hostPath": map[string]interface{}{
										"path": "/run/udev",
									},
								},
							},
						},
					})

					err := APIClient.Create(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(),
						"module with valid DRA hostPath volumes should be accepted")

					By("Cleanup: delete the module")

					err = APIClient.Delete(context.TODO(), module)
					Expect(err).ToNot(HaveOccurred(), "error deleting module")
				})

			It("should reject Module with DRA hostPath under /etc",
				reportxml.ID("89706"), func() {
					By("Create Module with /etc hostPath volume in DRA spec")

					module := newUnstructuredModule("test-etc-volume", nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"moduleLoader": map[string]interface{}{
							"container": map[string]interface{}{
								"modprobe": map[string]interface{}{
									"moduleName": "simple-kmod",
								},
								"kernelMappings": []interface{}{
									map[string]interface{}{
										"regexp":         "^.+$",
										"containerImage": image,
									},
								},
							},
						},
						"dra": map[string]interface{}{
							"driverName": "gpu.example.com",
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "etc-vol",
									"hostPath": map[string]interface{}{
										"path": "/etc/secrets",
									},
								},
							},
						},
					})

					err := APIClient.Create(context.TODO(), module)
					Expect(err).To(HaveOccurred(),
						"module with /etc hostPath in DRA spec should be rejected")
					Expect(err.Error()).To(ContainSubstring("not allowed"))
				})

			It("should reject Module with DRA hostPath traversal",
				reportxml.ID("89706"), func() {
					By("Create Module with path traversal in DRA hostPath volume")

					module := newUnstructuredModule("test-traversal-volume", nSpace, map[string]interface{}{
						"selector": GeneralConfig.WorkerLabelMap,
						"moduleLoader": map[string]interface{}{
							"container": map[string]interface{}{
								"modprobe": map[string]interface{}{
									"moduleName": "simple-kmod",
								},
								"kernelMappings": []interface{}{
									map[string]interface{}{
										"regexp":         "^.+$",
										"containerImage": image,
									},
								},
							},
						},
						"dra": map[string]interface{}{
							"driverName": "gpu.example.com",
							"container": map[string]interface{}{
								"image": "registry.k8s.io/e2e-test-images/busybox:1.36.1-1",
							},
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "traversal-vol",
									"hostPath": map[string]interface{}{
										"path": "/var/../etc/passwd",
									},
								},
							},
						},
					})

					err := APIClient.Create(context.TODO(), module)
					Expect(err).To(HaveOccurred(),
						"module with path traversal in DRA hostPath should be rejected")
				})
		})
	})
})

func newUnstructuredModule(name, nsname string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kmm.sigs.x-k8s.io/v1beta1",
			"kind":       "Module",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": nsname,
			},
			"spec": spec,
		},
	}
}
