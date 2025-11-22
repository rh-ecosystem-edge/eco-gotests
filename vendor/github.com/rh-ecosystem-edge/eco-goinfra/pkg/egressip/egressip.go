package egressip

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"k8s.io/klog/v2"

	egressipv1 "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/crd/egressip/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EgressIPBuilder provides a struct for EgressIP object.
type EgressIPBuilder struct {
	// EgressIP definition, used to create the EgressIP object.
	Definition *egressipv1.EgressIP
	// Created EgressIP object.
	Object *egressipv1.EgressIP
	// Used to store latest error message upon defining or mutating EgressIP definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewEgressIPBuilder creates a new instance of EgressIP builder.
func NewEgressIPBuilder(apiClient *clients.Settings, name string) *EgressIPBuilder {
	klog.V(100).Infof(
		"Initializing new EgressIP structure with the following params: name: %s", name)

	if apiClient == nil {
		klog.V(100).Info("EgressIP 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(egressipv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add 'egressip' scheme to client schemes")

		return nil
	}

	builder := &EgressIPBuilder{
		apiClient: apiClient.Client,
		Definition: &egressipv1.EgressIP{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name parameter of the EgressIP is empty")

		builder.errorMsg = "the name parameter of the EgressIP is empty"

		return builder
	}

	return builder
}

// WithEgressIPs applies egressIPs to the EgressIP spec definition.
func (builder *EgressIPBuilder) WithEgressIPs(egressIPs []string) *EgressIPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Applying egressIPs %s to EgressIP %q", egressIPs, builder.Definition.Name)

	if len(egressIPs) == 0 {
		klog.V(100).Info("The egressIP is empty")

		builder.errorMsg = "cannot accept empty list as egressIPs value"

		return builder
	}

	builder.Definition.Spec.EgressIPs = egressIPs

	return builder
}

// WithNamespaceSelector applies namespaceSelector to the EgressIP spec definition.
func (builder *EgressIPBuilder) WithNamespaceSelector(namespaceSelector metav1.LabelSelector) *EgressIPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Applying namespaceSelector %s to EgressIP %q",
		namespaceSelector, builder.Definition.Name)

	if len(namespaceSelector.MatchLabels) == 0 && len(namespaceSelector.MatchExpressions) == 0 {
		klog.V(100).Info("There are no labels for the namespaceSelector")

		builder.errorMsg = "EgressIP 'namespaceSelector' cannot be empty"

		return builder
	}

	builder.Definition.Spec.NamespaceSelector = namespaceSelector

	return builder
}

// WithPodSelector applies podSelector to the EgressIP spec definition.
func (builder *EgressIPBuilder) WithPodSelector(podSelector metav1.LabelSelector) *EgressIPBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Applying podSelector %s to EgressIP %q", podSelector, builder.Definition.Name)

	if len(podSelector.MatchLabels) == 0 && len(podSelector.MatchExpressions) == 0 {
		klog.V(100).Info("There are no labels for the podSelector")

		builder.errorMsg = "EgressIP 'podSelector' cannot be empty"

		return builder
	}

	builder.Definition.Spec.PodSelector = podSelector

	return builder
}

// Pull fetches existing egressIP from the cluster.
func Pull(apiClient *clients.Settings, name string) (*EgressIPBuilder, error) {
	klog.V(100).Infof("Pulling existing egressIP %q from cluster", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("egressIP's 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(egressipv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add egressIP scheme to client schemes")

		return nil, err
	}

	if name == "" {
		klog.V(100).Info("EgressIP's name cannot be empty")

		return nil, fmt.Errorf("egressIP's name cannot be empty")
	}

	builder := &EgressIPBuilder{
		apiClient: apiClient.Client,
		Definition: &egressipv1.EgressIP{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("egressIP object %q does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given egressIP exists.
func (builder *EgressIPBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if egressIP %q exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get fetches the egressIP from the cluster.
func (builder *EgressIPBuilder) Get() (*egressipv1.EgressIP, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting egressIP %q", builder.Definition.Name)

	egrIP := &egressipv1.EgressIP{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, egrIP)
	if err != nil {
		klog.V(100).Infof("Error retrieving egressIP: %v", err)

		return nil, err
	}

	return egrIP, err
}

// Create makes a egressIP in the cluster and stores the created object in struct.
func (builder *EgressIPBuilder) Create() (*EgressIPBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the egressIP %q", builder.Definition.Name)

	var err error

	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			klog.V(100).Infof("Created egressIP %q", builder.Definition.Name)

			builder.Object = builder.Definition

			return builder, nil
		}
	}

	return builder, err
}

// Delete removes egressIP from a cluster.
func (builder *EgressIPBuilder) Delete() (*EgressIPBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting egressIP %q", builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("egressIP %q does not exist", builder.Definition.Name)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Error deleting egressIP: %v", err)

		return builder, fmt.Errorf("failed to delete egressIP due to %w", err)
	}

	klog.V(100).Infof("Deleted egressIP %q", builder.Definition.Name)

	builder.Object = nil

	return builder, nil
}

// Update updates egressIP object on cluster with content in the builder.
func (builder *EgressIPBuilder) Update() (*EgressIPBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating egressIP %s", builder.Definition.Name)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Error updating egressIP: %v", err)

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, err
}

// GetAssignedEgressIPMap fetches the next recommended or conditional update for the cluster.
func (builder *EgressIPBuilder) GetAssignedEgressIPMap() (map[string]string, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Pull assigned egressIPs map for egressIP %q", builder.Definition.Name)

	if !builder.Exists() {
		return nil, fmt.Errorf("egressIP %q object does not exist", builder.Definition.Name)
	}

	if len(builder.Object.Status.Items) == 0 {
		return nil, fmt.Errorf("egressIP %q nodes assignment does not exist", builder.Definition.Name)
	}

	egressIPMap := make(map[string]string)
	for _, item := range builder.Object.Status.Items {
		egressIPMap[item.Node] = item.EgressIP
	}

	return egressIPMap, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *EgressIPBuilder) validate() (bool, error) {
	resourceCRD := "egressIP"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		builder.errorMsg = msg.UndefinedCrdObjectErrString(resourceCRD)
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		builder.errorMsg = fmt.Sprintf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
