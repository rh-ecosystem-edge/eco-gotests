package metallb

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlbtypesv1beta2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BGPPeerBuilder provides struct for the BGPPeer object containing connection to
// the cluster and the BGPPeer definitions.
type BGPPeerBuilder struct {
	Definition *mlbtypesv1beta2.BGPPeer
	Object     *mlbtypesv1beta2.BGPPeer
	apiClient  runtimeClient.Client
	errorMsg   string
}

// BGPPeerAdditionalOptions additional options for BGPPeer object.
type BGPPeerAdditionalOptions func(builder *BGPPeerBuilder) (*BGPPeerBuilder, error)

// NewBPGPeerBuilder creates a new instance of BGPPeer.
func NewBPGPeerBuilder(
	apiClient *clients.Settings, name, nsname, peerIP string, asn, remoteASN uint32) *BGPPeerBuilder {
	glog.V(100).Infof(
		"Initializing new BGPPeer structure with the following params: %s, %s %s %d %d",
		name, nsname, peerIP, asn, remoteASN)

	err := apiClient.AttachScheme(mlbtypesv1beta2.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add metallb scheme to client schemes")

		return nil
	}

	builder := &BGPPeerBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypesv1beta2.BGPPeer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			}, Spec: mlbtypesv1beta2.BGPPeerSpec{
				MyASN:   asn,
				ASN:     remoteASN,
				Address: peerIP,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the BGPPeer is empty")

		builder.errorMsg = "BGPPeer 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the BGPPeer is empty")

		builder.errorMsg = "BGPPeer 'nsname' cannot be empty"

		return builder
	}

	if net.ParseIP(peerIP) == nil {
		glog.V(100).Infof("The peerIP of the BGPPeer contains invalid ip address %s", peerIP)

		builder.errorMsg = "BGPPeer 'peerIP' of the BGPPeer contains invalid ip address"

		return builder
	}

	return builder
}

// NewBGPPeerBuilder creates a new instance of BGPPeer.
func NewBGPPeerBuilder(
	apiClient *clients.Settings, name, nsname string, asn, remoteASN uint32) *BGPPeerBuilder {
	glog.V(100).Infof(
		"Initializing new BGPPeer structure with the following params: %s, %s %s %d %d",
		name, nsname, asn, remoteASN)

	if apiClient == nil {
		glog.V(100).Info("BGPPeer 'apiClient' cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(mlbtypesv1beta2.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add metallb scheme to client schemes")

		return nil
	}

	builder := &BGPPeerBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypesv1beta2.BGPPeer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			}, Spec: mlbtypesv1beta2.BGPPeerSpec{
				MyASN: asn,
				ASN:   remoteASN,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the BGPPeer is empty")

		builder.errorMsg = "BGPPeer 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the BGPPeer is empty")

		builder.errorMsg = "BGPPeer 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Get returns BGPPeer object if found.
