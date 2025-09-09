package amdgpucommon

import (
	"context"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
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

	if nodeCount > 1 {
		return false, nil
	}

	node := nodes.Items[0]
	_, hasControlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, hasMaster := node.Labels["node-role.kubernetes.io/master"]

	if hasMaster || hasControlPlane {
		glog.V(100).Infof("Confirmed SNO: single node %s has master role or control-plane role", node.Name)

		return true, nil
	}

	glog.V(100).Infof("Not SNO: node count=%d, node %s labels: master=%v, control-plane=%v",
		len(nodes.Items), node.Name, hasMaster, hasControlPlane)

	return false, nil
}
