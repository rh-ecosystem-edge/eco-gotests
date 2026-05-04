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
)

const (
	controlplaneTestNS = "capoa-conv-ctrlplane"
)

var (
	cpV1Alpha2GVK = schema.GroupVersionKind{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1alpha2",
		Kind:    "OpenshiftAssistedControlPlane",
	}
	cpV1Alpha3GVK = schema.GroupVersionKind{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1alpha3",
		Kind:    "OpenshiftAssistedControlPlane",
	}
)

var _ = Describe(
	"ControlPlaneConversion",
	Ordered, ContinueOnFailure,
	Label(tsparams.LabelControlPlaneConversion), func() {
		BeforeAll(func() {
			By("Verifying hub API client is available")

			if HubAPIClient == nil {
				Skip("Hub API client is nil")
			}

			By("Verifying controlplane CRD serves v1alpha2")

			assertCRDServesVersion("openshiftassistedcontrolplanes.controlplane.cluster.x-k8s.io", "v1alpha2")

			By(fmt.Sprintf("Creating test namespace %s", controlplaneTestNS))

			nsBuilder := namespace.NewBuilder(HubAPIClient, controlplaneTestNS)

			_ = nsBuilder.DeleteAndWait(2 * time.Minute)

			_, err := namespace.NewBuilder(HubAPIClient, controlplaneTestNS).Create()
			Expect(err).ToNot(HaveOccurred(), "failed to create namespace %s", controlplaneTestNS)

			DeferCleanup(func() {
				nsBuilder := namespace.NewBuilder(HubAPIClient, controlplaneTestNS)
				_ = nsBuilder.DeleteAndWait(2 * time.Minute)
			})
		})

		It("Verifies Duration to seconds conversion in MachineTemplate",
			reportxml.ID("00004"), func() {
				resourceName := "conv-duration-to-seconds"

				By("Creating OpenshiftAssistedControlPlane via v1alpha2 with Duration fields")

				oacp := newControlPlaneV1Alpha2(resourceName, map[string]interface{}{
					"distributionVersion": "4.17.0",
					"replicas":            int64(3),
					"machineTemplate": map[string]interface{}{
						"infrastructureRef": map[string]interface{}{
							"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
							"kind":       "OpenshiftAssistedMachineTemplate",
							"name":       "test-machine-template",
							"namespace":  controlplaneTestNS,
						},
						"nodeDrainTimeout":        "5m30s",
						"nodeVolumeDetachTimeout": "2m0s",
						"nodeDeletionTimeout":     "10m0s",
					},
					"config": map[string]interface{}{
						"baseDomain":  "example.com",
						"clusterName": "test-cluster",
					},
					"openshiftAssistedConfigSpec": map[string]interface{}{
						"cpuArchitecture": "x86_64",
					},
				})

				err := HubAPIClient.Create(context.TODO(), oacp)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedControlPlane via v1alpha2")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oacp)
				})

				By("Reading back via v1alpha3 API")

				v3Obj := readResource(cpV1Alpha3GVK, resourceName, controlplaneTestNS)

				By("Verifying Duration fields are converted to seconds in MachineDeletionSpec")

				nodeDrainSeconds, found, _ := unstructured.NestedFloat64(v3Obj.Object,
					"spec", "machineTemplate", "deletion", "nodeDrainTimeoutSeconds")
				Expect(found).To(BeTrue(), "nodeDrainTimeoutSeconds should exist in v1alpha3")
				Expect(int32(nodeDrainSeconds)).To(Equal(int32(330)),
					"nodeDrainTimeout 5m30s should convert to 330 seconds")

				nodeVolumeSeconds, found, _ := unstructured.NestedFloat64(v3Obj.Object,
					"spec", "machineTemplate", "deletion", "nodeVolumeDetachTimeoutSeconds")
				Expect(found).To(BeTrue(), "nodeVolumeDetachTimeoutSeconds should exist in v1alpha3")
				Expect(int32(nodeVolumeSeconds)).To(Equal(int32(120)),
					"nodeVolumeDetachTimeout 2m0s should convert to 120 seconds")

				nodeDeletionSeconds, found, _ := unstructured.NestedFloat64(v3Obj.Object,
					"spec", "machineTemplate", "deletion", "nodeDeletionTimeoutSeconds")
				Expect(found).To(BeTrue(), "nodeDeletionTimeoutSeconds should exist in v1alpha3")
				Expect(int32(nodeDeletionSeconds)).To(Equal(int32(600)),
					"nodeDeletionTimeout 10m0s should convert to 600 seconds")
			})

		It("Verifies seconds to Duration reverse conversion in MachineTemplate",
			reportxml.ID("00005"), func() {
				resourceName := "conv-seconds-to-duration"

				By("Creating OpenshiftAssistedControlPlane via v1alpha3 with seconds fields")

				oacp := newControlPlaneV1Alpha3(resourceName, map[string]interface{}{
					"distributionVersion": "4.17.0",
					"replicas":            int64(3),
					"machineTemplate": map[string]interface{}{
						"infrastructureRef": map[string]interface{}{
							"kind":     "OpenshiftAssistedMachineTemplate",
							"name":     "test-machine-template",
							"apiGroup": "infrastructure.cluster.x-k8s.io",
						},
						"deletion": map[string]interface{}{
							"nodeDrainTimeoutSeconds":        int64(330),
							"nodeVolumeDetachTimeoutSeconds": int64(120),
							"nodeDeletionTimeoutSeconds":     int64(600),
						},
					},
					"config": map[string]interface{}{
						"baseDomain":  "example.com",
						"clusterName": "test-cluster",
					},
					"openshiftAssistedConfigSpec": map[string]interface{}{
						"cpuArchitecture": "x86_64",
					},
				})

				err := HubAPIClient.Create(context.TODO(), oacp)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedControlPlane via v1alpha3")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oacp)
				})

				By("Reading back via v1alpha2 API")

				v2Obj := readResource(cpV1Alpha2GVK, resourceName, controlplaneTestNS)

				By("Verifying seconds are converted back to Duration strings")

				nodeDrain, found, _ := unstructured.NestedString(v2Obj.Object,
					"spec", "machineTemplate", "nodeDrainTimeout")
				Expect(found).To(BeTrue(), "nodeDrainTimeout should exist in v1alpha2")
				Expect(nodeDrain).To(Equal("5m30s"),
					"330 seconds should convert back to 5m30s")

				nodeVolumeDetach, found, _ := unstructured.NestedString(v2Obj.Object,
					"spec", "machineTemplate", "nodeVolumeDetachTimeout")
				Expect(found).To(BeTrue(), "nodeVolumeDetachTimeout should exist in v1alpha2")
				Expect(nodeVolumeDetach).To(Equal("2m0s"),
					"120 seconds should convert back to 2m0s")

				nodeDeletion, found, _ := unstructured.NestedString(v2Obj.Object,
					"spec", "machineTemplate", "nodeDeletionTimeout")
				Expect(found).To(BeTrue(), "nodeDeletionTimeout should exist in v1alpha2")
				Expect(nodeDeletion).To(Equal("10m0s"),
					"600 seconds should convert back to 10m0s")
			})

		It("Verifies InfrastructureRef type conversion between ObjectReference and ContractVersionedObjectReference",
			reportxml.ID("00006"), func() {
				resourceName := "conv-infraref-type"

				By("Creating OpenshiftAssistedControlPlane via v1alpha2 with full ObjectReference")

				oacp := newControlPlaneV1Alpha2(resourceName, map[string]interface{}{
					"distributionVersion": "4.17.0",
					"replicas":            int64(3),
					"machineTemplate": map[string]interface{}{
						"infrastructureRef": map[string]interface{}{
							"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
							"kind":       "OpenshiftAssistedMachineTemplate",
							"name":       "infra-ref-test",
							"namespace":  controlplaneTestNS,
						},
					},
					"config": map[string]interface{}{
						"baseDomain":  "example.com",
						"clusterName": "test-cluster",
					},
					"openshiftAssistedConfigSpec": map[string]interface{}{
						"cpuArchitecture": "x86_64",
					},
				})

				err := HubAPIClient.Create(context.TODO(), oacp)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedControlPlane via v1alpha2")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oacp)
				})

				By("Reading back via v1alpha3 — should be ContractVersionedObjectReference with apiGroup")

				v3Obj := readResource(cpV1Alpha3GVK, resourceName, controlplaneTestNS)

				apiGroup, _, _ := unstructured.NestedString(v3Obj.Object,
					"spec", "machineTemplate", "infrastructureRef", "apiGroup")
				Expect(apiGroup).To(Equal("infrastructure.cluster.x-k8s.io"),
					"apiGroup should be extracted from apiVersion")

				kind, _, _ := unstructured.NestedString(v3Obj.Object,
					"spec", "machineTemplate", "infrastructureRef", "kind")
				Expect(kind).To(Equal("OpenshiftAssistedMachineTemplate"),
					"kind should be preserved")

				name, _, _ := unstructured.NestedString(v3Obj.Object,
					"spec", "machineTemplate", "infrastructureRef", "name")
				Expect(name).To(Equal("infra-ref-test"),
					"name should be preserved")

				By("Reading back via v1alpha2 — should reconstruct ObjectReference with apiVersion")

				v2Obj := readResource(cpV1Alpha2GVK, resourceName, controlplaneTestNS)

				apiVersion, _, _ := unstructured.NestedString(v2Obj.Object,
					"spec", "machineTemplate", "infrastructureRef", "apiVersion")
				Expect(apiVersion).To(Equal("infrastructure.cluster.x-k8s.io/v1beta1"),
					"apiVersion should be reconstructed as group/v1beta1 on reverse conversion")
			})

		It("Verifies replica pointer boxing between v1alpha2 int32 and v1alpha3 *int32",
			reportxml.ID("00007"), func() {
				resourceName := "conv-replica-boxing"

				By("Creating OpenshiftAssistedControlPlane via v1alpha2 with replicas=3")

				oacp := newControlPlaneV1Alpha2(resourceName, map[string]interface{}{
					"distributionVersion": "4.17.0",
					"replicas":            int64(3),
					"machineTemplate": map[string]interface{}{
						"infrastructureRef": map[string]interface{}{
							"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
							"kind":       "OpenshiftAssistedMachineTemplate",
							"name":       "replica-test",
							"namespace":  controlplaneTestNS,
						},
					},
					"config": map[string]interface{}{
						"baseDomain":  "example.com",
						"clusterName": "test-cluster",
					},
					"openshiftAssistedConfigSpec": map[string]interface{}{
						"cpuArchitecture": "x86_64",
					},
				})

				err := HubAPIClient.Create(context.TODO(), oacp)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedControlPlane via v1alpha2")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oacp)
				})

				By("Reading back via v1alpha3")

				v3Obj := readResource(cpV1Alpha3GVK, resourceName, controlplaneTestNS)

				replicas, found, _ := unstructured.NestedFloat64(v3Obj.Object, "spec", "replicas")
				Expect(found).To(BeTrue(), "replicas should be present in v1alpha3")
				Expect(int32(replicas)).To(Equal(int32(3)),
					"replicas value should be preserved through pointer boxing")

				By("Reading back via v1alpha2")

				v2Obj := readResource(cpV1Alpha2GVK, resourceName, controlplaneTestNS)

				replicasV2, found, _ := unstructured.NestedFloat64(v2Obj.Object, "spec", "replicas")
				Expect(found).To(BeTrue(), "replicas should be present in v1alpha2")
				Expect(int32(replicasV2)).To(Equal(int32(3)),
					"replicas value should survive round-trip")
			})

		It("Verifies config spec fields survive v1alpha2 to v1alpha3 round-trip",
			reportxml.ID("00008"), func() {
				resourceName := "conv-config-roundtrip"

				By("Creating OpenshiftAssistedControlPlane via v1alpha2 with full config")

				oacp := newControlPlaneV1Alpha2(resourceName, map[string]interface{}{
					"distributionVersion": "4.17.0",
					"replicas":            int64(3),
					"machineTemplate": map[string]interface{}{
						"infrastructureRef": map[string]interface{}{
							"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
							"kind":       "OpenshiftAssistedMachineTemplate",
							"name":       "config-test",
							"namespace":  controlplaneTestNS,
						},
					},
					"config": map[string]interface{}{
						"baseDomain":         "conv-test.example.com",
						"clusterName":        "conv-cluster",
						"sshAuthorizedKey":   "ssh-rsa CONV...",
						"mastersSchedulable": true,
						"apiVIPs":            []interface{}{"192.168.1.100"},
						"ingressVIPs":        []interface{}{"192.168.1.101"},
					},
					"openshiftAssistedConfigSpec": map[string]interface{}{
						"cpuArchitecture":  "x86_64",
						"sshAuthorizedKey": "ssh-rsa BOOTSTRAP...",
					},
				})

				err := HubAPIClient.Create(context.TODO(), oacp)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedControlPlane via v1alpha2")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oacp)
				})

				By("Reading back via v1alpha3")

				v3Obj := readResource(cpV1Alpha3GVK, resourceName, controlplaneTestNS)

				baseDomain, _, _ := unstructured.NestedString(v3Obj.Object, "spec", "config", "baseDomain")
				Expect(baseDomain).To(Equal("conv-test.example.com"),
					"config.baseDomain should be preserved")

				clusterName, _, _ := unstructured.NestedString(v3Obj.Object, "spec", "config", "clusterName")
				Expect(clusterName).To(Equal("conv-cluster"),
					"config.clusterName should be preserved")

				mastersSchedulable, _, _ := unstructured.NestedBool(v3Obj.Object, "spec", "config", "mastersSchedulable")
				Expect(mastersSchedulable).To(BeTrue(), "config.mastersSchedulable should be preserved")

				apiVIPs, _, _ := unstructured.NestedStringSlice(v3Obj.Object, "spec", "config", "apiVIPs")
				Expect(apiVIPs).To(Equal([]string{"192.168.1.100"}),
					"config.apiVIPs should be preserved")

				bootstrapArch, _, _ := unstructured.NestedString(v3Obj.Object,
					"spec", "openshiftAssistedConfigSpec", "cpuArchitecture")
				Expect(bootstrapArch).To(Equal("x86_64"),
					"openshiftAssistedConfigSpec.cpuArchitecture should be preserved")

				By("Reading back via v1alpha2 to verify round-trip")

				v2Obj := readResource(cpV1Alpha2GVK, resourceName, controlplaneTestNS)

				baseDomainV2, _, _ := unstructured.NestedString(v2Obj.Object, "spec", "config", "baseDomain")
				Expect(baseDomainV2).To(Equal("conv-test.example.com"),
					"config.baseDomain should survive round-trip")
			})

		It("Verifies MachineTemplate metadata labels and annotations survive conversion",
			reportxml.ID("00009"), func() {
				resourceName := "conv-mt-metadata"

				By("Creating OpenshiftAssistedControlPlane via v1alpha2 with MachineTemplate metadata")

				oacp := newControlPlaneV1Alpha2(resourceName, map[string]interface{}{
					"distributionVersion": "4.17.0",
					"replicas":            int64(3),
					"machineTemplate": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"env":  "test",
								"tier": "control-plane",
							},
							"annotations": map[string]interface{}{
								"description": "conversion-test-machines",
							},
						},
						"infrastructureRef": map[string]interface{}{
							"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
							"kind":       "OpenshiftAssistedMachineTemplate",
							"name":       "metadata-test",
							"namespace":  controlplaneTestNS,
						},
					},
					"config": map[string]interface{}{
						"baseDomain":  "example.com",
						"clusterName": "test-cluster",
					},
					"openshiftAssistedConfigSpec": map[string]interface{}{
						"cpuArchitecture": "x86_64",
					},
				})

				err := HubAPIClient.Create(context.TODO(), oacp)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedControlPlane via v1alpha2")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oacp)
				})

				By("Reading back via v1alpha3")

				v3Obj := readResource(cpV1Alpha3GVK, resourceName, controlplaneTestNS)

				labels, _, _ := unstructured.NestedStringMap(v3Obj.Object,
					"spec", "machineTemplate", "metadata", "labels")
				Expect(labels).To(HaveKeyWithValue("env", "test"),
					"MachineTemplate labels should be preserved")
				Expect(labels).To(HaveKeyWithValue("tier", "control-plane"),
					"MachineTemplate labels should be preserved")

				annotations, _, _ := unstructured.NestedStringMap(v3Obj.Object,
					"spec", "machineTemplate", "metadata", "annotations")
				Expect(annotations).To(HaveKeyWithValue("description", "conversion-test-machines"),
					"MachineTemplate annotations should be preserved")

				By("Reading back via v1alpha2 to verify round-trip")

				v2Obj := readResource(cpV1Alpha2GVK, resourceName, controlplaneTestNS)

				labelsV2, _, _ := unstructured.NestedStringMap(v2Obj.Object,
					"spec", "machineTemplate", "metadata", "labels")
				Expect(labelsV2).To(HaveKeyWithValue("env", "test"),
					"MachineTemplate labels should survive round-trip")
			})

		It("Verifies v1alpha2 bootstrap config spec is converted to v1alpha2 format in v1alpha3",
			reportxml.ID("00010"), func() {
				resourceName := "conv-bootstrap-spec-upgrade"

				By("Creating OpenshiftAssistedControlPlane via v1alpha2 with v1alpha1-based bootstrap config")

				oacp := newControlPlaneV1Alpha2(resourceName, map[string]interface{}{
					"distributionVersion": "4.17.0",
					"replicas":            int64(3),
					"machineTemplate": map[string]interface{}{
						"infrastructureRef": map[string]interface{}{
							"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
							"kind":       "OpenshiftAssistedMachineTemplate",
							"name":       "bootstrap-spec-test",
							"namespace":  controlplaneTestNS,
						},
					},
					"config": map[string]interface{}{
						"baseDomain":  "example.com",
						"clusterName": "test-cluster",
					},
					"openshiftAssistedConfigSpec": map[string]interface{}{
						"cpuArchitecture":      "x86_64",
						"sshAuthorizedKey":     "ssh-rsa TEST...",
						"additionalNTPSources": []interface{}{"pool.ntp.org"},
						"nodeRegistration": map[string]interface{}{
							"name": "worker-node",
						},
					},
				})

				err := HubAPIClient.Create(context.TODO(), oacp)
				Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedControlPlane via v1alpha2")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), oacp)
				})

				By("Reading back via v1alpha3")

				v3Obj := readResource(cpV1Alpha3GVK, resourceName, controlplaneTestNS)

				arch, _, _ := unstructured.NestedString(v3Obj.Object,
					"spec", "openshiftAssistedConfigSpec", "cpuArchitecture")
				Expect(arch).To(Equal("x86_64"),
					"bootstrap cpuArchitecture should be converted to v1alpha2 format in v1alpha3")

				ntp, _, _ := unstructured.NestedStringSlice(v3Obj.Object,
					"spec", "openshiftAssistedConfigSpec", "additionalNTPSources")
				Expect(ntp).To(Equal([]string{"pool.ntp.org"}),
					"bootstrap additionalNTPSources should be preserved")

				nodeName, _, _ := unstructured.NestedString(v3Obj.Object,
					"spec", "openshiftAssistedConfigSpec", "nodeRegistration", "name")
				Expect(nodeName).To(Equal("worker-node"),
					"bootstrap nodeRegistration.name should be preserved")

				By("Reading back via v1alpha2 to verify round-trip")

				v2Obj := readResource(cpV1Alpha2GVK, resourceName, controlplaneTestNS)

				archV2, _, _ := unstructured.NestedString(v2Obj.Object,
					"spec", "openshiftAssistedConfigSpec", "cpuArchitecture")
				Expect(archV2).To(Equal("x86_64"),
					"bootstrap cpuArchitecture should survive round-trip")
			})
	})

func newControlPlaneV1Alpha2(name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "controlplane.cluster.x-k8s.io/v1alpha2",
			"kind":       "OpenshiftAssistedControlPlane",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": controlplaneTestNS,
			},
			"spec": spec,
		},
	}
}

func newControlPlaneV1Alpha3(name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "controlplane.cluster.x-k8s.io/v1alpha3",
			"kind":       "OpenshiftAssistedControlPlane",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": controlplaneTestNS,
			},
			"spec": spec,
		},
	}
}
