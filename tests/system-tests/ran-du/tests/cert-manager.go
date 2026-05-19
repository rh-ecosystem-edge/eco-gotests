package ran_du_system_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/apiservers"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ingress"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/monitoring"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// Describe-scoped variables for Prometheus API and cert serials.
	prometheusAPI            promv1.API
	apiCertBaselineSerial    string
	apiRenewalBaselineSerial string
	ingressBaselineSerial    string
)

var _ = Describe(
	"CertManager",
	Ordered,
	ContinueOnFailure,
	Label(randuparams.LabelCertManager), func() {
		BeforeAll(func() {
			By("Verifying ClusterIssuer is Ready")

			issuerName := RanDuTestConfig.CertManager.IssuerName
			if issuerName == "" {
				issuerName = "acme-issuer"
			}

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
				Skip(fmt.Sprintf("ClusterIssuer %s is not Ready (type=%s, status=%s). Skipping all cert-manager tests.",
					issuerName, condType, condStatus))
			}

			By("Setting up Prometheus monitoring for cert-manager")

			// Label cert-manager namespace for cluster monitoring
			cmNamespace, err := namespace.Pull(APIClient, randuparams.CertManagerNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull cert-manager namespace")
			Expect(cmNamespace.Exists()).To(BeTrue(), "cert-manager namespace does not exist")

			cmNamespace.WithLabel("openshift.io/cluster-monitoring", "true")
			_, err = cmNamespace.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to label cert-manager namespace")

			// Create RBAC Role for Prometheus with two separate policy rules
			role := rbac.NewRoleBuilder(
				APIClient,
				"prometheus-k8s",
				randuparams.CertManagerNamespace,
				rbacv1.PolicyRule{
					APIGroups: []string{"", "discovery.k8s.io"},
					Resources: []string{"services", "endpoints", "pods", "endpointslices"},
					Verbs:     []string{"get", "list", "watch"},
				},
			)

			if !role.Exists() {
				_, err = role.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create prometheus-k8s Role")
			}

			// Create RoleBinding
			roleBinding := rbac.NewRoleBindingBuilder(
				APIClient,
				"prometheus-k8s",
				randuparams.CertManagerNamespace,
				"prometheus-k8s",
				rbacv1.Subject{
					Kind:      "ServiceAccount",
					Name:      "prometheus-k8s",
					Namespace: randuparams.CertManagerOpenshiftMonitoringNamespace,
				},
			)

			if !roleBinding.Exists() {
				_, err = roleBinding.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create prometheus-k8s RoleBinding")
			}

			By("Creating ServiceMonitor for cert-manager metrics")

			smBuilder := monitoring.NewBuilder(APIClient, "cert-manager-metrics", randuparams.CertManagerNamespace)
			smBuilder.WithEndpoints([]monv1.Endpoint{{
				Port:     "tcp-prometheus-servicemonitor",
				Interval: monv1.Duration("30s"),
			}})
			smBuilder.WithSelector(map[string]string{
				"app.kubernetes.io/instance": "cert-manager",
			})

			if !smBuilder.Exists() {
				_, err = smBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create cert-manager-metrics ServiceMonitor")
			}

			By("Verifying Prometheus RBAC via SubjectAccessReview")

			sar := &authorizationv1.SubjectAccessReview{
				Spec: authorizationv1.SubjectAccessReviewSpec{
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Namespace: randuparams.CertManagerNamespace,
						Verb:      "list",
						Resource:  "endpoints",
					},
					User: "system:serviceaccount:" + randuparams.CertManagerOpenshiftMonitoringNamespace + ":prometheus-k8s",
				},
			}

			result, err := APIClient.K8sClient.AuthorizationV1().SubjectAccessReviews().Create(
				context.TODO(), sar, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to create SubjectAccessReview")
			Expect(result.Status.Allowed).To(BeTrue(), "Prometheus ServiceAccount does not have required permissions")

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
			if err == nil {
				_, found, _ := unstructured.NestedFieldNoCopy(apiServerObj.Object, "spec", "servingCerts")
				if found {
					unstructured.RemoveNestedField(apiServerObj.Object, "spec", "servingCerts")

					_, err = APIClient.Resource(apiServerGVR).Update(context.TODO(), apiServerObj, metav1.UpdateOptions{})
					if err == nil {
						By("Waiting for kube-apiserver rollout after APIServer restore")

						kubeAPIServer, pullErr := apiservers.PullKubeAPIServer(APIClient)
						if pullErr == nil {
							_ = kubeAPIServer.WaitAllNodesAtTheLatestRevision(randuparams.CertManagerAPIServerRolloutTimeout)
						}
					}
				}
			}

			// Delete API cert resources
			By("Deleting API Server Certificate CR and Secret")

			_ = APIClient.Resource(certGVR).Namespace("openshift-config").Delete(
				context.TODO(), "api-server-certificate", metav1.DeleteOptions{})

			apiSecretBuilder, _ := secret.Pull(APIClient, "api-server-cert", "openshift-config")
			if apiSecretBuilder != nil && apiSecretBuilder.Exists() {
				_ = apiSecretBuilder.Delete()
			}

			By("Restoring IngressController default certificate")

			ingressBuilder, pullErr := ingress.Pull(APIClient, "default", "openshift-ingress-operator")
			if pullErr == nil && ingressBuilder.Exists() && ingressBuilder.Object.Spec.DefaultCertificate != nil {
				ingressBuilder.Definition.Spec.DefaultCertificate = nil

				_, updateErr := ingressBuilder.Update()
				if updateErr == nil {
					By("Waiting for router rollout after IngressController restore")

					routerDeploy, deployErr := deployment.Pull(APIClient, "router-default", "openshift-ingress")
					if deployErr == nil {
						routerDeploy.IsReady(randuparams.CertManagerDefaultTimeout)
					}
				}
			}

			By("Deleting Ingress Certificate CR and Secret")

			_ = APIClient.Resource(certGVR).Namespace("openshift-ingress").Delete(
				context.TODO(), "ingress-wildcard-certificate", metav1.DeleteOptions{})

			ingressSecretBuilder, _ := secret.Pull(APIClient, "ingress-wildcard-cert", "openshift-ingress")
			if ingressSecretBuilder != nil && ingressSecretBuilder.Exists() {
				_ = ingressSecretBuilder.Delete()
			}

			// Delete PrometheusRule
			By("Deleting PrometheusRule")

			_ = APIClient.Resource(prometheusRuleGVR).Namespace(randuparams.CertManagerOpenshiftMonitoringNamespace).Delete(
				context.TODO(), "cert-renewal-alert-test", metav1.DeleteOptions{})

			// Delete cert-test namespace
			By("Deleting cert-test namespace")

			certTestNS := namespace.NewBuilder(APIClient, randuparams.CertManagerTestNamespace)
			if certTestNS.Exists() {
				_ = certTestNS.DeleteAndWait(randuparams.DefaultTimeout)
			}

			By("Cleaning up monitoring resources")

			smBuilder, _ := monitoring.Pull(APIClient, "cert-manager-metrics", randuparams.CertManagerNamespace)
			if smBuilder != nil && smBuilder.Exists() {
				_, _ = smBuilder.Delete()
			}

			rbBuilder, _ := rbac.PullRoleBinding(APIClient, "prometheus-k8s", randuparams.CertManagerNamespace)
			if rbBuilder != nil && rbBuilder.Exists() {
				_ = rbBuilder.Delete()
			}

			roleBuilder, _ := rbac.PullRole(APIClient, "prometheus-k8s", randuparams.CertManagerNamespace)
			if roleBuilder != nil && roleBuilder.Exists() {
				_ = roleBuilder.Delete()
			}

			// Remove monitoring label
			By("Removing cluster monitoring label from cert-manager namespace")

			cmNamespace, _ := namespace.Pull(APIClient, randuparams.CertManagerNamespace)
			if cmNamespace != nil && cmNamespace.Exists() {
				cmNamespace.Definition.Labels = make(map[string]string)
				for k, v := range cmNamespace.Object.Labels {
					if k != "openshift.io/cluster-monitoring" {
						cmNamespace.Definition.Labels[k] = v
					}
				}

				_, _ = cmNamespace.Update()
			}

			// Delete Prometheus querier resources
			By("Deleting Prometheus querier resources")

			crbBuilder, _ := rbac.PullClusterRoleBinding(APIClient, randuparams.CertManagerPrometheusQuerierCRBName)
			if crbBuilder != nil && crbBuilder.Exists() {
				_ = crbBuilder.Delete()
			}

			saBuilder, _ := serviceaccount.Pull(APIClient, randuparams.CertManagerPrometheusQuerierSAName,
				randuparams.CertManagerOpenshiftMonitoringNamespace)
			if saBuilder != nil && saBuilder.Exists() {
				_ = saBuilder.Delete()
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
				}, 2*time.Minute, 10*time.Second).Should(BeTrue(),
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

			issuerName := RanDuTestConfig.CertManager.IssuerName
			if issuerName == "" {
				issuerName = "acme-issuer"
			}

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
				"23h51m",
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to create Certificate CR")

			By("Waiting for certificate to become ready")

			Eventually(func() (bool, error) {
				return isCertificateReady(randuparams.CertManagerTestNamespace, "alert-test-cert")
			}, randuparams.CertManagerDefaultTimeout, 10*time.Second).Should(BeTrue(),
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

			prometheusRule := buildCertRenewalPrometheusRule("alert-test-cert", 480, 360, 240)

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
			}, 2*time.Minute, 10*time.Second).Should(Succeed(), "Renewal metric not available or already past renewal time")

			By("Waiting for CertManagerCertRenewalInfo alert to fire (remaining < 480s)")

			Eventually(func() (string, error) {
				return queryPrometheusAlertState(prometheusAPI, "CertManagerCertRenewalInfo")
			}, randuparams.CertManagerAlertTimeout, randuparams.CertManagerAlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalInfo alert did not fire")

			By("Waiting for CertManagerCertRenewalWarning alert to fire (remaining < 360s)")

			Eventually(func() (string, error) {
				return queryPrometheusAlertState(prometheusAPI, "CertManagerCertRenewalWarning")
			}, 5*time.Minute, randuparams.CertManagerAlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalWarning alert did not fire")

			By("Waiting for CertManagerCertRenewalCritical alert to fire (remaining < 240s)")

			Eventually(func() (string, error) {
				return queryPrometheusAlertState(prometheusAPI, "CertManagerCertRenewalCritical")
			}, 5*time.Minute, randuparams.CertManagerAlertPollInterval).Should(Equal("firing"),
				"CertManagerCertRenewalCritical alert did not fire")
		})
	})

// Helper functions

// buildCertRenewalPrometheusRule constructs a PrometheusRule CR for cert-manager renewal alerts
// with configurable thresholds for info, warning, and critical severity levels.
func buildCertRenewalPrometheusRule(certName string, infoThreshold, warningThreshold, criticalThreshold int) *unstructured.Unstructured {
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
								"alert": "CertManagerCertRenewalInfo",
								"expr":  fmt.Sprintf(`(certmanager_certificate_renewal_timestamp_seconds{name="%s"} - time()) < %d`, certName, infoThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "info",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf("Certificate %s will renew in less than %d seconds", certName, infoThreshold),
								},
							},
							map[string]interface{}{
								"alert": "CertManagerCertRenewalWarning",
								"expr":  fmt.Sprintf(`(certmanager_certificate_renewal_timestamp_seconds{name="%s"} - time()) < %d`, certName, warningThreshold),
								"for":   "0m",
								"labels": map[string]interface{}{
									"severity": "warning",
								},
								"annotations": map[string]interface{}{
									"description": fmt.Sprintf("Certificate %s will renew in less than %d seconds", certName, warningThreshold),
								},
							},
							map[string]interface{}{
								"alert": "CertManagerCertRenewalCritical",
								"expr":  fmt.Sprintf(`(certmanager_certificate_renewal_timestamp_seconds{name="%s"} - time()) < %d`, certName, criticalThreshold),
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

	return "", fmt.Errorf("alert %s not found", alertName)
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

	conn, err := tls.DialWithDialer(dialer, "tcp", host+":"+port, &tls.Config{
		InsecureSkipVerify: true,
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
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 10 * time.Second}

			return d.DialContext(ctx, "udp", dnsServer+":53")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	records, err := resolver.LookupTXT(ctx, fqdn)
	if err != nil {
		// Handle "not found" as empty result (expected after ACME cleanup)
		var dnsErr *net.DNSError
		if ok := errors.As(err, &dnsErr); ok && dnsErr.IsNotFound {
			return []string{}, nil
		}

		return nil, fmt.Errorf("DNS lookup failed for %s: %w", fqdn, err)
	}

	return records, nil
}

// createPrometheusAPIClient creates a Prometheus API client using the Thanos Querier route.
func createPrometheusAPIClient() (promv1.API, error) {
	// Get Thanos Querier route
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

	// Create ServiceAccount
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

	// Create ClusterRoleBinding
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

	// Create bearer token
	token, err := saBuilder.CreateToken(24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	// Get router CA pool
	caPool, err := getClusterDefaultRouterCAPool()
	if err != nil {
		return nil, fmt.Errorf("failed to get router CA pool: %w", err)
	}

	// Create Prometheus API client
	client, err := promapi.NewClient(promapi.Config{
		Address: "https://" + address,
		RoundTripper: config.NewAuthorizationCredentialsRoundTripper(
			"Bearer",
			config.NewInlineSecret(token),
			&http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caPool,
				},
			},
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	return promv1.NewAPI(client), nil
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
