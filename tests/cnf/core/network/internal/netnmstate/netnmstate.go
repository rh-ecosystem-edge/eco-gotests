package netnmstate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	nmstateShared "github.com/nmstate/kubernetes-nmstate/api/shared"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"

	"k8s.io/apimachinery/pkg/util/wait"
)

// CreateNewNMStateAndWaitUntilItsRunning creates or recreates the new NMState instance and waits until
// all daemonsets and deployments are in Ready state.
func CreateNewNMStateAndWaitUntilItsRunning(timeout time.Duration) error {
	klog.V(90).Infof("Verifying if NMState instance is installed")

	nmstateInstance, err := nmstate.PullNMstate(APIClient, netparam.NMState)
	if err == nil {
		klog.V(90).Infof("NMState exists. Removing NMState.")

		_, err = nmstateInstance.Delete()
		if err != nil {
			return err
		}
	}

	klog.V(90).Infof("Creating a new NMState instance.")

	_, err = nmstate.NewBuilder(APIClient, netparam.NMState).Create()
	if err != nil {
		return err
	}

	err = isNMStateDeployedAndReady(timeout)
	if err != nil {
		return err
	}

	return nil
}

// CreatePolicyAndWaitUntilItsAvailable creates NodeNetworkConfigurationPolicy and waits until
// it's successfully applied.
func CreatePolicyAndWaitUntilItsAvailable(timeout time.Duration, nmstatePolicy *nmstate.PolicyBuilder) error {
	klog.V(90).Infof("Creating an NMState policy and wait for its successful application.")

	nmstatePolicy, err := nmstatePolicy.Create()
	if err != nil {
		return err
	}

	klog.V(90).Infof("Waiting for the policy to reach the Progressing state.")

	err = nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionProgressing, timeout)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Waiting for the policy to reach the Available state.")

	err = nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionAvailable, timeout)
	if err != nil {
		return err
	}

	return nil
}

// UpdatePolicyAndWaitUntilItsAvailable updates NodeNetworkConfigurationPolicy and waits until
// it's successfully applied.
func UpdatePolicyAndWaitUntilItsAvailable(timeout time.Duration, nmstatePolicy *nmstate.PolicyBuilder) error {
	klog.V(90).Infof("Updating an NMState policy and wait for its successful application.")

	nmstatePolicy, err := nmstatePolicy.Update(true)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Waiting for the policy to reach the Progressing state.")

	err = nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionProgressing, timeout)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Waiting for the policy to reach the Available state.")

	err = nmstatePolicy.WaitUntilCondition(nmstateShared.NodeNetworkConfigurationPolicyConditionAvailable, timeout)
	if err != nil {
		return err
	}

	return nil
}

// ConfigureVFsAndWaitUntilItsConfigured creates NodeNetworkConfigurationPolicy with VFs configuration and waits until
// it's successfully configured.
func ConfigureVFsAndWaitUntilItsConfigured(
	policyName string,
	sriovInterfaceName string,
	nodeLabel map[string]string,
	numberOfVFs uint8,
	timeout time.Duration) error {
	klog.V(90).Infof("Creating an NMState policy with VFs (%d) on interface %s"+
		" and wait for it to be successfully configured.", numberOfVFs, sriovInterfaceName)

	nmstatePolicy := nmstate.NewPolicyBuilder(
		APIClient, policyName, nodeLabel).WithInterfaceAndVFs(sriovInterfaceName, numberOfVFs)

	err := CreatePolicyAndWaitUntilItsAvailable(timeout, nmstatePolicy)
	if err != nil {
		return err
	}

	// NodeNetworkStates exist for each node and share the same name.
	nodeList, err := nodes.List(APIClient, metav1.ListOptions{LabelSelector: labels.Set(nodeLabel).String()})
	if err != nil {
		return err
	}

	for _, node := range nodeList {
		err = AreVFsCreated(node.Definition.Name, sriovInterfaceName, int(numberOfVFs))
		if err != nil {
			return err
		}
	}

	return nil
}

