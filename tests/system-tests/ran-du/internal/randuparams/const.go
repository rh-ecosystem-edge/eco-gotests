package randuparams

import (
	"time"
)

const (
	// Label represents RAN DU system tests label that can be used for test cases selection.
	Label = "randu"
	// LabelLaunchWorkloadTestCases represents tests labels related to test workload.
	LabelLaunchWorkloadTestCases = "launch-workload"
	// LabelCertManager represents cert-manager test cases label.
	LabelCertManager = "cert-manager"
	// DefaultTimeout is the timeout used for test resources creation.
	DefaultTimeout = 900 * time.Second
	// TestWorkloadShellLaunchMethod is used when using a shell script for launching the test workload.
	TestWorkloadShellLaunchMethod = "shell"
	// RanDuLogLevel configures logging level for RAN DU related tests.
	RanDuLogLevel = 90
	// CertManagerOperatorNamespace is the namespace for the cert-manager operator.
	CertManagerOperatorNamespace = "cert-manager-operator"
	// CertManagerNamespace is the namespace for cert-manager core components.
	CertManagerNamespace = "cert-manager"
	// CertManagerTestNamespace is the namespace for cert-manager test certificates.
	CertManagerTestNamespace = "cert-test"
	// CertManagerDefaultTimeout is the timeout for cert-manager certificate operations.
	CertManagerDefaultTimeout = 3 * time.Minute
	// CertManagerPollInterval is the default polling interval for cert-manager resource checks.
	CertManagerPollInterval = 10 * time.Second
	// CertManagerAlertPollInterval is the polling interval for alert status checks.
	CertManagerAlertPollInterval = 30 * time.Second
	// CertManagerAlertTimeout is the max time to wait for a single alert threshold.
	// Sized to cover up to two cert renewal cycles (in case the first cycle is missed
	// due to PrometheusRule loading delays).
	CertManagerAlertTimeout = 15 * time.Minute
	// CertManagerAPIServerRolloutTimeout is the timeout for kube-apiserver rollout.
	CertManagerAPIServerRolloutTimeout = 15 * time.Minute
	// CertManagerPrometheusQuerierSAName is the ServiceAccount name for Prometheus API access.
	CertManagerPrometheusQuerierSAName = "randu-prometheus-querier"
	// CertManagerPrometheusQuerierCRBName is the ClusterRoleBinding name for Prometheus API access.
	CertManagerPrometheusQuerierCRBName = "randu-prometheus-querier-crb"
	// CertManagerOpenshiftMonitoringNamespace is the namespace for OpenShift monitoring.
	CertManagerOpenshiftMonitoringNamespace = "openshift-monitoring"
	// CertManagerAlertNameInfo is the name of the info-level certificate renewal alert.
	CertManagerAlertNameInfo = "CertManagerCertRenewalInfo"
	// CertManagerAlertNameWarning is the name of the warning-level certificate renewal alert.
	CertManagerAlertNameWarning = "CertManagerCertRenewalWarning"
	// CertManagerAlertNameCritical is the name of the critical-level certificate renewal alert.
	CertManagerAlertNameCritical = "CertManagerCertRenewalCritical"
)
