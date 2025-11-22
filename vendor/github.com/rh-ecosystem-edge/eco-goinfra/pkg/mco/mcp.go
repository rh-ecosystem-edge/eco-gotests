package mco

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	fiveScds time.Duration = 5 * time.Second
)

// MCPBuilder provides struct for MachineConfigPool object which contains connection to cluster
// and MachineConfigPool definitions.
type MCPBuilder struct {
	// MachineConfigPool definition. Used to create MachineConfigPool object with minimum set of required elements.
	Definition *mcv1.MachineConfigPool
	// Created MachineConfigPool object on the cluster.
	Object *mcv1.MachineConfigPool
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg is processed before MachineConfigPool object is created.
	errorMsg string
}

// MCPAdditionalOptions additional options for mcp object.
type MCPAdditionalOptions func(builder *MCPBuilder) (*MCPBuilder, error)

// NewMCPBuilder method creates new instance of builder.
func NewMCPBuilder(apiClient *clients.Settings, mcpName string) *MCPBuilder {
	klog.V(100).Infof(
		"Initializing new MCPBuilder structure with the following params: %s", mcpName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the MachineConfigPool is nil")

		return nil
	}

	err := apiClient.AttachScheme(mcv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add machineconfig v1 scheme to client schemes")

		return nil
	}

	builder := &MCPBuilder{
		apiClient: apiClient.Client,
		Definition: &mcv1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{
				Name: mcpName,
			},
		},
	}

	if mcpName == "" {
		klog.V(100).Info("The name of the MachineConfigPool is empty")

		builder.errorMsg = "machineconfigpool 'name' cannot be empty"

		return builder
	}

	return builder
}