// AreVFsCreated verifies that the specified number of VFs has been created by NMState
// under the given interface.
func AreVFsCreated(nmstateName, sriovInterfaceName string, numberVFs int) error {
	klog.V(90).Infof("Verifying that NMState %s has requested the specified number of VFs (%d)"+
		" under the interface %s.", nmstateName, numberVFs, sriovInterfaceName)

	nodeNetworkState, err := nmstate.PullNodeNetworkState(APIClient, nmstateName)
	if err != nil {
		return err
	}

	numVFs, err := nodeNetworkState.GetTotalVFs(sriovInterfaceName)
	if err != nil {
		return err
	}

	if numVFs != numberVFs {
		return fmt.Errorf("not all VFs are configured, expected number: %d; actual number: %d", numberVFs, numVFs)
	}

	return nil
}

// GetPrimaryInterfaceBond returns master ovs system bond interface.
func GetPrimaryInterfaceBond(nodeName string) (string, error) {
	klog.V(90).Infof("Verifying that bond interface is a master ovs system interface on the node %s.", nodeName)

	nodeNetworkState, err := nmstate.PullNodeNetworkState(APIClient, nodeName)
	if err != nil {
		return "", err
	}

	ovsBridgeInterface, err := nodeNetworkState.GetInterfaceType("br-ex", "ovs-bridge")
	if err != nil {
		return "", err
	}

	for _, bridgePort := range ovsBridgeInterface.Bridge.Port {
		if strings.Contains(bridgePort["name"], "bond") {
			return bridgePort["name"], nil
		}
	}

	klog.V(90).Infof("There is no a bond interface in the br-ex bridge ports %v",
		ovsBridgeInterface.Bridge.Port)

	return "", nil
}

// GetBondSlaves returns slave ports under given Bond interface name.
func GetBondSlaves(bondName, nodeNetworkStateName string) ([]string, error) {
	klog.V(90).Infof("Getting slave ports under Bond interface %s", bondName)

	nodeNetworkState, err := nmstate.PullNodeNetworkState(APIClient, nodeNetworkStateName)
	if err != nil {
		return nil, err
	}

	bondInterface, err := nodeNetworkState.GetInterfaceType(bondName, "bond")
	if err != nil {
		return nil, err
	}

	return bondInterface.LinkAggregation.Port, nil
}

// GetBondMode returns Bond mode under given Bond interface name.
func GetBondMode(bondName, nodeNetworkStateName string) (string, error) {
	klog.V(90).Infof("Getting Bond mode under Bond interface %s", bondName)

	nodeNetworkState, err := nmstate.PullNodeNetworkState(APIClient, nodeNetworkStateName)
	if err != nil {
		return "", err
	}

	bondInterface, err := nodeNetworkState.GetInterfaceType(bondName, "bond")
	if err != nil {
		return "", err
	}

	return bondInterface.LinkAggregation.Mode, nil
}

