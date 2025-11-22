package tsparams

import (
	"time"

	"k8s.io/klog/v2"
)

type (
	// DeploymentType is the method of cluster deployment.
	DeploymentType string
	// PolicyType is the cluster policy type.
	PolicyType string
	// ClusterType is the type of cluster deployment.
	ClusterType string
	// MultiClusterType is either single cluster or multi cluster deployment.
	MultiClusterType string
)

const (
	// DeploymentSiteConfig is the siteconfig deployment type.
	DeploymentSiteConfig DeploymentType = "SiteConfig"
	// DeploymentImageBasedCI is the clusterinstance IBI deployment type.
	DeploymentImageBasedCI DeploymentType = "ClusterInstance ImageClusterInstall"
	// DeploymentAssistedCI is the clusterinstance AI deployment type.
	DeploymentAssistedCI DeploymentType = "ClusterInstance AgentClusterInstall"

	// PolicyPGT is the policygentemplate policy type.
	PolicyPGT PolicyType = "PolicyGenTemplate"
	// PolicyACMPG is the ACM PG policy type.
	PolicyACMPG PolicyType = "PolicyGenerator"
	// PolicyPGTHST is the policygentemplate with hub-side templating policy type.
	PolicyPGTHST PolicyType = "PolicyGenTemplate with hub-side templating"
	// PolicyACMPGHST is the ACM PG with hub-side templating policy type.
	PolicyACMPGHST PolicyType = "PolicyGenerator with hub-side templating"

	// MultiCluster is for a multiple cluster deployment.
	MultiCluster MultiClusterType = "multi cluster"
	// SingleCluster is for a single cluster deployment.
	SingleCluster MultiClusterType = "single cluster"

	// ClusterSNO is the SNO cluster type.
	ClusterSNO ClusterType = "SNO Cluster"
	// ClusterSNOPlusWorker is the SNO plus worker cluster type.
	ClusterSNOPlusWorker ClusterType = "SNO+Worker Cluster"
	// ClusterThreeNode is the three node cluster type.
	ClusterThreeNode ClusterType = "3 Node Cluster"
	// ClusterStandard is the standard cluster (3 masters, 2 workers) cluster type.
	ClusterStandard ClusterType = "Standard Cluster"
)

const (
	// LabelSuite is the label for all the tests in this suite.
	LabelSuite string = "deploymenttypes"
	// LabelDeploymentTypeTestCases is the label for deployment type checking.
	LabelDeploymentTypeTestCases string = "deployment-types"
	// ArgoCdPoliciesAppName is the name of the policies app in Argo CD.
	ArgoCdPoliciesAppName string = "policies"
	// ArgoCdClustersAppName is the name of the clusters app in Argo CD.
	ArgoCdClustersAppName string = "clusters"

	// ArgoCdChangeTimeout is the time to use for polling for changes to Argo CD.
	ArgoCdChangeTimeout = 10 * time.Minute

	// TestNamespace is the namespace used for deployment types tests.
	TestNamespace = "deployment-test"
	// LogLevel is the verbosity of glog statements in this test suite.
	LogLevel klog.Level = 90
)
