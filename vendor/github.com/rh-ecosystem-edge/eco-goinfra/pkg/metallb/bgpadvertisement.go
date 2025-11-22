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

// BGPAdvertisementBuilder provides struct for the BGPAdvertisement object containing connection to
// the cluster and the BGPAdvertisement definitions.
type BGPAdvertisementBuilder struct {
	Definition *mlbtypes.BGPAdvertisement
	Object     *mlbtypes.BGPAdvertisement
	apiClient  runtimeClient.Client
	errorMsg   string
}

// BGPAdvertisementAdditionalOptions additional options for BGPAdvertisement object.
type BGPAdvertisementAdditionalOptions func(builder *BGPAdvertisementBuilder) (*BGPAdvertisementBuilder, error)

// NewBGPAdvertisementBuilder creates a new instance of BGPAdvertisementBuilder.
func NewBGPAdvertisementBuilder(apiClient *clients.Settings, name, nsname string) *BGPAdvertisementBuilder {
	klog.V(100).Infof(
		"Initializing new BGPAdvertisement structure with the following params: %s, %s",
		name, nsname)

	err := apiClient.AttachScheme(mlbtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add metallb scheme to client schemes")

		return nil
	}

	builder := &BGPAdvertisementBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypes.BGPAdvertisement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			}, Spec: mlbtypes.BGPAdvertisementSpec{},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the BGPAdvertisement is empty")

		builder.errorMsg = "BGPAdvertisement 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the BGPAdvertisement is empty")

		builder.errorMsg = "BGPAdvertisement 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Exists checks whether the given BGPAdvertisement exists.