// WithOptionMaxTxRateOnFirstVf returns a func that mutate MaxTxRate value on the first VF.
func WithOptionMaxTxRateOnFirstVf(
	maxTxRate uint64, sriovInterfaceName string) func(*nmstate.PolicyBuilder) (*nmstate.PolicyBuilder, error) {
	return func(builder *nmstate.PolicyBuilder) (*nmstate.PolicyBuilder, error) {
		klog.V(90).Infof("Changing MaxTxRate value for the first VF on given SR-IOV interface to %d",
			maxTxRate)

		if sriovInterfaceName == "" {
			klog.V(90).Infof("The sriovInterfaceName can not be empty string")

			return builder, fmt.Errorf("the sriovInterfaceName is empty string")
		}

		var currentState nmstate.DesiredState

		err := yaml.Unmarshal(builder.Definition.Spec.DesiredState.Raw, &currentState)
		if err != nil {
			klog.V(90).Infof("Failed Unmarshal DesiredState")

			return builder, fmt.Errorf("failed Unmarshal DesiredState: %w", err)
		}

		var foundInterface bool

		for i, networkInterface := range currentState.Interfaces {
			if networkInterface.Name == sriovInterfaceName && networkInterface.Type == "ethernet" {
				value := int(maxTxRate)
				currentState.Interfaces[i].Ethernet.Sriov.Vfs = []nmstate.Vf{{ID: 0, MaxTxRate: &value}}
				foundInterface = true

				break
			}
		}

		if !foundInterface {
			klog.V(90).Infof("Failed to find given SR-IOV interface")

			return builder, fmt.Errorf("failed to find SR-IOV interface %s", sriovInterfaceName)
		}

		desiredStateYaml, err := yaml.Marshal(currentState)
		if err != nil {
			klog.V(90).Infof("Failed Marshal DesiredState")

			return builder, fmt.Errorf("failed to Marshal a new Desired state: %w", err)
		}

		builder.Definition.Spec.DesiredState = nmstateShared.NewState(string(desiredStateYaml))

		return builder, nil
	}
}

// WithBondOptionMiimon returns a function that mutates miimon value.
func WithBondOptionMiimon(
	miimon uint64, bondInterfaceName string) func(*nmstate.PolicyBuilder) (*nmstate.PolicyBuilder, error) {
	klog.V(90).Infof("Changing miimon value %d for the bondInterface to %s", miimon, bondInterfaceName)

	return withBondOptionMutator(func(options *nmstate.OptionsLinkAggregation) {
		options.Miimon = int(miimon)
	},
		bondInterfaceName,
	)
}

// WithBondOptionFailOverMac returns a function that mutates FailOverMac value.
func WithBondOptionFailOverMac(
	failOverMacValue, bondInterfaceName string) func(*nmstate.PolicyBuilder) (*nmstate.PolicyBuilder, error) {
	klog.V(90).Infof("Changing failOverMac value %s for the bondInterface to %s", failOverMacValue, bondInterfaceName)

	return withBondOptionMutator(func(options *nmstate.OptionsLinkAggregation) {
		options.FailOverMac = failOverMacValue
	},
		bondInterfaceName,
	)
}

// CheckThatWorkersDeployedWithBondVfs verifies whether workers have been deployed with the specified configuration
// of bonded VLAN virtual interfaces (VFs). This function ensures that the network setup adheres to the intended bond
// and VLAN configurations.
func CheckThatWorkersDeployedWithBondVfs(
	workerNodes []*nodes.Builder, namespace string) (string, []string, error) {
	klog.V(90).Infof("Verifying that the cluster deployed via bond interface")

	var (
		bondName string
		err      error
	)

	for _, worker := range workerNodes {
		bondName, err = GetPrimaryInterfaceBond(worker.Definition.Name)
		if err != nil {
			klog.V(90).Infof("Failed to get Slave Interfaces for the primary bond interface")

			return "", nil, err
		}

		if bondName == "" {
			klog.V(90).Infof("bondName is empty on worker %s", worker.Definition.Name)

			return "", nil, fmt.Errorf("primary interface on worker %s is not a bond interface",
				worker.Definition.Name)
		}
	}

	klog.V(90).Infof("Gathering enslave interfaces for the bond interface")

	bondSlaves, err := GetBondSlaves(bondName, workerNodes[0].Definition.Name)
	if err != nil {
		klog.V(90).Infof("Failed to get bond slave interfaces")

		return "", nil, err
	}

	klog.V(90).Infof(
		"Verifying that enslave interfaces are SR-IOV VF interfaces")

	for _, bondSlave := range bondSlaves {
		// If a baseInterface has SR-IOV PF, it means that the baseInterface is VF.
		_, err = cmd.GetSrIovPf(bondSlave, namespace, workerNodes[0].Definition.Name)
		if err != nil && strings.Contains(err.Error(), "No such file or directory") {
			klog.V(90).Infof("Failed to find PF for the baseInterface VFs")

			return "", nil, fmt.Errorf("bond slaves are not SR-IOV VFs")
		}

		if err != nil {
			klog.V(90).Infof("Failed to get SR-IOV PF interface")

			return "", nil, err
		}
	}

	return bondName, bondSlaves, nil
}

