package metallb

import (
	"context"
	"fmt"
	"net"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	frrtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/frrtypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// FrrConfigurationBuilder provides struct for the FrrConfiguration object containing connection to
// the cluster and the FrrConfiguration definitions.
type FrrConfigurationBuilder struct {
	Definition *frrtypes.FRRConfiguration
	Object     *frrtypes.FRRConfiguration
	apiClient  runtimeClient.Client
	errorMsg   string
}

// NewFrrConfigurationBuilder creates a new instance of FRRConfiguration.
func NewFrrConfigurationBuilder(
	apiClient *clients.Settings, name, nsname string) *FrrConfigurationBuilder {
	klog.V(100).Infof(
		"Initializing new Frrconfiguration structure with the following params: %s, %s",
		name, nsname)

	if apiClient == nil {
		klog.V(100).Info("failed to initialize the apiclient is empty")

		return nil
	}

	err := apiClient.AttachScheme(frrtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add metallb scheme to client schemes")

		return nil
	}

	builder := &FrrConfigurationBuilder{
		apiClient: apiClient,
		Definition: &frrtypes.FRRConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the frrConfiguration is empty")

		builder.errorMsg = "frrConfiguration 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the frrConfiguration is empty")

		builder.errorMsg = "frrConfiguration 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Create makes a FrrConfiguration in the cluster and stores the created object in struct.
func (builder *FrrConfigurationBuilder) Create() (*FrrConfigurationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the frrconfiguration %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			klog.V(100).Info("Failed to create MetalLb")

			return nil, err
		}

		builder.Object = builder.Definition
	}

	return builder, nil
}

// Get returns FRRConfiguration object if found.
func (builder *FrrConfigurationBuilder) Get() (*frrtypes.FRRConfiguration, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Collecting FRRConfiguration object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	frrConfig := &frrtypes.FRRConfiguration{}

	err := builder.apiClient.Get(context.TODO(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace}, frrConfig)
	if err != nil {
		klog.V(100).Infof(
			"metallb object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return frrConfig, nil
}

// Delete removes FrrConfigurationBuilder object from a cluster.
func (builder *FrrConfigurationBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the FrrConfiguration object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof(
			"frrConfiguration %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete frrConfiguration: %w", err)
	}

	builder.Object = nil

	return nil
}

// GetFrrConfigurationGVR returns FrrConfiguration GroupVersionResource which could be used for Clean function.
func GetFrrConfigurationGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group: FRRAPIGroup, Version: APIVersion, Resource: "frrconfigurations",
	}
}

// Exists checks whether the given FrrConfiguration exists.
func (builder *FrrConfigurationBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if FrrConfiguration %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithBGPRouter defines a BGP router as localASN.
func (builder *FrrConfigurationBuilder) WithBGPRouter(localASN uint32) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Define BGP router with an ASN: %v", localASN)

	// If there are routers, append a new one
	builder.Definition.Spec.BGP.Routers = append(builder.Definition.Spec.BGP.Routers, frrtypes.Router{
		ASN: localASN,
	})

	return builder
}

// WithToReceiveModeAll allows all incoming routes.
func (builder *FrrConfigurationBuilder) WithToReceiveModeAll(routerIndex, neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Allows the FRR nodes to receive all routes in namespace %s with mode type: all",
		builder.Definition.Namespace)

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	// Update the ToReceive mode to AllowAll for the specified neighbor
	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].ToReceive.Allowed.Mode = frrtypes.AllowAll

	return builder
}

// WithToReceiveModeFiltered allows only specified prefixes to be received.
func (builder *FrrConfigurationBuilder) WithToReceiveModeFiltered(prefixes []string,
	routerIndex, neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Allows the FRR nodes to receive specific routes in namespace %s in mode type: filtered",
		builder.Definition.Namespace)

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	// Validate CIDR prefixes
	for _, prefix := range prefixes {
		if _, _, err := net.ParseCIDR(prefix); err != nil {
			klog.V(100).Infof("the frrConfiguration prefix %s is not a valid CIDR", prefix)
			builder.errorMsg = fmt.Sprintf("the prefix %s is not a valid CIDR", prefix)

			return builder
		}
	}

	// Prepare the list of allowed prefixes
	allowedPrefixes := []frrtypes.PrefixSelector{}

	for _, prefix := range prefixes {
		klog.V(100).Infof("AllowMode 'filtered' receives only routes within the prefix %v", prefix)
		allowedPrefixes = append(allowedPrefixes, frrtypes.PrefixSelector{Prefix: prefix})
	}

	// Update the specific neighbor's ToReceive field
	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].ToReceive.Allowed.Prefixes = allowedPrefixes

	return builder
}

// WithBGPNeighbor defines a single neighbor IP and ASN number.
func (builder *FrrConfigurationBuilder) WithBGPNeighbor(bgpPeerIP string,
	remoteAS uint32, routerIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Define BGP Neighbor %s with peer IP %s and ASN %v",
		builder.Definition.Name, bgpPeerIP, remoteAS)

	if net.ParseIP(bgpPeerIP) == nil {
		klog.V(100).Infof("The peerIP for the frrConfiguration bgp peer contains invalid ip address %s",
			bgpPeerIP)

		builder.errorMsg = "frrConfiguration 'peerIP' of the BGPPeer contains invalid ip address"

		return builder
	}

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors =
		append(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors,
			frrtypes.Neighbor{
				Address: bgpPeerIP,
				ASN:     remoteAS,
			})

	return builder
}

