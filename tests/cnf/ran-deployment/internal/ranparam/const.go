package ranparam

import "k8s.io/klog/v2"

const (
	// Label represents the label for the ran test cases.
	Label string = "ran-deployment"

	// AcmOperatorNamespace ACM's namespace.
	AcmOperatorNamespace string = "rhacm"
	// MceOperatorNamespace is the namespace for the MCE operator.
	MceOperatorNamespace string = "multicluster-engine"
	// TalmOperatorHubNamespace TALM namespace.
	TalmOperatorHubNamespace string = "topology-aware-lifecycle-manager"
	// TalmContainerName is the name of the container in the talm pod.
	TalmContainerName string = "manager"
	// OpenshiftOperatorNamespace is the namespace where operators are.
	OpenshiftOperatorNamespace string = "openshift-operators"
	// OpenshiftGitOpsNamespace is the namespace for the GitOps operator.
	OpenshiftGitOpsNamespace string = "openshift-gitops"
	// OpenshiftGitopsRepoServer ocp git repo server.
	OpenshiftGitopsRepoServer string = "openshift-gitops-repo-server"

	// LogLevel is the verbosity for ran/internal packages.
	LogLevel klog.Level = 80
)
