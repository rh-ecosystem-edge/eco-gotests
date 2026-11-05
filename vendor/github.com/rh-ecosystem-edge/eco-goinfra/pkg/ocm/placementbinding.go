package ocm

import (
	"context"
	"errors"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

const (
	appsOpenClusterManagementIo   = "apps.open-cluster-management.io"
	kindPlacementRule             = "PlacementRule"
	policyOpenClusterManagementIo = "policy.open-cluster-management.io"
	kindPolicySet                 = "PolicySet"
	errEmptySubjectName           = "placementBinding's 'Subject.Name' cannot be empty"
)

var placementBindingGVK = policiesv1.GroupVersion.WithKind("PlacementBinding")

// PlacementBindingBuilder type definition.
type PlacementBindingBuilder struct {
	common.EmbeddableBuilder[policiesv1.PlacementBinding, *policiesv1.PlacementBinding]
	common.EmbeddableCreator[
		policiesv1.PlacementBinding, PlacementBindingBuilder, *policiesv1.PlacementBinding, *PlacementBindingBuilder]
	common.EmbeddableDeleteReturner[
		policiesv1.PlacementBinding, PlacementBindingBuilder, *policiesv1.PlacementBinding, *PlacementBindingBuilder]
	common.EmbeddableForceUpdater[
		policiesv1.PlacementBinding, PlacementBindingBuilder, *policiesv1.PlacementBinding, *PlacementBindingBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *PlacementBindingBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the PlacementBinding GVK for this builder.
func (builder *PlacementBindingBuilder) GetGVK() schema.GroupVersionKind {
	return placementBindingGVK
}

// NewPlacementBindingBuilder creates a new instance of PlacementBindingBuilder.
func NewPlacementBindingBuilder(
	apiClient *clients.Settings,
	name,
	nsname string,
	placementRef policiesv1.PlacementSubject,
	subject policiesv1.Subject) *PlacementBindingBuilder {
	builder := common.NewNamespacedBuilder[policiesv1.PlacementBinding, PlacementBindingBuilder](
		apiClient, policiesv1.AddToScheme, name, nsname)
	if builder.GetError() != nil {
		return builder
	}

	if placementRefErr := validatePlacementRef(placementRef); placementRefErr != "" {
		builder.SetError(errors.New(placementRefErr))

		return builder
	}

	if subjectErr := validateSubject(subject); subjectErr != "" {
		builder.SetError(errors.New(subjectErr))

		return builder
	}

	builder.Definition.PlacementRef = placementRef
	builder.Definition.Subjects = []policiesv1.Subject{subject}

	return builder
}

// PullPlacementBinding pulls existing placementBinding into Builder struct.
func PullPlacementBinding(apiClient *clients.Settings, name, nsname string) (*PlacementBindingBuilder, error) {
	return common.PullNamespacedBuilder[policiesv1.PlacementBinding, PlacementBindingBuilder](
		context.TODO(), apiClient, policiesv1.AddToScheme, name, nsname)
}

// WithAdditionalSubject appends a subject to the subjects list in the PlacementBinding definition.
func (builder *PlacementBindingBuilder) WithAdditionalSubject(subject policiesv1.Subject) *PlacementBindingBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Adding Subject %s to PlacementBinding %s", subject.Name, builder.Definition.Name)

	if err := validateSubject(subject); err != "" {
		builder.SetError(errors.New(err))

		return builder
	}

	builder.Definition.Subjects = append(builder.Definition.Subjects, subject)

	return builder
}

// validatePlacementRef validates all the fields of the PlacementRef and returns an error message based on the
// validation. The message will be empty for valid placement refs.
func validatePlacementRef(placementRef policiesv1.PlacementSubject) string {
	apiGroup := placementRef.APIGroup
	if apiGroup != appsOpenClusterManagementIo && apiGroup != "cluster.open-cluster-management.io" {
		klog.V(100).Info("The APIGroup of the PlacementRef of the PlacementBinding is invalid")

		return "placementBinding's 'PlacementRef.APIGroup' must be a valid option"
	}

	kind := placementRef.Kind
	if kind != kindPlacementRule && kind != "Placement" {
		klog.V(100).Info("The Kind of the PlacementRef of the PlacementBinding is invalid")

		return "placementBinding's 'PlacementRef.Kind' must be a valid option"
	}

	if placementRef.Name == "" {
		klog.V(100).Info("The Name of the PlacementRef of the PlacementBinding is empty")

		return "placementBinding's 'PlacementRef.Name' cannot be empty"
	}

	return ""
}

// validateSubject validates the fields of the Subject and returns an error message based on the validation. The message
// will be empty for valid subjects.
func validateSubject(subject policiesv1.Subject) string {
	if subject.APIGroup != policyOpenClusterManagementIo {
		klog.V(100).Info("The APIGroup of the PlacementBinding subject is invalid")

		return "placementBinding's 'Subject.APIGroup' must be 'policy.open-cluster-management.io'"
	}

	if subject.Kind != "Policy" && subject.Kind != kindPolicySet {
		klog.V(100).Info("The Kind of the subject of the PlacementBinding is invalid")

		return "placementBinding's 'Subject.Kind' must be a valid option"
	}

	if subject.Name == "" {
		klog.V(100).Info("The Name of the subject of the PlacementBinding is empty")

		return errEmptySubjectName
	}

	return ""
}
