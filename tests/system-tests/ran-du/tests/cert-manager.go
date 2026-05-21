package ran_du_system_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/apiservers"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ingress"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// GVRs for cert-manager and related resources.
	certGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
	clusterIssuerGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "clusterissuers",
	}
	crdGVR = schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	prometheusRuleGVR = schema.GroupVersionResource{
		Group:    "monitoring.coreos.com",
		Version:  "v1",
		Resource: "prometheusrules",
	}
	apiServerGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "apiservers",
	}

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

			issuerObj, err := APIClient.Resource(clusterIssuerGVR).Get(context.TODO(), issuerName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred(), "ClusterIssuer %s not found", issuerName)

			conditions, found, err := unstructured.NestedSlice(issuerObj.Object, "status", "conditions")
			Expect(err).ToNot(HaveOccurred(), "Failed to extract conditions from ClusterIssuer")
			Expect(found).To(BeTrue(), "ClusterIssuer has no conditions")
			Expect(len(conditions)).To(BeNumerically(">", 0), "ClusterIssuer conditions are empty")

			cond, ok := conditions[0].(map[string]interface{})
			Expect(ok).To(BeTrue(), "Failed to parse condition as map")

			condType, ok := cond["type"].(string)
			Expect(ok).To(BeTrue(), "Failed to parse condition type as string")

			condStatus, ok := cond["status"].(string)
			Expect(ok).To(BeTrue(), "Failed to parse condition status as string")

			if condType != "Ready" || condStatus != "True" {
				klog.V(randuparams.RanDuLogLevel).Infof(
					"ClusterIssuer %s is not Ready (type=%s, status=%s), skipping all cert-manager tests",
					issuerName, condType, condStatus)
				Skip(fmt.Sprintf("ClusterIssuer %s is not Ready (type=%s, status=%s)",
					issuerName, condType, condStatus))
			}

			By("Labeling cert-manager namespace for cluster monitoring")

			cmNamespace, err := namespace.Pull(APIClient, randuparams.CertManagerNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull cert-manager namespace")
			Expect(cmNamespace.Exists()).To(BeTrue(), "cert-manager namespace does not exist")

			cmNamespace.WithLabel("openshift.io/cluster-monitoring", "true")
			_, err = cmNamespace.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to label cert-manager namespace")

			By("Setting up Prometheus API client")

			var createErr error

			prometheusAPI, createErr = createPrometheusAPIClient()
			Expect(createErr).ToNot(HaveOccurred(), "Failed to create Prometheus API client")
		})

		AfterAll(func() {
			By("Cleaning up all cert-manager test resources")

			// Restore APIServer servingCerts if patched
			By("Restoring APIServer configuration")

			apiServerObj, err := APIClient.Resource(apiServerGVR).Get(context.TODO(), "cluster", metav1.GetOptions{})
			if err != nil {
				klog.V(100).Infof("Failed to get APIServer cluster object: %v", err)
			} else {
				_, found, _ := unstructured.NestedFieldNoCopy(apiServerObj.Object, "spec", "servingCerts")
				if found {
					klog.V(100).Infof("Removing servingCerts from APIServer spec")
					unstructured.RemoveNestedField(apiServerObj.Object, "spec", "servingCerts")

					_, err = APIClient.Resource(apiServerGVR).Update(context.TODO(), apiServerObj, metav1.UpdateOptions{})
					if err != nil {
						klog.V(100).Infof("Failed to update APIServer after removing servingCerts: %v", err)
					} else {
						By("Waiting for kube-apiserver rollout after APIServer restore")

						kubeAPIServer, pullErr := apiservers.PullKubeAPIServer(APIClient)
						if pullErr != nil {
							klog.V(100).Infof("Failed to pull KubeAPIServer: %v", pullErr)
						} else {
							if waitErr := kubeAPIServer.WaitAllNodesAtTheLatestRevision(
								randuparams.CertManagerAPIServerRolloutTimeout); waitErr != nil {
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

			err = APIClient.Resource(certGVR).Namespace("openshift-config").Delete(
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
						routerDeploy.IsReady(randuparams.CertManagerDefaultTimeout)
					}
				}
			}

			By("Deleting Ingress Certificate CR and Secret")

			err = APIClient.Resource(certGVR).Namespace("openshift-ingress").Delete(
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

			err = APIClient.Resource(prometheusRuleGVR).Namespace(randuparams.CertManagerOpenshiftMonitoringNamespace).Delete(
				context.TODO(), "cert-renewal-alert-test", metav1.DeleteOptions{})
			if err != nil && !k8serrors.IsNotFound(err) {
				klog.V(100).Infof("Failed to delete cert-renewal-alert-test PrometheusRule: %v", err)
			}

			// Delete cert-test namespace
			By("Deleting cert-test namespace")

			certTestNS := namespace.NewBuilder(APIClient, randuparams.CertManagerTestNamespace)
			if certTestNS.Exists() {
				if deleteErr := certTestNS.DeleteAndWait(randuparams.DefaultTimeout); deleteErr != nil {
					klog.V(100).Infof("Failed to delete namespace %s: %v",
						randuparams.CertManagerTestNamespace, deleteErr)
				}
			}

			// Remove monitoring label
			By("Removing cluster monitoring label from cert-manager namespace")

			cmNamespace, pullErr := namespace.Pull(APIClient, randuparams.CertManagerNamespace)
			if pullErr != nil {
				klog.V(100).Infof("Failed to pull cert-manager namespace: %v", pullErr)
			} else if cmNamespace.Exists() {
				klog.V(100).Infof("Removing cluster-monitoring label from namespace %s", randuparams.CertManagerNamespace)
				delete(cmNamespace.Object.Labels, "openshift.io/cluster-monitoring")
				cmNamespace.Definition.Labels = cmNamespace.Object.Labels

				if _, updateErr := cmNamespace.Update(); updateErr != nil {
					klog.V(100).Infof("Failed to update namespace %s labels: %v", randuparams.CertManagerNamespace, updateErr)
				}
			}

			// Delete Prometheus querier resources
			By("Deleting Prometheus querier resources")

			crbBuilder, pullErr := rbac.PullClusterRoleBinding(APIClient, randuparams.CertManagerPrometheusQuerierCRBName)
			if pullErr != nil && !k8serrors.IsNotFound(pullErr) {
				klog.V(100).Infof("Failed to pull ClusterRoleBinding %s: %v",
					randuparams.CertManagerPrometheusQuerierCRBName, pullErr)
			} else if crbBuilder != nil && crbBuilder.Exists() {
				if deleteErr := crbBuilder.Delete(); deleteErr != nil {
					klog.V(100).Infof("Failed to delete ClusterRoleBinding %s: %v",
						randuparams.CertManagerPrometheusQuerierCRBName, deleteErr)
				}
			}

			saBuilder, pullErr := serviceaccount.Pull(APIClient, randuparams.CertManagerPrometheusQuerierSAName,
				randuparams.CertManagerOpenshiftMonitoringNamespace)
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
				randuparams.CertManagerOperatorNamespace)
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
					pods, err := pod.ListByNamePattern(APIClient, prefix, randuparams.CertManagerNamespace)
					if err != nil || len(pods) == 0 {
						return false
					}

					for _, p := range pods {
						if p.Object.Status.Phase != corev1.PodRunning {
							return false
						}
					}

					return true
				}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
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
				_, err := APIClient.Resource(crdGVR).Get(context.TODO(), crdName, metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred(), "CRD %s not found", crdName)
			}
		})

		// 89042 - Verify certificate generation via DNS-01 ACME challenge
		It("Verifies certificate generation via DNS-01 ACME challenge", reportxml.ID("89042"), func() {
			By("Creating test namespace cert-test")

			issuerName := getIssuerName()

			certTestNS := namespace.NewBuilder(APIClient, randuparams.CertManagerTestNamespace)
			if certTestNS.Exists() {
				err := certTestNS.DeleteAndWait(randuparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete existing cert-test namespace")
			}

			_, err := certTestNS.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create cert-test namespace")

			By("Creating Certificate CR for test domain with short renewal window")

			certDomain := RanDuTestConfig.CertManager.CertDomain
			Expect(certDomain).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_CERT_DOMAIN must be set")

			err = createCertificateCR(
				"alert-test-cert",
				randuparams.CertManagerTestNamespace,
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
				return isCertificateReady(randuparams.CertManagerTestNamespace, "alert-test-cert")
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
				"Certificate alert-test-cert did not become ready")

			By("Verifying TLS secret was created with valid certificate data")

			tlsSecret, err := secret.Pull(APIClient, "alert-test-tls", randuparams.CertManagerTestNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull TLS secret")
			Expect(tlsSecret.Exists()).To(BeTrue(), "TLS secret does not exist")

			cert, err := parseCertFromSecret(tlsSecret)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse certificate from secret")
			Expect(cert.Subject.CommonName).To(Equal(certDomain),
				"Certificate CN does not match configured domain")

			By("Verifying ACME DNS TXT record was cleaned up after issuance")

			dnsServer := RanDuTestConfig.CertManager.DNSServer
			Expect(dnsServer).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_DNS_SERVER must be set")

			txtRecords, err := lookupDNSTXTRecord(dnsServer, "_acme-challenge."+certDomain)
			Expect(err).ToNot(HaveOccurred(), "DNS TXT record lookup failed")
			Expect(txtRecords).To(BeEmpty(), "TXT record was not cleaned up after certificate issuance")
		})

		// 89043 - Verify successful alerts escalation
		It("Verifies successful alerts escalation", reportxml.ID("89043"), func() {
			By("Creating PrometheusRule with accelerated alert thresholds")

			prometheusRule := buildCertRenewalPrometheusRule("alert-test-cert", 600, 420, 240)

			_, err := APIClient.Resource(prometheusRuleGVR).Namespace(
				randuparams.CertManagerOpenshiftMonitoringNamespace).Create(
				context.TODO(), prometheusRule, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to create PrometheusRule")

			By("Verifying renewal metric is available in Prometheus")

			Eventually(func() error {
				remainingSeconds, err := queryPrometheusRenewalMetric(prometheusAPI, "alert-test-cert")
				if err != nil {
					return err
				}

				if remainingSeconds <= 0 {
					return fmt.Errorf("renewal metric shows %f seconds, expected positive value", remainingSeconds)
				}

				return nil
			}, 5*time.Minute, randuparams.CertManagerPollInterval).Should(Succeed(), "Renewal metric not available or already past renewal time")

			By("Waiting for CertManagerCertRenewalInfo alert to fire (remaining < 600s)")

			Eventually(func() (string, error) {
				return queryPrometheusAlertState(prometheusAPI, randuparams.CertManagerAlertNameInfo)
			}, randuparams.CertManagerAlertTimeout, randuparams.CertManagerAlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalInfo alert did not fire")

			By("Waiting for CertManagerCertRenewalWarning alert to fire (remaining < 420s)")

			Eventually(func() (string, error) {
				return queryPrometheusAlertState(prometheusAPI, randuparams.CertManagerAlertNameWarning)
			}, 5*time.Minute, randuparams.CertManagerAlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalWarning alert did not fire")

			By("Waiting for CertManagerCertRenewalCritical alert to fire (remaining < 240s)")

			Eventually(func() (string, error) {
				return queryPrometheusAlertState(prometheusAPI, randuparams.CertManagerAlertNameCritical)
			}, 5*time.Minute, randuparams.CertManagerAlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalCritical alert did not fire")
		})

		// 89044 - Verify successful alert resolution
		It("Verifies successful alert resolution", reportxml.ID("89044"), func() {
			defer func() {
				By("Cleaning up alert test resources")

				// Delete Certificate CR
				err := APIClient.Resource(certGVR).Namespace(randuparams.CertManagerTestNamespace).Delete(
					context.TODO(), "alert-test-cert", metav1.DeleteOptions{})
				if err != nil && !k8serrors.IsNotFound(err) {
					klog.V(100).Infof("Failed to delete alert-test-cert Certificate: %v", err)
				}

				// Delete TLS secret if it exists
				tlsSecretCheck, pullErr := secret.Pull(APIClient, "alert-test-tls", randuparams.CertManagerTestNamespace)
				if pullErr == nil && tlsSecretCheck.Exists() {
					if deleteErr := tlsSecretCheck.Delete(); deleteErr != nil {
						klog.V(100).Infof("Failed to delete alert-test-tls secret: %v", deleteErr)
					}
				}

				// Delete PrometheusRule
				err = APIClient.Resource(prometheusRuleGVR).Namespace(
					randuparams.CertManagerOpenshiftMonitoringNamespace).Delete(
					context.TODO(), "cert-renewal-alert-test", metav1.DeleteOptions{})
				if err != nil && !k8serrors.IsNotFound(err) {
					klog.V(100).Infof("Failed to delete cert-renewal-alert-test PrometheusRule: %v", err)
				}

				// Delete cert-test namespace
				certTestNS := namespace.NewBuilder(APIClient, randuparams.CertManagerTestNamespace)
				if certTestNS.Exists() {
					if deleteErr := certTestNS.DeleteAndWait(randuparams.DefaultTimeout); deleteErr != nil {
					klog.V(100).Infof("Failed to delete namespace %s: %v",
						randuparams.CertManagerTestNamespace, deleteErr)
					}
				}
			}()

			By("Confirming all three cert-manager alerts are currently firing")

			alertNames := []string{randuparams.CertManagerAlertNameInfo, randuparams.CertManagerAlertNameWarning, randuparams.CertManagerAlertNameCritical}
			for _, alertName := range alertNames {
				alertState, err := queryPrometheusAlertState(prometheusAPI, alertName)
				Expect(err).ToNot(HaveOccurred(), "Failed to query state for alert %s", alertName)
				Expect(alertState).To(Equal("firing"), "Expected %s to be firing", alertName)
			}

			By("Forcing certificate renewal by deleting the TLS secret")

			tlsSecret, err := secret.Pull(APIClient, "alert-test-tls", randuparams.CertManagerTestNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull alert-test-tls secret")

			err = tlsSecret.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete alert-test-tls secret")

			By("Waiting for cert-manager to re-issue the certificate")

			Eventually(func() (bool, error) {
				return isCertificateReady(randuparams.CertManagerTestNamespace, "alert-test-cert")
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
				"Certificate alert-test-cert did not become ready after renewal")

			By("Verifying renewal timestamp metric has been updated")

			Eventually(func() (float64, error) {
				return queryPrometheusRenewalMetric(prometheusAPI, "alert-test-cert")
			}, 5*time.Minute, randuparams.CertManagerPollInterval).Should(BeNumerically(">", 0),
				"Renewal metric should show positive remaining time after renewal")

			By("Verifying all cert-manager alerts have resolved")

			Eventually(func() bool {
				infoState, infoErr := queryPrometheusAlertState(prometheusAPI, randuparams.CertManagerAlertNameInfo)
				warningState, warningErr := queryPrometheusAlertState(prometheusAPI, randuparams.CertManagerAlertNameWarning)
				criticalState, criticalErr := queryPrometheusAlertState(prometheusAPI, randuparams.CertManagerAlertNameCritical)

				if infoErr != nil || warningErr != nil || criticalErr != nil {
					return false
				}

				return infoState == "inactive" && warningState == "inactive" && criticalState == "inactive"
			}, 5*time.Minute, randuparams.CertManagerAlertPollInterval).Should(BeTrue(),
				"Info, Warning, and Critical alerts did not all resolve after certificate renewal")
		})

		// 89045 - Verify API Server certificate generation via DNS-01 ACME challenge
		It("Verifies API Server certificate generation via DNS-01 ACME challenge", reportxml.ID("89045"), func() {
			Skip("Skipping API Server certificate generation via DNS-01 ACME challenge")
			By("Creating API Server Certificate CR in openshift-config namespace")

			apiDomain := RanDuTestConfig.CertManager.APIDomain
			Expect(apiDomain).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_API_DOMAIN must be set")

			issuerName := getIssuerName()

			err := createCertificateCR(
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
				return isCertificateReady("openshift-config", "api-server-certificate")
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
				"API Server certificate did not become ready")

			By("Verifying API Server TLS secret contains correct certificate")

			apiSecret, err := secret.Pull(APIClient, "api-server-cert", "openshift-config")
			Expect(err).ToNot(HaveOccurred(), "Failed to pull API Server TLS secret")
			Expect(apiSecret.Exists()).To(BeTrue(), "API Server TLS secret does not exist")

			cert, err := parseCertFromSecret(apiSecret)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse API Server certificate from secret")
			Expect(cert.Subject.CommonName).To(Equal(apiDomain),
				"API Server certificate CN does not match configured domain")

			By("Applying APIServer configuration to use the cert-manager issued certificate")

			apiServerObj, err := APIClient.Resource(apiServerGVR).Get(context.TODO(), "cluster", metav1.GetOptions{})
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

			_, err = APIClient.Resource(apiServerGVR).Update(context.TODO(), apiServerObj, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to update APIServer with servingCerts")

			By("Waiting for kube-apiserver rollout to complete")

			Eventually(func() error {
				kubeAPIServer, err := apiservers.PullKubeAPIServer(APIClient)
				if err != nil {
					return fmt.Errorf("failed to pull KubeAPIServer: %w", err)
				}

				return kubeAPIServer.WaitAllNodesAtTheLatestRevision(10 * time.Minute)
			}, randuparams.CertManagerAPIServerRolloutTimeout, 30*time.Second).Should(Succeed(),
				"kube-apiserver rollout did not complete")

			By("Verifying API server is serving the cert-manager issued certificate")

			Eventually(func() error {
				cert, err := getTLSCertificateFromEndpoint(apiDomain, "6443", apiDomain)
				if err != nil {
					return fmt.Errorf("failed to get TLS certificate from API server: %w", err)
				}

				if cert.Subject.CommonName != apiDomain {
					return fmt.Errorf("certificate CN %s does not match API domain %s",
						cert.Subject.CommonName, apiDomain)
				}

				return nil
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(Succeed(),
				"API server is not serving the cert-manager issued certificate")
		})

		// 89046 - Verify successful API server certificate renewal
		It("Verifies successful API server certificate renewal", reportxml.ID("89046"), func() {
			Skip("Skipping successful API server certificate renewal")
			defer func() {
				By("Restoring APIServer to default certificate and cleaning up resources")

				// Remove servingCerts patch
				apiServerObj, err := APIClient.Resource(apiServerGVR).Get(context.TODO(), "cluster", metav1.GetOptions{})
				if err != nil {
					klog.V(100).Infof("Failed to get APIServer cluster object during cleanup: %v", err)
				} else {
					_, found, _ := unstructured.NestedFieldNoCopy(apiServerObj.Object, "spec", "servingCerts")
					if found {
						klog.V(100).Infof("Removing servingCerts from APIServer spec")
						unstructured.RemoveNestedField(apiServerObj.Object, "spec", "servingCerts")

						_, updateErr := APIClient.Resource(apiServerGVR).Update(context.TODO(), apiServerObj, metav1.UpdateOptions{})
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
							}, randuparams.CertManagerAPIServerRolloutTimeout, 30*time.Second).Should(Succeed(),
								"kube-apiserver rollout did not complete after restore")
						}
					} else {
						klog.V(100).Infof("APIServer spec.servingCerts not found, no restore needed")
					}
				}

				// Delete Certificate CR
				deleteErr := APIClient.Resource(certGVR).Namespace("openshift-config").Delete(
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

			cert, err := parseCertFromSecret(apiSecret)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse API Server certificate from secret")

			baselineSerial := cert.SerialNumber.String()

			By("Triggering API server certificate renewal by deleting TLS secret")

			err = apiSecret.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete api-server-cert secret")

			By("Waiting for API server certificate to be re-issued")

			Eventually(func() (bool, error) {
				return isCertificateReady("openshift-config", "api-server-certificate")
			}, 5*time.Minute, 15*time.Second).Should(BeTrue(),
				"API server certificate did not become ready after renewal")

			By("Waiting for kube-apiserver rollout to complete after renewal")

			Eventually(func() error {
				kubeAPIServer, err := apiservers.PullKubeAPIServer(APIClient)
				if err != nil {
					return fmt.Errorf("failed to pull KubeAPIServer: %w", err)
				}

				return kubeAPIServer.WaitAllNodesAtTheLatestRevision(10 * time.Minute)
			}, randuparams.CertManagerAPIServerRolloutTimeout, 30*time.Second).Should(Succeed(),
				"kube-apiserver rollout did not complete after renewal")

			By("Verifying API server is serving renewed certificate with new serial number")

			Eventually(func() error {
				renewedSecret, err := secret.Pull(APIClient, "api-server-cert", "openshift-config")
				if err != nil {
					return fmt.Errorf("failed to pull API Server TLS secret: %w", err)
				}

				if !renewedSecret.Exists() {
					return fmt.Errorf("API Server TLS secret does not exist")
				}

				renewedCert, err := parseCertFromSecret(renewedSecret)
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
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
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

			err := createCertificateCR(
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
				return isCertificateReady("openshift-ingress", "ingress-wildcard-certificate")
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
				"Ingress wildcard certificate did not become ready")

			By("Verifying Ingress wildcard TLS secret contains correct certificate")

			ingressSecret, err := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
			Expect(err).ToNot(HaveOccurred(), "Failed to pull Ingress wildcard TLS secret")
			Expect(ingressSecret.Exists()).To(BeTrue(), "Ingress wildcard TLS secret does not exist")

			cert, err := parseCertFromSecret(ingressSecret)
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

				return routerDeploy.IsReady(randuparams.CertManagerDefaultTimeout)
			}, randuparams.CertManagerDefaultTimeout+1*time.Minute, randuparams.CertManagerPollInterval).Should(BeTrue(),
				"router-default deployment did not become ready")

			By("Verifying wildcard certificate is served by the Ingress router")

			ingressIP := RanDuTestConfig.CertManager.IngressIP
			Expect(ingressIP).ToNot(BeEmpty(), "ECO_RANDU_CERTMANAGER_INGRESS_IP must be set")

			appsDomainWithoutWildcard := strings.TrimPrefix(appsDomain, "*.")

			routeHostname := "console-openshift-console." + appsDomainWithoutWildcard

			Eventually(func() error {
				servedCert, err := getTLSCertificateFromEndpoint(ingressIP, "443", routeHostname)
				if err != nil {
					return fmt.Errorf("failed to get TLS certificate from Ingress router: %w", err)
				}

				if servedCert.Subject.CommonName != appsDomain {
					return fmt.Errorf("certificate CN %s does not match apps domain %s",
						servedCert.Subject.CommonName, appsDomain)
				}

				return nil
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(Succeed(),
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
							routerDeploy, deployErr := deployment.Pull(APIClient, "router-default", "openshift-ingress")
							if deployErr != nil {
								return false
							}

							return routerDeploy.IsReady(randuparams.CertManagerDefaultTimeout)
						}, randuparams.CertManagerDefaultTimeout+1*time.Minute, randuparams.CertManagerPollInterval).Should(BeTrue(),
							"router-default deployment did not become ready after IngressController restore")
					}
				}

				// Delete Certificate CR
				deleteErr := APIClient.Resource(certGVR).Namespace("openshift-ingress").Delete(
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

			cert, err := parseCertFromSecret(ingressSecret)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse Ingress wildcard certificate from secret")

			baselineSerial := cert.SerialNumber.String()

			By("Triggering Ingress certificate renewal by deleting TLS secret")

			err = ingressSecret.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete ingress-wildcard-cert secret")

			By("Waiting for Ingress certificate to be re-issued")

			Eventually(func() (bool, error) {
				return isCertificateReady("openshift-ingress", "ingress-wildcard-certificate")
			}, 5*time.Minute, 15*time.Second).Should(BeTrue(),
				"Ingress certificate did not become ready after renewal")

			By("Waiting for router to reload with renewed certificate")

			Eventually(func() bool {
				routerDeploy, err := deployment.Pull(APIClient, "router-default", "openshift-ingress")
				if err != nil {
					return false
				}

				return routerDeploy.IsReady(randuparams.CertManagerDefaultTimeout)
			}, randuparams.CertManagerDefaultTimeout+1*time.Minute, randuparams.CertManagerPollInterval).Should(BeTrue(),
				"router-default deployment did not become ready after renewal")

			By("Verifying Ingress router is serving renewed certificate with new serial number")

			Eventually(func() error {
				renewedSecret, err := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
				if err != nil {
					return fmt.Errorf("failed to pull Ingress wildcard TLS secret: %w", err)
				}

				if !renewedSecret.Exists() {
					return fmt.Errorf("Ingress wildcard TLS secret does not exist")
				}

				renewedCert, err := parseCertFromSecret(renewedSecret)
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

				return routerDeploy.IsReady(randuparams.CertManagerDefaultTimeout)
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
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
			}, randuparams.CertManagerDefaultTimeout, randuparams.CertManagerPollInterval).Should(BeTrue(),
				"router-default pods are not all Running after renewal")
		})
	})

// Helper functions

// getIssuerName returns the configured ClusterIssuer name, defaulting to "acme-issuer".
func getIssuerName() string {
	if RanDuTestConfig.CertManager.IssuerName != "" {
		return RanDuTestConfig.CertManager.IssuerName
	}

	return "acme-issuer"
}

// buildCertRenewalPrometheusRule constructs a PrometheusRule CR for cert-manager renewal alerts
// with configurable thresholds for info, warning, and critical severity levels.
func buildCertRenewalPrometheusRule(certName string,
	infoThreshold, warningThreshold, criticalThreshold int) *unstructured.Unstructured {
	metricSelector := fmt.Sprintf(`certmanager_certificate_renewal_timestamp_seconds{name="%s"}`, certName)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "PrometheusRule",
			"metadata": map[string]interface{}{
				"name":      "cert-renewal-alert-test",
				"namespace": randuparams.CertManagerOpenshiftMonitoringNamespace,
			},
			"spec": map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "cert-manager-alert-test",
						"rules": []interface{}{
							map[string]interface{}{
								"alert": randuparams.CertManagerAlertNameInfo,
								"expr":  fmt.Sprintf(`(%s - time()) < %d`, metricSelector, infoThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "info",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf("Certificate %s will renew in less than %d seconds", certName, infoThreshold),
								},
							},
							map[string]interface{}{
								"alert": randuparams.CertManagerAlertNameWarning,
								"expr":  fmt.Sprintf(`(%s - time()) < %d`, metricSelector, warningThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "warning",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf("Certificate %s will renew in less than %d seconds", certName, warningThreshold),
								},
							},
							map[string]interface{}{
								"alert": randuparams.CertManagerAlertNameCritical,
								"expr":  fmt.Sprintf(`(%s - time()) < %d`, metricSelector, criticalThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "critical",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf("Certificate %s will renew in less than %d seconds", certName, criticalThreshold),
								},
							},
						},
					},
				},
			},
		},
	}
}

// queryPrometheusAlertState queries the state of a specific cert-manager alert rule.
func queryPrometheusAlertState(promAPI promv1.API, alertName string) (string, error) {
	result, err := promAPI.Rules(context.TODO())
	if err != nil {
		return "", fmt.Errorf("failed to query Prometheus rules: %w", err)
	}

	for _, group := range result.Groups {
		for _, rule := range group.Rules {
			alertRule, ok := rule.(promv1.AlertingRule)
			if !ok {
				continue
			}

			if alertRule.Name == alertName {
				return alertRule.State, nil
			}
		}
	}

	return "inactive", nil
}

// queryPrometheusRenewalMetric queries Prometheus for the certmanager_certificate_renewal_timestamp_seconds metric
// and returns the number of seconds remaining until renewal (renewal_timestamp - current_time).
func queryPrometheusRenewalMetric(promAPI promv1.API, certName string) (float64, error) {
	query := fmt.Sprintf(`certmanager_certificate_renewal_timestamp_seconds{name="%s"} - time()`, certName)

	result, _, err := promAPI.Query(context.TODO(), query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to query Prometheus renewal metric: %w", err)
	}

	vector, ok := result.(model.Vector)
	if !ok || len(vector) == 0 {
		return 0, fmt.Errorf("no renewal metric data found for certificate %s", certName)
	}

	return float64(vector[0].Value), nil
}

// createCertificateCR creates a cert-manager Certificate CR via the dynamic client.
func createCertificateCR(name, ns, commonName, secretName, issuerName string, dnsNames []string,
	duration, renewBefore string) error {
	cert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"isCA":       false,
				"commonName": commonName,
				"secretName": secretName,
				"dnsNames":   dnsNames,
				"privateKey": map[string]interface{}{
					"algorithm": "ECDSA",
					"size":      256,
				},
				"issuerRef": map[string]interface{}{
					"name":  issuerName,
					"kind":  "ClusterIssuer",
					"group": "cert-manager.io",
				},
			},
		},
	}

	if duration != "" {
		if err := unstructured.SetNestedField(cert.Object, duration, "spec", "duration"); err != nil {
			return fmt.Errorf("failed to set duration: %w", err)
		}
	}

	if renewBefore != "" {
		if err := unstructured.SetNestedField(cert.Object, renewBefore, "spec", "renewBefore"); err != nil {
			return fmt.Errorf("failed to set renewBefore: %w", err)
		}
	}

	_, err := APIClient.Resource(certGVR).Namespace(ns).Create(context.TODO(), cert, metav1.CreateOptions{})

	return err
}

// getTLSCertificateFromEndpoint connects to a TLS endpoint and returns the served leaf certificate.
func getTLSCertificateFromEndpoint(host, port, servername string) (*x509.Certificate, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	// InsecureSkipVerify is intentional: we are inspecting the served certificate content,
	// not relying on TLS chain validation for security.
	conn, err := tls.DialWithDialer(dialer, "tcp", host+":"+port, &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		ServerName:         servername,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s:%s: %w", host, port, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates returned from %s:%s", host, port)
	}

	return certs[0], nil
}

