package cnfclusterinfo

import (
	"context"
	"crypto/md5"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

var (
	// PreUpgradeClusterInfo holds the cluster info pre upgrade.
	PreUpgradeClusterInfo = ClusterStruct{}

	// PostUpgradeClusterInfo holds the cluster info post upgrade.
	PostUpgradeClusterInfo = ClusterStruct{}
)

// WorkloadObjects is a struct that holds the workload objects.
type WorkloadObjects struct {
	Deployment  []string
	StatefulSet []string
}

// WorkloadStruct is a struct that holds the workload info.
type WorkloadStruct struct {
	Namespace string
	Objects   WorkloadObjects
}

// WorkloadPV struct holds the information to test that persistent volume content is not lost during upgrade.
type WorkloadPV struct {
	Namespace string
	PodName   string
	FilePath  string
	Digest    string
}

// CertManagerInfo holds cert-manager state captured before and after upgrade.
type CertManagerInfo struct {
	IssuerReady         bool
	CertReady           bool
	TLSCertChecksum     string
	TLSCertSerial       string
	OperatorPodsRunning bool
}

// ClusterStruct is a struct that holds the cluster info pre and post upgrade.
type ClusterStruct struct {
	Version                  string
	ID                       string
	Name                     string
	Operators                []string
	NodeName                 string
	SriovNetworks            []string
	SriovNetworkNodePolicies []string
	WorkloadResources        []WorkloadStruct
	WorkloadPVs              WorkloadPV
	CertManager              CertManagerInfo
}

// SaveClusterInfo is a dedicated func to save cluster info.
//
//nolint:funlen
func (upgradeVar *ClusterStruct) SaveClusterInfo() error {
	clusterVersion, err := cluster.GetOCPClusterVersion(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve cluster version")

		return err
	}

	targetSnoClusterName, err := cluster.GetOCPClusterName(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve target sno cluster name")

		return err
	}

	csvList, err := olm.ListClusterServiceVersionInAllNamespaces(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve csv list")

		return err
	}

	var installedCSV []string

	for _, csv := range csvList {
		if !slices.Contains(installedCSV, csv.Object.Name) {
			if !strings.Contains(csv.Object.Name, "oadp-operator") &&
				!strings.Contains(csv.Object.Name, "packageserver") &&
				!strings.Contains(csv.Object.Name, "sriov-fec") {
				installedCSV = append(installedCSV, csv.Object.Name)
			}
		}
	}

	node, err := nodes.List(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve node list")

		return err
	}

	if len(node) == 0 {
		return errors.New("node list is empty")
	}

	sriovNet, err := sriov.List(cnfinittools.TargetSNOAPIClient, cnfinittools.CNFConfig.SriovOperatorNamespace)
	if err != nil {
		return err
	}

	for _, net := range sriovNet {
		upgradeVar.SriovNetworks = append(upgradeVar.SriovNetworks, net.Object.Name)
	}

	sriovPolicy, err := sriov.ListPolicy(cnfinittools.TargetSNOAPIClient, cnfinittools.CNFConfig.SriovOperatorNamespace)

	for _, policy := range sriovPolicy {
		if !strings.Contains(policy.Object.Name, "default") {
			upgradeVar.SriovNetworkNodePolicies = append(upgradeVar.SriovNetworkNodePolicies, policy.Object.Name)
		}
	}

	upgradeVar.Version = clusterVersion.Object.Status.Desired.Version
	upgradeVar.ID = string(clusterVersion.Object.Spec.ClusterID)
	upgradeVar.Name = targetSnoClusterName
	upgradeVar.Operators = installedCSV
	upgradeVar.NodeName = node[0].Object.Name

	_ = upgradeVar.getWorkloadInfo()

	if err != nil {
		return err
	}

	// Collect cert-manager info if deployed. Non-fatal: if cert-manager resources are missing,
	// the function logs and returns early, leaving CertManager fields at zero values.
	// The validation test skips via the TLSCertChecksum == "" check.
	upgradeVar.getCertManagerInfo()

	return nil
}

func (upgradeVar *ClusterStruct) getWorkloadInfo() error {
	for _, workloadNS := range strings.Split(cnfinittools.CNFConfig.IbuWorkloadNS, ",") {
		workloadMap := WorkloadStruct{
			Namespace: workloadNS,
			Objects: WorkloadObjects{
				Deployment:  []string{},
				StatefulSet: []string{},
			},
		}

		deployments, err := deployment.List(cnfinittools.TargetSNOAPIClient, workloadNS)
		if err != nil {
			return err
		}

		for _, deployment := range deployments {
			workloadMap.Objects.Deployment = append(workloadMap.Objects.Deployment, deployment.Definition.Name)
		}

		statefulsets, err := statefulset.List(cnfinittools.TargetSNOAPIClient, workloadNS)
		if err != nil {
			return err
		}

		for _, statefulset := range statefulsets {
			workloadMap.Objects.StatefulSet = append(workloadMap.Objects.StatefulSet, statefulset.Definition.Name)
		}

		upgradeVar.WorkloadResources = append(upgradeVar.WorkloadResources, workloadMap)
	}

	workloadPod, err := pod.Pull(
		cnfinittools.TargetSNOAPIClient,
		cnfinittools.CNFConfig.IbuWorkloadPVPod,
		cnfinittools.CNFConfig.IbuWorkloadPVNS,
	)
	if err != nil {
		return err
	}

	cmd := []string{"bash", "-c", "md5sum " + cnfinittools.CNFConfig.IbuWorkloadPVFilePath + " ||true"}

	getDigest, err := workloadPod.ExecCommand(cmd)
	if err != nil {
		return err
	}

	upgradeVar.WorkloadPVs.Namespace = cnfinittools.CNFConfig.IbuWorkloadPVNS
	upgradeVar.WorkloadPVs.PodName = cnfinittools.CNFConfig.IbuWorkloadPVPod
	upgradeVar.WorkloadPVs.FilePath = cnfinittools.CNFConfig.IbuWorkloadPVFilePath
	upgradeVar.WorkloadPVs.Digest = getDigest.String()

	return nil
}

var (
	certManagerCertGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
	certManagerClusterIssuerGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "clusterissuers",
	}
)

