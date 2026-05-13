package capoa_tls_test

import (
	"context"
	"crypto/tls"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-tls/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-tls/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/tlsprofile"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var customCiphers = []string{
	"ECDHE-RSA-AES128-GCM-SHA256",
	"ECDHE-RSA-AES256-GCM-SHA384",
	"ECDHE-ECDSA-AES128-GCM-SHA256",
	"ECDHE-ECDSA-AES256-GCM-SHA384",
}

func customTLSProfile(ciphers []string) configv1.TLSSecurityProfile {
	return configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers:       ciphers,
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
	}
}

var capoa = &tlsprofile.Component{
	Name:        "CAPOA",
	Namespace:   "multicluster-engine",
	RestartMode: tlsprofile.RestartModeContainerRestart,
	Endpoints: []tlsprofile.Endpoint{
		{
			ServiceName:    "capoa-bootstrap-webhook-service",
			LocalPort:      19443,
			RemotePort:     9443,
			DeploymentName: "capoa-bootstrap-controller-manager",
		},
		{
			ServiceName:    "capoa-controlplane-webhook-service",
			LocalPort:      19444,
			RemotePort:     9443,
			DeploymentName: "capoa-controlplane-controller-manager",
		},
	},
	Deployments: []tlsprofile.Deployment{
		{Name: "capoa-bootstrap-controller-manager", ContainerName: "manager"},
		{Name: "capoa-controlplane-controller-manager", ContainerName: "manager"},
	},
	ListPods: func(client *clients.Settings, ns string) ([]*pod.Builder, error) {
		return pod.ListByNamePattern(client, "capoa", ns)
	},
	ExpectedHealthyPods: 2,
	PodReadyTimeout:     5 * time.Minute,
	AutoRestartTimeout:  10 * time.Minute,
	HonoringLogPattern:  "honoring cluster-wide TLS profile",
	AllowedCipher:       tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	AllowedCipherAlt:    tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	DisallowedCipher:    tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	OldProfileCipher:    tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
}

const (
	adherenceStrictAllComponents = "StrictAllComponents"
	adherenceLegacyAdheringOnly  = "LegacyAdheringComponentsOnly"

	defaultsLogPattern = "using defaults"
)

func ensureTLSAdherence() {
	apiserverU := &unstructured.Unstructured{}
	apiserverU.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "config.openshift.io",
		Version: "v1",
		Kind:    "APIServer",
	})

	err := HubAPIClient.Get(context.TODO(),
		runtimeclient.ObjectKey{Name: "cluster"}, apiserverU)
	Expect(err).ToNot(HaveOccurred(), "failed to get apiserver/cluster")

	adherence, found, err := unstructured.NestedString(
		apiserverU.Object, "spec", "tlsAdherence")
	Expect(err).ToNot(HaveOccurred(), "failed to read spec.tlsAdherence from apiserver/cluster")

	if found && adherence == adherenceStrictAllComponents {
		return
	}

	By("Setting tlsAdherence to StrictAllComponents")
	tlsprofile.PatchTLSAdherence(HubAPIClient, adherenceStrictAllComponents)
}

