package certmanager

import (
	"time"
)

const (
	// OperatorNamespace is the namespace for the cert-manager operator.
	OperatorNamespace = "cert-manager-operator"
	// Namespace is the namespace for cert-manager core components.
	Namespace = "cert-manager"
	// TestNamespace is the namespace for cert-manager test certificates.
	TestNamespace = "cert-test"
	// OpenshiftMonitoringNamespace is the namespace for OpenShift monitoring.
	OpenshiftMonitoringNamespace = "openshift-monitoring"
	// DefaultTimeout is the timeout for cert-manager certificate operations.
	DefaultTimeout = 3 * time.Minute
	// PollInterval is the default polling interval for cert-manager resource checks.
	PollInterval = 10 * time.Second
	// AlertPollInterval is the polling interval for alert status checks.
	AlertPollInterval = 30 * time.Second
	// AlertTimeout is the max time to wait for a single alert threshold.
	// Sized to cover up to two cert renewal cycles (certificates in tests are configured
	// with 24h duration and 23h45m renewBefore, yielding a ~15min renewal window).
	// The timeout covers two cycles to account for PrometheusRule loading delays.
	AlertTimeout = 15 * time.Minute
	// APIServerRolloutTimeout is the timeout for kube-apiserver rollout.
	APIServerRolloutTimeout = 15 * time.Minute
	// AlertNameInfo is the name of the info-level certificate renewal alert.
	AlertNameInfo = "CertManagerCertRenewalInfo"
	// AlertNameWarning is the name of the warning-level certificate renewal alert.
	AlertNameWarning = "CertManagerCertRenewalWarning"
	// AlertNameCritical is the name of the critical-level certificate renewal alert.
	AlertNameCritical = "CertManagerCertRenewalCritical"
	// AlertStateInactive is the Prometheus alert state for an inactive alert.
	AlertStateInactive = "inactive"
)