// Cert-manager resource names matching the IBU test environment setup.
// Namespace constants are duplicated from tests/system-tests/internal/certmanager/constants.go
// because Go internal package visibility prevents import from tests/lca/.
const (
	certManagerOperatorNS = "cert-manager-operator"
	certManagerTestNS     = "cert-test"
	certManagerIssuerName = "acme-issuer"
	certManagerCertName   = "test-workload-cert"
	certManagerSecretName = "test-workload-tls"
)

// getCertManagerInfo collects cert-manager state (ClusterIssuer readiness, Certificate readiness,
// TLS secret checksum/serial, operator pod status) into upgradeVar.CertManager. Non-fatal: if any
// cert-manager resource is missing, it logs and returns early, leaving CertManager fields at zero values.
func (upgradeVar *ClusterStruct) getCertManagerInfo() {
	klog.V(cnfparams.CNFLogLevel).Infof("Collecting cert-manager info")

	issuerReady, err := isClusterIssuerReady(certManagerIssuerName)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof(
			"ClusterIssuer %s not found or not accessible, skipping cert-manager info: %v",
			certManagerIssuerName, err)

		return
	}

	upgradeVar.CertManager.IssuerReady = issuerReady

	operatorPods, err := pod.ListByNamePattern(
		cnfinittools.TargetSNOAPIClient, "cert-manager-operator-controller-manager", certManagerOperatorNS)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Failed to list cert-manager-operator pods: %v", err)

		return
	}

	upgradeVar.CertManager.OperatorPodsRunning = len(operatorPods) > 0
	for _, p := range operatorPods {
		if p.Object.Status.Phase != corev1.PodRunning {
			upgradeVar.CertManager.OperatorPodsRunning = false

			break
		}
	}

	certReady, err := isCertificateReady(certManagerTestNS, certManagerCertName)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof(
			"Certificate %s/%s not found, skipping cert-manager cert info: %v",
			certManagerTestNS, certManagerCertName, err)

		return
	}

	upgradeVar.CertManager.CertReady = certReady

	tlsSecret, err := secret.Pull(
		cnfinittools.TargetSNOAPIClient, certManagerSecretName, certManagerTestNS)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof(
			"TLS secret %s/%s not found, skipping cert checksum: %v",
			certManagerTestNS, certManagerSecretName, err)

		return
	}

	certPEM := tlsSecret.Object.Data["tls.crt"]
	if len(certPEM) == 0 {
		klog.V(cnfparams.CNFLogLevel).Infof("tls.crt not found in secret %s/%s",
			certManagerTestNS, certManagerSecretName)

		return
	}

	checksum := md5.Sum(certPEM) //nolint:gosec
	upgradeVar.CertManager.TLSCertChecksum = hex.EncodeToString(checksum[:])

	block, _ := pem.Decode(certPEM)
	if block == nil {
		klog.V(cnfparams.CNFLogLevel).Infof(
			"No PEM block found in secret %s/%s tls.crt, skipping serial extraction",
			certManagerTestNS, certManagerSecretName)

		return
	}

	cert, parseErr := x509.ParseCertificate(block.Bytes)
	if parseErr != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Failed to parse certificate: %v", parseErr)

		return
	}

	upgradeVar.CertManager.TLSCertSerial = cert.SerialNumber.String()

	klog.V(cnfparams.CNFLogLevel).Infof(
		"Cert-manager info collected: issuerReady=%t, certReady=%t, checksum=%s, serial=%s",
		upgradeVar.CertManager.IssuerReady,
		upgradeVar.CertManager.CertReady,
		upgradeVar.CertManager.TLSCertChecksum,
		upgradeVar.CertManager.TLSCertSerial)
}

// isClusterIssuerReady checks whether a cert-manager ClusterIssuer has a Ready=True condition.
// Returns (false, nil) if the issuer exists but has no Ready=True condition, or (false, err)
// if the issuer cannot be retrieved.
func isClusterIssuerReady(issuerName string) (bool, error) {
	issuerObj, err := cnfinittools.TargetSNOAPIClient.Resource(certManagerClusterIssuerGVR).Get(
		context.TODO(), issuerName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get ClusterIssuer %s: %w", issuerName, err)
	}

	conditions, found, err := unstructured.NestedSlice(issuerObj.Object, "status", "conditions")
	if err != nil || !found {
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

// isCertificateReady checks whether a cert-manager Certificate CR has a Ready=True condition.
// Returns (false, nil) if the certificate exists but has no Ready=True condition, or (false, err)
// if the certificate cannot be retrieved.
func isCertificateReady(namespace, name string) (bool, error) {
	certObj, err := cnfinittools.TargetSNOAPIClient.Resource(certManagerCertGVR).Namespace(namespace).Get(
		context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get Certificate %s/%s: %w", namespace, name, err)
	}

	conditions, found, err := unstructured.NestedSlice(certObj.Object, "status", "conditions")
	if err != nil || !found {
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
