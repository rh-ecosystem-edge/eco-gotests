package capoa_conversion_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-conversion/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-conversion/internal/tsparams"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	bootstrapTestNS = "capoa-conv-bootstrap"
)

var (
	bootstrapV1Alpha1GVK = schema.GroupVersionKind{
		Group:   "bootstrap.cluster.x-k8s.io",
		Version: "v1alpha1",
		Kind:    "OpenshiftAssistedConfig",
	}
	bootstrapV1Alpha2GVK = schema.GroupVersionKind{
		Group:   "bootstrap.cluster.x-k8s.io",
		Version: "v1alpha2",
		Kind:    "OpenshiftAssistedConfig",
	}
	bootstrapTemplateV1Alpha1GVK = schema.GroupVersionKind{
		Group:   "bootstrap.cluster.x-k8s.io",
		Version: "v1alpha1",
		Kind:    "OpenshiftAssistedConfigTemplate",
	}
	bootstrapTemplateV1Alpha2GVK = schema.GroupVersionKind{
		Group:   "bootstrap.cluster.x-k8s.io",
		Version: "v1alpha2",
		Kind:    "OpenshiftAssistedConfigTemplate",
	}
)

var _ = Describe(
	"BootstrapConversion",
	Ordered, ContinueOnFailure,
	Label(tsparams.LabelBootstrapConversion), func() {
		BeforeAll(func() {
			By("Verifying hub API client is available")

			if HubAPIClient == nil {
				Skip("Hub API client is nil")
			}

			By("Verifying bootstrap CRD serves v1alpha1")

			assertCRDServesVersion("openshiftassistedconfigs.bootstrap.cluster.x-k8s.io", "v1alpha1")

			By(fmt.Sprintf("Creating test namespace %s", bootstrapTestNS))

			nsBuilder := namespace.NewBuilder(HubAPIClient, bootstrapTestNS)

			_ = nsBuilder.DeleteAndWait(2 * time.Minute)

			_, err := namespace.NewBuilder(HubAPIClient, bootstrapTestNS).Create()
			Expect(err).ToNot(HaveOccurred(), "failed to create namespace %s", bootstrapTestNS)

			DeferCleanup(func() {
				nsBuilder := namespace.NewBuilder(HubAPIClient, bootstrapTestNS)
				_ = nsBuilder.DeleteAndWait(2 * time.Minute)
			})
		})

		It("Verifies spec fields survive v1alpha1 to v1alpha2 round-trip conversion",
			reportxml.ID("00001"), func() {
				resourceName := "conv-spec-roundtrip"

				By("Creating OpenshiftAssistedConfig via v1alpha1 API with populated spec")

				oac := newBootstrapV1Alpha1(resourceName, bootstrapTestNS, map[string]interface{}{
					"cpuArchitecture":      "x86_64",
					"sshAuthorizedKey":     "ssh-rsa AAAA...",
					"additionalNTPSources": []interface{}{"ntp1.example.com", "ntp2.example.com"},
					"nodeRegistration": map[string]interface{}{
						"name":               "test-node",
						"kubeletExtraLabels": []interface{}{"label1=val1", "label2=val2"},
					},
				})

				err := HubAPIClient.Create(context.TODO(), oac)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedConfig via v1alpha1")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oac)
				})

				By("Reading the resource back via v1alpha2 API")

				v2Obj := readResource(bootstrapV1Alpha2GVK, resourceName, bootstrapTestNS)

				By("Verifying spec fields are preserved after v1alpha1 -> v1alpha2 conversion")

				arch, _, _ := unstructured.NestedString(v2Obj.Object, "spec", "cpuArchitecture")
				Expect(arch).To(Equal("x86_64"), "cpuArchitecture should be preserved")

				sshKey, _, _ := unstructured.NestedString(v2Obj.Object, "spec", "sshAuthorizedKey")
				Expect(sshKey).To(Equal("ssh-rsa AAAA..."), "sshAuthorizedKey should be preserved")

				ntpSources, _, _ := unstructured.NestedStringSlice(v2Obj.Object, "spec", "additionalNTPSources")
				Expect(ntpSources).To(Equal([]string{"ntp1.example.com", "ntp2.example.com"}),
					"additionalNTPSources should be preserved")

				nodeName, _, _ := unstructured.NestedString(v2Obj.Object, "spec", "nodeRegistration", "name")
				Expect(nodeName).To(Equal("test-node"), "nodeRegistration.name should be preserved")

				labels, _, _ := unstructured.NestedStringSlice(v2Obj.Object, "spec", "nodeRegistration", "kubeletExtraLabels")
				Expect(labels).To(Equal([]string{"label1=val1", "label2=val2"}),
					"nodeRegistration.kubeletExtraLabels should be preserved")

				By("Reading the resource back via v1alpha1 API (reverse conversion)")

				v1Obj := readResource(bootstrapV1Alpha1GVK, resourceName, bootstrapTestNS)

				archV1, _, _ := unstructured.NestedString(v1Obj.Object, "spec", "cpuArchitecture")
				Expect(archV1).To(Equal("x86_64"), "cpuArchitecture should survive round-trip")

				sshKeyV1, _, _ := unstructured.NestedString(v1Obj.Object, "spec", "sshAuthorizedKey")
				Expect(sshKeyV1).To(Equal("ssh-rsa AAAA..."), "sshAuthorizedKey should survive round-trip")
			})

		It("Verifies v1alpha2-only fields are preserved when read back via v1alpha1",
			reportxml.ID("00002"), func() {
				resourceName := "conv-v2-extra-fields"

				By("Creating OpenshiftAssistedConfig via v1alpha2 API with v1alpha2-only fields")

				oac := newBootstrapV1Alpha2(resourceName, bootstrapTestNS, map[string]interface{}{
					"cpuArchitecture":      "x86_64",
					"preBootstrapCommands": []interface{}{"/usr/bin/pre-cmd"},
					"nodeRegistration": map[string]interface{}{
						"providerID": "nutanix://test-provider-id",
					},
				})

				err := HubAPIClient.Create(context.TODO(), oac)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedConfig via v1alpha2")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oac)
				})

				By("Reading the resource back via v1alpha1 API")

				v1Obj := readResource(bootstrapV1Alpha1GVK, resourceName, bootstrapTestNS)

				By("Verifying common fields are preserved")

				arch, _, _ := unstructured.NestedString(v1Obj.Object, "spec", "cpuArchitecture")
				Expect(arch).To(Equal("x86_64"), "cpuArchitecture should be preserved in v1alpha1 view")

				By("Reading back via v1alpha2 to confirm v1alpha2-only fields survived round-trip via MarshalData")

				v2Obj := readResource(bootstrapV1Alpha2GVK, resourceName, bootstrapTestNS)

				preCommands, _, _ := unstructured.NestedStringSlice(v2Obj.Object, "spec", "preBootstrapCommands")
				Expect(preCommands).To(Equal([]string{"/usr/bin/pre-cmd"}),
					"preBootstrapCommands should survive v1alpha2 -> v1alpha1 -> v1alpha2 round-trip")

				providerID, _, _ := unstructured.NestedString(v2Obj.Object, "spec", "nodeRegistration", "providerID")
				Expect(providerID).To(Equal("nutanix://test-provider-id"),
					"nodeRegistration.providerID should survive round-trip via MarshalData annotation")
			})

		It("Verifies OpenshiftAssistedConfigTemplate spec round-trip conversion",
			reportxml.ID("00003"), func() {
				resourceName := "conv-template-roundtrip"

				By("Creating OpenshiftAssistedConfigTemplate via v1alpha1 API")

				tmpl := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha1",
						"kind":       "OpenshiftAssistedConfigTemplate",
						"metadata": map[string]interface{}{
							"name":      resourceName,
							"namespace": bootstrapTestNS,
						},
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"test-label": "conversion-test",
									},
								},
								"spec": map[string]interface{}{
									"cpuArchitecture":  "aarch64",
									"sshAuthorizedKey": "ssh-ed25519 AAAA...",
								},
							},
						},
					},
				}

				err := HubAPIClient.Create(context.TODO(), tmpl)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedConfigTemplate via v1alpha1")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), tmpl)
				})

				By("Reading the template back via v1alpha2 API")

				v2Tmpl := readResource(bootstrapTemplateV1Alpha2GVK, resourceName, bootstrapTestNS)

				arch, _, _ := unstructured.NestedString(v2Tmpl.Object,
					"spec", "template", "spec", "cpuArchitecture")
				Expect(arch).To(Equal("aarch64"),
					"template spec cpuArchitecture should be preserved")

				labels, _, _ := unstructured.NestedStringMap(v2Tmpl.Object,
					"spec", "template", "metadata", "labels")
				Expect(labels).To(HaveKeyWithValue("test-label", "conversion-test"),
					"template metadata labels should be preserved")

				By("Reading the template back via v1alpha1 to verify round-trip")

				v1Tmpl := readResource(bootstrapTemplateV1Alpha1GVK, resourceName, bootstrapTestNS)

				archV1, _, _ := unstructured.NestedString(v1Tmpl.Object,
					"spec", "template", "spec", "cpuArchitecture")
				Expect(archV1).To(Equal("aarch64"),
					"template spec cpuArchitecture should survive round-trip")
			})
	})

func newBootstrapV1Alpha1(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha1",
			"kind":       "OpenshiftAssistedConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

func newBootstrapV1Alpha2(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
			"kind":       "OpenshiftAssistedConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

func readResource(gvk schema.GroupVersionKind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	err := HubAPIClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, obj)
	ExpectWithOffset(1, err).ToNot(HaveOccurred(),
		"failed to read %s/%s as %s", namespace, name, gvk.Version)

	return obj
}

func assertCRDServesVersion(crdName, version string) {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})

	err := HubAPIClient.Get(context.TODO(), types.NamespacedName{Name: crdName}, crd)
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), "CRD %s not found", crdName)

	versions, found, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
	ExpectWithOffset(1, found).To(BeTrue(), "CRD %s has no spec.versions", crdName)

	served := false

	for _, v := range versions {
		vMap, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		vName, _, _ := unstructured.NestedString(vMap, "name")
		vServed, _, _ := unstructured.NestedBool(vMap, "served")

		if vName == version && vServed {
			served = true

			break
		}
	}

	ExpectWithOffset(1, served).To(BeTrue(),
		"CRD %s does not serve version %s", crdName, version)
}
