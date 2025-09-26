package sriov

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"golang.org/x/exp/slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkBuilder provides struct for srIovNetwork object which contains connection to cluster and
// srIovNetwork definition.
type NetworkBuilder struct {
	// srIovNetwork definition. Used to create srIovNetwork object.
	Definition *srIovV1.SriovNetwork
	// Created srIovNetwork object.
	Object *srIovV1.SriovNetwork
	// Used in functions that define or mutate srIovNetwork definitions. errorMsg is processed before srIovNetwork
	// object is created.
	errorMsg string
	// apiClient opens api connection to the cluster.
	apiClient runtimeClient.Client
}

// NetworkAdditionalOptions additional options for SriovNetwork object.
type NetworkAdditionalOptions func(builder *NetworkBuilder) (*NetworkBuilder, error)

// NewNetworkBuilder creates new instance of Builder.
func NewNetworkBuilder(
	apiClient *clients.Settings, name, nsname, targetNsname, resName string) *NetworkBuilder {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriovv1 scheme to client schemes")

		return nil
	}

	builder := &NetworkBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: srIovV1.SriovNetworkSpec{
				ResourceName:     resName,
				NetworkNamespace: targetNsname,
			},
		},
	}

	if name == "" {
		builder.errorMsg = "SrIovNetwork 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		builder.errorMsg = "SrIovNetwork 'nsname' cannot be empty"

		return builder
	}

	if resName == "" {
		builder.errorMsg = "SrIovNetwork 'resName' cannot be empty"

		return builder
	}

	return builder
}

// WithTargetNamespace sets the target namespace for the SriovNetwork.
func (builder *NetworkBuilder) WithTargetNamespace(targetNsname string) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding targetNsname %s to SriovNetwork %s in namespace %s",
		targetNsname, builder.Definition.Name, builder.Definition.Namespace)

	if targetNsname == "" {
		glog.V(100).Infof("SrIovNetwork 'targetNsname' cannot be empty")

		builder.errorMsg = "SrIovNetwork 'targetNsname' cannot be empty"

		return builder
	}

	builder.Definition.Spec.NetworkNamespace = targetNsname

	return builder
}

// WithVLAN sets vlan id in the SrIovNetwork definition. Allowed vlanId range is between 0-4094.
func (builder *NetworkBuilder) WithVLAN(vlanID uint16) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if vlanID > 4094 {
		builder.errorMsg = "invalid vlanID, allowed vlanID values are between 0-4094"

		return builder
	}

	builder.Definition.Spec.Vlan = int(vlanID)

	return builder
}

// WithVlanProto sets the VLAN protocol for qinq tunneling protocol in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithVlanProto(vlanProtocol string) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting SriovNetwork vlanProtocol support for qinq")

	allowedVlanProto := []string{"802.1q", "802.1Q", "802.1ad", "802.1AD"}
	if !slices.Contains(allowedVlanProto, vlanProtocol) {
		builder.errorMsg = "invalid 'vlanProtocol' parameters"
	}

	builder.Definition.Spec.VlanProto = vlanProtocol

	return builder
}

// WithSpoof sets spoof flag based on the given argument in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithSpoof(enabled bool) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if enabled {
		builder.Definition.Spec.SpoofChk = "on"
	} else {
		builder.Definition.Spec.SpoofChk = "off"
	}

	return builder
}

// WithMetaPluginAllMultiFlag metaplugin activates allmulti multicast mode on a SriovNetwork configuration.
func (builder *NetworkBuilder) WithMetaPluginAllMultiFlag(allMultiFlag bool) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.MetaPluginsConfig = fmt.Sprintf(`{ "type": "tuning", "allmulti": %t }`, allMultiFlag)

	return builder
}

// WithMetaPluginRdma activates rdma plugin on a SriovNetwork configuration.
func (builder *NetworkBuilder) WithMetaPluginRdma() *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.MetaPluginsConfig = `{ "type": "rdma" }`

	return builder
}

// WithLinkState sets linkState parameters in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithLinkState(linkState string) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	allowedLinkStates := []string{"enable", "disable", "auto"}

	if !slices.Contains(allowedLinkStates, linkState) {
		builder.errorMsg = "invalid 'linkState' parameters"

		return builder
	}

	builder.Definition.Spec.LinkState = linkState

	return builder
}

// WithMaxTxRate sets maxTxRate parameters in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithMaxTxRate(maxTxRate uint16) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	maxTxRateInt := int(maxTxRate)

	builder.Definition.Spec.MaxTxRate = &maxTxRateInt

	return builder
}

// WithMinTxRate sets minTxRate parameters in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithMinTxRate(minTxRate uint16) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	maxTxRateInt := int(minTxRate)
	builder.Definition.Spec.MaxTxRate = &maxTxRateInt

	return builder
}

// WithTrustFlag sets trust flag based on the given argument in the SrIoVNetwork definition spec.
func (builder *NetworkBuilder) WithTrustFlag(enabled bool) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if enabled {
		builder.Definition.Spec.Trust = "on"
	} else {
		builder.Definition.Spec.Trust = "off"
	}

	return builder
}

// WithVlanQoS sets qoSClass parameters in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithVlanQoS(qoSClass uint16) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if qoSClass > 7 {
		builder.errorMsg = "Invalid QoS class. Supported vlan QoS class values are between 0...7"

		return builder
	}

	qoSClassInt := int(qoSClass)

	builder.Definition.Spec.VlanQoS = qoSClassInt

	return builder
}

