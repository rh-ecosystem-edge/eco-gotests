package assisted

import (
	"context"
	"fmt"
	"time"

	"math/rand"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	hiveextV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	hiveV1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/hive/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	agentInfraEnvLabel = "infraenvs.agent-install.openshift.io"
	agentBMHLabel      = "agent-install.openshift.io/bmh"
)

// InfraEnvBuilder provides struct for the infraenv object containing connection to
// the cluster and the infraenv definitions.
type InfraEnvBuilder struct {
	Definition *agentInstallV1Beta1.InfraEnv
	Object     *agentInstallV1Beta1.InfraEnv
	errorMsg   string
	apiClient  goclient.Client
}

// InfraEnvAdditionalOptions additional options for InfraEnv object.
type InfraEnvAdditionalOptions func(builder *InfraEnvBuilder) (*InfraEnvBuilder, error)

// NewInfraEnvBuilder creates a new instance of InfraEnvBuilder.
func NewInfraEnvBuilder(apiClient *clients.Settings, name, nsname, psName string) *InfraEnvBuilder {
	klog.V(100).Infof(
		"Initializing new infraenv structure with the following params: "+
			"name: %s, namespace: %s, pull-secret: %s",
		name, nsname, psName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	builder := &InfraEnvBuilder{
		apiClient: apiClient.Client,
		Definition: &agentInstallV1Beta1.InfraEnv{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: agentInstallV1Beta1.InfraEnvSpec{
				PullSecretRef: &corev1.LocalObjectReference{
					Name: psName,
				},
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the infraenv is empty")

		builder.errorMsg = "infraenv 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the infraenv is empty")

		builder.errorMsg = "infraenv 'namespace' cannot be empty"

		return builder
	}

	if psName == "" {
		klog.V(100).Info("The pull-secret ref of the infraenv is empty")

		builder.errorMsg = "infraenv 'pull-secret' cannot be empty"

		return builder
	}

	return builder
}

// WithClusterRef sets the cluster reference to be used by the infraenv.
func (builder *InfraEnvBuilder) WithClusterRef(name, nsname string) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding clusterRef %s in namespace %s to InfraEnv %s", name, nsname, builder.Definition.Name)

	if name == "" {
		klog.V(100).Info("The name of the infraenv clusterRef is empty")

		builder.errorMsg = "infraenv clusterRef 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the infraenv clusterRef is empty")

		builder.errorMsg = "infraenv clusterRef 'namespace' cannot be empty"

		return builder
	}

	builder.Definition.Spec.ClusterRef = &agentInstallV1Beta1.ClusterReference{
		Name:      name,
		Namespace: nsname,
	}

	return builder
}

// WithAdditionalNTPSource adds additional servers as NTP sources for the spoke cluster.
func (builder *InfraEnvBuilder) WithAdditionalNTPSource(ntpSource string) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding ntpSource %s to InfraEnv %s", ntpSource, builder.Definition.Name)

	builder.Definition.Spec.AdditionalNTPSources = append(builder.Definition.Spec.AdditionalNTPSources, ntpSource)

	return builder
}

// WithSSHAuthorizedKey sets the authorized ssh key for accessing the nodes during discovery.
func (builder *InfraEnvBuilder) WithSSHAuthorizedKey(sshAuthKey string) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding sshAuthorizedKey %s to InfraEnv %s", sshAuthKey, builder.Definition.Name)

	builder.Definition.Spec.SSHAuthorizedKey = sshAuthKey

	return builder
}

// WithAgentLabel adds labels to be applied to agents that boot from the infraenv.
func (builder *InfraEnvBuilder) WithAgentLabel(key, value string) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding agentLabel %s:%s to InfraEnv %s", key, value, builder.Definition.Name)

	if builder.Definition.Spec.AgentLabels == nil {
		builder.Definition.Spec.AgentLabels = make(map[string]string)
	}

	builder.Definition.Spec.AgentLabels[key] = value

	return builder
}

// WithProxy includes a proxy configuration to be used by the infraenv.
func (builder *InfraEnvBuilder) WithProxy(proxy agentInstallV1Beta1.Proxy) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding proxy %s to InfraEnv %s", proxy, builder.Definition.Name)

	builder.Definition.Spec.Proxy = &proxy

	return builder
}

// WithNmstateConfigLabelSelector adds a selector for identifying
// nmstateconfigs that should be applied to this infraenv.
func (builder *InfraEnvBuilder) WithNmstateConfigLabelSelector(selector metav1.LabelSelector) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding nmstateconfig selector %s to InfraEnv %s", &selector, builder.Definition.Name)

	builder.Definition.Spec.NMStateConfigLabelSelector = selector

	return builder
}

