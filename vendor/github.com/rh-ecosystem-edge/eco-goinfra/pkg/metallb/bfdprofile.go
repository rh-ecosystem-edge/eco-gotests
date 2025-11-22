package metallb

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BFDBuilder provides struct for the BFDProfile object containing connection to
// the cluster and the BFDProfile definitions.
type BFDBuilder struct {
	Definition *mlbtypes.BFDProfile
	Object     *mlbtypes.BFDProfile
	apiClient  runtimeClient.Client
	errorMsg   string
}

// BFDAdditionalOptions additional options for BFDProfile object.
type BFDAdditionalOptions func(builder *BFDBuilder) (*BFDBuilder, error)

// NewBFDBuilder creates a new instance of BFDBuilder.
func NewBFDBuilder(apiClient *clients.Settings, name, nsname string) *BFDBuilder {
	klog.V(100).Infof(
		"Initializing new BFDBuilder structure with the following params: %s, %s",
		name, nsname)

	err := apiClient.AttachScheme(mlbtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add metallb scheme to client schemes")

		return nil
	}

	builder := &BFDBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypes.BFDProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the BFDProfile is empty")

		builder.errorMsg = "BFDProfile 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the BFDProfile is empty")

		builder.errorMsg = "BFDProfile 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Get returns BFDProfile object if found.
func (builder *BFDBuilder) Get() (*mlbtypes.BFDProfile, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting BFDProfile object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	bfdProfile := &mlbtypes.BFDProfile{}

	err := builder.apiClient.Get(logging.DiscardContext(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		bfdProfile)
	if err != nil {
		klog.V(100).Infof(
			"BFDProfile object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return bfdProfile, nil
}

// Exists checks whether the given BFDProfile exists.
func (builder *BFDBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if BFDProfile %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// PullBFDProfile pulls existing bfdprofile from cluster.
func PullBFDProfile(apiClient *clients.Settings, name, nsname string) (*BFDBuilder, error) {
	klog.V(100).Infof("Pulling existing bfdprofile name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("bfdprofile 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(mlbtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add metallb scheme to client schemes")

		return nil, err
	}

	builder := &BFDBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypes.BFDProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the bfdprofile is empty")

		return nil, fmt.Errorf("bfdprofile 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the bfdprofile is empty")

		return nil, fmt.Errorf("bfdprofile 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("bfdprofile object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes a BFDProfile in the cluster and stores the created object in struct.
func (builder *BFDBuilder) Create() (*BFDBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the BFDProfile %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err != nil {
			klog.V(100).Info("Failed to create BFDProfile")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, err
}

// Delete removes BFDProfile object from a cluster.
func (builder *BFDBuilder) Delete() (*BFDBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the BFDProfile object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof("BFDProfile %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete BFDProfile: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing BFDProfile object with the BFDProfile definition in builder.
func (builder *BFDBuilder) Update(force bool) (*BFDBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the BFDProfile object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		return nil, fmt.Errorf("failed to update BFDProfile, object does not exist on cluster")
	}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("BFDProfile", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("BFDProfile", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, err
}

// WithRcvInterval defines the receiveInterval placed in the BFDProfile.
func (builder *BFDBuilder) WithRcvInterval(rcvInterval uint32) *BFDBuilder {
	return builder.withInterval("receiveInterval", rcvInterval)
}

// WithTransmitInterval defines the transmitInterval placed in the BFDProfile.
func (builder *BFDBuilder) WithTransmitInterval(transmitInterval uint32) *BFDBuilder {
	return builder.withInterval("transmitInterval", transmitInterval)
}

// WithEchoInterval defines the ecoInterval placed in the BFDProfile.
func (builder *BFDBuilder) WithEchoInterval(ecoInterval uint32) *BFDBuilder {
	return builder.withInterval("ecoInterval", ecoInterval)
}

// WithMultiplier defines the detectMultiplier placed in the BFDProfile.
func (builder *BFDBuilder) WithMultiplier(multiplier uint32) *BFDBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BFDProfile %s in namespace %s with this detectMultiplier: %d",
		builder.Definition.Name, builder.Definition.Namespace, multiplier)

	builder.Definition.Spec.DetectMultiplier = &multiplier

	return builder
}

// WithEchoMode defines the echoMode placed in the BFDProfile.
func (builder *BFDBuilder) WithEchoMode(echoMode bool) *BFDBuilder {
	return builder.withBoolFlagFor("echoMode", echoMode)
}

// WithPassiveMode defines the passiveMode placed in the BFDProfile.
func (builder *BFDBuilder) WithPassiveMode(passiveMode bool) *BFDBuilder {
	return builder.withBoolFlagFor("passiveMode", passiveMode)
}

// WithMinimumTTL defines the minimumTTTL placed in the BFDProfile.
func (builder *BFDBuilder) WithMinimumTTL(minimumTTL uint32) *BFDBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BFDProfile %s in namespace %s with this minimumTTL: %d",
		builder.Definition.Name, builder.Definition.Namespace, minimumTTL)

	builder.Definition.Spec.MinimumTTL = &minimumTTL

	return builder
}

// WithOptions creates BFDProfile with generic mutation options.
func (builder *BFDBuilder) WithOptions(options ...BFDAdditionalOptions) *BFDBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting BFDProfile additional options")

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

// GetBFDProfileGVR returns bfdprofile's GroupVersionResource which could be used for Clean function.
func GetBFDProfileGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: "bfdprofiles",
	}
}

func (builder *BFDBuilder) withBoolFlagFor(flagName string, flagValue bool) *BFDBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BFDProfile %s in namespace %s with flag %s: %t",
		builder.Definition.Name, builder.Definition.Namespace, flagName, flagValue)

	switch flagName {
	case "echoMode":
		builder.Definition.Spec.EchoMode = &flagValue
	case "passiveMode":
		builder.Definition.Spec.PassiveMode = &flagValue
	default:
		builder.errorMsg = "invalid bool flag name parameter"

		return builder
	}

	return builder
}

func (builder *BFDBuilder) withInterval(intervalName string, interval uint32) *BFDBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BFDProfile %s in namespace %s with interval %s: %d",
		builder.Definition.Name, builder.Definition.Namespace, intervalName, interval)

	switch intervalName {
	case "transmitInterval":
		builder.Definition.Spec.TransmitInterval = &interval
	case "receiveInterval":
		builder.Definition.Spec.ReceiveInterval = &interval
	case "ecoInterval":
		builder.Definition.Spec.EchoInterval = &interval
	default:
		builder.errorMsg = "invalid interval parameters"

		return builder
	}

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *BFDBuilder) validate() (bool, error) {
	resourceCRD := "BFDProfile"

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
