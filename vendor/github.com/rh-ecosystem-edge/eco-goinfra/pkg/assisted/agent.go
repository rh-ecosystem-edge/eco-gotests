package assisted

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/models"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nonExistentMsg = "cannot update non-existent agent"
)

// agentBuilder provides struct for the agent object containing connection to
// the cluster and the agent definitions.
type agentBuilder struct {
	Definition *agentInstallV1Beta1.Agent
	Object     *agentInstallV1Beta1.Agent
	errorMsg   string
	apiClient  goclient.Client
}

// AgentAdditionalOptions additional options for agent object.
type AgentAdditionalOptions func(builder *agentBuilder) (*agentBuilder, error)

// newAgentBuilder creates a new instance of agentBuilder
// Users cannot create agent resources themselves as they are generated from the operator.
func newAgentBuilder(apiClient goclient.Client, definition *agentInstallV1Beta1.Agent) *agentBuilder {
	if definition == nil {
		return nil
	}

	glog.V(100).Infof("Initializing new agent structure for the following agent %s",
		definition.Name)

	builder := agentBuilder{
		apiClient:  apiClient,
		Definition: definition,
		Object:     definition,
	}

	return &builder
}

// PullAgent pulls existing agent from cluster.
func PullAgent(apiClient *clients.Settings, name, nsname string) (*agentBuilder, error) {
	glog.V(100).Infof("Pulling existing agent name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	builder := &agentBuilder{
		apiClient: apiClient.Client,
		Definition: &agentInstallV1Beta1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the agent is empty")

		return nil, fmt.Errorf("agent 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the agent is empty")

		return nil, fmt.Errorf("agent 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("agent object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// WithHostName sets the hostname of the agent resource.
func (builder *agentBuilder) WithHostName(hostname string) *agentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting agent %s in namespace %s hostname to %s",
		builder.Definition.Name, builder.Definition.Namespace, hostname)

	if !builder.Exists() {
		glog.V(100).Infof("agent %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.errorMsg = nonExistentMsg

		return builder
	}

	builder.Definition.Spec.Hostname = hostname

	return builder
}

// WithRole sets the role of the agent resource.
func (builder *agentBuilder) WithRole(role string) *agentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting agent %s in namespace %s to role %s",
		builder.Definition.Name, builder.Definition.Namespace, role)

	if !builder.Exists() {
		glog.V(100).Infof("agent %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.errorMsg = nonExistentMsg

		return builder
	}

	builder.Definition.Spec.Role = models.HostRole(role)

	return builder
}

// WithInstallationDisk sets the installationDiskID of the agent.
func (builder *agentBuilder) WithInstallationDisk(diskID string) *agentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting agent %s in namespace %s installation disk id to %s",
		builder.Definition.Name, builder.Definition.Namespace, diskID)

	builder.Definition.Spec.InstallationDiskID = diskID

	return builder
}

// WithIgnitionConfigOverride sets the ignitionConfigOverrides of the agent.
func (builder *agentBuilder) WithIgnitionConfigOverride(override string) *agentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting agent %s in namespace %s ignitionConfigOverride to %s",
		builder.Definition.Name, builder.Definition.Namespace, override)

	builder.Definition.Spec.IgnitionConfigOverrides = override

	return builder
}

// WithApproval sets the approved field of the agent.
func (builder *agentBuilder) WithApproval(approved bool) *agentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting agent %s in namespace %s approval to %v",
		builder.Definition.Name, builder.Definition.Namespace, approved)

	builder.Definition.Spec.Approved = approved

	return builder
}

// WaitForState waits the specified timeout for the agent to report the specified state.
func (builder *agentBuilder) WaitForState(state string, timeout time.Duration) (*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Waiting for agent %s in namespace %s to report state %s",
		builder.Definition.Name, builder.Definition.Namespace, state)

	// Polls every retryInterval to determine if agent is in desired state.
	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			return builder.Object.Status.DebugInfo.State == state, nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, err
}

// WaitForStateInfo waits the specified timeout for the agent to report the specified stateInfo.
func (builder *agentBuilder) WaitForStateInfo(stateInfo string, timeout time.Duration) (*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Waiting for agent %s in namespace %s to report stateInfo %s",
		builder.Definition.Name, builder.Definition.Namespace, stateInfo)

	// Polls every retryInterval to determine if agent is in desired state.
	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			return builder.Object.Status.DebugInfo.StateInfo == stateInfo, nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, err
}

// WithOptions creates agent with generic mutation options.
func (builder *agentBuilder) WithOptions(options ...AgentAdditionalOptions) *agentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting agent additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				glog.V(100).Infof("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// Get fetches the defined agent from the cluster.
func (builder *agentBuilder) Get() (*agentInstallV1Beta1.Agent, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting agent %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	agent := &agentInstallV1Beta1.Agent{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, agent)
	if err != nil {
		return nil, err
	}

	return agent, nil
}

// Update modifies the agent resource on the cluster
// to match what is defined in the local definition of the builder.
func (builder *agentBuilder) Update() (*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating agent %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("agent %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("%s", nonExistentMsg)
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks if the defined agent has already been created.
func (builder *agentBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if agent %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes an agent from the cluster.
func (builder *agentBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the agent %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("agent %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete agent: %w", err)
	}

	builder.Object = nil

	return nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *agentBuilder) validate() (bool, error) {
	resourceCRD := "Agent"

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
