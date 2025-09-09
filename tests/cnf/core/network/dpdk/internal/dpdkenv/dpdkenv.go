package dpdkenv

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// DoesClusterSupportDpdkTests verifies if given cluster supports dpdk workload and test cases.
func DoesClusterSupportDpdkTests(
	apiClient *clients.Settings, netConfig *netconfig.NetworkConfig, requiredCPU int64, requestedRAMGb int) error {
	glog.V(90).Infof("Verifying if cluster supports dpdk tests")

	err := netenv.DoesClusterHasEnoughNodes(apiClient, netConfig, 1, 2)

	if err != nil {
		return err
	}

	workerNodeList, err := nodes.List(
		apiClient,
		metav1.ListOptions{LabelSelector: labels.Set(netConfig.WorkerLabelMap).String()},
	)

	if err != nil {
		return err
	}

	for _, worker := range workerNodeList {
		if int(worker.Object.Status.Capacity.Memory().Value()/1024/1024/1024) < requestedRAMGb {
			return fmt.Errorf("worker %s has less than required ram number: %d", worker.Object.Name, requestedRAMGb)
		}

		if worker.Object.Status.Capacity.Cpu().Value() < requiredCPU {
			return fmt.Errorf("worker %s has less than required cpu cores: %d", worker.Object.Name, requiredCPU)
		}
	}

	err = netenv.IsSriovDeployed(apiClient, netConfig)
	if err != nil {
		return err
	}

	return nil
}
