package capoa_networktype_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/capi"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/capoa"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	v1alpha3 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/capoa/controlplane/v1alpha3"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-networktype/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-networktype/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	clusterNameLabel         = "cluster.x-k8s.io/cluster-name"
	machineControlPlaneLabel = "cluster.x-k8s.io/control-plane"
	testClusterName          = "nt-test"
	testBaseDomain           = "networktype-test.example.com"
	pullSecretName           = "nt-test-pull-secret"
	// GA version whose release image exists on quay.io/openshift-release-dev/ocp-release.
	// Hub nightly versions are not published to quay.io, so we use a fixed GA version
	// that the CD controller can resolve for digest lookup.
	testDistributionVersion = "4.22.0"
)

func newCAPICluster(name, namespaceName string) *capi.ClusterBuilder {
	return capi.NewClusterBuilder(HubAPIClient, name, namespaceName).
		WithControlPlaneEndpoint("example.com", 8080).
		WithControlPlaneRef(
			"controlplane.cluster.x-k8s.io/v1alpha3",
			"OpenshiftAssistedControlPlane",
			name,
		)
}

func newOpenshiftAssistedConfig(name, namespaceName, clusterName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
			"kind":       "OpenshiftAssistedConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespaceName,
				"labels": map[string]interface{}{
					clusterNameLabel:         clusterName,
					machineControlPlaneLabel: "control-plane",
				},
			},
			"spec": map[string]interface{}{
				"cpuArchitecture": "x86_64",
			},
		},
	}
}

func newV1Alpha2OACP(name, namespaceName string, fields map[string]interface{}) *unstructured.Unstructured {
	configMap := map[string]interface{}{
		"baseDomain": testBaseDomain,
		"pullSecretRef": map[string]interface{}{
			"name": pullSecretName,
		},
	}

	for k, v := range fields {
		configMap[k] = v
	}

	spec := map[string]interface{}{
		"replicas":            int64(1),
		"distributionVersion": testDistributionVersion,
		"config":              configMap,
		"machineTemplate": map[string]interface{}{
			"infrastructureRef": map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"kind":       "Metal3MachineTemplate",
				"name":       name + "-mt",
			},
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "controlplane.cluster.x-k8s.io/v1alpha2",
			"kind":       "OpenshiftAssistedControlPlane",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespaceName,
				"labels": map[string]interface{}{
					clusterNameLabel: name,
				},
			},
			"spec": spec,
		},
	}
}

func testMachineTemplate(name string) v1alpha3.OpenshiftAssistedControlPlaneMachineTemplate {
	nodeDeletionTimeout := int32(10)

	return v1alpha3.OpenshiftAssistedControlPlaneMachineTemplate{
		ObjectMeta: v1alpha3.ObjectMeta{
			Labels: map[string]string{clusterNameLabel: name},
		},
		InfrastructureRef: v1alpha3.ContractVersionedObjectReference{
			APIGroup: "infrastructure.cluster.x-k8s.io",
			Kind:     "Metal3MachineTemplate",
			Name:     name + "-mt",
		},
		Deletion: v1alpha3.MachineDeletionSpec{
			NodeDeletionTimeoutSeconds: &nodeDeletionTimeout,
		},
	}
}

func waitForACI(name, namespaceName string) *assisted.AgentClusterInstallBuilder {
	var aciBuilder *assisted.AgentClusterInstallBuilder

	Eventually(func() error {
		var err error

		aciBuilder, err = assisted.PullAgentClusterInstall(HubAPIClient, name, namespaceName)

		return err
	}).WithTimeout(3*time.Minute).WithPolling(5*time.Second).Should(
		Succeed(), "timed out waiting for ACI to be created by controller")

	return aciBuilder
}