func (builder *BGPPeerBuilder) Get() (*mlbtypesv1beta2.BGPPeer, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting BGPPeer object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	bgpPeer := &mlbtypesv1beta2.BGPPeer{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		bgpPeer)
	if err != nil {
		glog.V(100).Infof(
			"Failed to Unmarshal BGPPeer: unstructured object to structure in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return bgpPeer, nil
}

// Exists checks whether the given BGPPeer exists.
func (builder *BGPPeerBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if BGPPeer %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// PullBGPPeer pulls existing bgppeer from cluster.
func PullBGPPeer(apiClient *clients.Settings, name, nsname string) (*BGPPeerBuilder, error) {
	glog.V(100).Infof("Pulling existing bgppeer name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("bgppeer 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(mlbtypesv1beta2.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add metallb scheme to client schemes")

		return nil, err
	}

	builder := &BGPPeerBuilder{
		apiClient: apiClient.Client,
		Definition: &mlbtypesv1beta2.BGPPeer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the bgppeer is empty")

		return nil, fmt.Errorf("bgppeer 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the bgppeer is empty")

		return nil, fmt.Errorf("bgppeer 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("bgppeer object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes a BGPPeer in the cluster and stores the created object in struct.
func (builder *BGPPeerBuilder) Create() (*BGPPeerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the BGPPeer %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, nil
}

// Delete removes BGPPeer object from a cluster.
func (builder *BGPPeerBuilder) Delete() (*BGPPeerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the BGPPeer object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		glog.V(100).Infof("BGPPeer object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete BGPPeer: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing BGPPeer object with the BGPPeer definition in builder.
func (builder *BGPPeerBuilder) Update(force bool) (*BGPPeerBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the BGPPeer object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("BGPPeer", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("BGPPeer", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	return builder, err
}

// WithBGPPeerIP defines the peer IP address.
func (builder *BGPPeerBuilder) WithBGPPeerIP(bgpPeerIP string) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating an BGPPeer %s in namespace %s with IP address: %s",
		builder.Definition.Name, builder.Definition.Namespace, bgpPeerIP)

	if net.ParseIP(bgpPeerIP) == nil {
		glog.V(100).Infof("The peerIP of the BGPPeer contains invalid ip address %s", bgpPeerIP)

		builder.errorMsg = "BGPPeer 'bgpPeerIP' of the BGPPeer contains invalid ip address"

		return builder
	}

	builder.Definition.Spec.Address = bgpPeerIP

	return builder
}

// WithIPUnnumbered defines the interface to be used with the interface BGPPeer spec.
func (builder *BGPPeerBuilder) WithIPUnnumbered(interfaceName string) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating an unnumbered BGPPeer %s in namespace %s with interface: %s",
		builder.Definition.Name, builder.Definition.Namespace, interfaceName)

	if interfaceName == "" {
		glog.V(100).Infof("Can not redefine BGPPeer with empty interface string")

		builder.errorMsg = "interface can not be empty string"

		return builder
	}

	builder.Definition.Spec.Interface = interfaceName

	return builder
}

// WithDynamicASN defines the dynamicASN as either internal (iBGP) or external (eBGP). Both remoteAS and dynamicASN
// configure the remote ASN. They are mutually exclusive and only one can be used per remote peer.
func (builder *BGPPeerBuilder) WithDynamicASN(dynamicASN mlbtypesv1beta2.DynamicASNMode) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s using a dynamicASN: %s",
		builder.Definition.Name, builder.Definition.Namespace, dynamicASN)

	if dynamicASN != "internal" && dynamicASN != "external" {
		glog.V(100).Infof("The dynamicASN of the BGPPeer is incorrect")

		builder.errorMsg = "bgpPeer 'dynamicASN' must be either internal or external"

		return builder
	}

	builder.Definition.Spec.ASN = 0
	builder.Definition.Spec.DynamicASN = dynamicASN

	return builder
}

// WithRouterID defines the routerID placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithRouterID(routerID string) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this routerID: %s",
		builder.Definition.Name, builder.Definition.Namespace, routerID)

	if net.ParseIP(routerID) == nil {
		glog.V(100).Infof("The routerID of the BGPPeer contains invalid ip address %s, "+
			"routerID should be present in ip address format", routerID)

		builder.errorMsg = fmt.Sprintf("the routerID of the BGPPeer contains invalid ip address %s", routerID)

		return builder
	}

	builder.Definition.Spec.RouterID = routerID

	return builder
}

// WithBFDProfile defines the bfdProfile placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithBFDProfile(bfdProfile string) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this bfdProfile: %s",
		builder.Definition.Name, builder.Definition.Namespace, bfdProfile)

	if bfdProfile == "" {
		glog.V(100).Infof("The bfdProfile of the BGPPeer can not be empty string")

		builder.errorMsg = "The bfdProfile is empty string"

		return builder
	}

	builder.Definition.Spec.BFDProfile = bfdProfile

	return builder
}

// WithSRCAddress defines the SRCAddress placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithSRCAddress(srcAddress string) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this srcAddress: %s",
		builder.Definition.Name, builder.Definition.Namespace, srcAddress)

	if net.ParseIP(srcAddress) == nil {
		glog.V(100).Infof("The srcAddress of the BGPPeer contains invalid ip address %s, "+
			"srcAddress should be present in ip address format", srcAddress)

		builder.errorMsg = fmt.Sprintf("the srcAddress of the BGPPeer contains invalid ip address %s", srcAddress)

		return builder
	}

	builder.Definition.Spec.SrcAddress = srcAddress

	return builder
}

// WithPort defines the port placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithPort(port uint16) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this port: %d",
		builder.Definition.Name, builder.Definition.Namespace, port)

	builder.Definition.Spec.Port = port

	return builder
}

