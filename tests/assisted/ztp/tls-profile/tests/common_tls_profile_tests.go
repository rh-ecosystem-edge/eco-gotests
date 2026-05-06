package tls_profile_test

import (
	"context"
	"crypto/tls"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/tlsprofile"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/tls-profile/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/tls-profile/internal/tsparams"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	for _, c := range allComponents {
		component := c
		registerCommonTLSTests(component)
	}
}

func registerCommonTLSTests(component *tlsprofile.Component) {
	var _ = Describe(
		component.Name+" TLS Profile",
		Ordered, ContinueOnFailure,
		Label(tsparams.LabelTLSProfileCommon, component.Label), func() {
			BeforeAll(func() {
				By("Verifying hub API client is available")

				if HubAPIClient == nil {
					Skip("Hub API client is nil")
				}

				By("Verifying TLS adherence is enabled on the cluster")

				apiserverU := &unstructured.Unstructured{}
				apiserverU.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "config.openshift.io",
					Version: "v1",
					Kind:    "APIServer",
				})

				err := HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: "cluster"}, apiserverU)
				Expect(err).ToNot(HaveOccurred(), "failed to get apiserver/cluster")

				adherence, _, _ := unstructured.NestedString(apiserverU.Object, "spec", "tlsAdherence")
				if adherence != "StrictAllComponents" {
					Skip("TLS adherence is not StrictAllComponents (got: " + adherence + ")")
				}

				By("Verifying " + component.Name + " pods are running")

				pods, err := component.ListPods(HubAPIClient, component.Namespace)
				Expect(err).ToNot(HaveOccurred(), "failed to list %s pods", component.Name)

				if len(pods) == 0 {
					Skip(component.Name + " pods not found in " + component.Namespace + " — not deployed")
				}

				tlsprofile.WaitPodsReady(HubAPIClient, component)
			})

			AfterAll(func() {
				By("Restoring default Intermediate TLS profile")
				tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
				tlsprofile.RestartPods(HubAPIClient, component)
				tlsprofile.StopAllPortForwards()
			})

			It("Verifies default Intermediate TLS profile on "+component.Name+" endpoints",
				reportxml.ID("88843"), func() {
					By("Confirming no tlsSecurityProfile is set on apiserver")
					tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
					tlsprofile.RestartPods(HubAPIClient, component)

					By("Verifying controller logs show honoring message")

					for _, d := range component.Deployments {
						tlsprofile.AssertControllerLogsContain(HubAPIClient, component,
							d, component.HonoringLogPattern)
					}

					for _, ep := range component.Endpoints {
						By("Probing TLS 1.2 on " + ep.ServiceName)
						tlsprofile.AssertTLSConnects(HubAPIClient, component, ep,
							tls.VersionTLS12, tls.VersionTLS12, nil)

						By("Probing TLS 1.3 on " + ep.ServiceName)
						tlsprofile.AssertTLSConnects(HubAPIClient, component, ep,
							tls.VersionTLS13, tls.VersionTLS13, nil)
					}

					By("Verifying TLS 1.1 is rejected on " + component.Endpoints[0].ServiceName)
					tlsprofile.AssertTLSRejectedVersion(HubAPIClient, component,
						component.Endpoints[0], tls.VersionTLS11)

					By("Verifying TLS 1.0 is rejected on " + component.Endpoints[0].ServiceName)
					tlsprofile.AssertTLSRejectedVersion(HubAPIClient, component,
						component.Endpoints[0], tls.VersionTLS10)
				})

			It("Verifies Old TLS profile enables broader cipher set on "+component.Name,
				reportxml.ID("88844"), func() {
					By("Applying Old TLS profile")
					tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
						Old:  &configv1.OldTLSProfile{},
					})

					By("Restarting " + component.Name + " pods to pick up Old profile")
					tlsprofile.RestartPods(HubAPIClient, component)

					By("Verifying controller logs show VersionTLS10")

					for _, d := range component.Deployments {
						tlsprofile.AssertControllerLogsContain(HubAPIClient, component,
							d, "VersionTLS10")
					}

					for _, ep := range component.Endpoints {
						By("Verifying Old-specific cipher connects on " + ep.ServiceName)
						tlsprofile.AssertTLSConnects(HubAPIClient, component, ep,
							tls.VersionTLS12, tls.VersionTLS12,
							[]uint16{component.OldProfileCipher})
					}
				})

			It("Verifies Modern TLS profile restricts to TLS 1.3 only on "+component.Name,
				reportxml.ID("88845"), func() {
					By("Applying Modern TLS profile")
					tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
						Type:   configv1.TLSProfileModernType,
						Modern: &configv1.ModernTLSProfile{},
					})

					By("Restarting " + component.Name + " pods to pick up Modern profile")
					tlsprofile.RestartPods(HubAPIClient, component)

					By("Verifying controller logs show VersionTLS13")

					for _, d := range component.Deployments {
						tlsprofile.AssertControllerLogsContain(HubAPIClient, component,
							d, "VersionTLS13")
					}

					for _, ep := range component.Endpoints {
						By("Verifying TLS 1.3 connects on " + ep.ServiceName)
						tlsprofile.AssertTLSConnects(HubAPIClient, component, ep,
							tls.VersionTLS13, tls.VersionTLS13, nil)

						By("Verifying TLS 1.2 is rejected on " + ep.ServiceName)
						tlsprofile.AssertTLSRejected(HubAPIClient, component, ep, nil)
					}
				})

			It("Verifies Custom TLS profile restricts to specified ciphers on "+component.Name,
				reportxml.ID("88846"), func() {
					By("Applying Custom TLS profile with 2 ciphers")
					tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384"},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					})

					By("Restarting " + component.Name + " pods to pick up Custom profile")
					tlsprofile.RestartPods(HubAPIClient, component)

					for _, ep := range component.Endpoints {
						By("Verifying allowed cipher connects on " + ep.ServiceName)
						tlsprofile.AssertTLSConnects(HubAPIClient, component, ep,
							tls.VersionTLS12, tls.VersionTLS12,
							[]uint16{component.AllowedCipher})

						By("Verifying disallowed cipher is rejected on " + ep.ServiceName)
						tlsprofile.AssertTLSRejected(HubAPIClient, component, ep,
							[]uint16{component.DisallowedCipher})
					}
				})

			It("Verifies profile change triggers automatic reconciliation on "+component.Name,
				reportxml.ID("88847"), func() {
					By("Restoring Intermediate baseline")
					tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
					tlsprofile.RestartPods(HubAPIClient, component)

					By("Recording baseline cipher connectivity")
					tlsprofile.AssertTLSConnects(HubAPIClient, component, component.Endpoints[0],
						tls.VersionTLS12, tls.VersionTLS12,
						[]uint16{component.AllowedCipher})

					By("Switching to Custom single-cipher profile")
					tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES128-GCM-SHA256"},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					})

					By("Waiting for automatic reconciliation (no manual pod restart)")
					tlsprofile.WaitPodsRestarted(HubAPIClient, component)

					for _, d := range component.Deployments {
						tlsprofile.AssertControllerLogsContain(HubAPIClient, component,
							d, component.HonoringLogPattern)
					}

					By("Verifying AES256 is now rejected under single-cipher Custom profile")
					tlsprofile.AssertTLSRejected(HubAPIClient, component, component.Endpoints[0],
						[]uint16{component.AllowedCipherAlt})

					By("Switching back to Intermediate")
					tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
					tlsprofile.WaitPodsRestarted(HubAPIClient, component)

					By("Verifying AES256 is restored under Intermediate")
					tlsprofile.AssertTLSConnects(HubAPIClient, component, component.Endpoints[0],
						tls.VersionTLS12, tls.VersionTLS12,
						[]uint16{component.AllowedCipherAlt})
				})

			It("Verifies webhook validation works after TLS profile change on "+component.Name,
				reportxml.ID("88848"), func() {
					if component.WebhookTest == nil {
						Skip(component.Name + " has no webhook validation test configured")
					}

					wt := component.WebhookTest

					By("Applying Custom TLS profile")
					tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384"},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					})
					tlsprofile.RestartPods(HubAPIClient, component)

					By("Ensuring test namespace does not exist")

					nsBuilder := namespace.NewBuilder(HubAPIClient, wt.TestNamespace)

					err := nsBuilder.DeleteAndWait(2 * time.Minute)
					Expect(err).ToNot(HaveOccurred(), "failed to clean up namespace %s", wt.TestNamespace)

					By("Creating test namespace")

					nsBuilder, err = namespace.NewBuilder(HubAPIClient, wt.TestNamespace).Create()
					Expect(err).ToNot(HaveOccurred(), "failed to create test namespace")

					DeferCleanup(func() {
						_ = nsBuilder.Delete()
					})

					By("Creating valid " + wt.Kind)

					resource := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": wt.APIVersion,
							"kind":       wt.Kind,
							"metadata": map[string]interface{}{
								"name":      wt.ResourceName,
								"namespace": wt.TestNamespace,
							},
							"spec": wt.CreateSpec,
						},
					}

					err = HubAPIClient.Create(context.TODO(), resource)
					Expect(err).ToNot(HaveOccurred(),
						"valid %s should be accepted", wt.Kind)

					By("Attempting to update spec (should be rejected)")

					err = HubAPIClient.Patch(context.TODO(), resource,
						runtimeclient.RawPatch(types.MergePatchType, wt.MutationPatch))
					Expect(err).To(HaveOccurred(),
						"spec update should be rejected by webhook")
					Expect(err.Error()).To(ContainSubstring(wt.RejectionSubstring),
						"rejection reason should mention %s", wt.RejectionSubstring)

					By("Updating metadata only (should succeed)")

					labelPatch := []byte(`{"metadata":{"labels":{"tls-profile-test":"validation"}}}`)

					err = HubAPIClient.Patch(context.TODO(), resource,
						runtimeclient.RawPatch(types.MergePatchType, labelPatch))
					Expect(err).ToNot(HaveOccurred(), "metadata update should be accepted")

					By("Deleting the resource")

					err = HubAPIClient.Delete(context.TODO(), resource)
					Expect(err).ToNot(HaveOccurred(), "deletion should succeed")
				})

			It("Verifies restore to default profile on "+component.Name,
				reportxml.ID("88850"), func() {
					By("Applying Custom TLS profile")
					tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384"},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					})
					tlsprofile.RestartPods(HubAPIClient, component)

					By("Removing Custom profile to restore default")
					tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
					tlsprofile.RestartPods(HubAPIClient, component)

					By("Verifying no tlsSecurityProfile remains on apiserver")

					apiserver := &configv1.APIServer{}

					err := HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: "cluster"}, apiserver)
					Expect(err).ToNot(HaveOccurred())
					Expect(apiserver.Spec.TLSSecurityProfile).To(BeNil(),
						"tlsSecurityProfile should be nil after restore")

					By("Verifying controller logs show Intermediate profile")

					for _, d := range component.Deployments {
						tlsprofile.AssertControllerLogsContain(HubAPIClient, component,
							d, "VersionTLS12")
					}

					By("Verifying Intermediate ciphers are available")
					tlsprofile.AssertTLSConnects(HubAPIClient, component, component.Endpoints[0],
						tls.VersionTLS12, tls.VersionTLS12,
						[]uint16{component.AllowedCipher})
					tlsprofile.AssertTLSConnects(HubAPIClient, component, component.Endpoints[0],
						tls.VersionTLS13, tls.VersionTLS13, nil)
				})

			It("Verifies SecurityProfileWatcher triggers restart on "+component.Name,
				reportxml.ID("88858"), func() {
					By("Ensuring Intermediate baseline and pods are ready")
					tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
					tlsprofile.WaitPodsReady(HubAPIClient, component)

					By("Changing TLS profile to Custom")
					tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384"},
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					})

					By("Waiting for automatic restart")
					tlsprofile.WaitPodsRestarted(HubAPIClient, component)
					tlsprofile.WaitPodsReady(HubAPIClient, component)

					By("Verifying controllers honour the new profile")

					for _, d := range component.Deployments {
						tlsprofile.AssertControllerLogsContain(HubAPIClient, component,
							d, component.HonoringLogPattern)
					}
				})
		})
}
