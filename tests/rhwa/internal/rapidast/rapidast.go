package rapidast

import (
	"time"

	"github.com/golang/glog"
	"github.com/openshift-kni/eco-goinfra/pkg/clients"
	"github.com/openshift-kni/eco-goinfra/pkg/nodes"
	"github.com/openshift-kni/eco-goinfra/pkg/pod"
	"github.com/openshift-kni/eco-goinfra/pkg/rbac"
	"github.com/openshift-kni/eco-goinfra/pkg/serviceaccount"
	. "github.com/openshift-kni/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/openshift-kni/eco-gotests/tests/rhwa/internal/rhwaparams"

	v1 "k8s.io/api/rbac/v1"
)

const (
	logLevel = rhwaparams.LogLevel
)

func PrepareRapidastPod(apiClient *clients.Settings) *pod.Builder {
	nodes, err := nodes.List(apiClient)
	if err != nil {
		glog.V(logLevel).Infof(
			"Error in node list retrieval %s", err.Error())
	}

	_, err = serviceaccount.NewBuilder(APIClient, "trivy-service-account", rhwaparams.TestNamespaceName).Create()
	if err != nil {
		glog.V(logLevel).Infof(
			"Error in service acount creation %s", err.Error())
	}

	_, err = rbac.NewClusterRoleBuilder(APIClient, "trivy-clusterrole", v1.PolicyRule{
		APIGroups: []string{
			"",
		},
		Resources: []string{
			"pods",
		},
		Verbs: []string{
			"get",
			"list",
			"watch",
		},
	}).Create()
	if err != nil {
		glog.V(logLevel).Infof(
			"Error in ClusterRoleBuilder creation %s", err.Error())
	}

	_, err = rbac.NewClusterRoleBindingBuilder(APIClient, "trivy-clusterrole-binding", "trivy-clusterrole", v1.Subject{
		Kind:      "ServiceAccount",
		Name:      "trivy-service-account",
		Namespace: rhwaparams.TestNamespaceName,
	}).Create()
	if err != nil {
		glog.V(logLevel).Infof(
			"Error in ClusterRoleBindingBuilder creation %s", err.Error())
	}

	dastTestPod := pod.NewBuilder(
		APIClient, "rapidastclientpod", rhwaparams.TestNamespaceName, rhwaparams.TestContainerDast).
		DefineOnNode(nodes[0].Object.Name).
		WithTolerationToMaster().
		WithPrivilegedFlag()
	if err != nil {
		glog.V(logLevel).Infof(
			"Error in rapidast client pod definition %s", err.Error())
	}

	dastTestPod.Definition.Spec.ServiceAccountName = "trivy-service-account"

	_, err = dastTestPod.CreateAndWaitUntilRunning(time.Minute)
	if err != nil {
		glog.V(logLevel).Infof(
			"Error in rapidast client pod creation %s", err.Error())
	}

	return dastTestPod

}
