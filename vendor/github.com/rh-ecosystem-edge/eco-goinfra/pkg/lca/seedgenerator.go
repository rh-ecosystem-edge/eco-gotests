package lca

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	lcasgv1 "github.com/openshift-kni/lifecycle-agent/api/seedgenerator/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	seedImageName = "seedimage"
)

// SeedGeneratorBuilder provides struct for the seedgenerator object containing connection to
// the cluster and the seedgenerator definitions.
type SeedGeneratorBuilder struct {
	// SeedGenerator definition. Used to store the seedgenerator object.
	Definition *lcasgv1.SeedGenerator
	// Created seedgenerator object.
	Object *lcasgv1.SeedGenerator
	// Used in functions that define or mutate the seedgenerator definition.
	// errorMsg is processed before the seedgenerator object is created
	errorMsg  string
	apiClient goclient.Client
}

// SeedGeneratorAdditionalOptions additional options for imagebasedupgrade object.
type SeedGeneratorAdditionalOptions func(builder *SeedGeneratorBuilder) (*SeedGeneratorBuilder, error)

// NewSeedGeneratorBuilder creates a new instance of SeedGenerator.
func NewSeedGeneratorBuilder(
	apiClient *clients.Settings,
	name string,
) *SeedGeneratorBuilder {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(lcasgv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add lcasg v1 scheme to client schemes")

		return nil
	}

	builder := &SeedGeneratorBuilder{
		apiClient: apiClient.Client,
		Definition: &lcasgv1.SeedGenerator{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name != seedImageName {
		glog.V(100).Infof("The name of the seedgenerator must be " + seedImageName)

		builder.errorMsg = "SeedGenerator name must be " + seedImageName

		return builder
	}

	return builder
}

// WithOptions creates seedgenerator with generic mutation options.
func (builder *SeedGeneratorBuilder) WithOptions(options ...SeedGeneratorAdditionalOptions) *SeedGeneratorBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting seedgenerator additional options")

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

// Create makes a seedgenerator in the cluster and stores the created object in struct.
func (builder *SeedGeneratorBuilder) Create() (*SeedGeneratorBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the seedgenerator %s",
		builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// PullSeedGenerator pulls existing seedgenerator from cluster.
func PullSeedGenerator(apiClient *clients.Settings, name string) (*SeedGeneratorBuilder, error) {
	glog.V(100).Infof("Pulling existing seedgenerator name %s from cluster", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	err := apiClient.AttachScheme(lcasgv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add lcasg v1 scheme to client schemes")

		return nil, err
	}

	builder := &SeedGeneratorBuilder{
		apiClient: apiClient.Client,
		Definition: &lcasgv1.SeedGenerator{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the seedgenerator is empty")

		return nil, fmt.Errorf("seedgenerator 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("seedgenerator object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Delete removes the existing seedgenerator from a cluster.
func (builder *SeedGeneratorBuilder) Delete() (*SeedGeneratorBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the seedgenerator %s",
		builder.Definition.Name)

	if !builder.Exists() {
		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete seedgenerator: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Get returns seedgenerator object if found.
func (builder *SeedGeneratorBuilder) Get() (*lcasgv1.SeedGenerator, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting seedgenerator %s",
		builder.Definition.Name)

	seedgenerator := &lcasgv1.SeedGenerator{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, seedgenerator)
	if err != nil {
		return nil, err
	}

	return seedgenerator, nil
}

// Exists checks whether the given seedgenerator exists.
func (builder *SeedGeneratorBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if seedgenerator %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithSeedImage sets the seed image used by the seedgenerator.
func (builder *SeedGeneratorBuilder) WithSeedImage(
	seedImage string) *SeedGeneratorBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if seedImage == "" {
		glog.V(100).Infof("The name of the seedImage is empty")

		builder.errorMsg = "seedImage 'name' cannot be empty"

		return builder
	}

	glog.V(100).Infof("Setting seed image %s in seedgenerator", seedImage)

	builder.Definition.Spec.SeedImage = seedImage

	return builder
}

// WithRecertImage sets the recert image used by the seedgenerator.
func (builder *SeedGeneratorBuilder) WithRecertImage(
	recertImage string) *SeedGeneratorBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if recertImage == "" {
		glog.V(100).Infof("The name of the recertImage is empty")

		builder.errorMsg = "recertImage 'name' cannot be empty"

		return builder
	}

	glog.V(100).Infof("Setting recert image %s in seedgenerator", recertImage)

	builder.Definition.Spec.RecertImage = recertImage

	return builder
}

// WaitUntilComplete waits the specified timeout for the seedgenerator to complete
// actions.
func (builder *SeedGeneratorBuilder) WaitUntilComplete(timeout time.Duration) (*SeedGeneratorBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Waiting for seedgenerator %s to complete actions",
		builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("The seedgenerator does not exist on the cluster")

		return builder, fmt.Errorf("%s", builder.errorMsg)
	}

	// Polls periodically to determine if seedgenerator is in desired state.
	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second*3, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			for _, condition := range builder.Object.Status.Conditions {
				if condition.Status == "True" && condition.Type == "SeedGenCompleted" &&
					condition.Reason == "Completed" {
					return true, nil
				}
			}

			return false, nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *SeedGeneratorBuilder) validate() (bool, error) {
	resourceCRD := "SeedGenerator"

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