// WithCPUType sets the cpu architecture for the discovery ISO.
func (builder *InfraEnvBuilder) WithCPUType(arch string) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding cpuArchitecture %s to InfraEnv %s", arch, builder.Definition.Name)

	builder.Definition.Spec.CpuArchitecture = arch

	return builder
}

// WithIgnitionConfigOverride includes the specified ignitionconfigoverride for discovery.
func (builder *InfraEnvBuilder) WithIgnitionConfigOverride(override string) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding ignitionConfigOverride %s to InfraEnv %s", override, builder.Definition.Name)

	builder.Definition.Spec.IgnitionConfigOverride = override

	return builder
}

// WithIPXEScriptType modifies the IPXE script type generated by the infraenv.
func (builder *InfraEnvBuilder) WithIPXEScriptType(scriptType agentInstallV1Beta1.IPXEScriptType) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding ipxeScriptType %s to InfraEnv %s", scriptType, builder.Definition.Name)

	builder.Definition.Spec.IPXEScriptType = scriptType

	return builder
}

// WithKernelArgument appends kernel configurations to be configured by the infraenv.
func (builder *InfraEnvBuilder) WithKernelArgument(kernelArg agentInstallV1Beta1.KernelArgument) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding kernelArgument %s to InfraEnv %s", kernelArg, builder.Definition.Name)

	builder.Definition.Spec.KernelArguments = append(builder.Definition.Spec.KernelArguments, kernelArg)

	return builder
}

// WithOptions creates InfraEnv with generic mutation options.
func (builder *InfraEnvBuilder) WithOptions(
	options ...InfraEnvAdditionalOptions) *InfraEnvBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting InfraEnv additional options")

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

// WaitForDiscoveryISOCreation waits the defined timeout for the discovery ISO to be generated.
func (builder *InfraEnvBuilder) WaitForDiscoveryISOCreation(timeout time.Duration) (*InfraEnvBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	// Polls every retryInterval to determine if infraenv in desired state.
	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			return builder.Object.Status.CreatedTime != nil, nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, err
}

// GetAllAgents returns a slice of agentBuilders of all agents belonging to the infraenv.
func (builder *InfraEnvBuilder) GetAllAgents() ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting all agents from infraenv %s",
		builder.Definition.Name)

	if !builder.Exists() {
		return nil, fmt.Errorf("cannot get agents from non-existent infraenv")
	}

	agents, err := builder.GetAgentsByLabel(agentInfraEnvLabel, builder.Definition.Name)
	if err != nil {
		return nil, err
	}

	return agents, nil
}

// GetAgentsByRole returns a slice of agentBuilders of agents matching specified role belonging to the infraenv.
func (builder *InfraEnvBuilder) GetAgentsByRole(role string) ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting agents from infraenv %s matching role %s",
		builder.Definition.Name, role)

	if !builder.Exists() {
		klog.V(100).Infof("Cannot get agents from non-existent infraenv: %s",
			role)

		return nil, fmt.Errorf("cannot get agents from non-existent infraenv")
	}

	var agents, agentsByRole []*agentBuilder

	agents, err := builder.GetAllAgents()
	if err != nil {
		return nil, err
	}

	for _, agent := range agents {
		if string(agent.Object.Status.Role) == role {
			agentsByRole = append(agentsByRole, agent)
		}
	}

	return agentsByRole, nil
}

// GetAgentByBMH returns an agentBuilder for the agent matching specified BMH belonging to the infraenv.
func (builder *InfraEnvBuilder) GetAgentByBMH(bmhName string) (*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting agent from infraenv %s matching bmh %s",
		builder.Definition.Name, bmhName)

	if !builder.Exists() {
		return nil, fmt.Errorf("cannot get agents from non-existent infraenv")
	}

	agents, err := builder.GetAgentsByLabel(agentBMHLabel, bmhName)
	if err != nil {
		return nil, err
	}

	switch len(agents) {
	case 1:
		return agents[0], nil
	case 0:
		klog.V(100).Infof("Found no agents referencing bmh %s", bmhName)

		return nil, fmt.Errorf("found no agents referencing bmh %s", bmhName)
	default:
		klog.V(100).Infof("Found multiple agent referencing bmh %s", bmhName)

		return nil, fmt.Errorf("found multiple agents referencing bmh %s", bmhName)
	}
}

// GetAgentByName returns an agentBuilder for the agent matching specified name belonging to the infraenv.
func (builder *InfraEnvBuilder) GetAgentByName(name string) (*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting agent from infraenv %s with name %s",
		builder.Definition.Name, name)

	if !builder.Exists() {
		return nil, fmt.Errorf("cannot get agents from non-existent infraenv")
	}

	agent := &agentBuilder{
		apiClient: builder.apiClient,
		Definition: &agentInstallV1Beta1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: builder.Definition.Namespace,
			},
		},
	}

	if !agent.Exists() {
		return nil, fmt.Errorf("agent object %s does not exist in namespace %s", name, builder.Definition.Namespace)
	}

	return agent, nil
}

