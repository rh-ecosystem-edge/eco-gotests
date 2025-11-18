package netparam

import (
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreparams"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(coreparams.Labels, Label)
	// LabelHostName contains the key for the hostname label.
	LabelHostName = "kubernetes.io/hostname"
	// OperatorResourceInjector defaults SR-IOV network resource injector daemonset.
	OperatorResourceInjector = "network-resources-injector"
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// MCOWaitTimeout represent timeout for mco operations.
	MCOWaitTimeout = 35 * time.Minute
	// VtySh represents default vtysh cmd prefix.
	VtySh = []string{"vtysh", "-c"}
	// ClusterMonitoringNSLabel represents Cluster Monitoring label for a NS to enable Prometheus Scraping.
	ClusterMonitoringNSLabel = map[string]string{"openshift.io/cluster-monitoring": "true"}
	// MlxVendorID is the Mellanox Sriov Vendor ID.
	MlxVendorID = "15b3"
	// IPForwardAndSleepCmd defines a shell command to enable IPv4 forwarding and keep a process running indefinitely.
	IPForwardAndSleepCmd = []string{
		"/bin/bash",
		"-c",
		`echo 1 > /proc/sys/net/ipv4/ip_forward 2>/dev/null || true; \
trap : TERM INT; sleep infinity & wait`,
	}
)
