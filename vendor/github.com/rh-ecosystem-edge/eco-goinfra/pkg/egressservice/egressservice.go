package egressservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"k8s.io/klog/v2"

	egresssvcv1 "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/crd/egressservice/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EgressServiceBuilder provides a struct for EgressService object.
type EgressServiceBuilder struct {
	// EgressService definition, used to create the EgressService object.
	Definition *egresssvcv1.EgressService
	// Created EgressService object.
	Object *egresssvcv1.EgressService
	// Used to store latest error message upon defining or mutating EgressService definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewEgressServiceBuilder creates a new instance of EgressService builder.
func NewEgressServiceBuilder(
	apiClient *clients.Settings, name, nsname, sourceIPBy string) *EgressServiceBuilder {
	klog.V(100).Infof(
		"Initializing new EgressService structure with the following params: "+
			"name: %s; namespace: %s; sourceIPBy: %s",
		name, nsname, sourceIPBy)

	if apiClient == nil {
		klog.V(100).Info("EgressService 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(egresssvcv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add 'egressservice' scheme to client schemes")

		return nil
	}

	builder := &EgressServiceBuilder{
		apiClient: apiClient.Client,
		Definition: &egresssvcv1.EgressService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: egresssvcv1.EgressServiceSpec{
				SourceIPBy: egresssvcv1.SourceIPMode(sourceIPBy),
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name parameter of the EgressService is empty")

		builder.errorMsg = "the name parameter of the EgressService is empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the EgressService is empty")

		builder.errorMsg = "the namespace of the EgressService is empty"

		return builder
	}

	if _, err := validateSourceIPMode(sourceIPBy); err != nil {
		klog.V(100).Info("Invalid sourceIPBy parameter for the EgressService")

		builder.errorMsg = "invalid sourceIPBy parameter for the EgressService"

		return builder
	}

	return builder
}

// WithNodeLabelSelector applies nodeSelector to the EgressService definition,
// which uses key:value pairs for nodes matching.
func (builder *EgressServiceBuilder) WithNodeLabelSelector(selector map[string]string) *EgressServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Applying nodeSelector %s to EgressService %q in namespace %q",
		selector, builder.Definition.Name, builder.Definition.Namespace)

	if len(selector) == 0 {
		klog.V(100).Info("The nodeselector is empty")

		builder.errorMsg = "cannot accept empty map as nodeSelector"

		return builder
	}

	builder.Definition.Spec.NodeSelector = metav1.LabelSelector{MatchLabels: selector}

	return builder
}

// WithVRFNetwork sets the network to be used for sending egress and corresponding ingress replies to.
func (builder *EgressServiceBuilder) WithVRFNetwork(vrfnet string) *EgressServiceBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting VRF network %q to EgressService %q in namespace %q",
		vrfnet, builder.Definition.Name, builder.Definition.Namespace)

	if strings.TrimSpace(vrfnet) == "" {
		klog.V(100).Info("Cannot use empty VRF network")

		builder.errorMsg = "cannot use empty VRF network"

		return builder
	}

	builder.Definition.Spec.Network = vrfnet

	return builder
}

// Pull fetches existing EgressService from the cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*EgressServiceBuilder, error) {
	klog.V(100).Infof("Pulling existing EgressService %q in namespace %q from cluster",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("egressService's 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(egresssvcv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add EgressService scheme to client schemes")

		return nil, err
	}

	if name == "" {
		klog.V(100).Info("EgressService's name cannot be empty")

		return nil, fmt.Errorf("egressService's name cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("EgressService's namespace cannot be empty")

		return nil, fmt.Errorf("egressService's namespace cannot be empty")
	}

	builder := &EgressServiceBuilder{
		apiClient: apiClient.Client,
		Definition: &egresssvcv1.EgressService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("egressService object %q does not exist in namespace %q",
			name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given EgressService exists.
func (builder *EgressServiceBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if EgressService %q exists in namespace %q",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get fetches the EgressService from the cluster.
func (builder *EgressServiceBuilder) Get() (*egresssvcv1.EgressService, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting EgressService %q in namespace %q",
		builder.Definition.Name, builder.Definition.Namespace)

	egrSvc := &egresssvcv1.EgressService{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, egrSvc)
	if err != nil {
		klog.V(100).Infof("Error retrieving EgressService: %v", err)

		return nil, err
	}

	return egrSvc, err
}

// Create makes a EgressService in the cluster and stores the created object in struct.
func (builder *EgressServiceBuilder) Create() (*EgressServiceBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the EgressServcice %q in namespace %q",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			klog.V(100).Infof("Created EgressServcice %q in namespace %q",
				builder.Definition.Name, builder.Definition.Namespace)

			builder.Object = builder.Definition

			return builder, nil
		}
	}

	return builder, err
}

// Delete removes EgressService from a cluster.
func (builder *EgressServiceBuilder) Delete() (*EgressServiceBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting EgressService %q in namespace %q",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("EgressService %q in namespace %q does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Error deleting EgressService: %v", err)

		return builder, fmt.Errorf("failed to delete EgressService due to %w", err)
	}

	klog.V(100).Infof("Deleted EgressService %q in %q namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Object = nil

	return builder, nil
}

// Update update EgressService object on cluster with content in the builder.
func (builder *EgressServiceBuilder) Update() (*EgressServiceBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating EgressService %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Error updating EgressService: %v", err)

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *EgressServiceBuilder) validate() (bool, error) {
	resourceCRD := "egressService"

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

func validateSourceIPMode(sourceIPMode string) (string, error) {
	klog.V(100).Infof("Validating requested sourceIPMode %q", sourceIPMode)

	switch {
	case strings.EqualFold(strings.TrimSpace(sourceIPMode), ""):
		klog.V(100).Infof("Invalid sourceIPMode %q(empty)", sourceIPMode)

		return "", fmt.Errorf("invalid empty sourceIPMode %q", sourceIPMode)
	case strings.EqualFold(strings.TrimSpace(sourceIPMode), string(egresssvcv1.SourceIPLoadBalancer)):
		klog.V(100).Infof("Valid sourceIPMode %q", sourceIPMode)

		return string(egresssvcv1.SourceIPLoadBalancer), nil
	case strings.EqualFold(strings.TrimSpace(sourceIPMode), string(egresssvcv1.SourceIPNetwork)):
		klog.V(100).Infof("Valid sourceIPMode %q", sourceIPMode)

		return string(egresssvcv1.SourceIPNetwork), nil
	default:
		klog.V(100).Infof("Invalid sourceIPMode %q", sourceIPMode)

		return "", fmt.Errorf("invalid sourceIPMode %s", sourceIPMode)
	}
}
