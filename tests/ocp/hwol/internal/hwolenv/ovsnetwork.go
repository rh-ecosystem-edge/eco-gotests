package hwolenv

import (
	"context"
	"fmt"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// HostLocalIPAM is the default host-local IPAM for HWOL attach networks
// (matches SR-IOV Network Operator network examples).
const HostLocalIPAM = `{
  "type": "host-local",
  "subnet": "10.56.217.0/24",
  "rangeStart": "10.56.217.171",
  "rangeEnd": "10.56.217.181"
}`

// OvsNetworkBuilder builds OVSNetwork CRs for the HWOL suite.
type OvsNetworkBuilder struct {
	// Definition is the desired OVSNetwork used to create the object.
	Definition *sriovv1.OVSNetwork
	// Object is the OVSNetwork as last seen on the cluster.
	Object *sriovv1.OVSNetwork
	// errorMsg is processed before the OVSNetwork object is created.
	errorMsg string
	// apiClient opens an API connection to the cluster.
	apiClient runtimeClient.Client
}

// NewOvsNetworkBuilder creates a new OvsNetworkBuilder.
func NewOvsNetworkBuilder(
	apiClient *clients.Settings, name, nsname, targetNsname, resName string) *OvsNetworkBuilder {
	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(sriovv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriovv1 scheme to client schemes")

		return nil
	}

	builder := &OvsNetworkBuilder{
		apiClient: apiClient.Client,
		Definition: &sriovv1.OVSNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
				Labels: map[string]string{
					tsparams.TestResourceLabelKey: tsparams.TestResourceLabelValue,
				},
			},
			Spec: sriovv1.OVSNetworkSpec{
				ResourceName:     resName,
				NetworkNamespace: targetNsname,
			},
		},
	}

	if name == "" {
		builder.errorMsg = "OVSNetwork 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		builder.errorMsg = "OVSNetwork 'nsname' cannot be empty"

		return builder
	}

	if resName == "" {
		builder.errorMsg = "OVSNetwork 'resName' cannot be empty"

		return builder
	}

	return builder
}

// WithIPAM sets IPAM JSON on the OVSNetwork definition.
func (builder *OvsNetworkBuilder) WithIPAM(ipam string) *OvsNetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if ipam == "" {
		builder.errorMsg = "OVSNetwork 'ipam' cannot be empty"

		return builder
	}

	builder.Definition.Spec.IPAM = ipam

	return builder
}

// WithBridge sets the OVS bridge name used by ovs-cni for VF representors.
func (builder *OvsNetworkBuilder) WithBridge(bridge string) *OvsNetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if bridge == "" {
		builder.errorMsg = "OVSNetwork 'bridge' cannot be empty"

		return builder
	}

	builder.Definition.Spec.Bridge = bridge

	return builder
}

// Get returns the OVSNetwork object if found.
func (builder *OvsNetworkBuilder) Get() (*sriovv1.OVSNetwork, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting OVSNetwork object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	network := &sriovv1.OVSNetwork{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		network)
	if err != nil {
		klog.V(100).Infof(
			"OVSNetwork object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return network, nil
}

// Exists checks whether the given OVSNetwork object exists in the cluster.
func (builder *OvsNetworkBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if OVSNetwork %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	// True only when Get succeeds. Treating non-NotFound errors as "exists"
	// would skip Create and leave tests assuming a network that was never applied.
	return err == nil
}

// Create generates an OVSNetwork in the cluster and stores the created object in the struct.
func (builder *OvsNetworkBuilder) Create() (*OvsNetworkBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if builder.Exists() {
		if err := ensureHwolResourceOwnership("OVSNetwork", builder.Object.Name, builder.Object.Labels); err != nil {
			return nil, err
		}

		return builder, nil
	}

	err := builder.apiClient.Create(context.TODO(), builder.Definition)
	if err != nil {
		klog.V(100).Info("Failed to create OVSNetwork")

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes the OVSNetwork object.
func (builder *OvsNetworkBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	if !builder.Exists() {
		klog.V(100).Info("OVSNetwork cannot be deleted because it does not exist")

		builder.Object = nil

		return nil
	}
	if err := ensureHwolResourceOwnership("OVSNetwork", builder.Object.Name, builder.Object.Labels); err != nil {
		return err
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// DeleteAndWait removes the OVSNetwork object and waits until it is gone.
// The SR-IOV operator may keep its managed bridge while an OVSNetwork is
// terminating, so HWOL teardown must not proceed to delete its policy first.
func (builder *OvsNetworkBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	if err := builder.Delete(); err != nil {
		return err
	}

	return builder.WaitUntilDeleted(timeout)
}

// WaitUntilDeleted waits until the OVSNetwork object no longer exists.
func (builder *OvsNetworkBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		})
}

func (builder *OvsNetworkBuilder) validate() (bool, error) {
	resourceCRD := "OVSNetwork"

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

// CreateOvsNetwork creates an OVSNetwork CR that generates a NAD using the ovs CNI plugin
// (not sriov CNI) in networkNS. If ipam is empty, HostLocalIPAM is used. Callers may pass
// other IPAM JSON later (for example NV-IPAM) without changing this helper.
// bridge may be empty; when set it is written into the NAD so representors attach to the
// managed HWOL bridge (e.g. br-0000_ca_00.0).
func CreateOvsNetwork(
	name, operatorNS, resourceName, networkNS, ipam, bridge string,
) (*OvsNetworkBuilder, error) {
	network := NewOvsNetworkBuilder(APIClient, name, operatorNS, networkNS, resourceName)
	if network == nil {
		return nil, fmt.Errorf("failed to init OVSNetwork builder")
	}

	if ipam == "" {
		ipam = HostLocalIPAM
	}

	network = network.WithIPAM(ipam)

	if bridge != "" {
		network = network.WithBridge(bridge)
	}

	created, err := network.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create OVSNetwork %s: %w", name, err)
	}

	return created, nil
}