// withBondOptionMutator returns a function that mutates a specific option for a bond interface.
func withBondOptionMutator(
	mutateFunc func(*nmstate.OptionsLinkAggregation),
	bondInterfaceName string) func(*nmstate.PolicyBuilder) (*nmstate.PolicyBuilder, error) {
	return func(builder *nmstate.PolicyBuilder) (*nmstate.PolicyBuilder, error) {
		klog.V(90).Infof("Mutating the bond interface %s", bondInterfaceName)

		if bondInterfaceName == "" {
			klog.V(90).Infof("The bondInterfaceName cannot be an empty string")

			return builder, fmt.Errorf("the bondInterfaceName is an empty string")
		}

		var currentState nmstate.DesiredState

		err := yaml.Unmarshal(builder.Definition.Spec.DesiredState.Raw, &currentState)
		if err != nil {
			klog.V(90).Infof("Failed to unmarshal DesiredState")

			return builder, fmt.Errorf("failed to unmarshal DesiredState: %w", err)
		}

		var foundInterface bool

		for i, networkInterface := range currentState.Interfaces {
			if networkInterface.Name == bondInterfaceName && networkInterface.Type == "bond" {
				mutateFunc(&currentState.Interfaces[i].LinkAggregation.Options)

				foundInterface = true

				break
			}
		}

		if !foundInterface {
			klog.V(90).Infof("Failed to find the given Bond interface")

			return builder, fmt.Errorf("failed to find Bond interface %s", bondInterfaceName)
		}

		desiredStateYaml, err := yaml.Marshal(currentState)
		if err != nil {
			klog.V(90).Infof("Failed to marshal DesiredState")

			return builder, fmt.Errorf("failed to marshal a new Desired state: %w", err)
		}

		builder.Definition.Spec.DesiredState = nmstateShared.NewState(string(desiredStateYaml))

		return builder, nil
	}
}

func isNMStateDeployedAndReady(timeout time.Duration) error {
	klog.V(90).Infof("Checking that NMState deployments and daemonsets are ready.")

	var (
		nmstateHandlerDs         *daemonset.Builder
		nmstateWebhookDeployment *deployment.Builder
		err                      error
	)

	klog.V(90).Infof("Pulling all NMState default daemonsets and deployments.")

	err = wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			nmstateHandlerDs, err = daemonset.Pull(
				APIClient, netparam.NMStateHandlerDsName, NetConfig.NMStateOperatorNamespace)
			if err != nil {
				klog.V(90).Infof("Error to pull daemonset %s from namespace %s, retry",
					netparam.NMStateHandlerDsName, NetConfig.NMStateOperatorNamespace)

				return false, nil
			}

			nmstateWebhookDeployment, err = deployment.Pull(
				APIClient, netparam.NMStateWebhookDeploymentName, NetConfig.NMStateOperatorNamespace)
			if err != nil {
				klog.V(90).Infof("Error to pull deployment %s namespace %s, retry",
					netparam.NMStateWebhookDeploymentName, NetConfig.NMStateOperatorNamespace)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return err
	}

	klog.V(90).Infof("Waiting until all NMState resources are Ready.")

	// Workaround to skip failure "nmstate handler daemonset is not ready"
	time.Sleep(10 * time.Second)

	if !nmstateHandlerDs.IsReady(timeout) {
		return fmt.Errorf("nmstate handler daemonset is not ready")
	}

	if !nmstateWebhookDeployment.IsReady(timeout) {
		return fmt.Errorf("nmstate webhook deployment is not ready")
	}

	return nil
}
