package day1day2env

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/day1day2/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

// DoesClusterSupportDay1Day2Tests verifies if given environment supports Day1Day2 tests.
func DoesClusterSupportDay1Day2Tests(requiredCPNodeNumber, requiredWorkerNodeNumber int) error {
	klog.V(90).Infof("Verifying if NMState operator is deployed")

	if err := isNMStateOperatorDeployed(); err != nil {
		return err
	}

	workerNodeList, err := nodes.List(
		APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()},
	)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Verifying if cluster has enough workers to run Day1Day2 tests")

	if len(workerNodeList) < requiredWorkerNodeNumber {
		return fmt.Errorf("cluster has less than %d worker nodes", requiredWorkerNodeNumber)
	}

	controlPlaneNodeList, err := nodes.List(
		APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.ControlPlaneLabelMap).String()},
	)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Verifying if cluster has enough control-plane nodes to run Day1Day2 tests")

	if len(controlPlaneNodeList) < requiredCPNodeNumber {
		return fmt.Errorf("cluster has less than %d control-plane nodes", requiredCPNodeNumber)
	}

	return nil
}

// GetBondModeViaCmd returns Bond mode for given Bond interface on a specific node.
func GetBondModeViaCmd(bondInterfaceName, nodeName string) (string, error) {
	klog.V(90).Infof("Getting Bond mode for bond interface %s on a node %s", bondInterfaceName, nodeName)

	bondMode, err := cmd.RunCommandOnHostNetworkPod(nodeName, tsparams.TestNamespaceName,
		fmt.Sprintf("cat /sys/class/net/%s/bonding/mode", bondInterfaceName))
	if err != nil {
		return "", err
	}

	return bondMode, nil
}

// GetBondInterfaceMiimon returns miimon value for given bond interface and node.
func GetBondInterfaceMiimon(nodeName, bondInterfaceName string) (int, error) {
	klog.V(90).Infof("Getting miimon value for bond interface %s on node %s", bondInterfaceName, nodeName)

	nodeNetworkState, err := nmstate.PullNodeNetworkState(APIClient, nodeName)
	if err != nil {
		return 0, err
	}

	bondInterface, err := nodeNetworkState.GetInterfaceType(bondInterfaceName, "bond")
	if err != nil {
		return 0, err
	}

	return bondInterface.LinkAggregation.Options.Miimon, nil
}

// GetFirstVfInterfaceMaxTxRate returns MaxTxRate value for given interface (VF0) and node.
func GetFirstVfInterfaceMaxTxRate(nodeName, interfaceName string) (int, error) {
	klog.V(90).Infof("Getting MaxTxRate value for first VF of interface %s on node %s", interfaceName, nodeName)

	nodeNetworkState, err := nmstate.PullNodeNetworkState(APIClient, nodeName)
	if err != nil {
		return 0, err
	}

	sriovInterface, err := nodeNetworkState.GetInterfaceType(interfaceName, "ethernet")
	if err != nil {
		return 0, err
	}

	return *sriovInterface.Ethernet.Sriov.Vfs[0].MaxTxRate, nil
}

// CheckConnectivityBetweenMasterAndWorkers creates a hostnetwork pod on the master node and ping all workers nodes.
// The Pod will be removed at the end.
func CheckConnectivityBetweenMasterAndWorkers() error {
	klog.V(90).Infof("Checking connectivity between master node and worker nodes")

	masterNodes, err := nodes.List(
		APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.ControlPlaneLabelMap).String()},
	)
	if err != nil {
		return err
	}

	workerNodeList, err := nodes.List(
		APIClient,
		metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()},
	)
	if err != nil {
		return err
	}

	podMaster, err := pod.NewBuilder(
		APIClient, "mastertestpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(masterNodes[0].Definition.Name).WithHostNetwork().
		WithPrivilegedFlag().WithTolerationToMaster().CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	if err != nil {
		return err
	}

	for _, workerNode := range workerNodeList {
		err = cmd.ICMPConnectivityCheck(podMaster, []string{workerNode.Object.Status.Addresses[0].Address + "/24"})
		if err != nil {
			return fmt.Errorf("connectivity check between %s and %s failed: %w",
				masterNodes[0].Definition.Name, workerNode.Object.Name, err)
		}
	}

	_, err = podMaster.DeleteAndWait(netparam.DefaultTimeout)
	if err != nil {
		return err
	}

	return nil
}

func isNMStateOperatorDeployed() error {
	nmstateNS := namespace.NewBuilder(APIClient, NetConfig.NMStateOperatorNamespace)
	if !nmstateNS.Exists() {
		return fmt.Errorf("error NMState namespace %s doesn't exist", nmstateNS.Definition.Name)
	}

	nmstateOperatorDeployment, err := deployment.Pull(
		APIClient, "nmstate-operator", NetConfig.NMStateOperatorNamespace)
	if err != nil {
		return fmt.Errorf("error to pull nmstate-operator deployment from the cluster")
	}

	if !nmstateOperatorDeployment.IsReady(30 * time.Second) {
		return fmt.Errorf("error nmstate-operator deployment is not in ready/ready state")
	}

	return nil
}