// GetAgentsByLabel returns a slice of agentBuilders for agents matching specified label.
func (builder *InfraEnvBuilder) GetAgentsByLabel(key, value string) ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting agent matching label %s:%s",
		key, value)

	if !builder.Exists() {
		return nil, fmt.Errorf("cannot get agents from non-existent infraenv")
	}

	matchLabel := map[string]string{key: value}

	var agents agentInstallV1Beta1.AgentList

	err := builder.apiClient.List(logging.DiscardContext(), &agents, goclient.MatchingLabels(matchLabel))
	if err != nil {
		return nil, err
	}

	return builder.createBuilderListFromAgentList(agents.Items), nil
}

// WaitForAgentsToRegister waits the specified time for agents to register
// matching the provisioninRequirements of the related AgentClusterInstall.
func (builder *InfraEnvBuilder) WaitForAgentsToRegister(timeout time.Duration) ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("cannot get agents from non-existent infraenv")
	}

	agentclusterinstall, err := builder.GetAgentClusterInstallFromInfraEnv()
	if err != nil {
		return nil, err
	}

	var agentList []*agentBuilder

	agentCount := agentclusterinstall.Spec.ProvisionRequirements.ControlPlaneAgents +
		agentclusterinstall.Spec.ProvisionRequirements.WorkerAgents

	// Polls every retryInterval to determine if agent has registered.
	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			agentList, err = builder.GetAllAgents()
			if err != nil {
				return false, err
			}

			return len(agentList) == agentCount, nil
		})

	return agentList, err
}

// WaitForMasterAgents waits the specified time for agents with the role master
// to register and match the ControlPlaneAgents count in ProvisionRequirements of the related AgentClusterInstall.
func (builder *InfraEnvBuilder) WaitForMasterAgents(timeout time.Duration) ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	agentclusterinstall, err := builder.GetAgentClusterInstallFromInfraEnv()
	if err != nil {
		return nil, err
	}

	var agentList []*agentBuilder

	agentCount := agentclusterinstall.Spec.ProvisionRequirements.ControlPlaneAgents

	// Polls every retryInterval to determine if agent has registered.
	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			agentList, err = builder.GetAgentsByRole("master")
			if err != nil {
				return false, err
			}

			return len(agentList) == agentCount, nil
		})

	return agentList, err
}

// WaitForMasterAgentCount waits the specified time for agents
// with the role master to register and match the specified count.
func (builder *InfraEnvBuilder) WaitForMasterAgentCount(count int, timeout time.Duration) ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	var agentList []*agentBuilder

	// Polls every retryInterval to determine if agent has registered.
	err := wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			agentList, err := builder.GetAgentsByRole("master")
			if err != nil {
				return false, err
			}

			return len(agentList) == count, nil
		})

	return agentList, err
}

// GetRandomMasterAgent returns an agentBuilder of a random agent that has it's role set to master.
func (builder *InfraEnvBuilder) GetRandomMasterAgent() (*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	agentList, err := builder.GetAgentsByRole("master")
	if err != nil {
		return nil, err
	}

	if len(agentList) == 0 {
		return nil, fmt.Errorf("could not find any master agents")
	}

	randInt := random.Intn(len(agentList))

	return agentList[randInt], nil
}

// WaitForWorkerAgents waits the specified time for agents with the role worker to register and match the WorkerAgents
// count in ProvisionRequirements of the related AgentClusterInstall.
func (builder *InfraEnvBuilder) WaitForWorkerAgents(timeout time.Duration) ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	agentclusterinstall, err := builder.GetAgentClusterInstallFromInfraEnv()
	if err != nil {
		return nil, err
	}

	var agentList []*agentBuilder

	agentCount := agentclusterinstall.Spec.ProvisionRequirements.WorkerAgents

	// Polls every retryInterval to determine if agent has registered.
	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			agentList, err = builder.GetAgentsByRole("worker")
			if err != nil {
				return false, err
			}

			return len(agentList) == agentCount, nil
		})

	return agentList, err
}

// WaitForWorkerAgentCount waits the specified time
// for agents with the role worker to register and match the specified count.
func (builder *InfraEnvBuilder) WaitForWorkerAgentCount(count int, timeout time.Duration) ([]*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	var agentList []*agentBuilder

	// Polls every retryInterval to determine if agent has registered.
	err := wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			agentList, err := builder.GetAgentsByRole("worker")
			if err != nil {
				return false, err
			}

			return len(agentList) == count, nil
		})

	return agentList, err
}

// GetRandomWorkerAgent returns an agentBuilder of a random agent that has it's role set to worker.
func (builder *InfraEnvBuilder) GetRandomWorkerAgent() (*agentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	agentList, err := builder.GetAgentsByRole("worker")
	if err != nil {
		return nil, err
	}

	if len(agentList) == 0 {
		return nil, fmt.Errorf("could not find any worker agents")
	}

	randInt := random.Intn(len(agentList))

	return agentList[randInt], nil
}

