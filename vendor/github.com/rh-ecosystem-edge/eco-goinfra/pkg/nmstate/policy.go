package nmstate

import (
	"context"
	"fmt"
	"net"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/golang/glog"

	nmstateShared "github.com/nmstate/kubernetes-nmstate/api/shared"
	nmstateV1 "github.com/nmstate/kubernetes-nmstate/api/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/strings/slices"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nodeNetConfPolIntError = "nodenetworkconfigurationpolicy 'interfaceName' cannot be empty"
)

var (
	// allowedBondModes represents all allowed modes for Bond interface.
	allowedBondModes = []string{"balance-rr", "active-backup", "balance-xor", "broadcast", "802.3ad"}
)

// AdditionalOptions additional options for pod object.
type AdditionalOptions func(builder *PolicyBuilder) (*PolicyBuilder, error)

// PolicyBuilder provides struct for the NodeNetworkConfigurationPolicy object containing connection to
// the cluster and the NodeNetworkConfigurationPolicy definition.
type PolicyBuilder struct {
	// srIovPolicy definition. Used to create srIovPolicy object.
	Definition *nmstateV1.NodeNetworkConfigurationPolicy
	// Created srIovPolicy object
	Object *nmstateV1.NodeNetworkConfigurationPolicy
	// apiClient opens API connection to the cluster.
	apiClient goclient.Client
	// errorMsg is processed before the srIovPolicy object is created.
	errorMsg string
}

// NewPolicyBuilder creates a new instance of PolicyBuilder.
func NewPolicyBuilder(apiClient *clients.Settings, name string, nodeSelector map[string]string) *PolicyBuilder {
	glog.V(100).Infof(
		"Initializing new NodeNetworkConfigurationPolicy structure with the following params: %s", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(nmstateV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add nmstate v1 scheme to client schemes")

		return nil
	}

	builder := &PolicyBuilder{
		apiClient: apiClient.Client,
		Definition: &nmstateV1.NodeNetworkConfigurationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			}, Spec: nmstateShared.NodeNetworkConfigurationPolicySpec{
				NodeSelector: nodeSelector,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the NodeNetworkConfigurationPolicy is empty")

		builder.errorMsg = "nodeNetworkConfigurationPolicy 'name' cannot be empty"

		return builder
	}

	if len(nodeSelector) == 0 {
		glog.V(100).Infof("The nodeSelector of the NodeNetworkConfigurationPolicy is empty")

		builder.errorMsg = "nodeNetworkConfigurationPolicy 'nodeSelector' cannot be empty map"

		return builder
	}

	return builder
}

// Get returns NodeNetworkConfigurationPolicy object if found.
func (builder *PolicyBuilder) Get() (*nmstateV1.NodeNetworkConfigurationPolicy, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting NodeNetworkConfigurationPolicy object %s", builder.Definition.Name)

	nmstatePolicy := &nmstateV1.NodeNetworkConfigurationPolicy{}
	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, nmstatePolicy)

	if err != nil {
		glog.V(100).Infof("NodeNetworkConfigurationPolicy object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return nmstatePolicy, nil
}

// Exists checks whether the given NodeNetworkConfigurationPolicy exists.
func (builder *PolicyBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if NodeNetworkConfigurationPolicy %s exists",
		builder.Definition.Name)

	var err error
	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a NodeNetworkConfigurationPolicy in the cluster and stores the created object in struct.
func (builder *PolicyBuilder) Create() (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the NodeNetworkConfigurationPolicy %s", builder.Definition.Name)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(context.TODO(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes NodeNetworkConfigurationPolicy object from a cluster.
func (builder *PolicyBuilder) Delete() (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the NodeNetworkConfigurationPolicy object %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("NodeNetworkConfigurationPolicy %s cannot be deleted because it does not exist",
			builder.Definition.Name)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)

	if err != nil {
		return builder, fmt.Errorf("can not delete NodeNetworkConfigurationPolicy: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing NodeNetworkConfigurationPolicy object
// with the NodeNetworkConfigurationPolicy definition in builder.
func (builder *PolicyBuilder) Update(force bool) (*PolicyBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the NodeNetworkConfigurationPolicy object %s",
		builder.Definition.Name,
	)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)

	if err == nil {
		builder.Object = builder.Definition
	} else if force {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("NodeNetworkConfigurationPolicy", builder.Definition.Name))

			builder, err := builder.Delete()

			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("NodeNetworkConfigurationPolicy", builder.Definition.Name))

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, err
}

// WithInterfaceAndVFs adds SR-IOV VF configuration to the NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) WithInterfaceAndVFs(sriovInterface string, numberOfVF uint8) *PolicyBuilder {
	if valid, err := builder.validate(); !valid {
		builder.errorMsg = err.Error()

		return builder
	}

	glog.V(100).Infof(
		"Creating NodeNetworkConfigurationPolicy %s with SR-IOV VF configuration: %d",
		builder.Definition.Name, numberOfVF)

	if sriovInterface == "" {
		glog.V(100).Infof("The sriovInterface  can not be empty string")

		builder.errorMsg = "The sriovInterface is empty string"

		return builder
	}

	intNumberOfVF := int(numberOfVF)
	newInterface := NetworkInterface{
		Name:  sriovInterface,
		Type:  "ethernet",
		State: "up",
		Ethernet: Ethernet{
			Sriov: Sriov{TotalVfs: &intNumberOfVF},
		},
	}

	return builder.withInterface(newInterface)
}

// WithBondInterface adds Bond interface configuration to the NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) WithBondInterface(slavePorts []string, bondName, mode string,
	options ...OptionsLinkAggregation) *PolicyBuilder {
	if valid, err := builder.validate(); !valid {
		builder.errorMsg = err.Error()

		return builder
	}

	glog.V(100).Infof("Creating NodeNetworkConfigurationPolicy %s with Bond interface configuration:"+
		" BondName %s, Mode %s, SlavePorts %v", builder.Definition.Name, bondName, mode, slavePorts)

	if !slices.Contains(allowedBondModes, mode) {
		glog.V(100).Infof("error to add Bond mode %s, allowed modes are %v", mode, allowedBondModes)

		builder.errorMsg = "invalid Bond mode parameter"

		return builder
	}

	if bondName == "" {
		glog.V(100).Infof("The bondName can not be empty string")

		builder.errorMsg = "The bondName is empty sting"

		return builder
	}

	newInterface := NetworkInterface{
		Name:  bondName,
		Type:  "bond",
		State: "up",
		LinkAggregation: LinkAggregation{
			Mode: mode,
			Port: slavePorts,
		},
	}

	if len(options) > 0 {
		newInterface.LinkAggregation.Options = options[0]
	}

	return builder.withInterface(newInterface)
}