// WithHoldTime defines the holdTime placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithHoldTime(holdTime metav1.Duration) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this holdTime: %s",
		builder.Definition.Name, builder.Definition.Namespace, holdTime)

	builder.Definition.Spec.HoldTime = &holdTime

	return builder
}

// WithKeepalive defines the keepAliveTime placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithKeepalive(keepalive metav1.Duration) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this keepalive: %s",
		builder.Definition.Name, builder.Definition.Namespace, keepalive)

	builder.Definition.Spec.KeepaliveTime = &keepalive

	return builder
}

// WithConnectTime defines the reconnect timer between BGP neighbors.
func (builder *BGPPeerBuilder) WithConnectTime(connectTime metav1.Duration) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this connectTime: %s",
		builder.Definition.Name, builder.Definition.Namespace, connectTime)

	duration := connectTime.Duration

	if duration < time.Second || duration > 65535*time.Second {
		glog.V(100).Infof("A valid connect time is between 1-65535")

		builder.errorMsg = "bgppeer 'connectTime' value is not valid"

		return builder
	}

	builder.Definition.Spec.ConnectTime = &connectTime

	return builder
}

// WithNodeSelector defines the nodeSelector placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithNodeSelector(nodeSelector map[string]string) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this nodeSelector: %s",
		builder.Definition.Name, builder.Definition.Namespace, nodeSelector)

	if len(nodeSelector) == 0 {
		glog.V(100).Infof("Can not redefine BGPPeer with empty nodeSelector map")

		builder.errorMsg = "BGPPeer 'nodeSelector' cannot be empty map"

		return builder
	}

	builder.Definition.Spec.NodeSelectors = []metav1.LabelSelector{{
		MatchLabels: nodeSelector,
	}}

	return builder
}

// WithPassword defines the password placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithPassword(password string) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this password: %s",
		builder.Definition.Name, builder.Definition.Namespace, password)

	if password == "" {
		glog.V(100).Infof("Can not redefine BGPPeer with empty password")

		builder.errorMsg = "password can not be empty string"

		return builder
	}

	builder.Definition.Spec.Password = password

	return builder
}

// WithEBGPMultiHop defines the EBGPMultiHop bool flag placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithEBGPMultiHop(eBGPMultiHop bool) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this eBGPMultiHop flag: %t",
		builder.Definition.Name, builder.Definition.Namespace, eBGPMultiHop)

	builder.Definition.Spec.EBGPMultiHop = eBGPMultiHop

	return builder
}

// WithGracefulRestart defines the EnableGracefulRestart bool flag placed in the BGPPeer spec.
func (builder *BGPPeerBuilder) WithGracefulRestart(gracefulRestart bool) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof(
		"Creating BGPPeer %s in namespace %s with this EnableGracefulRestart flag: %t",
		builder.Definition.Name, builder.Definition.Namespace, gracefulRestart)

	builder.Definition.Spec.EnableGracefulRestart = gracefulRestart

	return builder
}

// WithDisableMP disables Multiprotocol BGP for this peer.
func (builder *BGPPeerBuilder) WithDisableMP(disableMP bool) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting BGPPeer %s disableMP to %t", builder.Definition.Name, disableMP)

	builder.Definition.Spec.DisableMP = disableMP

	return builder
}

// WithDualStackAddressFamily enables dual stack address family for this BGP peer.
// This allows to advertise/receive IPv4 prefixes over IPv6 sessions and vice versa.
func (builder *BGPPeerBuilder) WithDualStackAddressFamily(dualStackAddressFamily bool) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting BGPPeer %s dualStackAddressFamily to %t", builder.Definition.Name, dualStackAddressFamily)

	builder.Definition.Spec.DualStackAddressFamily = dualStackAddressFamily

	return builder
}

// WithOptions creates BGPPeer with generic mutation options.
func (builder *BGPPeerBuilder) WithOptions(options ...BGPPeerAdditionalOptions) *BGPPeerBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting BGPPeer additional options")

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

// GetBGPPeerGVR returns bgppeer's GroupVersionResource which could be used for Clean function.
func GetBGPPeerGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: APIGroup, Version: APIVersion, Resource: "bgppeers",
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *BGPPeerBuilder) validate() (bool, error) {
	resourceCRD := "BGPPeer"

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
