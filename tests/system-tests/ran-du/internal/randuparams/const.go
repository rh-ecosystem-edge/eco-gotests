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
	// CertManagerPrometheusQuerierSAName is the ServiceAccount name for Prometheus API access.
	CertManagerPrometheusQuerierSAName = "randu-prometheus-querier"
	// CertManagerPrometheusQuerierCRBName is the ClusterRoleBinding name for Prometheus API access.
	CertManagerPrometheusQuerierCRBName = "randu-prometheus-querier-crb"
)
