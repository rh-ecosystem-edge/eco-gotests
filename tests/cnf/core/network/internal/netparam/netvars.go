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
	// OperatorConfigDaemon defaults SR-IOV config daemon daemonset.
	OperatorConfigDaemon = "sriov-network-config-daemon"
	// OperatorDevicePlugin defaults SR-IOV device plugin daemonset.
	OperatorDevicePlugin = "sriov-device-plugin"
	// OperatorWebhook defaults SR-IOV webhook daemonset.
	OperatorWebhook = "operator-webhook"
	// OperatorResourceInjector defaults SR-IOV network resource injector daemonset.
	OperatorResourceInjector = "network-resources-injector"
	// OperatorSriovDaemonsets represents all default SR-IOV operator daemonset names.
	OperatorSriovDaemonsets = []string{OperatorConfigDaemon, OperatorWebhook, OperatorResourceInjector}
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// MCOWaitTimeout represent timeout for mco operations.
	MCOWaitTimeout = 35 * time.Minute
	// VtySh represents default vtysh cmd prefix.
	VtySh = []string{"vtysh", "-c"}
	// MlxDeviceID is the Mellanox SRIOV device ID.
	MlxDeviceID = "1017"
	// MlxBFDeviceID is the Mellanox Bluefield SRIOV Device ID.
	MlxBFDeviceID = "a2d6"
	// 	MlxConnectX6 is the Mellanox Bluefield SRIOV ConnectX-6 Device ID.
	MlxConnectX6 = "101f"
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