// parseCertFromSecret extracts and parses the tls.crt from an eco-goinfra secret builder.
func parseCertFromSecret(secretBuilder *secret.Builder) (*x509.Certificate, error) {
	certPEM := secretBuilder.Object.Data["tls.crt"]
	if len(certPEM) == 0 {
		return nil, fmt.Errorf("tls.crt not found in secret")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from tls.crt")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// isCertificateReady checks whether a cert-manager Certificate CR has a Ready=True condition.
func isCertificateReady(ns, name string) (bool, error) {
	certObj, err := APIClient.Resource(certGVR).Namespace(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get certificate %s/%s: %w", ns, name, err)
	}

	conditions, found, err := unstructured.NestedSlice(certObj.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("failed to extract conditions: %w", err)
	}

	if !found || len(conditions) == 0 {
		return false, nil
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		if cond["type"] == "Ready" && cond["status"] == "True" {
			return true, nil
		}
	}

	return false, nil
}

// lookupDNSTXTRecord queries a specific DNS server for TXT records at a given FQDN.
func lookupDNSTXTRecord(dnsServer, fqdn string) ([]string, error) {
	host := dnsServer
	if _, _, err := net.SplitHostPort(dnsServer); err == nil {
		host, _, _ = net.SplitHostPort(dnsServer)
	}

	output, err := shell.ExecuteCmd(fmt.Sprintf("dig @%s %s TXT +short", host, fqdn))
	if err != nil {
		return nil, fmt.Errorf("dig lookup failed for %s: %w", fqdn, err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []string{}, nil
	}

	var records []string
	for _, line := range strings.Split(trimmed, "\n") {
		record := strings.Trim(strings.TrimSpace(line), "\"")
		if record != "" {
			records = append(records, record)
		}
	}

	return records, nil
}

// createPrometheusAPIClient creates a Prometheus API client using the Thanos Querier route.
// When the route hostname is not DNS-resolvable (common in CI environments without *.apps DNS),
// it falls back to dialing via the API server hostname from KUBECONFIG. This works on SNO clusters
// where the API server and ingress router share the same node.
func createPrometheusAPIClient() (promv1.API, error) {
	thanosRoute, err := route.Pull(APIClient, "thanos-querier", randuparams.CertManagerOpenshiftMonitoringNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull thanos-querier route: %w", err)
	}

	if !thanosRoute.Exists() {
		return nil, fmt.Errorf("thanos-querier route not found")
	}

	if len(thanosRoute.Object.Status.Ingress) == 0 {
		return nil, fmt.Errorf("thanos-querier route has no ingress")
	}

	address := thanosRoute.Object.Status.Ingress[0].Host

	saBuilder := serviceaccount.NewBuilder(
		APIClient,
		randuparams.CertManagerPrometheusQuerierSAName,
		randuparams.CertManagerOpenshiftMonitoringNamespace,
	)

	if !saBuilder.Exists() {
		_, err := saBuilder.Create()
		if err != nil {
			return nil, fmt.Errorf("failed to create ServiceAccount: %w", err)
		}
	}

	crbBuilder := rbac.NewClusterRoleBindingBuilder(
		APIClient,
		randuparams.CertManagerPrometheusQuerierCRBName,
		"cluster-monitoring-view",
		rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      randuparams.CertManagerPrometheusQuerierSAName,
			Namespace: randuparams.CertManagerOpenshiftMonitoringNamespace,
		},
	)

	if !crbBuilder.Exists() {
		_, err := crbBuilder.Create()
		if err != nil {
			return nil, fmt.Errorf("failed to create ClusterRoleBinding: %w", err)
		}
	}

	token, err := saBuilder.CreateToken(24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	caPool, err := getClusterDefaultRouterCAPool()
	if err != nil {
		return nil, fmt.Errorf("failed to get router CA pool: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caPool,
		},
	}

	_, dnsErr := net.LookupHost(address)

	if dnsErr != nil {
		dialHost, parseErr := extractAPIServerHostname()
		if parseErr != nil {
			return nil, fmt.Errorf(
				"route hostname %s is unresolvable and failed to extract API server hostname: %w", address, parseErr)
		}

		transport.TLSClientConfig.ServerName = address
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, _ := net.SplitHostPort(addr)
			if port == "" {
				port = "443"
			}

			return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(dialHost, port))
		}
	}

	client, err := promapi.NewClient(promapi.Config{
		Address: "https://" + address,
		RoundTripper: config.NewAuthorizationCredentialsRoundTripper(
			"Bearer",
			config.NewInlineSecret(token),
			transport,
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	return promv1.NewAPI(client), nil
}

// extractAPIServerHostname returns the hostname (or IP) from the KUBECONFIG API server URL.
func extractAPIServerHostname() (string, error) {
	apiURL, err := url.Parse(APIClient.Config.Host)
	if err != nil {
		return "", fmt.Errorf("failed to parse API server URL %q: %w", APIClient.Config.Host, err)
	}

	hostname := apiURL.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("API server URL %q has no hostname", APIClient.Config.Host)
	}

	return hostname, nil
}

// getClusterDefaultRouterCAPool retrieves the default router CA pool.
func getClusterDefaultRouterCAPool() (*x509.CertPool, error) {
	routerCASecret, err := secret.Pull(APIClient, "router-certs-default", "openshift-ingress")
	if err != nil {
		return nil, fmt.Errorf("failed to pull router-certs-default secret: %w", err)
	}

	if routerCASecret == nil || !routerCASecret.Exists() {
		return nil, fmt.Errorf("router-certs-default secret not found")
	}

	caPEM := routerCASecret.Object.Data["tls.crt"]
	if len(caPEM) == 0 {
		return nil, fmt.Errorf("tls.crt not found in router-certs-default secret")
	}

	caPool, err := x509.SystemCertPool()
	if err != nil {
		caPool = x509.NewCertPool()
	}

	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to append router CA to cert pool")
	}

	return caPool, nil
}