// Tests are ordered to minimize TLS profile changes and cluster churn.
// Flow: Intermediate → Old → Modern → Custom → (reuse) → reconciliation → restore
// → LegacyAdheringComponentsOnly/StrictAllComponents adherence transitions → restore.
var _ = Describe(
	"CAPOA TLS Profile",
	Ordered, ContinueOnFailure,
	Label(tsparams.LabelSuite), func() {
		BeforeAll(func() {
			By("Verifying hub API client is available")

			if HubAPIClient == nil {
				Skip("Hub API client is nil")
			}

			By("Ensuring TLS adherence is set on the cluster")
			ensureTLSAdherence()

			By("Waiting for cluster to stabilize")
			tlsprofile.WaitForClusterStability(HubAPIClient, 15*time.Minute)

			By("Ensuring Intermediate baseline")
			tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)

			By("Verifying CAPOA pods are running")

			pods, err := capoa.ListPods(HubAPIClient, capoa.Namespace)
			Expect(err).ToNot(HaveOccurred(), "failed to list CAPOA pods")

			if len(pods) == 0 {
				Skip("CAPOA pods not found — not deployed")
			}

			tlsprofile.WaitPodsReady(HubAPIClient, capoa)
		})

		AfterAll(func() {
			By("Restoring default Intermediate TLS profile")
			tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
			tlsprofile.StopAllPortForwards()
		})

		// 1. Intermediate (no profile change — already baseline)
		It("Verifies default Intermediate TLS profile on CAPOA endpoints",
			reportxml.ID("88843"), func() {
				By("Verifying controller logs show honoring message")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, capoa.HonoringLogPattern)
				}

				for _, endpoint := range capoa.Endpoints {
					By("Probing TLS 1.2 on " + endpoint.ServiceName)
					tlsprofile.AssertTLSConnects(HubAPIClient, capoa, endpoint,
						tls.VersionTLS12, tls.VersionTLS12, nil)

					By("Probing TLS 1.3 on " + endpoint.ServiceName)
					tlsprofile.AssertTLSConnects(HubAPIClient, capoa, endpoint,
						tls.VersionTLS13, tls.VersionTLS13, nil)
				}

				By("Verifying TLS 1.1 is rejected")
				tlsprofile.AssertTLSRejectedVersion(HubAPIClient, capoa,
					capoa.Endpoints[0], tls.VersionTLS11)

				By("Verifying TLS 1.0 is rejected")
				tlsprofile.AssertTLSRejectedVersion(HubAPIClient, capoa,
					capoa.Endpoints[0], tls.VersionTLS10)
			})

		// 2. Intermediate → Old (1 change)
		It("Verifies Old TLS profile enables broader cipher set on CAPOA",
			reportxml.ID("88844"), func() {
				By("Applying Old TLS profile")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient,
					configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
						Old:  &configv1.OldTLSProfile{},
					})

				By("Waiting for CAPOA pods to pick up Old profile")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)

				By("Verifying controller logs show VersionTLS10")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, "VersionTLS10")
				}

				for _, endpoint := range capoa.Endpoints {
					By("Verifying Old-specific cipher connects on " + endpoint.ServiceName)
					tlsprofile.AssertTLSConnects(HubAPIClient, capoa, endpoint,
						tls.VersionTLS12, tls.VersionTLS12,
						[]uint16{capoa.OldProfileCipher})
				}
			})

		// 3. Old → Modern (1 change)
		It("Verifies Modern TLS profile restricts to TLS 1.3 only on CAPOA",
			reportxml.ID("88845"), func() {
				By("Applying Modern TLS profile")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient,
					configv1.TLSSecurityProfile{
						Type:   configv1.TLSProfileModernType,
						Modern: &configv1.ModernTLSProfile{},
					})

				By("Waiting for CAPOA pods to pick up Modern profile")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)

				By("Verifying controller logs show VersionTLS13")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, "VersionTLS13")
				}

				for _, endpoint := range capoa.Endpoints {
					By("Verifying TLS 1.3 connects on " + endpoint.ServiceName)
					tlsprofile.AssertTLSConnects(HubAPIClient, capoa, endpoint,
						tls.VersionTLS13, tls.VersionTLS13, nil)

					By("Verifying TLS 1.2 is rejected on " + endpoint.ServiceName)
					tlsprofile.AssertTLSRejected(HubAPIClient, capoa, endpoint, nil)
				}
			})

		// 4. Modern → Custom (1 change)
		It("Verifies Custom TLS profile restricts to specified ciphers on CAPOA",
			reportxml.ID("88846"), func() {
				By("Applying Custom TLS profile")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient,
					customTLSProfile(customCiphers))

				By("Waiting for CAPOA pods to pick up Custom profile")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)

				for _, endpoint := range capoa.Endpoints {
					By("Verifying allowed cipher connects on " + endpoint.ServiceName)
					tlsprofile.AssertTLSConnects(HubAPIClient, capoa, endpoint,
						tls.VersionTLS12, tls.VersionTLS12,
						[]uint16{capoa.AllowedCipher})

					By("Verifying disallowed cipher is rejected on " + endpoint.ServiceName)
					tlsprofile.AssertTLSRejected(HubAPIClient, capoa, endpoint,
						[]uint16{capoa.DisallowedCipher})
				}
			})

		// 5. Custom (no change — reuse from 88846)
		It("Verifies webhook validation works after TLS profile change on CAPOA",
			reportxml.ID("88848"), func() {
				By("Ensuring test namespace does not exist")

				nsBuilder := namespace.NewBuilder(HubAPIClient, tsparams.TestNamespace)
				_ = nsBuilder.DeleteAndWait(2 * time.Minute)

				By("Creating test namespace")

				_, err := nsBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "failed to create test namespace")

				DeferCleanup(func() {
					_ = nsBuilder.DeleteAndWait(2 * time.Minute)
				})

				By("Creating valid OpenshiftAssistedConfig")

				resource := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
						"kind":       "OpenshiftAssistedConfig",
						"metadata": map[string]interface{}{
							"name":      "test-webhook-validation",
							"namespace": tsparams.TestNamespace,
						},
						"spec": map[string]interface{}{
							"cpuArchitecture": "x86_64",
						},
					},
				}

				err = HubAPIClient.Create(context.TODO(), resource)
				Expect(err).ToNot(HaveOccurred(),
					"valid OpenshiftAssistedConfig should be accepted")

				By("Attempting to update spec (should be rejected)")

				mutationPatch := []byte(
					`{"spec":{"cpuArchitecture":"aarch64"}}`)
				err = HubAPIClient.Patch(context.TODO(), resource,
					runtimeclient.RawPatch(types.MergePatchType, mutationPatch))
				Expect(err).To(HaveOccurred(),
					"spec update should be rejected by webhook")
				Expect(err.Error()).To(ContainSubstring("immutable"),
					"rejection reason should mention immutable")

				By("Updating metadata only (should succeed)")

				labelPatch := []byte(
					`{"metadata":{"labels":{"tls-profile-test":"validation"}}}`)
				err = HubAPIClient.Patch(context.TODO(), resource,
					runtimeclient.RawPatch(types.MergePatchType, labelPatch))
				Expect(err).ToNot(HaveOccurred(),
					"metadata update should be accepted")

				By("Deleting the resource")

				err = HubAPIClient.Delete(context.TODO(), resource)
				Expect(err).ToNot(HaveOccurred(), "deletion should succeed")
			})

		// 6. Custom → Intermediate → Custom (2 changes, test auto-restart)
		It("Verifies SecurityProfileWatcher triggers restart on CAPOA",
			reportxml.ID("88858"), func() {
				By("Ensuring Intermediate baseline")
				tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitPodsReady(HubAPIClient, capoa)

				By("Changing TLS profile to Custom")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient,
					customTLSProfile(customCiphers))

				By("Waiting for automatic restart")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitPodsReady(HubAPIClient, capoa)

				By("Verifying controllers honour the new profile")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, capoa.HonoringLogPattern)
				}
			})

		// 7. Custom → single-cipher Custom → Intermediate (3 changes, reconciliation)
		It("Verifies profile change triggers automatic reconciliation on CAPOA",
			reportxml.ID("88847"), func() {
				By("Recording baseline cipher connectivity")
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{capoa.AllowedCipher})

				singleCiphers := []string{
					"ECDHE-RSA-AES128-GCM-SHA256",
					"ECDHE-ECDSA-AES128-GCM-SHA256",
				}

				By("Switching to Custom single-cipher profile")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient,
					customTLSProfile(singleCiphers))

				By("Waiting for automatic reconciliation (no manual pod restart)")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitForClusterStability(HubAPIClient, 15*time.Minute)

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, capoa.HonoringLogPattern)
				}

				By("Verifying AES256 is now rejected")
				tlsprofile.AssertTLSRejected(HubAPIClient, capoa, capoa.Endpoints[0],
					[]uint16{capoa.AllowedCipherAlt})

				By("Switching back to Intermediate")
				tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitForClusterStability(HubAPIClient, 15*time.Minute)

				By("Verifying AES256 is restored under Intermediate")
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{capoa.AllowedCipherAlt})
			})

		// 8. Intermediate (no change — verify restore from 88847)
		It("Verifies restore to default profile on CAPOA",
			reportxml.ID("88850"), func() {
				By("Waiting for cluster to stabilize after previous profile changes")
				tlsprofile.WaitForClusterStability(HubAPIClient, 15*time.Minute)

				By("Verifying no tlsSecurityProfile remains on apiserver")

				apiserver := &configv1.APIServer{}

				err := HubAPIClient.Get(context.TODO(),
					runtimeclient.ObjectKey{Name: "cluster"}, apiserver)
				Expect(err).ToNot(HaveOccurred())
				Expect(apiserver.Spec.TLSSecurityProfile).To(BeNil(),
					"tlsSecurityProfile should be nil after restore")

				By("Verifying controller logs show VersionTLS12")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, "VersionTLS12")
				}

				By("Verifying Intermediate ciphers are available")
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{capoa.AllowedCipher})
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS13, tls.VersionTLS13, nil)
			})

		// 9. LegacyAdheringComponentsOnly ignores profile, StrictAllComponents enforces it
		It("Verifies non-honoring adherence ignores TLS profile and StrictAllComponents enforces it on CAPOA",
			reportxml.ID("89003"), func() {
				Skip("Blocked by ACM-34017: SecurityProfileWatcher restart loop — https://redhat.atlassian.net/browse/ACM-34017")

				By("Setting adherence to LegacyAdheringComponentsOnly")
				tlsprofile.PatchTLSAdherence(HubAPIClient, adherenceLegacyAdheringOnly)

				By("Waiting for CAPOA pods to restart after adherence change")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitPodsReady(HubAPIClient, capoa)

				By("Applying Modern TLS profile (TLS 1.3 only)")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient,
					configv1.TLSSecurityProfile{
						Type:   configv1.TLSProfileModernType,
						Modern: &configv1.ModernTLSProfile{},
					})

				By("Waiting for CAPOA pods to restart after profile change")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitPodsReady(HubAPIClient, capoa)

				By("Verifying controller logs show defaults path (not honoring)")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, defaultsLogPattern)
				}

				By("Verifying TLS 1.2 still connects (Modern profile is ignored)")
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS12, tls.VersionTLS12, nil)

				By("Switching adherence to StrictAllComponents")
				tlsprofile.PatchTLSAdherence(HubAPIClient, adherenceStrictAllComponents)

				By("Waiting for CAPOA pods to restart after adherence change")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitPodsReady(HubAPIClient, capoa)

				By("Verifying controller logs show honoring message")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, capoa.HonoringLogPattern)
				}

				By("Verifying TLS 1.2 is now rejected (Modern profile enforced)")
				tlsprofile.AssertTLSRejected(HubAPIClient, capoa, capoa.Endpoints[0], nil)

				By("Verifying TLS 1.3 connects")
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS13, tls.VersionTLS13, nil)
			})

		// 10. StrictAllComponents → LegacyAdheringComponentsOnly reverts to Intermediate defaults
		It("Verifies switching to non-honoring adherence reverts CAPOA to Intermediate defaults",
			reportxml.ID("89004"), func() {
				Skip("Blocked by ACM-34017: SecurityProfileWatcher restart loop — https://redhat.atlassian.net/browse/ACM-34017")

				By("Ensuring StrictAllComponents + Modern baseline from previous test")
				tlsprofile.WaitPodsReady(HubAPIClient, capoa)

				By("Verifying TLS 1.2 is rejected under Modern profile")
				tlsprofile.AssertTLSRejected(HubAPIClient, capoa, capoa.Endpoints[0], nil)

				By("Switching adherence to LegacyAdheringComponentsOnly")
				tlsprofile.PatchTLSAdherence(HubAPIClient, adherenceLegacyAdheringOnly)

				By("Waiting for CAPOA pods to restart after adherence change")
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
				tlsprofile.WaitPodsReady(HubAPIClient, capoa)

				By("Verifying controller logs show defaults path")

				for _, d := range capoa.Deployments {
					tlsprofile.AssertControllerLogsContain(HubAPIClient, capoa,
						d, defaultsLogPattern)
				}

				By("Verifying TLS 1.2 connects again (Intermediate defaults)")
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS12, tls.VersionTLS12, nil)

				By("Verifying TLS 1.3 also connects")
				tlsprofile.AssertTLSConnects(HubAPIClient, capoa, capoa.Endpoints[0],
					tls.VersionTLS13, tls.VersionTLS13, nil)

				By("Restoring StrictAllComponents for suite cleanup")
				tlsprofile.PatchTLSAdherence(HubAPIClient, adherenceStrictAllComponents)
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)

				By("Removing Modern profile to restore Intermediate baseline")
				tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
				tlsprofile.WaitPodsRestarted(HubAPIClient, capoa)
			})
	})