// createBuilderListFromAgentList takes an Agent slice and transforms it into an *agentBuilder slice.
func (builder *InfraEnvBuilder) createBuilderListFromAgentList(agents []agentInstallV1Beta1.Agent) []*agentBuilder {
	if valid, _ := builder.validate(); !valid {
		return nil
	}

	var buliderList []*agentBuilder

	for _, agent := range agents {
		copiedAgent := agent
		buliderList = append(buliderList, newAgentBuilder(builder.apiClient, &copiedAgent))
	}

	return buliderList
}

// GetAgentClusterInstallFromInfraEnv returns the AgentClusterInstall that is referenced by this InfraEnv.
func (builder *InfraEnvBuilder) GetAgentClusterInstallFromInfraEnv() (*hiveextV1Beta1.AgentClusterInstall, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	if !builder.Exists() {
		klog.V(100).Infof("Getting infraenv %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot wait from agents to register with non-existent infraenv")
	}

	var clusterdeployment hiveV1.ClusterDeployment

	klog.V(100).Infof("Getting clusterdeployment %s in namespace %s",
		builder.Object.Spec.ClusterRef.Name, builder.Object.Spec.ClusterRef.Namespace)

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Object.Spec.ClusterRef.Name,
		Namespace: builder.Object.Spec.ClusterRef.Namespace,
	}, &clusterdeployment)
	if err != nil {
		klog.V(100).Infof("Unable to get clusterdeployment %s referenced by infraenv %s",
			builder.Object.Spec.ClusterRef.Name, builder.Definition.Name)

		return nil, err
	}

	klog.V(100).Infof("Getting agentclusterinstall %s",
		clusterdeployment.Spec.ClusterInstallRef.Name)

	var agentclusterinstall hiveextV1Beta1.AgentClusterInstall

	err = builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      clusterdeployment.Spec.ClusterInstallRef.Name,
		Namespace: clusterdeployment.Namespace,
	}, &agentclusterinstall)
	if err != nil {
		klog.V(100).Infof("Unable to get agentclusterinstall %s referenced by clusterdeployment %s",
			clusterdeployment.Spec.ClusterInstallRef.Name, clusterdeployment.Name)

		return nil, err
	}

	return &agentclusterinstall, nil
}

// Get fetches the defined infraenv from the cluster.
func (builder *InfraEnvBuilder) Get() (*agentInstallV1Beta1.InfraEnv, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting infraenv %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	infraEnv := &agentInstallV1Beta1.InfraEnv{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, infraEnv)
	if err != nil {
		return nil, err
	}

	return infraEnv, nil
}

// PullInfraEnvInstall pulls existing infraenv from cluster.
func PullInfraEnvInstall(apiClient *clients.Settings, name, nsname string) (*InfraEnvBuilder, error) {
	klog.V(100).Infof("Pulling existing infraenv name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	builder := &InfraEnvBuilder{
		apiClient: apiClient.Client,
		Definition: &agentInstallV1Beta1.InfraEnv{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the infraenv is empty")

		return nil, fmt.Errorf("infraenv 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the infraenv is empty")

		return nil, fmt.Errorf("infraenv 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("infraenv object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create generates a infraenv on the cluster.
func (builder *InfraEnvBuilder) Create() (*InfraEnvBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the infraenv %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies an existing infraenv on the cluster.
func (builder *InfraEnvBuilder) Update(force bool) (*InfraEnvBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating infraenv %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("infraenv %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent infraenv")
	}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("infraenv", builder.Definition.Name, builder.Definition.Namespace))

			err = builder.DeleteAndWait(time.Second * 5)
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof(
					"Failed to update the infraenv object %s in namespace %s, "+
						"due to error in delete function",
					builder.Definition.Name, builder.Definition.Namespace,
				)

				return nil, err
			}

			return builder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes an infraenv from the cluster.
func (builder *InfraEnvBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the infraenv %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("infraenv %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete infraenv: %w", err)
	}

	builder.Object = nil

	return nil
}

// DeleteAndWait deletes an InfraEnv and waits until it is removed from the cluster.
func (builder *InfraEnvBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof(`Deleting InfraEnv %s and
	waiting for the defined period until it is removed`,
		builder.Definition.Name)

	if err := builder.Delete(); err != nil {
		return err
	}

	// Polls the InfraEnv every second until it is removed.
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err != nil {
				return true, nil
			}

			return false, err
		})
}

// Exists checks if the defined infraenv has already been created.
func (builder *InfraEnvBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if infraenv %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *InfraEnvBuilder) validate() (bool, error) {
	resourceCRD := "InfraEnv"

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
