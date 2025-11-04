package clusterlogging

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	observabilityv1 "github.com/openshift/cluster-logging-operator/api/observability/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterLogForwarderBuilder provides a struct for clusterlogforwarder object from the
// cluster and a clusterlogforwarder definition.
type ClusterLogForwarderBuilder struct {
	// clusterlogforwarder definition, used to create the clusterlogforwarder object.
	Definition *observabilityv1.ClusterLogForwarder
	// Created clusterlogforwarder object.
	Object *observabilityv1.ClusterLogForwarder
	// api client to interact with the cluster.
	apiClient goclient.Client
	// errorMsg is processed before clusterlogforwarder object is created.
	errorMsg string
}

// NewClusterLogForwarderBuilder method creates new instance of builder.
func NewClusterLogForwarderBuilder(
	apiClient *clients.Settings, name, nsname string) *ClusterLogForwarderBuilder {
	glog.V(100).Infof("Initializing new clusterlogforwarder structure with the following params: "+
		"name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("clusterLogForwarder 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(observabilityv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add observabilityv1 scheme to client schemes")

		return nil
	}

	builder := &ClusterLogForwarderBuilder{
		apiClient: apiClient.Client,
		Definition: &observabilityv1.ClusterLogForwarder{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the clusterlogforwarder is empty")

		builder.errorMsg = "clusterlogforwarder 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the clusterlogforwarder is empty")

		builder.errorMsg = "clusterlogforwarder 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// WithManagementState sets the clusterlogforwarder operator's managementState configuration.
func (builder *ClusterLogForwarderBuilder) WithManagementState(
	managementState observabilityv1.ManagementState) *ClusterLogForwarderBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if managementState != observabilityv1.ManagementStateManaged &&
		managementState != observabilityv1.ManagementStateUnmanaged {
		glog.V(100).Infof("The management state of the clusterlogforwarder is unsupported: %s;"+
			"accepted only %s or %s",
			managementState, observabilityv1.ManagementStateManaged, observabilityv1.ManagementStateUnmanaged)

		builder.errorMsg = fmt.Sprintf("the management state of the clusterlogforwarder is unsupported: \"%s\";"+
			"accepted only %s or %s states",
			managementState, observabilityv1.ManagementStateManaged, observabilityv1.ManagementStateUnmanaged)

		return builder
	}

	glog.V(100).Infof(
		"Setting clusterlogforwarder %s in namespace %s with the managementState config: %v",
		builder.Definition.Name, builder.Definition.Namespace, managementState)

	builder.Definition.Spec.ManagementState = managementState

	return builder
}

// WithServiceAccount sets the clusterlogforwarder operator's serviceAccount configuration.
func (builder *ClusterLogForwarderBuilder) WithServiceAccount(serviceAccount string) *ClusterLogForwarderBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if serviceAccount == "" {
		glog.V(100).Infof("The serviceAccount of the clusterlogforwarder is empty")

		builder.errorMsg = "clusterlogforwarder 'serviceAccount' cannot be empty"

		return builder
	}

	glog.V(100).Infof(
		"Setting clusterlogforwarder %s in namespace %s with the serviceAccount config: %v",
		builder.Definition.Name, builder.Definition.Namespace, serviceAccount)

	builder.Definition.Spec.ServiceAccount.Name = serviceAccount

	return builder
}

// WithOutput sets the output on the clusterlogforwarder definition.
func (builder *ClusterLogForwarderBuilder) WithOutput(
	outputSpec *observabilityv1.OutputSpec) *ClusterLogForwarderBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting output %v on clusterlogforwarder %s in namespace %s",
		outputSpec, builder.Definition.Name, builder.Definition.Namespace)

	if outputSpec == nil {
		glog.V(100).Infof("The 'outputSpec' of the deployment is empty")

		builder.errorMsg = "'outputSpec' parameter is empty"

		return builder
	}

	if builder.Definition.Spec.Outputs == nil {
		builder.Definition.Spec.Outputs = []observabilityv1.OutputSpec{*outputSpec}
	} else {
		builder.Definition.Spec.Outputs = append(builder.Definition.Spec.Outputs, *outputSpec)
	}

	return builder
}

// WithPipeline sets the pipeline on the clusterlogforwarder definition.
func (builder *ClusterLogForwarderBuilder) WithPipeline(
	pipelineSpec *observabilityv1.PipelineSpec) *ClusterLogForwarderBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting pipeline %v on clusterlogforwarder %s in namespace %s",
		pipelineSpec, builder.Definition.Name, builder.Definition.Namespace)

	if pipelineSpec == nil {
		glog.V(100).Infof("The 'pipelineSpec' of the deployment is empty")

		builder.errorMsg = "'pipelineSpec' parameter is empty"

		return builder
	}

	if builder.Definition.Spec.Pipelines == nil {
		builder.Definition.Spec.Pipelines = []observabilityv1.PipelineSpec{*pipelineSpec}
	} else {
		builder.Definition.Spec.Pipelines = append(builder.Definition.Spec.Pipelines, *pipelineSpec)
	}

	return builder
}

// PullClusterLogForwarder retrieves an existing clusterlogforwarder object from the cluster.
func PullClusterLogForwarder(apiClient *clients.Settings, name, nsname string) (*ClusterLogForwarderBuilder, error) {
	glog.V(100).Infof("Pulling existing clusterlogforwarder %s in nsname %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("clusterlogforwarder 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(observabilityv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add observabilityv1 scheme to client schemes")

		return nil, err
	}

	builder := &ClusterLogForwarderBuilder{
		apiClient: apiClient.Client,
		Definition: &observabilityv1.ClusterLogForwarder{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the clusterlogforwarder is empty")

		return nil, fmt.Errorf("clusterlogforwarder 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The nsname of the clusterlogforwarder is empty")

		return nil, fmt.Errorf("clusterlogforwarder 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("clusterlogforwarder object %s does not exist in namespace %s", name, nsname)
	}

	return builder, nil
}

// Get returns clusterlogforwarder object if found.
func (builder *ClusterLogForwarderBuilder) Get() (*observabilityv1.ClusterLogForwarder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting clusterlogforwarder %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	clusterLogForwarder := &observabilityv1.ClusterLogForwarder{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, clusterLogForwarder)
	if err != nil {
		return nil, err
	}

	return clusterLogForwarder, nil
}

// Create makes a clusterlogforwarder in the cluster and stores the created object in struct.
func (builder *ClusterLogForwarderBuilder) Create() (*ClusterLogForwarderBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the clusterlogforwarder %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes clusterlogforwarder from a cluster.
func (builder *ClusterLogForwarderBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the clusterlogforwarder %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("Clusterlogforwarder %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete clusterlogforwarder: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given clusterlogforwarder exists.
func (builder *ClusterLogForwarderBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if clusterlogforwarder %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing clusterlogforwarder object with clusterlogforwarder definition in builder.
func (builder *ClusterLogForwarderBuilder) Update(force bool) (*ClusterLogForwarderBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Info("Updating clusterlogforwarder %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("clusterlogforwarder", builder.Definition.Name, builder.Definition.Namespace))

			err := builder.Delete()
			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError(
						"clusterlogforwarder", builder.Definition.Name, builder.Definition.Namespace))

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

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ClusterLogForwarderBuilder) validate() (bool, error) {
	resourceCRD := "clusterLogForwarder"

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
		glog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