// WithLogLevel sets logLevel parameter in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithLogLevel(logLevel string) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	allowedLogLevels := []string{"panic", "error", "warning", "info", "debug", ""}

	if !slices.Contains(allowedLogLevels, logLevel) {
		builder.errorMsg = "invalid logLevel value, allowed logLevel values are:" +
			" panic, error, warning, info, debug or empty"

		return builder
	}

	builder.Definition.Spec.LogLevel = logLevel

	return builder
}

// WithIPAddressSupport sets ips capabilities in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithIPAddressSupport() *NetworkBuilder {
	return builder.withCapabilities("ips")
}

// WithMacAddressSupport sets mac capabilities in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithMacAddressSupport() *NetworkBuilder {
	return builder.withCapabilities("mac")
}

// WithStaticIpam sets static IPAM in the SrIovNetwork definition spec.
func (builder *NetworkBuilder) WithStaticIpam() *NetworkBuilder {
	return builder.withIpam("static")
}

// WithOptions creates SriovNetwork with generic mutation options.
func (builder *NetworkBuilder) WithOptions(options ...NetworkAdditionalOptions) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting SriovNetwork additional options")

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

// PullNetwork pulls existing sriovnetwork from cluster.
func PullNetwork(apiClient *clients.Settings, name, nsname string) (*NetworkBuilder, error) {
	glog.V(100).Infof("Pulling existing sriovnetwork name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("sriovnetwork 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add sriovv1 scheme to client schemes")

		return nil, err
	}

	builder := &NetworkBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the sriovnetwork is empty")

		return nil, fmt.Errorf("sriovnetwork 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the sriovnetwork is empty")

		return nil, fmt.Errorf("sriovnetwork 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("sriovnetwork object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns CatalogSource object if found.
func (builder *NetworkBuilder) Get() (*srIovV1.SriovNetwork, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting SriovNetwork object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	network := &srIovV1.SriovNetwork{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		network)
	if err != nil {
		glog.V(100).Infof(
			"SriovNetwork object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return network, nil
}

// Create generates SrIovNetwork in a cluster and stores the created object in struct.
func (builder *NetworkBuilder) Create() (*NetworkBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			glog.V(100).Infof("Failed to create SriovNetwork")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes SrIovNetwork object.
func (builder *NetworkBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	if !builder.Exists() {
		glog.V(100).Infof("SriovNetwork cannot be deleted because it does not exist")

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

// DeleteAndWait deletes the SrIovNetwork resource and waits until it is deleted.
func (builder *NetworkBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting SrIovNetwork %s in namespace %s and waiting for the defined period until it is removed",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.Delete()
	if err != nil {
		return err
	}

	return builder.WaitUntilDeleted(timeout)
}

// WaitUntilDeleted waits for the duration of the defined timeout or until the SrIovNetwork is deleted.
func (builder *NetworkBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(
		"Waiting for the defined period until SrIovNetwork %s in namespace %s is deleted",
		builder.Definition.Name, builder.Definition.Namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				glog.V(100).Infof("SrIovNetwork %s/%s still present", builder.Definition.Name, builder.Definition.Namespace)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				glog.V(100).Infof("SrIovNetwork %s/%s is gone", builder.Definition.Name, builder.Definition.Namespace)

				return true, nil
			}

			glog.V(100).Infof("Failed to get SrIovNetwork %s/%s: %w", builder.Definition.Name, builder.Definition.Namespace, err)

			return false, err
		})
}

// Exists checks whether the given SrIovNetwork object exists in a cluster.
func (builder *NetworkBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if SriovNetwork %s exists",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Update renovates the existing SrIovNetwork object with the SrIovNetwork definition in builder.
func (builder *NetworkBuilder) Update(force bool) (*NetworkBuilder, error) {
	if valid, _ := builder.validate(); !valid {
		return builder, nil
	}

	glog.V(100).Infof("Updating the SrIovNetwork object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		return nil, fmt.Errorf("failed to update SriovNetwork, object does not exist on cluster")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	} else if force {
		glog.V(100).Infof(
			msg.FailToUpdateNotification("SrIovNetwork", builder.Definition.Name, builder.Definition.Namespace))

		err = builder.Delete()
		if err != nil {
			glog.V(100).Infof(
				msg.FailToUpdateError("SrIovNetwork", builder.Definition.Name, builder.Definition.Namespace))

			return nil, err
		}

		return builder.Create()
	}

	return builder, err
}

// GetSriovNetworksGVR returns SriovNetwork's GroupVersionResource which could be used for Clean function.
func GetSriovNetworksGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: "sriovnetwork.openshift.io", Version: "v1", Resource: "sriovnetworks",
	}
}

func (builder *NetworkBuilder) withCapabilities(capability string) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(builder.Definition.Spec.Capabilities) == 0 {
		builder.Definition.Spec.Capabilities = fmt.Sprintf(`{ "%s": true }`, capability)
	} else {
		builder.Definition.Spec.Capabilities =
			"{" + builder.Definition.Spec.Capabilities[1:len(builder.Definition.Spec.Capabilities)-2] +
				fmt.Sprintf(`, "%s": true `, capability) + "}"
	}

	return builder
}

func (builder *NetworkBuilder) withIpam(ipamType string) *NetworkBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if ipamType == "" {
		glog.V(100).Infof("sriov network 'ipamType' parameter can not be empty")

		builder.errorMsg = "failed to configure IPAM, 'ipamType' parameter is empty"

		return builder
	}

	builder.Definition.Spec.IPAM = fmt.Sprintf(`{ "type": "%s" }`, ipamType)

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NetworkBuilder) validate() (bool, error) {
	resourceCRD := "SriovNetwork"

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
