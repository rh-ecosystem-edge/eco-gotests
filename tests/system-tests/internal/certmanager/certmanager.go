// Package certmanager provides helper functions for cert-manager operations including
// certificate CR creation, ClusterIssuer readiness checks, certificate parsing from
// secrets, TLS endpoint validation, and DNS TXT record lookups.
package certmanager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GVRs for cert-manager and related resources.
var (
	CertGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
	ClusterIssuerGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "clusterissuers",
	}
	CrdGVR = schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	PrometheusRuleGVR = schema.GroupVersionResource{
		Group:    "monitoring.coreos.com",
		Version:  "v1",
		Resource: "prometheusrules",
	}
	APIServerGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "apiservers",
	}
)

// IsClusterIssuerReady checks whether a cert-manager ClusterIssuer has a Ready=True condition.
// Returns (false, nil) if the issuer exists but has no Ready=True condition, or (false, err)
// if the issuer cannot be retrieved.
func IsClusterIssuerReady(apiClient *clients.Settings, issuerName string) (bool, error) {
	issuerObj, err := apiClient.Resource(ClusterIssuerGVR).Get(context.TODO(), issuerName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get ClusterIssuer %s: %w", issuerName, err)
	}

	conditions, found, err := unstructured.NestedSlice(issuerObj.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("failed to extract conditions from ClusterIssuer: %w", err)
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

// CreateCertificateCR creates a cert-manager Certificate CR via the dynamic client.
func CreateCertificateCR(apiClient *clients.Settings, name, namespace, commonName, secretName, issuerName string,
	dnsNames []string, duration, renewBefore string) error {
	cert := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
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

	_, err := apiClient.Resource(CertGVR).Namespace(namespace).Create(context.TODO(), cert, metav1.CreateOptions{})

	return err
}

// IsCertificateReady checks whether a cert-manager Certificate CR has a Ready=True condition.
func IsCertificateReady(apiClient *clients.Settings, ns, name string) (bool, error) {
	certObj, err := apiClient.Resource(CertGVR).Namespace(ns).Get(context.TODO(), name, metav1.GetOptions{})
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

// ParseCertFromSecret extracts and parses the tls.crt field from a Kubernetes secret's Data map.
func ParseCertFromSecret(secretData map[string][]byte) (*x509.Certificate, error) {
	certPEM := secretData["tls.crt"]
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

// GetTLSCertificateFromEndpoint connects to a TLS endpoint and returns the served leaf certificate.
func GetTLSCertificateFromEndpoint(host, port, servername string) (*x509.Certificate, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	// InsecureSkipVerify is intentional: this function inspects the certificate content
	// served by an endpoint (e.g., verifying cert-manager issued the correct cert).
	// TLS chain validation is not required since we are validating certificate attributes
	// (CN, SANs, expiry), not authenticating the server or protecting data in transit.
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

// LookupDNSTXTRecord queries a specific DNS server for TXT records at a given FQDN.
func LookupDNSTXTRecord(dnsServer, fqdn string) ([]string, error) {
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
