package networkpolicy

import (
	"context"
	"fmt"

	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// NetworkPolicyBuilder provides struct for networkPolicy object.
type NetworkPolicyBuilder struct {
	// NetworkPolicy definition. Used to create networkPolicy object with minimum set of required elements.
	Definition *netv1.NetworkPolicy
	// Created networkPolicy object on the cluster.
	Object *netv1.NetworkPolicy
	// api client to interact with the cluster.
	apiClient runtimeClient.Client
	// errorMsg is processed before NetworkPolicy object is created.
	errorMsg string
}

// NewNetworkPolicyBuilder method creates new instance of builder.
func NewNetworkPolicyBuilder(apiClient *clients.Settings, name, nsname string) *NetworkPolicyBuilder {
	klog.V(100).Infof("Initializing new NetworkPolicyBuilder structure with the following params: name: %s, namespace: %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(netv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add NetworkPolicy v1 scheme to client schemes")

		return nil
	}

	builder := &NetworkPolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &netv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the networkPolicy is empty")

		builder.errorMsg = "The networkPolicy 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the networkPolicy is empty")

		builder.errorMsg = "The networkPolicy 'namespace' cannot be empty"

		return builder
	}

	return builder
}

// WithNamespaceIngressRule applies ingress rule for the networkPolicy.
func (builder *NetworkPolicyBuilder) WithNamespaceIngressRule(
	namespaceIngressMatchLabels map[string]string,
	podIngressMatchLabels map[string]string) *NetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Applying Ingress rule to networkPolicy %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if len(namespaceIngressMatchLabels) == 0 && len(podIngressMatchLabels) == 0 {
		klog.V(100).Info("At least one type of the selector for NetworkPolicy ingress rule should be defined")

		builder.errorMsg = "Both namespaceIngressMatchLabels and podIngressMatchLabels parameters are empty maps"

		return builder
	}

	var peerRule netv1.NetworkPolicyPeer

	if len(namespaceIngressMatchLabels) != 0 {
		klog.V(100).Infof(
			"Applying Ingress rule with namespaceIngressMatchLabels %v parameter to networkPolicy %s in namespace %s",
			namespaceIngressMatchLabels, builder.Definition.Name, builder.Definition.Namespace)

		peerRule.NamespaceSelector = &metav1.LabelSelector{
			MatchLabels: namespaceIngressMatchLabels,
		}
	}

	if len(podIngressMatchLabels) != 0 {
		klog.V(100).Infof(
			"Applying Ingress rule with podIngressMatchLabels %v parameter to networkPolicy %s in namespace %s",
			podIngressMatchLabels, builder.Definition.Name, builder.Definition.Namespace)

		peerRule.PodSelector = &metav1.LabelSelector{
			MatchLabels: podIngressMatchLabels,
		}
	}

	if builder.Definition.Spec.Ingress == nil {
		builder.Definition.Spec.Ingress = []netv1.NetworkPolicyIngressRule{}
	}

	builder.Definition.Spec.Ingress = append(builder.Definition.Spec.Ingress, netv1.NetworkPolicyIngressRule{
		From: []netv1.NetworkPolicyPeer{peerRule},
	})

	return builder
}

// WithPolicyType add policyType to the networkPolicy.
func (builder *NetworkPolicyBuilder) WithPolicyType(policyType netv1.PolicyType) *NetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating networkPolicy %s in %s namespace with the policyType defined: %v",
		builder.Definition.Name, builder.Definition.Namespace, policyType)

	if policyType == "" {
		klog.V(100).Info("The policyType value has to be provided")

		builder.errorMsg = "The policyType is an empty string"

		return builder
	}

	if builder.Definition.Spec.PolicyTypes == nil {
		builder.Definition.Spec.PolicyTypes = []netv1.PolicyType{}
	}

	builder.Definition.Spec.PolicyTypes = append(builder.Definition.Spec.PolicyTypes, policyType)

	return builder
}

// WithPodSelector add podSelector to the networkPolicy.
func (builder *NetworkPolicyBuilder) WithPodSelector(podSelectorMatchLabels map[string]string) *NetworkPolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating networkPolicy %s in %s namespace with podSelector defined: %v",
		builder.Definition.Name, builder.Definition.Namespace, podSelectorMatchLabels)

	if len(podSelectorMatchLabels) == 0 {
		klog.V(100).Info("The podSelector could not be empty")

		builder.errorMsg = "The podSelector is an empty string"

		return builder
	}

	builder.Definition.Spec.PodSelector = metav1.LabelSelector{MatchLabels: podSelectorMatchLabels}

	return builder
}

// Pull loads an existing networkPolicy into the Builder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*NetworkPolicyBuilder, error) {
	klog.V(100).Infof("Pulling existing networkPolicy name: %s namespace:%s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(netv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add NetworkPolicy v1 scheme to client schemes")

		return nil, err
	}

	builder := &NetworkPolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &netv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the networkPolicy is empty")

		return nil, fmt.Errorf("networkPolicy 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the networkPolicy is empty")

		return nil, fmt.Errorf("networkPolicy 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		klog.V(100).Infof("Failed to pull networkPolicy object %s from namespace %s. Object does not exist",
			name, nsname)

		return nil, fmt.Errorf("networkPolicy object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns MultiNetworkPolicy object if found.
func (builder *NetworkPolicyBuilder) Get() (*netv1.NetworkPolicy, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting NetworkPolicy object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	netPolicy := &netv1.NetworkPolicy{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		netPolicy)
	if err != nil {
		klog.V(100).Infof(
			"NetworkPolicy object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return netPolicy, nil
}

// Create makes a networkPolicy in cluster and stores the created object in struct.
func (builder *NetworkPolicyBuilder) Create() (*NetworkPolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the networkPolicy %s in %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			klog.V(100).Info("Failed to create NetworkPolicy object")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, err
}

// Exists checks whether the given NetworkPolicy exists.
func (builder *NetworkPolicyBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if networkPolicy %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete removes a networkPolicy object from a cluster.
func (builder *NetworkPolicyBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the networkPolicy object %s from %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("The networkPolicy object %s does not exist in %s namespace", builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)

	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("cannot delete networkPolicy: %w", err)
	}

	builder.Object = nil

	return nil
}

// Update renovates the existing networkPolicy object with networkPolicy definition in builder.
func (builder *NetworkPolicyBuilder) Update() (*NetworkPolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating networkPolicy %s in %s namespace ",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("failed to update NetworkPolicy, object does not exist on cluster")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NetworkPolicyBuilder) validate() (bool, error) {
	resourceCRD := "NetworkPolicy"

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

// GetGVR returns networkPolicy's GroupVersionResource which could be used for Clean function.
func GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
}
