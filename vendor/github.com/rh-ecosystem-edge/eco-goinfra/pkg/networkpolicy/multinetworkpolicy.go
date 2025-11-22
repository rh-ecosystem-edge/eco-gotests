package networkpolicy

import (
	"context"
	"fmt"

	"github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiNetworkPolicyBuilder provides struct for MultiNetworkPolicy object.
type MultiNetworkPolicyBuilder struct {
	// MultiNetworkPolicy definition. Used to create MultiNetworkPolicy object with minimum set of required elements.
	Definition *v1beta1.MultiNetworkPolicy
	// Created MultiNetworkPolicy object on the cluster.
	Object *v1beta1.MultiNetworkPolicy
	// api client to interact with the cluster.
	apiClient runtimeClient.Client
	// errorMsg is processed before MultiNetworkPolicy object is created.
	errorMsg string
}

// NewMultiNetworkPolicyBuilder method creates new instance of builder.
func NewMultiNetworkPolicyBuilder(apiClient *clients.Settings, name, nsname string) *MultiNetworkPolicyBuilder {
	klog.V(100).Infof(
		"Initializing new MultiNetworkPolicyBuilder structure with the following params: name: %s, namespace: %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(v1beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add multi-networkpolicy v1beta1 scheme to client schemes")

		return nil
	}

	builder := &MultiNetworkPolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &v1beta1.MultiNetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the MultiNetworkPolicy is empty")

		builder.errorMsg = "The MultiNetworkPolicy 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the MultiNetworkPolicy is empty")

		builder.errorMsg = "The MultiNetworkPolicy 'namespace' cannot be empty"

		return builder
	}

	return builder
}

// WithPodSelector adds podSelector to MultiNetworkPolicy.
func (builder *MultiNetworkPolicyBuilder) WithPodSelector(podSelector metav1.LabelSelector) *MultiNetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating MultiNetworkPolicy %s in %s namespace with the podSelector defined: %v",
		builder.Definition.Name, builder.Definition.Namespace, podSelector)

	builder.Definition.Spec.PodSelector = podSelector

	return builder
}

// WithNetwork adds network name to the MultiNetworkPolicy.
func (builder *MultiNetworkPolicyBuilder) WithNetwork(networkName string) *MultiNetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating MultiNetworkPolicy %s in %s namespace with the networkName defined: %v",
		builder.Definition.Name, builder.Definition.Namespace, networkName)

	if networkName == "" {
		klog.V(100).Info("The networkName can not be empty string")

		builder.errorMsg = "The networkName is an empty string"

		return builder
	}

	if builder.Definition.Annotations == nil {
		builder.Definition.Annotations = make(map[string]string)
	}

	builder.Definition.Annotations["k8s.v1.cni.cncf.io/policy-for"] = networkName

	return builder
}

// WithEmptyIngress adds empty ingress rule to the MultiNetworkPolicy. Empty ingress denies all.
func (builder *MultiNetworkPolicyBuilder) WithEmptyIngress() *MultiNetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating MultiNetworkPolicy %s in %s namespace with the empty Ingress rule deny all",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Ingress = []v1beta1.MultiNetworkPolicyIngressRule{}

	return builder
}

// WithIngressRule adds Ingress rule to the MultiNetworkPolicy. Empty rule is allowed and works as allow all traffic.
func (builder *MultiNetworkPolicyBuilder) WithIngressRule(
	ingressRule v1beta1.MultiNetworkPolicyIngressRule) *MultiNetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating multiNetworkPolicy %s in %s namespace with the Ingress rule defined: %v",
		builder.Definition.Name, builder.Definition.Namespace, ingressRule)

	builder.Definition.Spec.Ingress = append(builder.Definition.Spec.Ingress, ingressRule)

	return builder
}

// WithEgressRule adds Egress rule to the MultiNetworkPolicy. Empty rule is allowed and works as allow all traffic.
func (builder *MultiNetworkPolicyBuilder) WithEgressRule(
	egressRule v1beta1.MultiNetworkPolicyEgressRule) *MultiNetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating multiNetworkPolicy %s in %s namespace with the Egress rule defined: %v",
		builder.Definition.Name, builder.Definition.Namespace, egressRule)

	builder.Definition.Spec.Egress = append(builder.Definition.Spec.Egress, egressRule)

	return builder
}

// WithPolicyType adds policyType to the MultiNetworkPolicy.
func (builder *MultiNetworkPolicyBuilder) WithPolicyType(
	policyType v1beta1.MultiPolicyType) *MultiNetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating multiNetworkPolicy %s in %s namespace with the Policy type defined: %v",
		builder.Definition.Name, builder.Definition.Namespace, policyType)

	if policyType == "" {
		klog.V(100).Info("The policy type can not be an empty string")

		builder.errorMsg = "The policy Type is an empty string"

		return builder
	}

	builder.Definition.Spec.PolicyTypes = append(builder.Definition.Spec.PolicyTypes, policyType)

	return builder
}

// PullMultiNetworkPolicy loads an existing MultiNetworkPolicy into the Builder struct.
func PullMultiNetworkPolicy(apiClient *clients.Settings, name, nsname string) (*MultiNetworkPolicyBuilder, error) {
	klog.V(100).Infof("Pulling existing MultiNetworkPolicy name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(v1beta1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add multi-networkpolicy v1beta1 scheme to client schemes")

		return nil, err
	}

	builder := MultiNetworkPolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &v1beta1.MultiNetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the MultiNetworkPolicy is empty")

		return nil, fmt.Errorf("MultiNetworkPolicy 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the MultiNetworkPolicy is empty")

		return nil, fmt.Errorf("MultiNetworkPolicy 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		klog.V(100).Infof(
			"Failed to pull MultiNetworkPolicy object %s from namespace %s. Object does not exist",
			name, nsname)

		return nil, fmt.Errorf("MultiNetworkPolicy object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get returns MultiNetworkPolicy object if found.
func (builder *MultiNetworkPolicyBuilder) Get() (*v1beta1.MultiNetworkPolicy, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting MultiNetworkPolicy object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	network := &v1beta1.MultiNetworkPolicy{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		network)
	if err != nil {
		klog.V(100).Infof(
			"MultiNetworkPolicy object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return network, nil
}

// Create makes a MultiNetworkPolicy in cluster and stores the created object in struct.
func (builder *MultiNetworkPolicyBuilder) Create() (*MultiNetworkPolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the MultiNetworkPolicy %s in %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			klog.V(100).Info("Failed to create MultiNetworkPolicy object")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, err
}

// Exists checks whether the given MultiNetworkPolicy exists.
func (builder *MultiNetworkPolicyBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if MultiNetworkPolicy %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a MultiNetworkPolicy object from a cluster.
func (builder *MultiNetworkPolicyBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the MultiNetworkPolicy object %s from %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete MultiNetworkPolicy: %w", err)
	}

	builder.Object = nil

	return nil
}

// Update renovates the existing MultiNetworkPolicy object with MultiNetworkPolicy definition in builder.
func (builder *MultiNetworkPolicyBuilder) Update() (*MultiNetworkPolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating MultiNetworkPolicy %s in %s namespace ",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("failed to update MultiNetworkPolicy, object does not exist on cluster")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// GetMultiNetworkGVR returns MultiNetworkPolicy's GroupVersionResource which could be used for Clean function.
func GetMultiNetworkGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "k8s.cni.cncf.io", Version: "v1beta1", Resource: "multi-networkpolicies"}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *MultiNetworkPolicyBuilder) validate() (bool, error) {
	resourceCRD := "MultiNetworkPolicy"

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
