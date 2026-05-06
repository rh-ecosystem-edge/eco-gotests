package tls_profile_test

import (
	"context"
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/tlsprofile"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/tls-profile/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/tls-profile/internal/tsparams"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe(
	"IBIO TLS Profile — Component-Specific",
	Ordered, ContinueOnFailure,
	Label(tsparams.LabelIBIOTLSProfile), func() {
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

			By("Verifying IBIO pods are running")

			pods, err := ibio.ListPods(HubAPIClient, ibio.Namespace)
			Expect(err).ToNot(HaveOccurred(), "failed to list IBIO pods")

			if len(pods) == 0 {
				Skip("IBIO pods not found in " + ibio.Namespace + " — not deployed")
			}

			tlsprofile.WaitPodsReady(HubAPIClient, ibio)
		})

		AfterAll(func() {
			By("Restoring default Intermediate TLS profile")
			tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
			tlsprofile.RestartPods(HubAPIClient, ibio)
			tlsprofile.StopAllPortForwards()
		})

		It("Verifies config server enforces same TLS profile as webhook on IBIO",
			reportxml.ID("88930"), func() {
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

				By("Waiting for IBIO pod restart")
				tlsprofile.WaitPodsRestarted(HubAPIClient, ibio)

				webhookEP := ibio.Endpoints[0]
				configEP := ibio.Endpoints[1]

				By("Verifying allowed cipher on webhook endpoint")
				tlsprofile.AssertTLSConnects(HubAPIClient, ibio, webhookEP,
					tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256})

				By("Verifying same allowed cipher on config server endpoint")
				tlsprofile.AssertTLSConnects(HubAPIClient, ibio, configEP,
					tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256})

				By("Verifying disallowed cipher is rejected on webhook endpoint")
				tlsprofile.AssertTLSRejected(HubAPIClient, ibio, webhookEP,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256})

				By("Verifying disallowed cipher is also rejected on config server endpoint")
				tlsprofile.AssertTLSRejected(HubAPIClient, ibio, configEP,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256})
			})

		It("Verifies manager SecurityProfileWatcher triggers restart on IBIO",
			reportxml.ID("88931"), func() {
				By("Ensuring Intermediate baseline")
				tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
				tlsprofile.RestartPods(HubAPIClient, ibio)

				By("Changing TLS profile to Custom with 3 ciphers")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers: []string{
								"ECDHE-RSA-AES128-GCM-SHA256",
								"ECDHE-RSA-AES256-GCM-SHA384",
								"ECDHE-ECDSA-AES128-GCM-SHA256",
							},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})

				By("Waiting for automatic pod replacement")
				tlsprofile.WaitPodsRestarted(HubAPIClient, ibio)
				tlsprofile.WaitPodsReady(HubAPIClient, ibio)

				By("Verifying manager logs show TLS profile change")
				tlsprofile.AssertControllerLogsContain(HubAPIClient, ibio,
					ibio.Deployments[0], ibio.HonoringLogPattern)
			})

		It("Verifies manager restarts on TLS adherence policy change on IBIO",
			reportxml.ID("88932"), func() {
				By("Ensuring Custom profile baseline")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-ECDSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES256-GCM-SHA384"},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})
				tlsprofile.WaitPodsRestarted(HubAPIClient, ibio)
				tlsprofile.WaitPodsReady(HubAPIClient, ibio)

				By("Removing TLS adherence policy")
				apiserverU := &unstructured.Unstructured{}
				apiserverU.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "config.openshift.io",
					Version: "v1",
					Kind:    "APIServer",
				})

				err := HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: "cluster"}, apiserverU)
				Expect(err).ToNot(HaveOccurred(), "failed to get apiserver/cluster")

				patchBytes := []byte(`{"spec":{"tlsAdherence":""}}`)
				err = HubAPIClient.Patch(context.TODO(), apiserverU,
					runtimeclient.RawPatch("application/merge-patch+json", patchBytes))
				Expect(err).ToNot(HaveOccurred(), "failed to remove tlsAdherence")

				By("Waiting for pod restart after adherence change")
				tlsprofile.WaitPodsRestarted(HubAPIClient, ibio)
				tlsprofile.WaitPodsReady(HubAPIClient, ibio)

				By("Restoring TLS adherence")
				patchBytes = []byte(`{"spec":{"tlsAdherence":"StrictAllComponents"}}`)
				err = HubAPIClient.Patch(context.TODO(), apiserverU,
					runtimeclient.RawPatch("application/merge-patch+json", patchBytes))
				Expect(err).ToNot(HaveOccurred(), "failed to restore tlsAdherence")

				By("Waiting for restart after restoring adherence")
				tlsprofile.WaitPodsRestarted(HubAPIClient, ibio)
				tlsprofile.WaitPodsReady(HubAPIClient, ibio)
			})

		It("Verifies server container independently detects TLS change on IBIO",
			reportxml.ID("88933"), func() {
				By("Ensuring Intermediate baseline and pods ready")
				tlsprofile.RemoveAPIServerTLSProfile(HubAPIClient)
				tlsprofile.RestartPods(HubAPIClient, ibio)

				By("Changing TLS profile to Custom single-cipher")
				tlsprofile.PatchAPIServerTLSProfile(HubAPIClient, configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-ECDSA-AES128-GCM-SHA256"},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})

				By("Waiting for pod replacement")
				tlsprofile.WaitPodsRestarted(HubAPIClient, ibio)
				tlsprofile.WaitPodsReady(HubAPIClient, ibio)

				By("Verifying server container logs show TLS change detection")
				serverDeploy := tlsprofile.Deployment{
					Name:          ibio.Deployments[0].Name,
					ContainerName: "server",
				}
				tlsprofile.AssertControllerLogsContain(HubAPIClient, ibio,
					serverDeploy, "shutdown")
			})

		It("Verifies webhook validation after TLS profile change on IBIO",
			reportxml.ID("88934"), func() {
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

				By("Waiting for IBIO pod restart")
				tlsprofile.WaitPodsRestarted(HubAPIClient, ibio)
				tlsprofile.WaitPodsReady(HubAPIClient, ibio)

				By("Verifying webhook serves TLS correctly")
				tlsprofile.AssertTLSConnects(HubAPIClient, ibio, ibio.Endpoints[0],
					tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256})
			})
	})