// Pull pulls existing machineconfigpool from cluster.
func Pull(apiClient *clients.Settings, name string) (*MCPBuilder, error) {
	klog.V(100).Infof("Pulling existing machineconfigpool name %s from cluster", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the MachineConfigPool is nil")

		return nil, fmt.Errorf("machineconfigpool 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(mcv1.Install)
	if err != nil {
		klog.V(100).Info("Failed to add machineconfig v1 scheme to client schemes")

		return nil, err
	}

	builder := &MCPBuilder{
		apiClient: apiClient.Client,
		Definition: &mcv1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the machineconfigpool is empty")

		return nil, fmt.Errorf("machineconfigpool 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("machineconfigpool object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the MachineConfigPool object if found.
func (builder *MCPBuilder) Get() (*mcv1.MachineConfigPool, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting MachineConfigPool object %s", builder.Definition.Name)

	machineConfigPool := &mcv1.MachineConfigPool{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeclient.ObjectKey{Name: builder.Definition.Name}, machineConfigPool)
	if err != nil {
		klog.V(100).Infof("MachineConfigPool object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return machineConfigPool, nil
}

// Create makes a MachineConfigPool in cluster and stores the created object in struct.
func (builder *MCPBuilder) Create() (*MCPBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the MachineConfigPool %s",
		builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes a MachineConfigPool object from a cluster.
func (builder *MCPBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the MachineConfigPool object %s",
		builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("MachineConfigPool %s cannot be deleted because it does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Object)
	if err != nil {
		return fmt.Errorf("cannot delete machineconfigpool: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given MachineConfigPool exists.
func (builder *MCPBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if the MachineConfigPool object %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithMcSelector defines the machineConfigSelector in the machine config pool.
func (builder *MCPBuilder) WithMcSelector(mcSelector map[string]string) *MCPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("WithMcSelector updates builder object with "+
		"machineConfigSelector label: %v", mcSelector)

	if len(mcSelector) == 0 {
		builder.errorMsg = "machineConfigSelector 'MatchLabels' field cannot be empty"

		return builder
	}

	if builder.Definition.Spec.MachineConfigSelector == nil {
		builder.Definition.Spec.MachineConfigSelector = &metav1.LabelSelector{}
	}

	builder.Definition.Spec.MachineConfigSelector.MatchLabels = mcSelector

	return builder
}

// WaitToBeInCondition waits for a specific time duration until the MachineConfigPool will have a
// specified condition type with the expected status.
func (builder *MCPBuilder) WaitToBeInCondition(
	conditionType mcv1.MachineConfigPoolConditionType,
	conditionStatus corev1.ConditionStatus,
	timeout time.Duration,
) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("WaitToBeInCondition waits up to specified time duration %v until "+
		"MachineConfigPool condition %v is met", timeout, conditionType)

	return wait.PollUntilContextTimeout(
		context.TODO(), fiveScds, timeout, true, func(ctx context.Context) (bool, error) {
			mcp, err := builder.Get()
			if err != nil {
				return false, nil
			}

			for _, condition := range mcp.Status.Conditions {
				if condition.Type == conditionType && condition.Status == conditionStatus {
					return true, nil
				}
			}

			return false, nil
		})
}

// WaitForUpdate waits for a MachineConfigPool to be updating and then updated.
func (builder *MCPBuilder) WaitForUpdate(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("WaitForUpdate waits up to specified time %v until updating"+
		" machineConfigPool object is updated", timeout)

	mcpUpdating, err := builder.Get()
	if err != nil {
		return err
	}

	for _, condition := range mcpUpdating.Status.Conditions {
		if condition.Type == "Updating" && condition.Status == corev1.ConditionTrue {
			err := wait.PollUntilContextTimeout(
				context.TODO(), fiveScds, timeout, true, func(ctx context.Context) (bool, error) {
					mcpUpdated, err := builder.Get()
					if err != nil {
						return false, nil
					}

					for _, condition := range mcpUpdated.Status.Conditions {
						if condition.Type == "Updated" && condition.Status == corev1.ConditionTrue {
							return true, nil
						}
					}

					return false, nil
				})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// WaitToBeStableFor waits on MachineConfigPool to stable for a time duration or until timeout.
func (builder *MCPBuilder) WaitToBeStableFor(stableDuration time.Duration, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("WaitToBeStableFor waits up to duration of %v for "+
		"MachineConfigPool to be stable for %v", timeout, stableDuration)

	isMcpStable := true

	// Wait 5 secs in each iteration before condition function () returns true or errors
	// or times out after stableDuration
	err := wait.PollUntilContextTimeout(
		context.TODO(), fiveScds, timeout, true, func(ctx context.Context) (bool, error) {
			isMcpStable = true

			_ = wait.PollUntilContextTimeout(
				context.TODO(), fiveScds, stableDuration, true, func(ctx2 context.Context) (done bool, err error) {
					if !builder.Exists() {
						return false, nil
					}

					if builder.Object.Status.ReadyMachineCount != builder.Object.Status.MachineCount ||
						builder.Object.Status.MachineCount != builder.Object.Status.UpdatedMachineCount ||
						builder.Object.Status.DegradedMachineCount != 0 {
						klog.V(100).Infof("MachineConfigPool: %v degraded and has a mismatch in "+
							"machineCount: %v "+"vs machineCountUpdated: "+"%v vs readyMachineCount: %v and "+
							"degradedMachineCount is : %v \n", builder.Object.Name,
							builder.Object.Status.MachineCount, builder.Object.Status.UpdatedMachineCount,
							builder.Object.Status.ReadyMachineCount, builder.Object.Status.DegradedMachineCount)

						isMcpStable = false

						return true, nil
					}

					return false, nil
				})

			if isMcpStable {
				klog.V(100).Infof("MachineConfigPool was stable during during stableDuration: %v",
					stableDuration)

				// this will exit the outer wait.PollUntilContextTimeout block since the mcp was stable during stableDuration
				return true, nil
			}

			klog.V(100).Infof("MachineConfigPool was not stable during stableDuration: %v, retrying ...",
				stableDuration)

			// keep iterating in the outer wait.PollUntilContextTimeout waiting for cluster to be stable
			return false, nil
		})

	// After the timout in outer wait.PollUntilContextTimeout.
	if err == nil {
		klog.V(100).Infof("Cluster was stable during stableDuration: %v", stableDuration)
	} else {
		// Here err is "timed out waiting for the condition"
		klog.V(100).Infof("Cluster was Un-stable during stableDuration: %v", stableDuration)
	}

	return err
}

// WithOptions creates mcp with generic mutation options.
func (builder *MCPBuilder) WithOptions(options ...MCPAdditionalOptions) *MCPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting mcp additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// IsInCondition parses MachineConfigPool conditions.
// Returns true if given MachineConfigPool is in given condition, otherwise false.
func (builder *MCPBuilder) IsInCondition(mcpConditionType mcv1.MachineConfigPoolConditionType) bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("IsInCondition returns true"+
		" if MachineConfigPool object is in a given condition %v, otherwise false", mcpConditionType)

	if builder.Exists() {
		for _, condition := range builder.Object.Status.Conditions {
			if condition.Type == mcpConditionType && condition.Status == corev1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *MCPBuilder) validate() (bool, error) {
	resourceCRD := "MachineConfigPool"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
