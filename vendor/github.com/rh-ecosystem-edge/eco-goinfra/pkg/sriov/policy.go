package sriov

import (
	"context"
	"fmt"

	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"golang.org/x/exp/slices"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicyBuilder provides struct for srIovPolicy object containing connection to the cluster and the srIovPolicy
// definitions.
type PolicyBuilder struct {
	// srIovPolicy definition. Used to create srIovPolicy object.
	Definition *srIovV1.SriovNetworkNodePolicy
	// Created srIovPolicy object.
	Object *srIovV1.SriovNetworkNodePolicy
	// Used in functions that define or mutate srIovPolicy definition. errorMsg is processed before the srIovPolicy
	// object is created.
	errorMsg string
	// apiClient opens api connection to the cluster.
	apiClient runtimeClient.Client
}

// PolicyAdditionalOptions additional options for SriovNetworkNodePolicy object.
type PolicyAdditionalOptions func(builder *PolicyBuilder) (*PolicyBuilder, error)

// NewPolicyBuilder creates a new instance of PolicyBuilder.
func NewPolicyBuilder(
	apiClient *clients.Settings,
	name string,
	nsname string,
	resName string,
	vfsNumber int,
	nicNames []string,
	nodeSelector map[string]string) *PolicyBuilder {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriovv1 scheme to client schemes")

		return nil
	}

	builder := &PolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovNetworkNodePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: srIovV1.SriovNetworkNodePolicySpec{
				NodeSelector: nodeSelector,
				NumVfs:       vfsNumber,
				ResourceName: resName,
				Priority:     1,
				NicSelector: srIovV1.SriovNetworkNicSelector{
					PfNames: nicNames,
				},
			},
		},
	}

	if name == "" {
		builder.errorMsg = "SriovNetworkNodePolicy 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		builder.errorMsg = "SriovNetworkNodePolicy 'nsname' cannot be empty"

		return builder
	}

	if resName == "" {
		builder.errorMsg = "SriovNetworkNodePolicy 'resName' cannot be empty"

		return builder
	}

	if len(nicNames) == 0 {
		builder.errorMsg = "SriovNetworkNodePolicy 'nicNames' cannot be empty list"

		return builder
	}

	if len(nodeSelector) == 0 {
		builder.errorMsg = "SriovNetworkNodePolicy 'nodeSelector' cannot be empty map"

		return builder
	}

	if vfsNumber <= 0 {
		builder.errorMsg = "SriovNetworkNodePolicy 'vfsNumber' cannot be zero of negative"

		return builder
	}

	return builder
}

// WithDevType sets device type in the SriovNetworkNodePolicy definition. Allowed devTypes are vfio-pci and netdevice.
func (builder *PolicyBuilder) WithDevType(devType string) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	allowedDevTypes := []string{"vfio-pci", "netdevice"}

	if !slices.Contains(allowedDevTypes, devType) {
		builder.errorMsg = "invalid device type, allowed devType values are: vfio-pci or netdevice"

		return builder
	}

	builder.Definition.Spec.DeviceType = devType

	return builder
}

// WithVFRange sets specific VF range for each configured PF.
func (builder *PolicyBuilder) WithVFRange(firstVF, lastVF int) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if firstVF < 0 || lastVF < 0 {
		builder.errorMsg = "firstPF or lastVF can not be less than 0"

		return builder
	}

	if firstVF > lastVF {
		builder.errorMsg = "firstPF argument can not be greater than lastPF"

		return builder
	}

	if lastVF > 63 {
		builder.errorMsg = "lastVF can not be greater than 63"

		return builder
	}

	var partitionedPFs []string
	for _, pf := range builder.Definition.Spec.NicSelector.PfNames {
		partitionedPFs = append(partitionedPFs, fmt.Sprintf("%s#%d-%d", pf, firstVF, lastVF))
	}

	builder.Definition.Spec.NicSelector.PfNames = partitionedPFs

	return builder
}

// WithMTU sets required MTU in the given SriovNetworkNodePolicy.
func (builder *PolicyBuilder) WithMTU(mtu int) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if 1 > mtu || mtu > 9192 {
		builder.errorMsg = fmt.Sprintf("invalid mtu size %d allowed mtu should be in range 1...9192", mtu)

		return builder
	}

	builder.Definition.Spec.Mtu = mtu

	return builder
}

// WithRDMA sets RDMA mode in SriovNetworkNodePolicy object.
func (builder *PolicyBuilder) WithRDMA(rdma bool) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.IsRdma = rdma

	return builder
}

// WithVhostNet sets Vhost mode in in SriovNetworkNodePolicy object.
func (builder *PolicyBuilder) WithVhostNet(vhost bool) *PolicyBuilder {
	glog.V(100).Infof("Redefining SriovNetworkNodePolicy %s with"+
		" NeedVhostNet: %t", builder.Definition.Name, vhost)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.NeedVhostNet = vhost

	return builder
}

// WithExternallyManaged sets ExternallyManaged option in SriovNetworkNodePolicy object.
func (builder *PolicyBuilder) WithExternallyManaged(externallyManaged bool) *PolicyBuilder {
	glog.V(100).Infof("Redefining SriovNetworkNodePolicy %s with"+
		" externallyManaged: %t", builder.Definition.Name, externallyManaged)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.ExternallyManaged = externallyManaged

	return builder
}

// WithOptions creates SriovNetworkNodePolicy with generic mutation options.
func (builder *PolicyBuilder) WithOptions(options ...PolicyAdditionalOptions) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting SriovNetworkNodePolicy additional options")

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

// PullPolicy pulls existing sriovnetworknodepolicy from cluster.
func PullPolicy(apiClient *clients.Settings, name, nsname string) (*PolicyBuilder, error) {
	glog.V(100).Infof("Pulling existing sriovnetworknodepolicy name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("sriovnetworknodepolicy 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriovv1 scheme to client schemes")

		return nil, err
	}

	builder := &PolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovNetworkNodePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the sriovnetworknodepolicy is empty")

		return nil, fmt.Errorf("sriovnetworknodepolicy 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the sriovnetworknodepolicy is empty")

		return nil, fmt.Errorf("sriovnetworknodepolicy 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("sriovnetworknodepolicy object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns CatalogSource object if found.
func (builder *PolicyBuilder) Get() (*srIovV1.SriovNetworkNodePolicy, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting SriovNetworkNodePolicy object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	nodePolicy := &srIovV1.SriovNetworkNodePolicy{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		nodePolicy)
	if err != nil {
		glog.V(100).Infof(
			"SriovNetworkNodePolicy object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return nodePolicy, nil
}

// Create generates an SriovNetworkNodePolicy in the cluster and stores the created object in struct.
func (builder *PolicyBuilder) Create() (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes an SriovNetworkNodePolicy object.
func (builder *PolicyBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	if !builder.Exists() {
		glog.V(100).Infof("SriovNetworkNodePolicy %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given SriovNetworkNodePolicy object exists in the cluster.
func (builder *PolicyBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if SriovNetworkNodePolicy %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PolicyBuilder) validate() (bool, error) {
	resourceCRD := "SriovNetworkNodePolicy"

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
