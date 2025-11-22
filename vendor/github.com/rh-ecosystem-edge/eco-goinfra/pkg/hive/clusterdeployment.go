package hive

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	hiveextV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	hiveV1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1/agent"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterDeploymentBuilder provides struct for the clusterdeployment object containing connection to
// the cluster and the clusterdeployment definitions.
type ClusterDeploymentBuilder struct {
	Definition *hiveV1.ClusterDeployment
	Object     *hiveV1.ClusterDeployment
	errorMsg   string
	apiClient  goclient.Client
}

// ClusterDeploymentAdditionalOptions additional options for ClusterDeployment object.
type ClusterDeploymentAdditionalOptions func(builder *ClusterDeploymentBuilder) (*ClusterDeploymentBuilder, error)

// NewABMClusterDeploymentBuilder creates a new instance of
// ClusterDeploymentBuilder with platform type set to agentBareMetal.
func NewABMClusterDeploymentBuilder(
	apiClient *clients.Settings,
	name string,
	nsname string,
	clusterName string,
	baseDomain string,
	clusterInstallRef string,
	agentSelector metav1.LabelSelector) *ClusterDeploymentBuilder {
	klog.V(100).Infof(
		`Initializing new agentbaremetal clusterdeployment structure with the following params: name: %s, namespace: %s,
		  clusterName: %s, baseDomain: %s, clusterInstallRef: %s, agentSelector: %s`,
		name, nsname, clusterName, baseDomain, clusterInstallRef, agentSelector)

	return NewClusterDeploymentByInstallRefBuilder(apiClient, name, nsname, clusterName, baseDomain,
		hiveV1.ClusterInstallLocalReference{
			Group:   hiveextV1Beta1.Group,
			Version: hiveextV1Beta1.Version,
			Kind:    "AgentClusterInstall",
			Name:    clusterInstallRef,
		}, hiveV1.Platform{
			AgentBareMetal: &agent.BareMetalPlatform{
				AgentSelector: agentSelector,
			},
		})
}

// NewClusterDeploymentByInstallRefBuilder creates a new instance of
// ClusterDeploymentBuilder with the provided install reference and platform.
func NewClusterDeploymentByInstallRefBuilder(
	apiClient *clients.Settings,
	name string,
	nsname string,
	clusterName string,
	baseDomain string,
	clusterInstallRef hiveV1.ClusterInstallLocalReference,
	platform hiveV1.Platform) *ClusterDeploymentBuilder {
	klog.V(100).Infof(
		`Initializing new agentbaremetal clusterdeployment structure with the following params: name: %s, namespace: %s,
		  clusterName: %s, baseDomain: %s, clusterInstallRef: %v, platform: %v`,
		name, nsname, clusterName, baseDomain, clusterInstallRef, platform)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(hiveV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add hive v1 scheme to client schemes")

		return nil
	}

	builder := &ClusterDeploymentBuilder{
		apiClient: apiClient.Client,
		Definition: &hiveV1.ClusterDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: hiveV1.ClusterDeploymentSpec{
				ClusterName:       clusterName,
				BaseDomain:        baseDomain,
				ClusterInstallRef: &clusterInstallRef,
				Platform:          platform,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the clusterdeployment is empty")

		builder.errorMsg = "clusterdeployment 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the clusterdeployment is empty")

		builder.errorMsg = "clusterdeployment 'namespace' cannot be empty"

		return builder
	}

	if clusterName == "" {
		klog.V(100).Info("The clusterName of the clusterdeployment is empty")

		builder.errorMsg = "clusterdeployment 'clusterName' cannot be empty"

		return builder
	}

	if baseDomain == "" {
		klog.V(100).Info("The baseDomain of the clusterdeployment is empty")

		builder.errorMsg = "clusterdeployment 'baseDomain' cannot be empty"

		return builder
	}

	if clusterInstallRef.Name == "" {
		klog.V(100).Info("The clusterInstallRef name of the clusterdeployment is empty")

		builder.errorMsg = "clusterdeployment 'clusterInstallRef.name' cannot be empty"

		return builder
	}

	return builder
}