// WithVlanInterface adds VLAN interface configuration to the NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) WithVlanInterface(baseInterface string, vlanID uint16) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Creating NodeNetworkConfigurationPolicy %s with VLAN interface %s and vlanID %d",
		builder.Definition.Name, baseInterface, vlanID)

	if baseInterface == "" {
		glog.V(100).Infof("The baseInterface can not be empty string")

		builder.errorMsg = "nodenetworkconfigurationpolicy 'baseInterface' cannot be empty"

		return builder
	}

	if vlanID > 4094 {
		builder.errorMsg = "invalid vlanID, allowed vlanID values are between 0-4094"

		return builder
	}

	newInterface := NetworkInterface{
		Name:  fmt.Sprintf("%s.%d", baseInterface, vlanID),
		Type:  "vlan",
		State: "up",
		Vlan: Vlan{
			BaseIface: baseInterface,
			ID:        int(vlanID),
		},
	}

	return builder.withInterface(newInterface)
}

// WithVlanInterfaceIP adds VLAN interface with IP configuration to the NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) WithVlanInterfaceIP(baseInterface, ipv4Addresses, ipv6Addresses string,
	vlanID uint16) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Creating NodeNetworkConfigurationPolicy %s with VLAN interface %s and vlanID %d",
		builder.Definition.Name, baseInterface, vlanID)

	if baseInterface == "" {
		glog.V(100).Infof("The baseInterface can not be empty string")

		builder.errorMsg = "nodenetworkconfigurationpolicy 'baseInterface' cannot be empty"

		return builder
	}

	if vlanID > 4094 {
		glog.V(100).Infof("the vlanID is out of range, allowed vlanID values are between 0-4094")

		builder.errorMsg = "invalid vlanID, allowed vlanID values are between 0-4094"

		return builder
	}

	if net.ParseIP(ipv4Addresses) == nil {
		glog.V(100).Infof("the vlanInterface contains an invalid ipv4 address")

		builder.errorMsg = "vlanInterfaceIP 'ipv4Addresses' is an invalid ipv4 address"

		return builder
	}

	if net.ParseIP(ipv6Addresses) == nil {
		glog.V(100).Infof("the vlanInterface contains an invalid ipv6 address")

		builder.errorMsg = "vlanInterfaceIP 'ipv6Addresses' is an invalid ipv6 address"

		return builder
	}

	newInterface := NetworkInterface{
		Name:  fmt.Sprintf("%s.%d", baseInterface, vlanID),
		Type:  "vlan",
		State: "up",
		Vlan: Vlan{
			BaseIface: baseInterface,
			ID:        int(vlanID),
		},
		Ipv4: InterfaceIpv4{
			Enabled: true,
			Dhcp:    false,
			Address: []InterfaceIPAddress{{
				PrefixLen: 24,
				IP:        net.ParseIP(ipv4Addresses),
			}},
		},
		Ipv6: InterfaceIpv6{
			Enabled: true,
			Dhcp:    false,
			Address: []InterfaceIPAddress{{
				PrefixLen: 64,
				IP:        net.ParseIP(ipv6Addresses),
			}},
		},
	}

	return builder.withInterface(newInterface)
}

