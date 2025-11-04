package network

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// OperatorBuilder provides a struct for network.operator object from the cluster and a network.operator definition.
type OperatorBuilder struct {
	// network.operator definition, used to create the network.operator object.
	Definition *operatorv1.Network
	// Created network.operator object.
	Object *operatorv1.Network
	// api client to interact with the cluster.
	apiClient goclient.Client
	errorMsg  string
}

// PullOperator loads an existing network.operator into OperatorBuilder struct.
func PullOperator(apiClient *clients.Settings) (*OperatorBuilder, error) {
	glog.V(100).Infof("Pulling existing network.operator name: %s", clusterNetworkName)

	if apiClient == nil {
		glog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("network.operator 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(operatorv1.Install)
	if err != nil {
		glog.V(100).Info("Failed to add operator v1 scheme to client schemes")

		return nil, err
	}

	builder := &OperatorBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorv1.Network{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterNetworkName,
			},
		},
	}

	if !builder.Exists() {
		glog.V(100).Infof("network.operator object %s does not exist", clusterNetworkName)

		return nil, fmt.Errorf("network.operator object %s does not exist", clusterNetworkName)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given network.operator exists.
func (builder *OperatorBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if network.operator %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns network.operator object.
func (builder *OperatorBuilder) Get() (*operatorv1.Network, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	clusterNetwork := &operatorv1.Network{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{Name: builder.Definition.Name}, clusterNetwork)
	if err != nil {
		glog.V(100).Infof("Failed to get network.operator object %s: %v", builder.Definition.Name, err)

		return nil, err
	}

	return clusterNetwork, nil
}

// Update renovates the existing network.operator object with the new definition in builder.
func (builder *OperatorBuilder) Update() (*OperatorBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the network.operator object %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("network.operator object %s does not exist", builder.Definition.Name)

		return nil, fmt.Errorf("network.operator object %s does not exist", builder.Definition.Name)
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		glog.V(100).Infof("Failed to update network.operator object %s: %v", builder.Definition.Name, err)

		return nil, err
	}

	return builder, nil
}

// SetLocalGWMode switches network.operator OVN mode from/to local mode.
func (builder *OperatorBuilder) SetLocalGWMode(state bool, timeout time.Duration) (*OperatorBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	var err error

	if builder.Definition.Spec.DefaultNetwork.OVNKubernetesConfig.GatewayConfig.RoutingViaHost != state {
		builder.Definition.Spec.DefaultNetwork.OVNKubernetesConfig.GatewayConfig.RoutingViaHost = state

		builder, err := builder.Update()
		if err != nil {
			return nil, err
		}

		err = builder.WaitUntilInCondition(
			operatorv1.OperatorStatusTypeProgressing, 300*time.Second, operatorv1.ConditionTrue)
		if err != nil {
			return nil, err
		}

		err = builder.WaitUntilInCondition(
			operatorv1.OperatorStatusTypeProgressing, timeout, operatorv1.ConditionFalse)
		if err != nil {
			return nil, err
		}

		return builder, builder.WaitUntilInCondition(
			operatorv1.OperatorStatusTypeAvailable, 60*time.Second, operatorv1.ConditionTrue)
	}

	return builder, err
}

// SetMultiNetworkPolicy enables network.operator multinetworkpolicy feature.
func (builder *OperatorBuilder) SetMultiNetworkPolicy(state bool, timeout time.Duration) (*OperatorBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Applying MultiNetworkPolicy flag %t to network.operator %s", state, builder.Definition.Name)

	var err error

	if *builder.Definition.Spec.UseMultiNetworkPolicy != state {
		builder.Definition.Spec.UseMultiNetworkPolicy = &state

		builder, err := builder.Update()
		if err != nil {
			return nil, err
		}

		err = builder.WaitUntilInCondition(
			operatorv1.OperatorStatusTypeProgressing, 60*time.Second, operatorv1.ConditionTrue)
		if err != nil {
			return nil, err
		}

		err = builder.WaitUntilInCondition(
			operatorv1.OperatorStatusTypeProgressing, timeout, operatorv1.ConditionFalse)
		if err != nil {
			return nil, err
		}

		return builder, builder.WaitUntilInCondition(
			operatorv1.OperatorStatusTypeAvailable, 60*time.Second, operatorv1.ConditionTrue)
	}

	return builder, err
}

// WaitUntilInCondition waits for a specific time duration until the network.operator will have a
// specified condition type with the expected status.
func (builder *OperatorBuilder) WaitUntilInCondition(
	condition string, timeout time.Duration, status operatorv1.ConditionStatus) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Wait until network.operator object %s is in condition %v",
		builder.Definition.Name, condition)

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			if !builder.Exists() {
				return false, fmt.Errorf("network.operator object %s does not exist", builder.Definition.Name)
			}

			for _, c := range builder.Object.Status.Conditions {
				if c.Type == condition && c.Status == status {
					return true, nil
				}
			}

			return false, nil
		})

	return err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *OperatorBuilder) validate() (bool, error) {
	resourceCRD := "network.operator"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
