package tsparams

import "github.com/golang/glog"

const (
	// LabelSuite is the label for all tests in the PTP suite.
	LabelSuite = "ptp"
	// LabelEventConsumer is the label for all tests in the PTP event consumer suite.
	LabelEventConsumer = "event-consumer"
	// LabelEventsAndMetrics is the label for all tests in the PTP events and metrics suite.
	LabelEventsAndMetrics = "events-and-metrics"
	// LabelNodeReboot is the label for all tests in the PTP node reboot suite.
	LabelNodeReboot = "node-reboot"
	// LabelInterfaces is the label for all tests in the PTP interfaces suite.
	LabelInterfaces = "interfaces"
	// LabelProcessRestart is the label for all tests in the PTP process restart suite.
	LabelProcessRestart = "process-restart"
	// LabelOC2Port is the label for all tests in the PTP OC 2 port suite.
	LabelOC2Port = "oc-two-port"

	// CloudEventsNamespace is the namespace used for the cloud events consumer and associated resources.
	CloudEventsNamespace = "cloud-events"
)

// LogLevel is the glog level used for all helpers in the PTP suite. It is set so that eco-goinfra is 100, cnf/ran is
// 90, and the suite itself is 80.
const LogLevel glog.Level = 80
