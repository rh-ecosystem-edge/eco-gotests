package amdgpucommon

import (
	"context"

	"github.com/golang/glog"
	"github.com/openshift-kni/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsSingleNodeOpenShift determines if this is a Single Node OpenShift cluster.
func IsSingleNodeOpenShift(apiClient *clients.Settings) (bool, error) {
	nodes, err := apiClient.CoreV1Interface.Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, err
	}

	nodeCount := len(nodes.Items)
	glog.V(100).Infof("Detected %d nodes in cluster", nodeCount)

	if nodeCount == 1 {
		node := nodes.Items[0]
		if _, hasControlPlane := node.Labels["node-role.kubernetes.io/control-plane"]; hasControlPlane {
			glog.V(100).Infof("Confirmed SNO: single node %s has control-plane role", node.Name)

			return true, nil
		}

		if _, hasMaster := node.Labels["node-role.kubernetes.io/master"]; hasMaster {
			glog.V(100).Infof("Confirmed SNO: single node %s has master role", node.Name)

			return true, nil
		}

		glog.V(100).Infof("Warning: single node %s lacks master/control-plane role", node.Name)
	}

	return nodeCount == 1, nil
}