var _ = Describe(
	"CAPOA networkType controller propagation",
	Ordered, ContinueOnFailure,
	Label(tsparams.LabelSuite), func() {
		var (
			testNS         *namespace.Builder
			clusterBuilder *capi.ClusterBuilder
			oacpBuilder    *capoa.OpenshiftAssistedControlPlaneBuilder
		)

		BeforeAll(func() {
			By("Creating test namespace")

			var err error

			testNS, err = namespace.NewBuilder(HubAPIClient, tsparams.TestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "failed to create test namespace")

			By("Copying hub pull secret into test namespace")

			_, err = secret.NewBuilder(
				HubAPIClient,
				pullSecretName,
				tsparams.TestNamespace,
				corev1.SecretTypeDockerConfigJson,
			).WithData(tsparams.HubPullSecretData).Create()
			Expect(err).ToNot(HaveOccurred(), "failed to create pull secret")

			By("Creating CAPI Cluster resource")

			clusterBuilder, err = newCAPICluster(testClusterName, tsparams.TestNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "failed to create CAPI Cluster")

			By("Creating ClusterDeployment with CAPI cluster label")

			cdBuilder := hive.NewABMClusterDeploymentBuilder(
				HubAPIClient,
				testClusterName,
				tsparams.TestNamespace,
				testClusterName,
				testBaseDomain,
				testClusterName,
				metav1.LabelSelector{
					MatchLabels: map[string]string{"dummy": "label"},
				},
			).WithPullSecret(pullSecretName)

			cdBuilder.Definition.Labels = map[string]string{
				clusterNameLabel: testClusterName,
			}

			_, err = cdBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "failed to create ClusterDeployment")

			By("Creating OpenshiftAssistedConfig for architecture detection")

			oac := newOpenshiftAssistedConfig(
				fmt.Sprintf("%s-config", testClusterName),
				tsparams.TestNamespace,
				testClusterName,
			)

			err = HubAPIClient.Create(context.TODO(), oac)
			Expect(err).ToNot(HaveOccurred(), "failed to create OpenshiftAssistedConfig")
		})

		AfterAll(func() {
			By("Deleting test namespace and all contained resources")

			if testNS != nil {
				err := testNS.DeleteAndWait(2 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "failed to delete test namespace")
			}
		})

		It("propagates networkType from OACP to ACI",
			reportxml.ID("90048"), func() {
				By("Creating OACP with networkType Cilium")

				clusterUID := clusterBuilder.Object.GetUID()

				oacpBuilder = capoa.NewOpenshiftAssistedControlPlaneBuilder(
					HubAPIClient,
					testClusterName,
					tsparams.TestNamespace,
					testBaseDomain,
					testDistributionVersion,
					1,
				).WithNetworkType("Cilium").
					WithPullSecretRef(pullSecretName).
					WithMachineTemplate(testMachineTemplate(testClusterName))

				oacpBuilder.Definition.Labels = map[string]string{
					clusterNameLabel: testClusterName,
				}
				oacpBuilder.Definition.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "cluster.x-k8s.io/v1beta1",
						Kind:       "Cluster",
						Name:       testClusterName,
						UID:        clusterUID,
					},
				}

				var err error

				oacpBuilder, err = oacpBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "failed to create OACP")

				By("Waiting for controller to create ACI and verifying networkType")

				aciBuilder := waitForACI(testClusterName, tsparams.TestNamespace)
				Expect(aciBuilder.Object.Spec.Networking.NetworkType).To(
					Equal("Cilium"),
					"ACI networkType should match OACP networkType",
				)
			})

		It("propagates updated networkType to ACI",
			reportxml.ID("90054"), func() {
				Expect(oacpBuilder).ToNot(BeNil(), "OACP builder must exist from previous test")

				By("Re-fetching OACP to get current resourceVersion")

				var err error

				oacpBuilder, err = capoa.PullOpenshiftAssistedControlPlane(
					HubAPIClient, testClusterName, tsparams.TestNamespace)
				Expect(err).ToNot(HaveOccurred(), "failed to re-fetch OACP")

				By("Updating OACP networkType from Cilium to OVNKubernetes")

				oacpBuilder.Definition.Spec.Config.NetworkType = "OVNKubernetes"

				oacpBuilder, err = oacpBuilder.Update(false)
				Expect(err).ToNot(HaveOccurred(), "failed to update OACP networkType")

				By("Verifying ACI networkType is updated by controller")

				Eventually(func() string {
					aciBuilder, err := assisted.PullAgentClusterInstall(
						HubAPIClient, testClusterName, tsparams.TestNamespace)
					if err != nil {
						return ""
					}

					return aciBuilder.Object.Spec.Networking.NetworkType
				}).WithTimeout(3*time.Minute).WithPolling(5*time.Second).Should(
					Equal("OVNKubernetes"),
					"ACI networkType should be updated to OVNKubernetes",
				)
			})

		It("preserves backward compatibility when networkType is omitted",
			reportxml.ID("90050"), func() {
				By("Deleting existing OACP")

				if oacpBuilder != nil {
					err := oacpBuilder.DeleteAndWait(1 * time.Minute)
					Expect(err).ToNot(HaveOccurred(), "failed to delete existing OACP")
				}

				By("Waiting for controller to clean up ACI")

				Eventually(func() bool {
					_, err := assisted.PullAgentClusterInstall(
						HubAPIClient, testClusterName, tsparams.TestNamespace)

					return err != nil
				}).WithTimeout(2*time.Minute).WithPolling(5*time.Second).Should(
					BeTrue(), "ACI should be removed after OACP deletion")

				By("Creating new OACP without networkType")

				clusterUID := clusterBuilder.Object.GetUID()

				oacpBuilder = capoa.NewOpenshiftAssistedControlPlaneBuilder(
					HubAPIClient,
					testClusterName,
					tsparams.TestNamespace,
					testBaseDomain,
					testDistributionVersion,
					1,
				).WithPullSecretRef(pullSecretName).
					WithMachineTemplate(testMachineTemplate(testClusterName))

				oacpBuilder.Definition.Labels = map[string]string{
					clusterNameLabel: testClusterName,
				}
				oacpBuilder.Definition.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: "cluster.x-k8s.io/v1beta1",
						Kind:       "Cluster",
						Name:       testClusterName,
						UID:        clusterUID,
					},
				}

				var err error

				oacpBuilder, err = oacpBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "failed to create OACP without networkType")

				By("Waiting for ACI to be created by controller")

				aciBuilder := waitForACI(testClusterName, tsparams.TestNamespace)
				Expect(aciBuilder.Object.Spec.Networking.NetworkType).To(BeEmpty(),
					"ACI networkType should be empty on first appearance")

				By("Verifying ACI networkType remains empty across subsequent polls")

				Consistently(func() string {
					aciBuilder, err := assisted.PullAgentClusterInstall(
						HubAPIClient, testClusterName, tsparams.TestNamespace)
					if err != nil {
						return "PULL_ERROR"
					}

					return aciBuilder.Object.Spec.Networking.NetworkType
				}).WithTimeout(30*time.Second).WithPolling(5*time.Second).Should(
					BeEmpty(),
					"ACI networkType should remain empty when OACP omits it",
				)
			})
	})

