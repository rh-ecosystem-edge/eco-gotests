package sriovocpenv

import (
	"context"
	"fmt"
	"time"

	nmstateShared "github.com/nmstate/kubernetes-nmstate/api/shared"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	// NmstateName is the default NMState instance name.
	NmstateName = "nmstate"
	// NmstateHandlerDsName is the NMState handler daemonset name.
	NmstateHandlerDsName = "nmstate-handler"
	// NmstateWebhookDeploymentName is the NMState webhook deployment name.
	NmstateWebhookDeploymentName = "nmstate-webhook"
	// NmstateOperatorNamespace is the openshift-nmstate operator namespace.
	NmstateOperatorNamespace = "openshift-nmstate"
)

// IsNMStateOperatorInstalled reports whether the NMState operator namespace exists.
func IsNMStateOperatorInstalled() bool {
	_, err := namespace.Pull(APIClient, NmstateOperatorNamespace)

	return err == nil
}

// CreateNewNMStateAndWaitUntilItsRunning creates a new NMState instance and waits until it is ready.
func CreateNewNMStateAndWaitUntilItsRunning(timeout time.Duration) error {
	nmstateInstance, err := nmstate.PullNMstate(APIClient, NmstateName)
	if err == nil {
		_, err = nmstateInstance.Delete()
		if err != nil {
			return err
		}
	}

	_, err = nmstate.NewBuilder(APIClient, NmstateName).Create()
	if err != nil {
		return err
	}

	return isNMStateDeployedAndReady(timeout)
}

func isNMStateDeployedAndReady(timeout time.Duration) error {
	var (
		nmstateHandlerDs         *daemonset.Builder
		nmstateWebhookDeployment *deployment.Builder
		err                      error
	)

	err = wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			nmstateHandlerDs, err = daemonset.Pull(
				APIClient, NmstateHandlerDsName, NmstateOperatorNamespace)
			if err != nil {
				return false, nil
			}

			nmstateWebhookDeployment, err = deployment.Pull(
				APIClient, NmstateWebhookDeploymentName, NmstateOperatorNamespace)
			if err != nil {
				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return err
	}

	time.Sleep(10 * time.Second)

	if !nmstateHandlerDs.IsReady(timeout) {
		return fmt.Errorf("nmstate handler daemonset is not ready")
	}

	if !nmstateWebhookDeployment.IsReady(timeout) {
		return fmt.Errorf("nmstate webhook deployment is not ready")
	}

	return nil
}

// ConfigureVFsAndWaitUntilItsConfigured creates an NMState policy with VF configuration and waits until applied.
func ConfigureVFsAndWaitUntilItsConfigured(
	policyName string,
	sriovInterfaceName string,
	nodeLabel map[string]string,
	numberOfVFs uint8,
	timeout time.Duration) error {
	nmstatePolicy := nmstate.NewPolicyBuilder(
		APIClient, policyName, nodeLabel).WithInterfaceAndVFs(sriovInterfaceName, numberOfVFs)

	nmstatePolicy, err := nmstatePolicy.Create()
	if err != nil {
		return err
	}

	return nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionAvailable, timeout)
}

// UpdatePolicyAndWaitUntilItsAvailable updates an NMState policy and waits until it is available.
func UpdatePolicyAndWaitUntilItsAvailable(timeout time.Duration, nmstatePolicy *nmstate.PolicyBuilder) error {
	nmstatePolicy, err := nmstatePolicy.Update(true)
	if err != nil {
		return err
	}

	err = nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionProgressing, timeout)
	if err != nil {
		return err
	}

	return nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionAvailable, timeout)
}

// AreVFsCreated verifies that the specified number of VFs has been created under the given interface.
func AreVFsCreated(nodeName, sriovInterfaceName string, numberVFs int) error {
	klog.V(90).Infof("Verifying that node %s has %d VFs under interface %s",
		nodeName, numberVFs, sriovInterfaceName)

	sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
		APIClient, nodeName, SriovOcpConfig.OcpSriovOperatorNamespace)

	if err := sriovNetworkState.Discover(); err != nil {
		return err
	}

	numVFs, err := sriovNetworkState.GetNumVFs(sriovInterfaceName)
	if err != nil {
		return err
	}

	if numVFs != numberVFs {
		return fmt.Errorf("not all VFs are configured, expected number: %d; actual number: %d", numberVFs, numVFs)
	}

	return nil
}

// WaitUntilVfsCreated waits until the expected number of VFs exist on all given nodes.
func WaitUntilVfsCreated(
	nodeList []*nodes.Builder,
	sriovInterfaceName string,
	numberOfVfs int,
	timeout time.Duration,
) error {
	for _, node := range nodeList {
		err := wait.PollUntilContextTimeout(
			context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
				sriovNetworkState := sriov.NewNetworkNodeStateBuilder(
					APIClient, node.Object.Name, SriovOcpConfig.OcpSriovOperatorNamespace)

				if discoverErr := sriovNetworkState.Discover(); discoverErr != nil {
					return false, nil
				}

				sriovNumVfs, numErr := sriovNetworkState.GetNumVFs(sriovInterfaceName)
				if numErr != nil {
					return false, nil
				}

				return sriovNumVfs == numberOfVfs, nil
			})
		if err != nil {
			return err
		}
	}

	return nil
}
