package ranparam

import (
	"time"

	"github.com/golang/glog"
)

const (
	// Label represents the label for the ran test cases.
	Label = "ran"
	// LabelNoContainer is the label for RAN test cases that should not be executed in a container.
	LabelNoContainer = "no-container"
	// AcmOperatorNamespace ACM's namespace.
	AcmOperatorNamespace = "rhacm"
	// MceOperatorNamespace is the namespace for the MCE operator.
	MceOperatorNamespace = "multicluster-engine"
	// TalmOperatorHubNamespace TALM namespace.
	TalmOperatorHubNamespace = "topology-aware-lifecycle-manager"
	// TalmContainerName is the name of the container in the talm pod.
	TalmContainerName = "manager"
	// OpenshiftOperatorNamespace is the namespace where operators are.
	OpenshiftOperatorNamespace = "openshift-operators"
	// OpenshiftGitOpsNamespace is the namespace for the GitOps operator.
	OpenshiftGitOpsNamespace = "openshift-gitops"
	// OpenshiftGitopsRepoServer ocp git repo server.
	OpenshiftGitopsRepoServer = "openshift-gitops-repo-server"
	// PtpContainerName is the name of the container in the PTP daemon pod.
	PtpContainerName = "linuxptp-daemon-container"
	// PtpDaemonsetLabelSelector is the label selector to find the PTP daemon pod.
	PtpDaemonsetLabelSelector = "app=linuxptp-daemon"
	// PtpOperatorNamespace is the namespace for the PTP operator.
	PtpOperatorNamespace = "openshift-ptp"
	// LogLevel is the verbosity for ran/internal packages.
	LogLevel glog.Level = 80
	// RetryInterval retry interval for node exec commands.
	RetryInterval = 10 * time.Second
	// RetryCount retry count for node exec commands.
	RetryCount = 3
)

// HubOperatorName represets the possible operator names that may have associated versions on the hub cluster.
type HubOperatorName string

const (
	// ACM is the name of the advanced cluster management operator.
	ACM HubOperatorName = "advanced-cluster-management"
	// TALM is the name of the topology aware lifecycle manager operator.
	TALM HubOperatorName = "topology-aware-lifecycle-manager"
	// GitOps is the name of the GitOps operator.
	GitOps HubOperatorName = "openshift-gitops-operator"
	// MCE is the name of the multicluster engine operator.
	MCE HubOperatorName = "multicluster-engine"
)

// ClusterType represents spoke cluster type.
type ClusterType string

const (
	// SNOCluster represents spoke cluster type as single-node openshift (SNO) cluster.
	SNOCluster ClusterType = "SNO"
	// HighlyAvailableCluster represents spoke cluster type as multi-node openshift (MNO) cluster.
	HighlyAvailableCluster ClusterType = "HighlyAvailable"
)
