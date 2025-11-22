package ocm

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PlacementBindingBuilder type definition.
type PlacementBindingBuilder struct {
	// placementBinding Definition, used to create the placementBinding object.
	Definition *policiesv1.PlacementBinding
	// created placementBinding object.
	Object *policiesv1.PlacementBinding
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// used to store latest error message upon defining or mutating placementBinding definition.
	errorMsg string
}

// NewPlacementBindingBuilder creates a new instance of PlacementBindingBuilder.
func NewPlacementBindingBuilder(
	apiClient *clients.Settings,
	name,
	nsname string,
	placementRef policiesv1.PlacementSubject,
	subject policiesv1.Subject) *PlacementBindingBuilder {
	klog.V(100).Infof(
		"Initializing new placement binding structure with the following params: name: %s, nsname: %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the PlacementBinding is nil")

		return nil
	}

	err := apiClient.AttachScheme(policiesv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add PlacementBinding scheme to client schemes")

		return nil
	}

	builder := &PlacementBindingBuilder{
		apiClient: apiClient.Client,
		Definition: &policiesv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			PlacementRef: placementRef,
			Subjects:     []policiesv1.Subject{subject},
		},
	}

	if name == "" {
		builder.errorMsg = "placementBinding's 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		builder.errorMsg = "placementBinding's 'nsname' cannot be empty"

		return builder
	}

	if placementRefErr := validatePlacementRef(placementRef); placementRefErr != "" {
		builder.errorMsg = placementRefErr

		return builder
	}

	if subjectErr := validateSubject(subject); subjectErr != "" {
		builder.errorMsg = subjectErr

		return builder
	}

	return builder
}

// PullPlacementBinding pulls existing placementBinding into Builder struct.
func PullPlacementBinding(apiClient *clients.Settings, name, nsname string) (*PlacementBindingBuilder, error) {
	klog.V(100).Infof("Pulling existing placementBinding name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("placementBinding's 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(policiesv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add PlacementBinding scheme to client schemes")

		return nil, err
	}

	builder := &PlacementBindingBuilder{
		apiClient: apiClient.Client,
		Definition: &policiesv1.PlacementBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the placementBinding is empty")

		return nil, fmt.Errorf("placementBinding's 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the placementBinding is empty")

		return nil, fmt.Errorf("placementBinding's 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("placementBinding object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given placementBinding exists.
func (builder *PlacementBindingBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if placementBinding %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns a placementBinding object if found.
func (builder *PlacementBindingBuilder) Get() (*policiesv1.PlacementBinding, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting placementBinding %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	placementBinding := &policiesv1.PlacementBinding{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, placementBinding)
	if err != nil {
		klog.V(100).Infof("Failed to get placementBinding %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return placementBinding, err
}

// Create makes a placementBinding in the cluster and stores the created object in struct.
func (builder *PlacementBindingBuilder) Create() (*PlacementBindingBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the placementBinding %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes a placementBinding from a cluster.
func (builder *PlacementBindingBuilder) Delete() (*PlacementBindingBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the placementBinding %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("placementBinding %s cannot be deleted because it does not exist",
			builder.Definition.Name)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete placementBinding: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing placementBinding object with the placementBinding definition in builder.
func (builder *PlacementBindingBuilder) Update(force bool) (*PlacementBindingBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		klog.V(100).Infof(
			"PlacementBinding %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent placementBinding")
	}

	klog.V(100).Infof("Updating the placementBinding object: %s in namespace: %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("placementBinding", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("placementBinding", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	builder.Object = builder.Definition

	return builder, err
}

// WithAdditionalSubject appends a subject to the subjects list in the PlacementBinding definition.
func (builder *PlacementBindingBuilder) WithAdditionalSubject(subject policiesv1.Subject) *PlacementBindingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Adding Subject %s to PlacementBinding %s", subject.Name, builder.Definition.Name)

	if err := validateSubject(subject); err != "" {
		builder.errorMsg = err

		return builder
	}

	builder.Definition.Subjects = append(builder.Definition.Subjects, subject)

	return builder
}

// validatePlacementRef validates all the fields of the PlacementRef and returns an errorMsg based on the validation.
// The errorMsg will be empty for valid Subjects.
func validatePlacementRef(placementRef policiesv1.PlacementSubject) string {
	apiGroup := placementRef.APIGroup
	if apiGroup != "apps.open-cluster-management.io" && apiGroup != "cluster.open-cluster-management.io" {
		klog.V(100).Info("The APIGroup of the PlacementRef of the PlacementBinding is invalid")

		return "placementBinding's 'PlacementRef.APIGroup' must be a valid option"
	}

	kind := placementRef.Kind
	if kind != "PlacementRule" && kind != "Placement" {
		klog.V(100).Info("The Kind of the PlacementRef of the PlacementBinding is invalid")

		return "placementBinding's 'PlacementRef.Kind' must be a valid option"
	}

	if placementRef.Name == "" {
		klog.V(100).Info("The Name of the PlacementRef of the PlacementBinding is empty")

		return "placementBinding's 'PlacementRef.Name' cannot be empty"
	}

	return ""
}

// validateSubject validates the fields of the Subject and returns an errorMsg based on the validation. The errorMsg
// will be empty for valid Subjects.
func validateSubject(subject policiesv1.Subject) string {
	if subject.APIGroup != "policy.open-cluster-management.io" {
		klog.V(100).Info("The APIGroup of the PlacementBinding subject is invalid")

		return "placementBinding's 'Subject.APIGroup' must be 'policy.open-cluster-management.io'"
	}

	if subject.Kind != "Policy" && subject.Kind != "PolicySet" {
		klog.V(100).Info("The Kind of the subject of the PlacementBinding is invalid")

		return "placementBinding's 'Subject.Kind' must be a valid option"
	}

	if subject.Name == "" {
		klog.V(100).Info("The Name of the subject of the PlacementBinding is empty")

		return "placementBinding's 'Subject.Name' cannot be empty"
	}

	return ""
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PlacementBindingBuilder) validate() (bool, error) {
	resourceCRD := "PlacementBinding"

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