var _ = Describe(
	"CAPOA networkType CRD validation",
	Label(tsparams.LabelSuite), func() {
		It("rejects invalid networkType value",
			reportxml.ID("90049"), func() {
				By("Ensuring test namespace exists")

				_, err := namespace.NewBuilder(HubAPIClient, tsparams.TestNamespace).Create()
				if err != nil {
					Expect(apierrors.IsAlreadyExists(err)).To(BeTrue(),
						"unexpected error creating test namespace: "+err.Error())
				}

				By("Attempting to create OACP with invalid networkType Flannel")

				invalidOACP := capoa.NewOpenshiftAssistedControlPlaneBuilder(
					HubAPIClient,
					"nt-validation-test",
					tsparams.TestNamespace,
					testBaseDomain,
					testDistributionVersion,
					1,
				).WithNetworkType("Flannel").
					WithPullSecretRef(pullSecretName).
					WithMachineTemplate(testMachineTemplate("nt-validation-test"))

				_, err = invalidOACP.Create()
				Expect(err).To(HaveOccurred(),
					"OACP with invalid networkType should be rejected by CRD validation")
				Expect(err.Error()).To(
					ContainSubstring("networkType"),
					"error should reference networkType field",
				)
			})
	})

var _ = Describe(
	"CAPOA machineTemplate optional fields",
	Label(tsparams.LabelSuite), func() {
		It("accepts OACP with only infrastructureRef in machineTemplate",
			reportxml.ID("90160"), func() {
				Skip("Blocked by https://redhat.atlassian.net/browse/MGMT-24965" +
					": machineTemplate.metadata and .deletion missing omitzero tag")

				By("Ensuring test namespace exists")

				_, err := namespace.NewBuilder(HubAPIClient, tsparams.TestNamespace).Create()
				if err != nil {
					Expect(apierrors.IsAlreadyExists(err)).To(BeTrue(),
						"unexpected error creating test namespace: "+err.Error())
				}

				By("Creating OACP with minimal machineTemplate (no metadata/deletion)")

				minimalTemplate := v1alpha3.OpenshiftAssistedControlPlaneMachineTemplate{
					InfrastructureRef: v1alpha3.ContractVersionedObjectReference{
						APIGroup: "infrastructure.cluster.x-k8s.io",
						Kind:     "Metal3MachineTemplate",
						Name:     "nt-minprops-mt",
					},
				}

				oacp := capoa.NewOpenshiftAssistedControlPlaneBuilder(
					HubAPIClient,
					"nt-minprops-test",
					tsparams.TestNamespace,
					testBaseDomain,
					testDistributionVersion,
					1,
				).WithMachineTemplate(minimalTemplate)

				_, err = oacp.Create()
				Expect(err).ToNot(HaveOccurred(),
					"OACP with only infrastructureRef in machineTemplate should be accepted")

				DeferCleanup(func() {
					_ = oacp.Delete()
				})
			})
	})

