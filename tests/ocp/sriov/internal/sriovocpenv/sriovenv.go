package sriovocpenv

import (
	"fmt"

	sriovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// DoesClusterHaveEnoughNodes verifies if given cluster has enough nodes to run tests.
func DoesClusterHaveEnoughNodes(
	requiredCPNodeNumber,
	requiredWorkerNodeNumber int) error {
	klog.V(90).Infof("Verifying if cluster has enough workers to run tests")

	workerNodeList, err := nodes.List(
		APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()},
	)
	if err != nil {
		return err
	}

	if len(workerNodeList) < requiredWorkerNodeNumber {
		return fmt.Errorf("cluster has less than %d worker nodes", requiredWorkerNodeNumber)
	}

	controlPlaneNodeList, err := nodes.List(
		APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.ControlPlaneLabelMap).String()},
	)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Verifying if cluster has enough control-plane nodes to run tests")

	if len(controlPlaneNodeList) < requiredCPNodeNumber {
		return fmt.Errorf("cluster has less than %d control-plane nodes", requiredCPNodeNumber)
	}

	return nil
}

// ValidateSriovInterfaces checks that provided interfaces by env var exist on the nodes.
func ValidateSriovInterfaces(
	workerNodeList []*nodes.Builder,
	requestedNumber int) error {
	klog.V(90).Infof("Validating SR-IOV interfaces on cluster nodes")

	if len(workerNodeList) == 0 {
		return fmt.Errorf("workerNodeList is empty, cannot validate SR-IOV interfaces")
	}

	var validSriovInterfaceList []sriovV1.InterfaceExt

	klog.V(90).Infof("Getting available SR-IOV interfaces from node %s", workerNodeList[0].Definition.Name)

	availableUpSriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(APIClient,
		workerNodeList[0].Definition.Name, SriovOcpConfig.SriovOperatorNamespace).GetUpNICs()
	if err != nil {
		return fmt.Errorf("failed to get SR-IOV devices from the node %s: %w", workerNodeList[0].Definition.Name, err)
	}

	klog.V(90).Infof("Getting requested SR-IOV interfaces (requested number: %d)", requestedNumber)

	requestedSriovInterfaceList, err := SriovOcpConfig.GetSriovInterfaces(requestedNumber)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Validating that requested interfaces exist on the node")

	for _, availableUpSriovInterface := range availableUpSriovInterfaces {
		for _, requestedSriovInterface := range requestedSriovInterfaceList {
			if availableUpSriovInterface.Name == requestedSriovInterface {
				validSriovInterfaceList = append(validSriovInterfaceList, availableUpSriovInterface)
			}
		}
	}

	if len(validSriovInterfaceList) < requestedNumber {
		return fmt.Errorf("requested interfaces %v are not present on the cluster node", requestedSriovInterfaceList)
	}

	klog.V(90).Infof("SR-IOV interface validation completed successfully")

	return nil
}