// WithBGPPassword defines the password used between BGP peers to form adjacency and attaches the password using the
// neighbor index from the neighbor list.
func (builder *FrrConfigurationBuilder) WithBGPPassword(bgpPassword string,
	routerIndex, neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Define FRRConfiguration %s in namespace %s with the password: %s",
		builder.Definition.Name, builder.Definition.Namespace, bgpPassword)

	if bgpPassword == "" {
		klog.V(100).Info("The bgpPassword should not be blank")

		builder.errorMsg = fmt.Sprintf("the bgpPassword %s is an empty string", bgpPassword)

		return builder
	}

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	// Set the password for the specified neighbor
	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].Password = bgpPassword

	return builder
}

// WithEBGPMultiHop sets the EBGP multihop setting on the remote BGP peer.
func (builder *FrrConfigurationBuilder) WithEBGPMultiHop(routerIndex, neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Sets the EBGP multihop setting to true in the FrrConfiguration %s",
		builder.Definition.Name)

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].EBGPMultiHop = true

	return builder
}

// WithBGPNeighborDisableMP sets the DisableMP flag for the specified neighbor.
func (builder *FrrConfigurationBuilder) WithBGPNeighborDisableMP(disableMP bool,
	routerIndex, neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting FRRConfiguration %s neighbor disableMP to %t",
		builder.Definition.Name, disableMP)

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	// Set the DisableMP for the specified neighbor
	//nolint:staticcheck // DisableMP is deprecated, keep for backwards compatibility.
	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].DisableMP = disableMP

	return builder
}

// WithBGPNeighborDualStackAddressFamily enables dual stack address family for the specified BGP neighbor.
// This allows to advertise/receive IPv4 prefixes over IPv6 sessions and vice versa.
func (builder *FrrConfigurationBuilder) WithBGPNeighborDualStackAddressFamily(dualStackAddressFamily bool,
	routerIndex, neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Setting FRRConfiguration %s neighbor dualStackAddressFamily to %t",
		builder.Definition.Name, dualStackAddressFamily)

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	// Set the DualStackAddressFamily for the specified neighbor
	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].
		DualStackAddressFamily = dualStackAddressFamily

	return builder
}

// WithHoldTime defines the holdTime placed in the FrrConfiguration spec.
func (builder *FrrConfigurationBuilder) WithHoldTime(holdTime metav1.Duration, routerIndex,
	neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Adding a neighbor specific holdTime in the FrrConfiguration %s with holdTime: %v",
		builder.Definition.Name, holdTime)

	duration := holdTime.Duration

	if duration < time.Second || duration > 65535*time.Second {
		klog.V(100).Info("A valid holdtime is between 1-65535")

		builder.errorMsg = "frrConfiguration 'holdtime' value is not valid"

		return builder
	}

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].HoldTime = &holdTime

	return builder
}

// WithKeepalive defines the keepAliveTime placed in the FrrConfiguration spec.
func (builder *FrrConfigurationBuilder) WithKeepalive(keepAlive metav1.Duration, routerIndex,
	neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Add a keepAlive timer to a specific neighbor in FrrConfiguration %s with keepalive: %v",
		builder.Definition.Name, keepAlive)

	duration := keepAlive.Duration

	if duration < time.Second || duration > 65535*time.Second {
		klog.V(100).Info("A valid keepAlive time is between 1-65535")

		builder.errorMsg = "frrConfiguration 'keepAlive' value is not valid"

		return builder
	}

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].KeepaliveTime = &keepAlive

	return builder
}

// WithConnectTime defines the time to wait until trying to connect to a BGP peer.
func (builder *FrrConfigurationBuilder) WithConnectTime(connectTime metav1.Duration, routerIndex,
	neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Defines a connect retry hold time for a specific neighbor in FrrConfiguration %s with holdTime %s ",
		builder.Definition.Name, connectTime)

	duration := connectTime.Duration

	if duration < time.Second || duration > 65535*time.Second {
		klog.V(100).Info("A valid connect time is between 1-65535")

		builder.errorMsg = "frrConfiguration 'connectTime' value is not valid"

		return builder
	}

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].ConnectTime = &connectTime

	return builder
}

// WithPort defines the tcp port used to make BGP adjacency.
func (builder *FrrConfigurationBuilder) WithPort(port uint16, routerIndex,
	neighborIndex uint) *FrrConfigurationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Defines the tcp port to establish adjaceny to a neighbor in FrrConfiguration %s port number %d",
		builder.Definition.Name, port)

	if port > 16384 {
		klog.V(100).Infof("Invalid port number can not be greater then 16384: %d", port)

		builder.errorMsg = fmt.Sprintf("invalid port number: %d", port)

		return builder
	}

	// Check if the routerIndex is within bounds
	if routerIndex >= uint(len(builder.Definition.Spec.BGP.Routers)) {
		klog.V(100).Infof("Invalid routerIndex: %d", routerIndex)

		builder.errorMsg = fmt.Sprintf("invalid routerIndex: %d", routerIndex)

		return builder
	}

	// Check if the neighborIndex is within bounds
	if neighborIndex >= uint(len(builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors)) {
		klog.V(100).Infof("Invalid neighborIndex: %d", neighborIndex)

		builder.errorMsg = fmt.Sprintf("invalid neighborIndex: %d", neighborIndex)

		return builder
	}

	builder.Definition.Spec.BGP.Routers[routerIndex].Neighbors[neighborIndex].Port = &port

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *FrrConfigurationBuilder) validate() (bool, error) {
	resourceCRD := "FRRConfiguration"

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
		klog.V(100).Infof("the %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
