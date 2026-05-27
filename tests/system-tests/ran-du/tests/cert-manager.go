package ran_du_system_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/apiservers"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ingress"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/certmanager"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe(
	"CertManager",
	Ordered,
	ContinueOnFailure,
	Label(randuparams.LabelCertManager), func() {
		var prometheusAPI promv1.API

		BeforeAll(func() {
			By("Verifying ClusterIssuer is Ready")

			issuerName := getIssuerName()

			ready, err := certmanager.IsClusterIssuerReady(APIClient, issuerName)
			Expect(err).ToNot(HaveOccurred(), "Failed to check ClusterIssuer %s readiness", issuerName)

			if !ready {
				klog.V(randuparams.RanDuLogLevel).Infof(
					"ClusterIssuer %s is not Ready, skipping all cert-manager tests", issuerName)
				Skip(fmt.Sprintf("ClusterIssuer %s is not Ready", issuerName))
			}

			By("Labeling cert-manager namespace for cluster monitoring")

			cmNamespace, err := namespace.Pull(APIClient, certmanager.Namespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull cert-manager namespace")
			Expect(cmNamespace.Exists()).To(BeTrue(), "cert-manager namespace does not exist")

			cmNamespace.WithLabel("openshift.io/cluster-monitoring", "true")
			_, err = cmNamespace.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to label cert-manager namespace")

			By("Setting up Prometheus API client")

			var createErr error

			prometheusAPI, createErr = certmanager.NewPrometheusAPI(
				APIClient,
				randuparams.CertManagerPrometheusQuerierSAName,
				randuparams.CertManagerPrometheusQuerierCRBName,
				certmanager.OpenshiftMonitoringNamespace,
			)
			Expect(createErr).ToNot(HaveOccurred(), "Failed to create Prometheus API client")
		})

		AfterAll(func() {
			By("Cleaning up all cert-manager test resources")

			// Restore APIServer servingCerts if patched
			By("Restoring APIServer configuration")

			apiServerObj, err := APIClient.Resource(certmanager.APIServerGVR).Get(
				context.TODO(), "cluster", metav1.GetOptions{})
			if err != nil {
				klog.V(100).Infof("Failed to get APIServer cluster object: %v", err)
			} else {
				_, found, _ := unstructured.NestedFieldNoCopy(apiServerObj.Object, "spec", "servingCerts")
				if found {
					klog.V(100).Infof("Removing servingCerts from APIServer spec")
					unstructured.RemoveNestedField(apiServerObj.Object, "spec", "servingCerts")

					_, err = APIClient.Resource(certmanager.APIServerGVR).Update(
						context.TODO(), apiServerObj, metav1.UpdateOptions{})
					if err != nil {
						klog.V(100).Infof("Failed to update APIServer after removing servingCerts: %v", err)
					} else {
						By("Waiting for kube-apiserver rollout after APIServer restore")

						kubeAPIServer, pullErr := apiservers.PullKubeAPIServer(APIClient)
						if pullErr != nil {
							klog.V(100).Infof("Failed to pull KubeAPIServer: %v", pullErr)
						} else {
							if waitErr := kubeAPIServer.WaitAllNodesAtTheLatestRevision(
								certmanager.APIServerRolloutTimeout); waitErr != nil {
								klog.V(100).Infof("KubeAPIServer rollout wait failed: %v", waitErr)
							}
						}
					}
				} else {
					klog.V(100).Infof("APIServer spec.servingCerts not found, no restore needed")
				}
			}

			// Delete API cert resources
			By("Deleting API Server Certificate CR and Secret")

			err = APIClient.Resource(certmanager.CertGVR).Namespace("openshift-config").Delete(
				context.TODO(), "api-server-certificate", metav1.DeleteOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				klog.V(100).Infof("Failed to delete api-server-certificate: %v", err)
			}

			apiSecretBuilder, pullErr := secret.Pull(APIClient, "api-server-cert", "openshift-config")
			if pullErr == nil && apiSecretBuilder.Exists() {
				if deleteErr := apiSecretBuilder.Delete(); deleteErr != nil {
					klog.V(100).Infof("Failed to delete api-server-cert secret: %v", deleteErr)
				}
			}

			By("Restoring IngressController default certificate")

			ingressBuilder, pullErr := ingress.Pull(APIClient, "default", "openshift-ingress-operator")
			if pullErr != nil {
				klog.V(100).Infof("Failed to pull IngressController: %v", pullErr)
			} else if !ingressBuilder.Exists() {
				klog.V(100).Infof("IngressController 'default' does not exist, skipping restore")
			} else if ingressBuilder.Object.Spec.DefaultCertificate == nil {
				klog.V(100).Infof("IngressController has no custom defaultCertificate, no restore needed")
			} else {
				klog.V(100).Infof("Removing defaultCertificate from IngressController")

				patch := []byte(`{"spec":{"defaultCertificate":null}}`)

				updateErr := APIClient.Patch(context.TODO(), ingressBuilder.Object,
					runtimeClient.RawPatch(types.MergePatchType, patch))
				if updateErr != nil {
					klog.V(100).Infof("Failed to patch IngressController: %v", updateErr)
				} else {
					By("Waiting for router rollout after IngressController restore")

					routerDeploy, deployErr := deployment.Pull(APIClient, "router-default", "openshift-ingress")
					if deployErr != nil {
						klog.V(100).Infof("Failed to pull router-default deployment: %v", deployErr)
					} else {
						routerDeploy.IsReady(certmanager.DefaultTimeout)
					}
				}
			}

			By("Deleting Ingress Certificate CR and Secret")

			err = APIClient.Resource(certmanager.CertGVR).Namespace("openshift-ingress").Delete(
				context.TODO(), "ingress-wildcard-certificate", metav1.DeleteOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				klog.V(100).Infof("Failed to delete ingress-wildcard-certificate: %v", err)
			}

			ingressSecretBuilder, pullErr := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
			if pullErr == nil && ingressSecretBuilder.Exists() {
				if deleteErr := ingressSecretBuilder.Delete(); deleteErr != nil {
					klog.V(100).Infof("Failed to delete ingress-wildcard-cert secret: %v", deleteErr)
				}
			}

			// Delete PrometheusRule
			By("Deleting PrometheusRule")

			err = APIClient.Resource(certmanager.PrometheusRuleGVR).Namespace(
				certmanager.OpenshiftMonitoringNamespace).Delete(
				context.TODO(), "cert-renewal-alert-test", metav1.DeleteOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				klog.V(100).Infof("Failed to delete cert-renewal-alert-test PrometheusRule: %v", err)
			}

			// Delete cert-test namespace
			By("Deleting cert-test namespace")

			certTestNS := namespace.NewBuilder(APIClient, certmanager.TestNamespace)
			if certTestNS.Exists() {
				if deleteErr := certTestNS.DeleteAndWait(randuparams.DefaultTimeout); deleteErr != nil {
					klog.V(100).Infof("Failed to delete namespace %s: %v",
						certmanager.TestNamespace, deleteErr)
				}
			}

			// Remove monitoring label
			By("Removing cluster monitoring label from cert-manager namespace")

			cmNamespace, pullErr := namespace.Pull(APIClient, certmanager.Namespace)
			if pullErr != nil {
				klog.V(100).Infof("Failed to pull cert-manager namespace: %v", pullErr)
			} else if cmNamespace.Exists() {
				klog.V(100).Infof("Removing cluster-monitoring label from namespace %s", certmanager.Namespace)
				delete(cmNamespace.Object.Labels, "openshift.io/cluster-monitoring")
				cmNamespace.Definition.Labels = cmNamespace.Object.Labels

				if _, updateErr := cmNamespace.Update(); updateErr != nil {
					klog.V(100).Infof("Failed to update namespace %s labels: %v",
						certmanager.Namespace, updateErr)
				}
			}

			// Delete Prometheus querier resources
			By("Deleting Prometheus querier resources")

			crbBuilder, pullErr := rbac.PullClusterRoleBinding(APIClient,
				randuparams.CertManagerPrometheusQuerierCRBName)
			if pullErr != nil && !k8serrors.IsNotFound(pullErr) {
				klog.V(100).Infof("Failed to pull ClusterRoleBinding %s: %v",
					randuparams.CertManagerPrometheusQuerierCRBName, pullErr)
			} else if crbBuilder != nil && crbBuilder.Exists() {
				if deleteErr := crbBuilder.Delete(); deleteErr != nil {
					klog.V(100).Infof("Failed to delete ClusterRoleBinding %s: %v",
						randuparams.CertManagerPrometheusQuerierCRBName, deleteErr)
				}
			}

			saBuilder, pullErr := serviceaccount.Pull(APIClient,
				randuparams.CertManagerPrometheusQuerierSAName,
				certmanager.OpenshiftMonitoringNamespace)
			if pullErr != nil && !k8serrors.IsNotFound(pullErr) {
				klog.V(100).Infof("Failed to pull ServiceAccount %s: %v",
					randuparams.CertManagerPrometheusQuerierSAName, pullErr)
			} else if saBuilder != nil && saBuilder.Exists() {
				if deleteErr := saBuilder.Delete(); deleteErr != nil {
					klog.V(100).Infof("Failed to delete ServiceAccount %s: %v",
						randuparams.CertManagerPrometheusQuerierSAName, deleteErr)
				}
			}
		})

		// 89041 - Verify cert-manager operator installation
		It("Verifies cert-manager operator installation", reportxml.ID("89041"), func() {
			By("Verifying cert-manager-operator controller-manager pod is running")

			pods, err := pod.ListByNamePattern(APIClient, "cert-manager-operator-controller-manager",
				certmanager.OperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list cert-manager-operator pods")
			Expect(len(pods)).To(BeNumerically(">=", 1), "No cert-manager-operator controller-manager pod found")

			for _, p := range pods {
				Expect(p.Object.Status.Phase).To(Equal(corev1.PodRunning),
					"cert-manager-operator controller-manager pod %s is not Running", p.Object.Name)
			}

			By("Verifying cert-manager core pods are running in cert-manager namespace")

			corePrefixes := []string{"cert-manager-", "cert-manager-cainjector-", "cert-manager-webhook-"}
			for _, prefix := range corePrefixes {
				Eventually(func() bool {
					pods, err := pod.ListByNamePattern(APIClient, prefix, certmanager.Namespace)
					if err != nil || len(pods) == 0 {
						return false
					}

					for _, p := range pods {
						if p.Object.Status.Phase != corev1.PodRunning {
							return false
						}
					}

					return true
				}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
					"cert-manager core pods with prefix %s are not all Running", prefix)
			}

			By("Verifying cert-manager Custom Resource Definitions exist")

			crdNames := []string{
				"certificaterequests.cert-manager.io",
				"certificates.cert-manager.io",
				"challenges.acme.cert-manager.io",
				"clusterissuers.cert-manager.io",
				"issuers.cert-manager.io",
				"orders.acme.cert-manager.io",
			}

			for _, crdName := range crdNames {
				_, err := APIClient.Resource(certmanager.CrdGVR).Get(context.TODO(), crdName, metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred(), "CRD %s not found", crdName)
			}
		})

		// 89042 - Verify certificate generation via DNS-01 ACME challenge
		It("Verifies certificate generation via DNS-01 ACME challenge", reportxml.ID("89042"), func() {
			By("Creating test namespace cert-test")

			issuerName := getIssuerName()

			certTestNS := namespace.NewBuilder(APIClient, certmanager.TestNamespace)
			if certTestNS.Exists() {
				err := certTestNS.DeleteAndWait(randuparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete existing cert-test namespace")
			}

			_, err := certTestNS.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create cert-test namespace")

			By("Creating Certificate CR for test domain with short renewal window")

			certDomain := RanDuTestConfig.CertManager.CertDomain
			Expect(certDomain).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_CERT_DOMAIN must be set")

			err = certmanager.CreateCertificateCR(
				APIClient,
				"alert-test-cert",
				certmanager.TestNamespace,
				certDomain,
				"alert-test-tls",
				issuerName,
				[]string{certDomain},
				"24h",
				"23h45m",
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to create Certificate CR")

			By("Waiting for certificate to become ready")

			Eventually(func() (bool, error) {
				return certmanager.IsCertificateReady(APIClient, certmanager.TestNamespace, "alert-test-cert")
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
				"Certificate alert-test-cert did not become ready")

			By("Verifying TLS secret was created with valid certificate data")

			tlsSecret, err := secret.Pull(APIClient, "alert-test-tls", certmanager.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull TLS secret")
			Expect(tlsSecret.Exists()).To(BeTrue(), "TLS secret does not exist")

			cert, err := certmanager.ParseCertFromSecret(tlsSecret.Object.Data)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse certificate from secret")
			Expect(cert.Subject.CommonName).To(Equal(certDomain),
				"Certificate CN does not match configured domain")

			By("Verifying ACME DNS TXT record was cleaned up after issuance")

			dnsServer := RanDuTestConfig.CertManager.DNSServer
			Expect(dnsServer).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_DNS_SERVER must be set")

			txtRecords, err := certmanager.LookupDNSTXTRecord(dnsServer, "_acme-challenge."+certDomain)
			Expect(err).ToNot(HaveOccurred(), "DNS TXT record lookup failed")
			Expect(txtRecords).To(BeEmpty(), "TXT record was not cleaned up after certificate issuance")
		})

		// 89043 - Verify successful alerts escalation
		It("Verifies successful alerts escalation", reportxml.ID("89043"), func() {
			By("Creating PrometheusRule with accelerated alert thresholds")

			prometheusRule := certmanager.BuildCertRenewalPrometheusRule(
				certmanager.OpenshiftMonitoringNamespace, "alert-test-cert", 600, 420, 240)

			_, err := APIClient.Resource(certmanager.PrometheusRuleGVR).Namespace(
				certmanager.OpenshiftMonitoringNamespace).Create(
				context.TODO(), prometheusRule, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to create PrometheusRule")

			By("Verifying renewal metric is available in Prometheus")

			Eventually(func() error {
				remainingSeconds, err := certmanager.QueryPrometheusRenewalMetric(prometheusAPI, "alert-test-cert")
				if err != nil {
					return err
				}

				if remainingSeconds <= 0 {
					return fmt.Errorf("renewal metric shows %f seconds, expected positive value", remainingSeconds)
				}

				return nil
			}, 5*time.Minute, certmanager.PollInterval).Should(Succeed(),
				"Renewal metric not available or already past renewal time")

			By("Waiting for CertManagerCertRenewalInfo alert to fire (remaining < 600s)")

			Eventually(func() (string, error) {
				return certmanager.QueryPrometheusAlertState(prometheusAPI, certmanager.AlertNameInfo)
			}, certmanager.AlertTimeout, certmanager.AlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalInfo alert did not fire")

			By("Waiting for CertManagerCertRenewalWarning alert to fire (remaining < 420s)")

			Eventually(func() (string, error) {
				return certmanager.QueryPrometheusAlertState(prometheusAPI, certmanager.AlertNameWarning)
			}, 5*time.Minute, certmanager.AlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalWarning alert did not fire")

			By("Waiting for CertManagerCertRenewalCritical alert to fire (remaining < 240s)")

			Eventually(func() (string, error) {
				return certmanager.QueryPrometheusAlertState(prometheusAPI, certmanager.AlertNameCritical)
			}, 5*time.Minute, certmanager.AlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalCritical alert did not fire")
		})

		// 89044 - Verify successful alert resolution
		It("Verifies successful alert resolution", reportxml.ID("89044"), func() {
			defer func() {
				By("Cleaning up alert test resources")

				// Delete Certificate CR
				err := APIClient.Resource(certmanager.CertGVR).Namespace(certmanager.TestNamespace).Delete(
					context.TODO(), "alert-test-cert", metav1.DeleteOptions{})
				if err != nil && !k8serrors.IsNotFound(err) {
					klog.V(100).Infof("Failed to delete alert-test-cert Certificate: %v", err)
				}

				// Delete TLS secret if it exists
				tlsSecretCheck, pullErr := secret.Pull(APIClient, "alert-test-tls", certmanager.TestNamespace)
				if pullErr == nil && tlsSecretCheck.Exists() {
					if deleteErr := tlsSecretCheck.Delete(); deleteErr != nil {
						klog.V(100).Infof("Failed to delete alert-test-tls secret: %v", deleteErr)
					}
				}

				// Delete PrometheusRule
				err = APIClient.Resource(certmanager.PrometheusRuleGVR).Namespace(
					certmanager.OpenshiftMonitoringNamespace).Delete(
					context.TODO(), "cert-renewal-alert-test", metav1.DeleteOptions{})
				if err != nil && !k8serrors.IsNotFound(err) {
					klog.V(100).Infof("Failed to delete cert-renewal-alert-test PrometheusRule: %v", err)
				}

				// Delete cert-test namespace
				certTestNS := namespace.NewBuilder(APIClient, certmanager.TestNamespace)
				if certTestNS.Exists() {
					if deleteErr := certTestNS.DeleteAndWait(randuparams.DefaultTimeout); deleteErr != nil {
						klog.V(100).Infof("Failed to delete namespace %s: %v",
							certmanager.TestNamespace, deleteErr)
					}
				}
			}()

			By("Confirming all three cert-manager alerts are currently firing")

			alertNames := []string{
				certmanager.AlertNameInfo,
				certmanager.AlertNameWarning,
				certmanager.AlertNameCritical,
			}
			for _, alertName := range alertNames {
				alertState, err := certmanager.QueryPrometheusAlertState(prometheusAPI, alertName)
				Expect(err).ToNot(HaveOccurred(), "Failed to query state for alert %s", alertName)
				Expect(alertState).To(Equal("firing"), "Expected %s to be firing", alertName)
			}

			By("Forcing certificate renewal by deleting the TLS secret")

			tlsSecret, err := secret.Pull(APIClient, "alert-test-tls", certmanager.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull alert-test-tls secret")

			err = tlsSecret.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete alert-test-tls secret")

			By("Waiting for cert-manager to re-issue the certificate")

			Eventually(func() (bool, error) {
				return certmanager.IsCertificateReady(APIClient, certmanager.TestNamespace, "alert-test-cert")
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
				"Certificate alert-test-cert did not become ready after renewal")

			By("Verifying renewal timestamp metric has been updated")

			Eventually(func() (float64, error) {
				return certmanager.QueryPrometheusRenewalMetric(prometheusAPI, "alert-test-cert")
			}, 5*time.Minute, certmanager.PollInterval).Should(BeNumerically(">", 0),
				"Renewal metric should show positive remaining time after renewal")

			By("Verifying all cert-manager alerts have resolved")

			Eventually(func() bool {
				infoState, infoErr := certmanager.QueryPrometheusAlertState(
					prometheusAPI, certmanager.AlertNameInfo)
				warningState, warningErr := certmanager.QueryPrometheusAlertState(
					prometheusAPI, certmanager.AlertNameWarning)
				criticalState, criticalErr := certmanager.QueryPrometheusAlertState(
					prometheusAPI, certmanager.AlertNameCritical)

				if infoErr != nil || warningErr != nil || criticalErr != nil {
					return false
				}

				return infoState == certmanager.AlertStateInactive &&
					warningState == certmanager.AlertStateInactive &&
					criticalState == certmanager.AlertStateInactive
			}, 5*time.Minute, certmanager.AlertPollInterval).Should(BeTrue(),
				"Info, Warning, and Critical alerts did not all resolve after certificate renewal")
		})

		// 89045 - Verify API Server certificate generation via DNS-01 ACME challenge
		It("Verifies API Server certificate generation via DNS-01 ACME challenge", reportxml.ID("89045"), func() {
			By("Creating API Server Certificate CR in openshift-config namespace")

			apiDomain := RanDuTestConfig.CertManager.APIDomain
			Expect(apiDomain).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_API_DOMAIN must be set")

			issuerName := getIssuerName()

			err := certmanager.CreateCertificateCR(
				APIClient,
				"api-server-certificate",
				"openshift-config",
				apiDomain,
				"api-server-cert",
				issuerName,
				[]string{apiDomain},
				"",
				"",
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to create API Server Certificate CR")

			By("Waiting for API Server certificate to become ready")

			Eventually(func() (bool, error) {
				return certmanager.IsCertificateReady(APIClient, "openshift-config", "api-server-certificate")
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
				"API Server certificate did not become ready")

			By("Verifying API Server TLS secret contains correct certificate")

			apiSecret, err := secret.Pull(APIClient, "api-server-cert", "openshift-config")
			Expect(err).ToNot(HaveOccurred(), "Failed to pull API Server TLS secret")
			Expect(apiSecret.Exists()).To(BeTrue(), "API Server TLS secret does not exist")

			cert, err := certmanager.ParseCertFromSecret(apiSecret.Object.Data)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse API Server certificate from secret")
			Expect(cert.Subject.CommonName).To(Equal(apiDomain),
				"API Server certificate CN does not match configured domain")

			By("Applying APIServer configuration to use the cert-manager issued certificate")

			apiServerObj, err := APIClient.Resource(certmanager.APIServerGVR).Get(
				context.TODO(), "cluster", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to get APIServer cluster resource")

			err = unstructured.SetNestedSlice(apiServerObj.Object, []interface{}{
				map[string]interface{}{
					"names": []interface{}{apiDomain},
					"servingCertificate": map[string]interface{}{
						"name": "api-server-cert",
					},
				},
			}, "spec", "servingCerts", "namedCertificates")
			Expect(err).ToNot(HaveOccurred(), "Failed to set servingCerts in APIServer spec")

			_, err = APIClient.Resource(certmanager.APIServerGVR).Update(
				context.TODO(), apiServerObj, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to update APIServer with servingCerts")

			By("Waiting for kube-apiserver rollout to complete")

			Eventually(func() error {
				kubeAPIServer, err := apiservers.PullKubeAPIServer(APIClient)
				if err != nil {
					return fmt.Errorf("failed to pull KubeAPIServer: %w", err)
				}

				return kubeAPIServer.WaitAllNodesAtTheLatestRevision(10 * time.Minute)
			}, certmanager.APIServerRolloutTimeout, 30*time.Second).Should(Succeed(),
				"kube-apiserver rollout did not complete")

			By("Verifying API server is serving the cert-manager issued certificate")

			Eventually(func() error {
				cert, err := certmanager.GetTLSCertificateFromEndpoint(apiDomain, "6443", apiDomain)
				if err != nil {
					return fmt.Errorf("failed to get TLS certificate from API server: %w", err)
				}

				if cert.Subject.CommonName != apiDomain {
					return fmt.Errorf("certificate CN %s does not match API domain %s",
						cert.Subject.CommonName, apiDomain)
				}

				return nil
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(Succeed(),
				"API server is not serving the cert-manager issued certificate")
		})

		// 89046 - Verify successful API server certificate renewal
		It("Verifies successful API server certificate renewal", reportxml.ID("89046"), func() {
			defer func() {
				By("Restoring APIServer to default certificate and cleaning up resources")

				// Remove servingCerts patch
				apiServerObj, err := APIClient.Resource(certmanager.APIServerGVR).Get(
					context.TODO(), "cluster", metav1.GetOptions{})
				if err != nil {
					klog.V(100).Infof("Failed to get APIServer cluster object during cleanup: %v", err)
				} else {
					_, found, _ := unstructured.NestedFieldNoCopy(apiServerObj.Object, "spec", "servingCerts")
					if found {
						klog.V(100).Infof("Removing servingCerts from APIServer spec")
						unstructured.RemoveNestedField(apiServerObj.Object, "spec", "servingCerts")

						_, updateErr := APIClient.Resource(certmanager.APIServerGVR).Update(
							context.TODO(), apiServerObj, metav1.UpdateOptions{})
						if updateErr != nil {
							klog.V(100).Infof("Failed to update APIServer after removing servingCerts: %v", updateErr)
						} else {
							By("Waiting for kube-apiserver rollout after APIServer restore")

							Eventually(func() error {
								kubeAPIServer, pullErr := apiservers.PullKubeAPIServer(APIClient)
								if pullErr != nil {
									return fmt.Errorf("failed to pull KubeAPIServer: %w", pullErr)
								}

								return kubeAPIServer.WaitAllNodesAtTheLatestRevision(10 * time.Minute)
							}, certmanager.APIServerRolloutTimeout, 30*time.Second).Should(Succeed(),
								"kube-apiserver rollout did not complete after restore")
						}
					} else {
						klog.V(100).Infof("APIServer spec.servingCerts not found, no restore needed")
					}
				}

				// Delete Certificate CR
				deleteErr := APIClient.Resource(certmanager.CertGVR).Namespace("openshift-config").Delete(
					context.TODO(), "api-server-certificate", metav1.DeleteOptions{})
				if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
					klog.V(100).Infof("Failed to delete api-server-certificate: %v", deleteErr)
				}

				// Delete secret if it exists
				apiSecretCheck, pullErr := secret.Pull(APIClient, "api-server-cert", "openshift-config")
				if pullErr == nil && apiSecretCheck.Exists() {
					if deleteErr := apiSecretCheck.Delete(); deleteErr != nil {
						klog.V(100).Infof("Failed to delete api-server-cert secret: %v", deleteErr)
					}
				}
			}()

			By("Recording baseline API server certificate serial number and expiry")

			apiSecret, err := secret.Pull(APIClient, "api-server-cert", "openshift-config")
			Expect(err).ToNot(HaveOccurred(), "Failed to pull API Server TLS secret")
			Expect(apiSecret.Exists()).To(BeTrue(), "API Server TLS secret does not exist")

			cert, err := certmanager.ParseCertFromSecret(apiSecret.Object.Data)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse API Server certificate from secret")

			baselineSerial := cert.SerialNumber.String()

			By("Triggering API server certificate renewal by deleting TLS secret")

			err = apiSecret.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete api-server-cert secret")

			By("Waiting for API server certificate to be re-issued")

			Eventually(func() (bool, error) {
				return certmanager.IsCertificateReady(APIClient, "openshift-config", "api-server-certificate")
			}, 5*time.Minute, 15*time.Second).Should(BeTrue(),
				"API server certificate did not become ready after renewal")

			By("Waiting for kube-apiserver rollout to complete after renewal")

			Eventually(func() error {
				kubeAPIServer, err := apiservers.PullKubeAPIServer(APIClient)
				if err != nil {
					return fmt.Errorf("failed to pull KubeAPIServer: %w", err)
				}

				return kubeAPIServer.WaitAllNodesAtTheLatestRevision(10 * time.Minute)
			}, certmanager.APIServerRolloutTimeout, 30*time.Second).Should(Succeed(),
				"kube-apiserver rollout did not complete after renewal")

			By("Verifying API server is serving renewed certificate with new serial number")

			Eventually(func() error {
				renewedSecret, err := secret.Pull(APIClient, "api-server-cert", "openshift-config")
				if err != nil {
					return fmt.Errorf("failed to pull API Server TLS secret: %w", err)
				}

				if !renewedSecret.Exists() {
					return fmt.Errorf("api server TLS secret does not exist")
				}

				renewedCert, err := certmanager.ParseCertFromSecret(renewedSecret.Object.Data)
				if err != nil {
					return fmt.Errorf("failed to parse renewed certificate: %w", err)
				}

				newSerial := renewedCert.SerialNumber.String()
				if newSerial == baselineSerial {
					return fmt.Errorf("certificate serial did not change (still %s)", newSerial)
				}

				return nil
			}, 3*time.Minute, 15*time.Second).Should(Succeed(),
				"Certificate serial did not change after renewal")

			By("Verifying cluster is fully functional after API server certificate renewal")

			Eventually(func() bool {
				pods, err := pod.ListByNamePattern(APIClient, "kube-apiserver", "openshift-kube-apiserver")
				if err != nil || len(pods) == 0 {
					return false
				}

				for _, p := range pods {
					if p.Object.Status.Phase != corev1.PodRunning {
						return false
					}
				}

				return true
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
				"kube-apiserver pods are not all Running after renewal")
		})

		// 89047 - Verify Ingress wildcard certificate generation via DNS-01 ACME challenge
		It("Verifies Ingress wildcard certificate generation via DNS-01 ACME challenge", reportxml.ID("89047"), func() {
			By("Creating Ingress wildcard Certificate CR in openshift-ingress namespace")

			appsDomain := RanDuTestConfig.CertManager.AppsDomain
			Expect(appsDomain).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_APPS_DOMAIN must be set")
			Expect(appsDomain).To(HavePrefix("*."),
				"ECO_RANDU_CERTMANAGER_APPS_DOMAIN must be a wildcard domain (e.g., *.apps.example.com)")

			issuerName := getIssuerName()

			err := certmanager.CreateCertificateCR(
				APIClient,
				"ingress-wildcard-certificate",
				"openshift-ingress",
				appsDomain,
				"ingress-wildcard-cert",
				issuerName,
				[]string{appsDomain},
				"",
				"",
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to create Ingress wildcard Certificate CR")

			By("Waiting for Ingress wildcard certificate to become ready")

			Eventually(func() (bool, error) {
				return certmanager.IsCertificateReady(APIClient, "openshift-ingress", "ingress-wildcard-certificate")
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
				"Ingress wildcard certificate did not become ready")

			By("Verifying Ingress wildcard TLS secret contains correct certificate")

			ingressSecret, err := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
			Expect(err).ToNot(HaveOccurred(), "Failed to pull Ingress wildcard TLS secret")
			Expect(ingressSecret.Exists()).To(BeTrue(), "Ingress wildcard TLS secret does not exist")

			cert, err := certmanager.ParseCertFromSecret(ingressSecret.Object.Data)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse Ingress wildcard certificate from secret")
			Expect(cert.Subject.CommonName).To(Equal(appsDomain),
				"Ingress wildcard certificate CN does not match configured domain")
			Expect(cert.DNSNames).To(ContainElement(appsDomain),
				"Ingress wildcard certificate SAN does not contain configured domain")

			By("Patching IngressController to use the cert-manager issued wildcard certificate")

			ingressController, err := ingress.Pull(APIClient, "default", "openshift-ingress-operator")
			Expect(err).ToNot(HaveOccurred(), "Failed to pull IngressController")
			Expect(ingressController.Exists()).To(BeTrue(), "IngressController default does not exist")

			patch := []byte(`{"spec":{"defaultCertificate":{"name":"ingress-wildcard-cert"}}}`)
			err = APIClient.Patch(context.TODO(), ingressController.Object,
				runtimeClient.RawPatch(types.MergePatchType, patch))
			Expect(err).ToNot(HaveOccurred(), "Failed to update IngressController with defaultCertificate")

			By("Waiting for router-default deployment rollout to complete")

			Eventually(func() bool {
				routerDeploy, err := deployment.Pull(APIClient, "router-default", "openshift-ingress")
				if err != nil {
					return false
				}

				return routerDeploy.IsReady(certmanager.DefaultTimeout)
			}, certmanager.DefaultTimeout+1*time.Minute, certmanager.PollInterval).Should(BeTrue(),
				"router-default deployment did not become ready")

			By("Verifying wildcard certificate is served by the Ingress router")

			ingressIP := RanDuTestConfig.CertManager.IngressIP
			Expect(ingressIP).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_INGRESS_IP must be set")

			appsDomainWithoutWildcard := strings.TrimPrefix(appsDomain, "*.")

			routeHostname := "console-openshift-console." + appsDomainWithoutWildcard

			Eventually(func() error {
				servedCert, err := certmanager.GetTLSCertificateFromEndpoint(ingressIP, "443", routeHostname)
				if err != nil {
					return fmt.Errorf("failed to get TLS certificate from Ingress router: %w", err)
				}

				if servedCert.Subject.CommonName != appsDomain {
					return fmt.Errorf("certificate CN %s does not match apps domain %s",
						servedCert.Subject.CommonName, appsDomain)
				}

				return nil
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(Succeed(),
				"Ingress router is not serving the cert-manager issued wildcard certificate")
		})

		// 89048 - Verify successful ingress certificate renewal
		It("Verifies successful ingress certificate renewal", reportxml.ID("89048"), func() {
			defer func() {
				By("Restoring IngressController to default certificate and cleaning up resources")

				// Remove defaultCertificate patch
				ingressController, err := ingress.Pull(APIClient, "default", "openshift-ingress-operator")
				if err != nil {
					klog.V(100).Infof("Failed to pull IngressController during cleanup: %v", err)
				} else if !ingressController.Exists() {
					klog.V(100).Infof("IngressController 'default' does not exist, skipping restore")
				} else if ingressController.Object.Spec.DefaultCertificate == nil {
					klog.V(100).Infof("IngressController has no custom defaultCertificate, no restore needed")
				} else {
					klog.V(100).Infof("Removing defaultCertificate from IngressController")

					patch := []byte(`{"spec":{"defaultCertificate":null}}`)

					updateErr := APIClient.Patch(context.TODO(), ingressController.Object,
						runtimeClient.RawPatch(types.MergePatchType, patch))
					if updateErr != nil {
						klog.V(100).Infof("Failed to patch IngressController: %v", updateErr)
					} else {
						By("Waiting for router rollout after IngressController restore")

						Eventually(func() bool {
							routerDeploy, deployErr := deployment.Pull(
								APIClient, "router-default", "openshift-ingress")
							if deployErr != nil {
								return false
							}

							return routerDeploy.IsReady(certmanager.DefaultTimeout)
						}, certmanager.DefaultTimeout+1*time.Minute, certmanager.PollInterval).Should(BeTrue(),
							"router-default deployment did not become ready after IngressController restore")
					}
				}

				// Delete Certificate CR
				deleteErr := APIClient.Resource(certmanager.CertGVR).Namespace("openshift-ingress").Delete(
					context.TODO(), "ingress-wildcard-certificate", metav1.DeleteOptions{})
				if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
					klog.V(100).Infof("Failed to delete ingress-wildcard-certificate: %v", deleteErr)
				}

				// Delete secret if it exists
				ingressSecretCheck, pullErr := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
				if pullErr == nil && ingressSecretCheck.Exists() {
					if deleteErr := ingressSecretCheck.Delete(); deleteErr != nil {
						klog.V(100).Infof("Failed to delete ingress-wildcard-cert secret: %v", deleteErr)
					}
				}
			}()

			By("Recording baseline Ingress certificate serial number")

			ingressSecret, err := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
			Expect(err).ToNot(HaveOccurred(), "Failed to pull Ingress wildcard TLS secret")
			Expect(ingressSecret.Exists()).To(BeTrue(), "Ingress wildcard TLS secret does not exist")

			cert, err := certmanager.ParseCertFromSecret(ingressSecret.Object.Data)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse Ingress wildcard certificate from secret")

			baselineSerial := cert.SerialNumber.String()

			By("Triggering Ingress certificate renewal by deleting TLS secret")

			err = ingressSecret.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete ingress-wildcard-cert secret")

			By("Waiting for Ingress certificate to be re-issued")

			Eventually(func() (bool, error) {
				return certmanager.IsCertificateReady(APIClient, "openshift-ingress", "ingress-wildcard-certificate")
			}, 5*time.Minute, 15*time.Second).Should(BeTrue(),
				"Ingress certificate did not become ready after renewal")

			By("Waiting for router to reload with renewed certificate")

			Eventually(func() bool {
				routerDeploy, err := deployment.Pull(APIClient, "router-default", "openshift-ingress")
				if err != nil {
					return false
				}

				return routerDeploy.IsReady(certmanager.DefaultTimeout)
			}, certmanager.DefaultTimeout+1*time.Minute, certmanager.PollInterval).Should(BeTrue(),
				"router-default deployment did not become ready after renewal")

			By("Verifying Ingress router is serving renewed certificate with new serial number")

			Eventually(func() error {
				renewedSecret, err := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
				if err != nil {
					return fmt.Errorf("failed to pull Ingress wildcard TLS secret: %w", err)
				}

				if !renewedSecret.Exists() {
					return fmt.Errorf("ingress wildcard TLS secret does not exist")
				}

				renewedCert, err := certmanager.ParseCertFromSecret(renewedSecret.Object.Data)
				if err != nil {
					return fmt.Errorf("failed to parse renewed certificate: %w", err)
				}

				newSerial := renewedCert.SerialNumber.String()
				if newSerial == baselineSerial {
					return fmt.Errorf("certificate serial did not change (still %s)", newSerial)
				}

				return nil
			}, 3*time.Minute, 15*time.Second).Should(Succeed(),
				"Certificate serial did not change after renewal")

			By("Verifying cluster is fully functional after Ingress certificate renewal")

			Eventually(func() bool {
				routerDeploy, err := deployment.Pull(APIClient, "router-default", "openshift-ingress")
				if err != nil {
					return false
				}

				return routerDeploy.IsReady(certmanager.DefaultTimeout)
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
				"router-default deployment is not ready after renewal")

			// Also verify router pods are running
			Eventually(func() bool {
				pods, err := pod.ListByNamePattern(APIClient, "router-default", "openshift-ingress")
				if err != nil || len(pods) == 0 {
					return false
				}

				for _, p := range pods {
					if p.Object.Status.Phase != corev1.PodRunning {
						return false
					}
				}

				return true
			}, certmanager.DefaultTimeout, certmanager.PollInterval).Should(BeTrue(),
				"router-default pods are not all Running after renewal")
		})
	})

// getIssuerName returns the configured ClusterIssuer name, defaulting to "acme-issuer".
func getIssuerName() string {
	if RanDuTestConfig.CertManager.IssuerName != "" {
		return RanDuTestConfig.CertManager.IssuerName
	}

	return "acme-issuer"
}