// WithAdditionalAgentSelectorLabels inserts additional labels
// into the clusterdeployment label selector.
func (builder *ClusterDeploymentBuilder) WithAdditionalAgentSelectorLabels(
	agentSelector map[string]string) *ClusterDeploymentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Adding agentSelectors %s to clusterdeployment %s in namespace %s",
		agentSelector, builder.Definition.Name, builder.Definition.Namespace)

	if builder.Definition.Spec.Platform.AgentBareMetal == nil {
		klog.V(100).Info("The clusterdeployment platform is not agentBareMetal")

		builder.errorMsg = "clusterdeployment type must be AgentBareMetal to use agentSelector"

		return builder
	}

	if len(agentSelector) == 0 {
		klog.V(100).Info("The clusterdeployment agentSelector is empty")

		builder.errorMsg = "agentSelector cannot be empty"

		return builder
	}

	if len(builder.Definition.Spec.Platform.AgentBareMetal.AgentSelector.MatchLabels) == 0 {
		builder.Definition.Spec.Platform.AgentBareMetal.AgentSelector.MatchLabels = agentSelector
	} else {
		for k, v := range agentSelector {
			builder.Definition.Spec.Platform.AgentBareMetal.AgentSelector.MatchLabels[k] = v
		}
	}

	return builder
}

// WithPullSecret adds a pull-secret reference to the clusterdeployment.
func (builder *ClusterDeploymentBuilder) WithPullSecret(psName string) *ClusterDeploymentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Adding pull-secret ref %s to clusterdeployment %s in namespace %s",
		psName, builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.PullSecretRef = &corev1.LocalObjectReference{Name: psName}

	return builder
}

// Get fetches the defined clusterdeployment from the cluster.
func (builder *ClusterDeploymentBuilder) Get() (*hiveV1.ClusterDeployment, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting clusterdeployment %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	clusterDeployment := &hiveV1.ClusterDeployment{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, clusterDeployment)
	if err != nil {
		return nil, err
	}

	return clusterDeployment, err
}

// PullClusterDeployment pulls existing clusterdeployment from cluster.
func PullClusterDeployment(apiClient *clients.Settings, name, nsname string) (*ClusterDeploymentBuilder, error) {
	klog.V(100).Infof("Pulling existing clusterdeployment name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(hiveV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add hive v1 scheme to client schemes")

		return nil, err
	}

	builder := &ClusterDeploymentBuilder{
		apiClient: apiClient.Client,
		Definition: &hiveV1.ClusterDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the clusterdeployment is empty")

		return nil, fmt.Errorf("clusterdeployment 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the clusterdeployment is empty")

		return nil, fmt.Errorf("clusterdeployment 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("clusterdeployment object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create generates a clusterdeployment on the cluster.
func (builder *ClusterDeploymentBuilder) Create() (*ClusterDeploymentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the clusterdeployment %s in namespace %s",
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

// WithOptions creates ClusterDeployment with generic mutation options.
func (builder *ClusterDeploymentBuilder) WithOptions(
	options ...ClusterDeploymentAdditionalOptions) *ClusterDeploymentBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting ClusterDeployment additional options")

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

// Update modifies an existing clusterdeployment on the cluster.
func (builder *ClusterDeploymentBuilder) Update(force bool) (*ClusterDeploymentBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating clusterdeployment %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("clusterdeployment", builder.Definition.Name, builder.Definition.Namespace))

			err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("clusterdeployment", builder.Definition.Name, builder.Definition.Namespace))

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

// Delete removes a clusterdeployment from the cluster.
func (builder *ClusterDeploymentBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the clusterdeployment %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("clusterdeployment %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete clusterdeployment: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks if the defined clusterdeployment has already been created.
func (builder *ClusterDeploymentBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if clusterdeployment %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ClusterDeploymentBuilder) validate() (bool, error) {
	resourceCRD := "ClusterDeployment"

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