var _ = Describe(
	"CAPOA networkType API conversion",
	Ordered, ContinueOnFailure,
	Label(tsparams.LabelSuite), func() {
		const conversionTestName = "nt-conversion-test"

		BeforeAll(func() {
			By("Ensuring test namespace exists")

			_, err := namespace.NewBuilder(HubAPIClient, tsparams.TestNamespace).Create()
			if err != nil {
				Expect(apierrors.IsAlreadyExists(err)).To(BeTrue(),
					"unexpected error creating test namespace: "+err.Error())
			}

			By("Ensuring pull secret exists in test namespace")

			_, err = secret.NewBuilder(
				HubAPIClient,
				pullSecretName,
				tsparams.TestNamespace,
				corev1.SecretTypeDockerConfigJson,
			).WithData(tsparams.HubPullSecretData).Create()
			if err != nil {
				Expect(apierrors.IsAlreadyExists(err)).To(BeTrue(),
					"unexpected error creating pull secret: "+err.Error())
			}
		})

		It("preserves networkType during v1alpha2 to v1alpha3 conversion",
			reportxml.ID("90051"), func() {
				By("Creating OACP via v1alpha2 API with networkType OVNKubernetes")

				v1alpha2OACP := newV1Alpha2OACP(
					conversionTestName+"-v2",
					tsparams.TestNamespace,
					map[string]interface{}{
						"networkType": "OVNKubernetes",
					},
				)

				err := HubAPIClient.Create(context.TODO(), v1alpha2OACP)
				Expect(err).ToNot(HaveOccurred(),
					"failed to create OACP via v1alpha2 API")

				DeferCleanup(func() {
					_ = HubAPIClient.Delete(context.TODO(), v1alpha2OACP)
				})

				By("Reading back via v1alpha3 typed builder")

				v3Builder, err := capoa.PullOpenshiftAssistedControlPlane(
					HubAPIClient,
					conversionTestName+"-v2",
					tsparams.TestNamespace,
				)
				Expect(err).ToNot(HaveOccurred(),
					"failed to pull OACP via v1alpha3 API")

				Expect(v3Builder.GetNetworkType()).To(
					Equal("OVNKubernetes"),
					"networkType should be preserved during v1alpha2 → v1alpha3 conversion",
				)
			})

		It("preserves networkType during v1alpha3 to v1alpha2 conversion",
			reportxml.ID("90051"), func() {
				By("Creating OACP via v1alpha3 typed builder with networkType Calico")

				v3Builder := capoa.NewOpenshiftAssistedControlPlaneBuilder(
					HubAPIClient,
					conversionTestName+"-v3",
					tsparams.TestNamespace,
					testBaseDomain,
					testDistributionVersion,
					1,
				).WithNetworkType("Calico").
					WithPullSecretRef(pullSecretName).
					WithMachineTemplate(testMachineTemplate(conversionTestName + "-v3"))

				var err error

				v3Builder, err = v3Builder.Create()
				Expect(err).ToNot(HaveOccurred(),
					"failed to create OACP via v1alpha3 typed builder")

				DeferCleanup(func() {
					_ = v3Builder.DeleteAndWait(1 * time.Minute)
				})

				By("Reading back via v1alpha2 unstructured API")

				v2OACP := &unstructured.Unstructured{}
				v2OACP.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "controlplane.cluster.x-k8s.io",
					Version: "v1alpha2",
					Kind:    "OpenshiftAssistedControlPlane",
				})

				err = HubAPIClient.Get(
					context.TODO(),
					types.NamespacedName{
						Name:      conversionTestName + "-v3",
						Namespace: tsparams.TestNamespace,
					},
					v2OACP,
				)
				Expect(err).ToNot(HaveOccurred(),
					"failed to read OACP via v1alpha2 API")

				spec, found, err := unstructured.NestedString(
					v2OACP.Object, "spec", "config", "networkType")
				Expect(err).ToNot(HaveOccurred(), "failed to extract networkType from v1alpha2 spec")
				Expect(found).To(BeTrue(), "networkType should exist in v1alpha2 spec")
				Expect(spec).To(
					Equal("Calico"),
					"networkType should be preserved during v1alpha3 → v1alpha2 conversion",
				)
			})
	})
