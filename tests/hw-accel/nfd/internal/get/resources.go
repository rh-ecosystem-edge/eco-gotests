package get

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	master     = "nfd-master"
	worker     = "nfd-worker"
	controller = "nfd-controller-manager"
	topology   = "nfd-topology"
)

func initMap(nfdResourceCount map[string]int) {
	nfdResourceCount[master] = 1
	nfdResourceCount[worker] = 0
	nfdResourceCount[controller] = 1
	nfdResourceCount[topology] = 0
}

// NfdResourceCount count nfd topology and worker pods.
func NfdResourceCount(apiClient *clients.Settings) map[string]int {
	nodelistt, _ := nodes.List(apiClient, metav1.ListOptions{})

	nfdResourceCount := make(map[string]int)

	initMap(nfdResourceCount)

	for _, node := range nodelistt {
		if _, ok := node.Object.Labels["node-role.kubernetes.io/worker"]; ok {
			nfdResourceCount[worker]++
			nfdResourceCount[topology]++
		}
	}

	return nfdResourceCount
}