// WithEthernetInterface adds type ethernet interface and IPs configuration to the NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) WithEthernetInterface(interfaceName, ipv4Address, ipv6Address string) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Creating NodeNetworkConfigurationPolicy %s with an ethernet interface %s",
		builder.Definition.Name, interfaceName)

	if interfaceName == "" {
		glog.V(100).Infof("The interfaceName can not be empty string")

		builder.errorMsg = nodeNetConfPolIntError

		return builder
	}

	if net.ParseIP(ipv4Address) == nil {
		glog.V(100).Infof("the ethernet interface contains an invalid ipv4 address")

		builder.errorMsg = "ethernet interface 'ipv4Addresses' is an invalid ipv4 address"

		return builder
	}

	if net.ParseIP(ipv6Address) == nil {
		glog.V(100).Infof("the ethernet interface contains an invalid ipv6 address")

		builder.errorMsg = "ethernet interface 'ipv6Addresses' is an invalid ipv6 address"

		return builder
	}

	newInterface := NetworkInterface{
		Name:  interfaceName,
		Type:  "ethernet",
		State: "up",
		Ipv4: InterfaceIpv4{
			Enabled: true,
			Dhcp:    false,
			Address: []InterfaceIPAddress{{
				PrefixLen: 24,
				IP:        net.ParseIP(ipv4Address),
			}},
		},
		Ipv6: InterfaceIpv6{
			Enabled:  true,
			Dhcp:     false,
			Autoconf: false,
			Address: []InterfaceIPAddress{{
				PrefixLen: 64,
				IP:        net.ParseIP(ipv6Address),
			}},
		},
	}

	return builder.withInterface(newInterface)
}

// WithEthernetIPv6LinkLocalInterface enables IPv6 and adds link-local address to interface configuration in the
// NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) WithEthernetIPv6LinkLocalInterface(interfaceName string) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Creating NodeNetworkConfigurationPolicy %s with an ethernet interface %s",
		builder.Definition.Name, interfaceName)

	if interfaceName == "" {
		glog.V(100).Infof("The interfaceName can not be empty string")

		builder.errorMsg = nodeNetConfPolIntError

		return builder
	}

	newInterface := NetworkInterface{
		Name:  interfaceName,
		Type:  "ethernet",
		State: "up",
		Ipv4: InterfaceIpv4{
			Enabled: false,
		},
		Ipv6: InterfaceIpv6{
			Enabled: true,
		},
	}

	return builder.withInterface(newInterface)
}

// WithAbsentInterface appends the configuration for an absent interface to the NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) WithAbsentInterface(interfaceName string) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Creating NodeNetworkConfigurationPolicy %s with absent interface configuration:"+
		" interface %s", builder.Definition.Name, interfaceName)

	if interfaceName == "" {
		glog.V(100).Infof("The interfaceName can not be empty string")

		builder.errorMsg = nodeNetConfPolIntError

		return builder
	}

	newInterface := NetworkInterface{
		Name:  interfaceName,
		State: "absent",
	}

	return builder.withInterface(newInterface)
}

// WithOptions creates pod with generic mutation options.
func (builder *PolicyBuilder) WithOptions(options ...AdditionalOptions) *PolicyBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting pod additional options")

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

// WaitUntilCondition waits for the duration of the defined timeout or until the
// NodeNetworkConfigurationPolicy gets to a specific condition.
func (builder *PolicyBuilder) WaitUntilCondition(condition nmstateShared.ConditionType, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Waiting for the defined period until NodeNetworkConfigurationPolicy %s has condition %v",
		builder.Definition.Name, condition)

	if !builder.Exists() {
		return fmt.Errorf("cannot wait for NodeNetworkConfigurationPolicy condition because it does not exist")
	}

	// Polls every retryInterval to determine if NodeNetworkConfigurationPolicy is in desired condition.
	var err error

	return wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()

			if err != nil {
				return false, nil
			}

			for _, cond := range builder.Object.Status.Conditions {
				if cond.Type == condition && cond.Status == corev1.ConditionTrue {
					return true, nil
				}
			}

			return false, nil
		})
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PolicyBuilder) validate() (bool, error) {
	resourceCRD := "NodeNetworkConfigurationPolicy"

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

// withInterface adds given network interface to the NodeNetworkConfigurationPolicy.
func (builder *PolicyBuilder) withInterface(networkInterface NetworkInterface) *PolicyBuilder {
	if valid, err := builder.validate(); !valid {
		builder.errorMsg = err.Error()

		return builder
	}

	glog.V(100).Infof("Creating NodeNetworkConfigurationPolicy %s with network interface %s",
		builder.Definition.Name, networkInterface.Name)

	var CurrentState DesiredState

	err := yaml.Unmarshal(builder.Definition.Spec.DesiredState.Raw, &CurrentState)

	if err != nil {
		glog.V(100).Infof("Failed Unmarshal DesiredState")

		builder.errorMsg = "Failed Unmarshal DesiredState"

		return builder
	}

	CurrentState.Interfaces = append(CurrentState.Interfaces, networkInterface)

	desiredStateYaml, err := yaml.Marshal(CurrentState)

	if err != nil {
		glog.V(100).Infof("Failed Marshal DesiredState")

		builder.errorMsg = "failed to Marshal a new Desired state"

		return builder
	}

	builder.Definition.Spec.DesiredState = nmstateShared.NewState(string(desiredStateYaml))

	return builder
}