func (builder *BGPAdvertisementBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if BGPAdvertisement %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns BGPAdvertisement object if found.
func (builder *BGPAdvertisementBuilder) Get() (*mlbtypes.BGPAdvertisement, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting BGPAdvertisement object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	bgpAdvertisement := &mlbtypes.BGPAdvertisement{}

	err := builder.apiClient.Get(logging.DiscardContext(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		bgpAdvertisement)
	if err != nil {
		klog.V(100).Infof(
			"BGPAdvertisement object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return bgpAdvertisement, nil
}

// PullBGPAdvertisement pulls existing bgpadvertisement from cluster.
func PullBGPAdvertisement(apiClient *clients.Settings, name, nsname string) (*BGPAdvertisementBuilder, error) {
	klog.V(100).Infof("Pulling existing bgpadvertisement name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("bgpadvertisement 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(mlbtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add metallb scheme to client schemes")

		return nil, err
	}

	builder := &BGPAdvertisementBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypes.BGPAdvertisement{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the bgpadvertisement is empty")

		return nil, fmt.Errorf("bgpadvertisement 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the bgpadvertisement is empty")

		return nil, fmt.Errorf("bgpadvertisement 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("bgpadvertisement object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes a BGPAdvertisement in the cluster and stores the created object in struct.
func (builder *BGPAdvertisementBuilder) Create() (*BGPAdvertisementBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the BGPAdvertisement %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err != nil {
			klog.V(100).Info("Failed to create BGPAdvertisement")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes BGPAdvertisement object from a cluster.
func (builder *BGPAdvertisementBuilder) Delete() (*BGPAdvertisementBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the BGPAdvertisement object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof("BGPAdvertisement object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete BGPAdvertisement: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing BGPAdvertisement object with the BGPAdvertisement definition in builder.
func (builder *BGPAdvertisementBuilder) Update(force bool) (*BGPAdvertisementBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the BGPAdvertisement object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof(
			"Failed to update the BGPAdvertisement object %s in namespace %s. "+
				"Resource does not exist",
			builder.Definition.Name, builder.Definition.Namespace,
		)

		return nil, fmt.Errorf("failed to update BGPAdvertisement, resource does not exist")
	}

	builder.Object.Spec = builder.Definition.Spec

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("BGPAdvertisement", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("BGPAdvertisement", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, err
}

// WithAggregationLength4 adds the specified AggregationLength to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithAggregationLength4(aggregationLength int32) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with aggregationLength: %d",
		builder.Definition.Name, builder.Definition.Namespace, aggregationLength)

	if aggregationLength < 0 || aggregationLength > 32 {
		builder.errorMsg = fmt.Sprintf("AggregationLength %d is invalid, the value shoud be in range 0...32",
			aggregationLength)

		return builder
	}

	builder.Definition.Spec.AggregationLength = &aggregationLength

	return builder
}

// WithAggregationLength6 adds the specified AggregationLengthV6 to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithAggregationLength6(aggregationLength int32) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with aggregationLength6: %d",
		builder.Definition.Name, builder.Definition.Namespace, aggregationLength)

	if aggregationLength < 0 || aggregationLength > 128 {
		fmt.Printf("%d", aggregationLength)
		builder.errorMsg = fmt.Sprintf("AggregationLength %d is invalid, the value shoud be in range 0...128",
			aggregationLength)

		return builder
	}

	builder.Definition.Spec.AggregationLengthV6 = &aggregationLength

	return builder
}

// WithLocalPref adds the specified LocalPref to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithLocalPref(localPreference uint32) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with LocalPref: %d",
		builder.Definition.Name, builder.Definition.Namespace, localPreference)

	builder.Definition.Spec.LocalPref = localPreference

	return builder
}

// WithCommunities adds the specified Communities to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithCommunities(communities []string) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with Communities: %s",
		builder.Definition.Name, builder.Definition.Namespace, communities)

	if len(communities) < 1 {
		builder.errorMsg = "error: community setting is empty list, the list should contain at least one element"

		return builder
	}

	builder.Definition.Spec.Communities = communities

	return builder
}

// WithIPAddressPools adds the specified IPAddressPools to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithIPAddressPools(ipAddressPools []string) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with IPAddressPools: %s",
		builder.Definition.Name, builder.Definition.Namespace, ipAddressPools)

	if len(ipAddressPools) < 1 {
		builder.errorMsg = "error: IPAddressPools setting is empty list, the list should contain at least one element"

		return builder
	}

	builder.Definition.Spec.IPAddressPools = ipAddressPools

	return builder
}

// WithIPAddressPoolsSelectors adds the specified IPAddressPoolSelectors to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithIPAddressPoolsSelectors(
	poolSelector []metav1.LabelSelector) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with IPAddressPoolSelectors: %s",
		builder.Definition.Name, builder.Definition.Namespace, poolSelector)

	if len(poolSelector) < 1 {
		builder.errorMsg = "error: IPAddressPoolSelectors setting is empty list, the list should contain at least one element"

		return builder
	}

	builder.Definition.Spec.IPAddressPoolSelectors = poolSelector

	return builder
}

// WithNodeSelector adds the specified NodeSelectors to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithNodeSelector(
	nodeSelectors []metav1.LabelSelector) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with WithIPAddressPools: %v",
		builder.Definition.Name, builder.Definition.Namespace, nodeSelectors)

	if len(nodeSelectors) < 1 {
		builder.errorMsg = "error: nodeSelectors setting is empty list, the list should contain at least one element"

		return builder
	}

	builder.Definition.Spec.NodeSelectors = nodeSelectors

	return builder
}

// WithPeers adds the specified Peers to the BGPAdvertisement.
func (builder *BGPAdvertisementBuilder) WithPeers(peers []string) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating BGPAdvertisement %s in namespace %s with Peers: %v",
		builder.Definition.Name, builder.Definition.Namespace, peers)

	if len(peers) < 1 {
		builder.errorMsg = "error: peers setting is empty list, the list should contain at least one element"

		return builder
	}

	builder.Definition.Spec.Peers = peers

	return builder
}

// WithOptions creates BGPAdvertisement with generic mutation options.
func (builder *BGPAdvertisementBuilder) WithOptions(
	options ...BGPAdvertisementAdditionalOptions) *BGPAdvertisementBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting BGPAdvertisement additional options")

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

// GetBGPAdvertisementGVR returns bgpadvertisement's GroupVersionResource, which could be used for Clean function.
func GetBGPAdvertisementGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: "bgpadvertisements",
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *BGPAdvertisementBuilder) validate() (bool, error) {
	resourceCRD := "BGPAdvertisement"

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
