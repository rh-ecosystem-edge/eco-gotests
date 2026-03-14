package frrconfig

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"text/template"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

// DefineBaseConfig creates a map of strings for the frr configuration.
func DefineBaseConfig(daemonsConfig, frrConfig, vtyShConfig string) map[string]string {
	configMapData := make(map[string]string)
	configMapData["daemons"] = daemonsConfig
	configMapData["frr.conf"] = frrConfig
	configMapData["vtysh.conf"] = vtyShConfig

	return configMapData
}

// CreateStaticIPAnnotations creates a static ip annotation used together with the nad in a pod for IP configuration.
func CreateStaticIPAnnotations(internalNADName, externalNADName string, internalIPAddresses,
	externalIPAddresses []string) []*types.NetworkSelectionElement {
	ipAnnotation := pod.StaticIPAnnotation(internalNADName, internalIPAddresses)
	ipAnnotation = append(ipAnnotation,
		pod.StaticIPAnnotation(externalNADName, externalIPAddresses)...)

	return ipAnnotation
}

// DaemonsParams holds the customizable parameters for the daemons file.
// Only Bgpd, Bfdd, and BGPPort are exposed; all other options are fixed.
type DaemonsParams struct {
	// Bgpd enables/disables the BGP daemon: "yes" or "no"
	Bgpd string
	// Bfdd enables/disables the BFD daemon: "yes" or "no"
	Bfdd string
	// BGPPort is the port for bgpd to listen on (used in bgpd_options: -A 127.0.0.1 -p <port>)
	BGPPort int
}

// WithBGPPort updates the BGP port for the daemons config.
func (params *DaemonsParams) WithBGPPort(port int) *DaemonsParams {
	params.BGPPort = port

	return params
}

// DefaultDaemonsParams returns the default daemon config (BGP + BFD enabled).
// BGPPort defaults to 179 (standard BGP port).
func DefaultDaemonsParams() *DaemonsParams {
	return &DaemonsParams{
		Bgpd:    "yes",
		Bfdd:    "yes",
		BGPPort: 179,
	}
}

// DefaultDaemonsConfig returns the default daemons config.
func DefaultDaemonsConfig() (string, error) {
	return RenderDaemonsWith(DefaultDaemonsParams())
}

// DaemonsConfigWithBGPPort returns the daemons config with the given BGP port.
func DaemonsConfigWithBGPPort(port int) (string, error) {
	return RenderDaemonsWith(&DaemonsParams{
		Bgpd:    "yes",
		Bfdd:    "yes",
		BGPPort: port,
	})
}

// RenderDaemonsWith renders the daemons config from template.
func RenderDaemonsWith(params *DaemonsParams) (string, error) {
	tmpl, err := template.New("daemons").Parse(daemonsTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse daemons template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute daemons template: %w", err)
	}

	return buf.String(), nil
}

// BGPNeighbor represents a BGP neighbor for the FRR config.
type BGPNeighbor struct {
	// Address is the neighbor IP address.
	Address string
	// RemoteAS is the remote BGP AS number for this neighbor.
	RemoteAS int
	// BGPPassword is the BGP session password for this neighbor.
	BGPPassword string
	// EnableBFD enables BFD for this neighbor when true.
	EnableBFD bool
	// EnableMultiHop enables ebgp-multihop 2 for this neighbor when true.
	EnableMultiHop bool
	// ActivateIPv4 when true, activates this neighbor in address-family ipv4 unicast.
	ActivateIPv4 bool
	// ActivateIPv6 when true, activates this neighbor in address-family ipv6 unicast.
	ActivateIPv6 bool
}

// BGPNeighborWithIPv4 returns a BGP neighbor activated only in the IPv4 address family.
func BGPNeighborWithIPv4(addr string, remoteAS int, password string, enableBFD, enableMultiHop bool) BGPNeighbor {
	return BGPNeighborWith(addr, remoteAS, password, enableBFD, enableMultiHop, true, false)
}

// BGPNeighborWithIPv6 returns a BGP neighbor activated only in the IPv6 address family.
func BGPNeighborWithIPv6(addr string, remoteAS int, password string, enableBFD, enableMultiHop bool) BGPNeighbor {
	return BGPNeighborWith(addr, remoteAS, password, enableBFD, enableMultiHop, false, true)
}

// BGPNeighborWithIPv4AndIPv6 returns a BGP neighbor activated in both IPv4 and IPv6 address families.
func BGPNeighborWithIPv4AndIPv6(
	addr string, remoteAS int, password string, enableBFD, enableMultiHop bool) BGPNeighbor {
	return BGPNeighborWith(addr, remoteAS, password, enableBFD, enableMultiHop, true, true)
}

// BGPNeighborWith creates a BGPNeighbor with the given address, remote AS, password, and flags.
func BGPNeighborWith(addr string, remoteAS int, password string, enableBFD, enableMultiHop bool,
	activateIPv4, activateIPv6 bool) BGPNeighbor {
	return BGPNeighbor{
		Address:        addr,
		RemoteAS:       remoteAS,
		BGPPassword:    password,
		EnableBFD:      enableBFD,
		EnableMultiHop: enableMultiHop,
		ActivateIPv4:   activateIPv4,
		ActivateIPv6:   activateIPv6,
	}
}

// BGPUnnumberedConfig holds parameters for BGP unnumbered (interface-based) peering.
// When set on FRRConfigParams, the template renders unnumbered config instead of IP-based neighbors.
type BGPUnnumberedConfig struct {
	// InterfaceName is the interface for unnumbered BGP (e.g. "eth1").
	InterfaceName string
	// RemoteAS is the remote BGP AS number.
	RemoteAS int
	// BGPPassword is the BGP session password.
	BGPPassword string
	// EnableBFD enables BFD for the unnumbered peer.
	EnableBFD bool
	// EnableMultiHop enables ebgp-multihop 2.
	EnableMultiHop bool
	// TimersKeepalive is the BGP keepalive timer in seconds (default: 30).
	TimersKeepalive int
	// TimersHold is the BGP hold timer in seconds (default: 90).
	TimersHold int
}

// StaticRoute represents an FRR static IP route (destination prefix -> nexthop).
type StaticRoute struct {
	// Destination is the route prefix (e.g. "192.168.1.1/32" or "10.0.0.0/24").
	Destination string
	// Nexthop is the gateway IP for the route.
	Nexthop string
	// AddressFamily is "ipv4" or "ipv6" and determines whether FRR renders "ip route" or "ipv6 route".
	AddressFamily string
}

// addressFamilyForDestination returns "ipv4" or "ipv6" by parsing the IP in dest (prefix before "/" if present).
func addressFamilyForDestination(dest string) string {
	if idx := strings.Index(dest, "/"); idx != -1 {
		dest = dest[:idx]
	}

	if ip := net.ParseIP(dest); ip != nil {
		if ip.To4() != nil {
			return "ipv4"
		}

		return "ipv6"
	}

	return "ipv4"
}

// normalizeStaticRouteDestination upgrades a bare host IP to /32 (IPv4) or /128 (IPv6).
// If dest already contains a prefix length or is not a parseable IP, dest is returned unchanged.
func normalizeStaticRouteDestination(dest string) string {
	if !strings.Contains(dest, "/") {
		if ip := net.ParseIP(dest); ip != nil {
			if ip.To4() != nil {
				return dest + "/32"
			}

			return dest + "/128"
		}
	}

	return dest
}

// BuildStaticRoutes builds a slice of StaticRoute from parallel slices of destinations and nexthops.
// It panics if the lengths of the slices are not equal.
func BuildStaticRoutes(destinations, nexthops []string) []StaticRoute {
	routes := []StaticRoute{}

	for idx := 0; idx < len(destinations); idx++ {
		dest := normalizeStaticRouteDestination(destinations[idx])

		routes = append(routes, StaticRoute{
			Destination:   dest,
			Nexthop:       nexthops[idx],
			AddressFamily: addressFamilyForDestination(dest),
		})
	}

	return routes
}

// FRRConfigParams holds the parameters for rendering frr.conf.
type FRRConfigParams struct {
	// RouterID is the BGP router-id (default: "10.10.10.11").
	RouterID string
	// LocalAS is the local BGP AS number.
	LocalAS int
	// Neighbors is the list of BGP neighbors (each has RemoteAS, BGPPassword, EnableBFD, EnableMultiHop).
	// Ignored when Unnumbered is set.
	Neighbors []BGPNeighbor
	// IPv4Networks is the list of IPv4 networks to advertise (network statements in ipv4 unicast).
	IPv4Networks []string
	// IPv6Networks is the list of IPv6 networks to advertise (network statements in ipv6 unicast).
	IPv6Networks []string
	// Unnumbered configures BGP unnumbered (interface-based) peering. When set, Neighbors is ignored.
	Unnumbered *BGPUnnumberedConfig
	// StaticRoutes is the list of static IP routes (ip route <destination> <nexthop>).
	StaticRoutes []StaticRoute
}

// FRRConfigParamsBuilder builds FRRConfigParams using a fluent API.
type FRRConfigParamsBuilder struct {
	params FRRConfigParams
}

// NewFRRConfigParams returns a builder with default values (RouterID: "10.10.10.11").
func NewFRRConfigParams() *FRRConfigParamsBuilder {
	return &FRRConfigParamsBuilder{
		params: FRRConfigParams{
			RouterID:     "10.10.10.11",
			Neighbors:    nil,
			IPv4Networks: nil,
			IPv6Networks: nil,
		},
	}
}

// WithRouterID sets the BGP router-id.
func (b *FRRConfigParamsBuilder) WithRouterID(routerID string) *FRRConfigParamsBuilder {
	b.params.RouterID = routerID

	return b
}

// WithLocalAS sets the local BGP AS number.
func (b *FRRConfigParamsBuilder) WithLocalAS(localAS int) *FRRConfigParamsBuilder {
	b.params.LocalAS = localAS

	return b
}

// WithIPv4BGPNeighbors appends neighbors activated only in IPv4.
func (b *FRRConfigParamsBuilder) WithIPv4BGPNeighbors(addrs []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool) *FRRConfigParamsBuilder {
	for _, addr := range addrs {
		n := BGPNeighborWithIPv4(addr, remoteAS, password, enableBFD, enableMultiHop)
		b.params.Neighbors = append(b.params.Neighbors, n)
	}

	return b
}

// WithIPv6BGPNeighbors appends neighbors activated only in IPv6.
func (b *FRRConfigParamsBuilder) WithIPv6BGPNeighbors(addrs []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool) *FRRConfigParamsBuilder {
	for _, addr := range addrs {
		n := BGPNeighborWithIPv6(addr, remoteAS, password, enableBFD, enableMultiHop)
		b.params.Neighbors = append(b.params.Neighbors, n)
	}

	return b
}

// WithIPv4AndIPv6BGPNeighbors appends neighbors activated in both IPv4 and IPv6.
func (b *FRRConfigParamsBuilder) WithIPv4AndIPv6BGPNeighbors(addrs []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool) *FRRConfigParamsBuilder {
	for _, addr := range addrs {
		n := BGPNeighborWithIPv4AndIPv6(addr, remoteAS, password, enableBFD, enableMultiHop)
		b.params.Neighbors = append(b.params.Neighbors, n)
	}

	return b
}

// WithIPv4Networks sets the IPv4 networks to advertise (replaces any existing).
func (b *FRRConfigParamsBuilder) WithIPv4Networks(networks []string) *FRRConfigParamsBuilder {
	b.params.IPv4Networks = networks

	return b
}

// WithIPv6Networks sets the IPv6 networks to advertise (replaces any existing).
func (b *FRRConfigParamsBuilder) WithIPv6Networks(networks []string) *FRRConfigParamsBuilder {
	b.params.IPv6Networks = networks

	return b
}

// WithUnnumbered sets BGP unnumbered (interface-based) peering. Replaces any Neighbors.
func (b *FRRConfigParamsBuilder) WithUnnumbered(cfg BGPUnnumberedConfig) *FRRConfigParamsBuilder {
	if cfg.TimersKeepalive == 0 {
		cfg.TimersKeepalive = 30
	}

	if cfg.TimersHold == 0 {
		cfg.TimersHold = 90
	}

	b.params.Unnumbered = &cfg
	b.params.Neighbors = nil

	return b
}

// WithStaticRoutes sets the static IP routes (replaces any existing).
func (b *FRRConfigParamsBuilder) WithStaticRoutes(routes []StaticRoute) *FRRConfigParamsBuilder {
	b.params.StaticRoutes = routes

	return b
}

// WithStaticRoute appends a static IP route.
func (b *FRRConfigParamsBuilder) WithStaticRoute(destination, nexthop string) *FRRConfigParamsBuilder {
	dest := normalizeStaticRouteDestination(destination)

	b.params.StaticRoutes = append(b.params.StaticRoutes, StaticRoute{
		Destination:   dest,
		Nexthop:       nexthop,
		AddressFamily: addressFamilyForDestination(dest),
	})

	return b
}

// Build returns the built FRRConfigParams.
func (b *FRRConfigParamsBuilder) Build() FRRConfigParams {
	return b.params
}

// BGPParamsIPv4Family returns params for BGP with neighbors activated in IPv4 only (no network advertising).
func BGPParamsIPv4Family(localAS int, remoteAddresses []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithIPv4BGPNeighbors(remoteAddresses, remoteAS, password, enableBFD, enableMultiHop).
		Build()
}

// BGPParamsIPv4FamilyWithNetworks returns params for BGP with IPv4 neighbors and IPv4 networks to advertise.
func BGPParamsIPv4FamilyWithNetworks(localAS int, remoteAddresses []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool, ipv4Networks []string) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithIPv4BGPNeighbors(remoteAddresses, remoteAS, password, enableBFD, enableMultiHop).
		WithIPv4Networks(ipv4Networks).
		Build()
}

// BGPParamsIPv6Family returns params for BGP with neighbors activated in IPv6 only (no network advertising).
func BGPParamsIPv6Family(localAS int, remoteAddresses []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithIPv6BGPNeighbors(remoteAddresses, remoteAS, password, enableBFD, enableMultiHop).
		Build()
}

// BGPParamsIPv6FamilyWithNetworks returns params for BGP with IPv6 neighbors and IPv6 networks to advertise.
func BGPParamsIPv6FamilyWithNetworks(localAS int, remoteAddresses []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool, ipv6Networks []string) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithIPv6BGPNeighbors(remoteAddresses, remoteAS, password, enableBFD, enableMultiHop).
		WithIPv6Networks(ipv6Networks).
		Build()
}

// BGPParamsDualStackFamily returns params for BGP with all neighbors in both IPv4 and IPv6 address families.
func BGPParamsDualStackFamily(localAS int, remoteAddresses []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithIPv4AndIPv6BGPNeighbors(remoteAddresses, remoteAS, password, enableBFD, enableMultiHop).
		Build()
}

// BGPParamsDualStackFamilyWithNetworks returns params for BGP with dual-stack neighbors and IPv4/IPv6 networks.
func BGPParamsDualStackFamilyWithNetworks(localAS int, remoteAddresses []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool, ipv4Networks, ipv6Networks []string) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithIPv4AndIPv6BGPNeighbors(remoteAddresses, remoteAS, password, enableBFD, enableMultiHop).
		WithIPv4Networks(ipv4Networks).
		WithIPv6Networks(ipv6Networks).
		Build()
}

// BGPParamsDualStackFamilyWithNetworksAndStaticRoutes returns params for dual-stack BGP plus static routes.
func BGPParamsDualStackFamilyWithNetworksAndStaticRoutes(localAS int, remoteAddresses []string, remoteAS int,
	password string, enableBFD, enableMultiHop bool, ipv4Networks, ipv6Networks []string,
	staticRoutes []StaticRoute) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithIPv4AndIPv6BGPNeighbors(remoteAddresses, remoteAS, password, enableBFD, enableMultiHop).
		WithIPv4Networks(ipv4Networks).
		WithIPv6Networks(ipv6Networks).
		WithStaticRoutes(staticRoutes).
		Build()
}

// BGPParamsByAddressFamily returns params for BGP with each neighbor in the AF matching its IP (v4→IPv4, v6→IPv6).
func BGPParamsByAddressFamily(localAS int, remoteAddresses []string, remoteAS int, password string,
	enableBFD, enableMultiHop bool) FRRConfigParams {
	params := NewFRRConfigParams().WithLocalAS(localAS)

	for _, ipAddress := range remoteAddresses {
		if net.ParseIP(ipAddress).To4() == nil {
			params.WithIPv6BGPNeighbors([]string{ipAddress}, remoteAS, password, enableBFD, enableMultiHop)
		} else {
			params.WithIPv4BGPNeighbors([]string{ipAddress}, remoteAS, password, enableBFD, enableMultiHop)
		}
	}

	return params.Build()
}

// BGPParamsForUnnumbered returns params for BGP unnumbered (interface-based) peering.
func BGPParamsForUnnumbered(localAS int, interfaceName string, remoteAS int, password string,
	enableBFD, enableMultiHop bool, ipv4Networks, ipv6Networks []string) FRRConfigParams {
	return NewFRRConfigParams().
		WithLocalAS(localAS).
		WithUnnumbered(BGPUnnumberedConfig{
			InterfaceName:   interfaceName,
			RemoteAS:        remoteAS,
			BGPPassword:     password,
			EnableBFD:       enableBFD,
			EnableMultiHop:  enableMultiHop,
			TimersKeepalive: 30,
			TimersHold:      90,
		}).
		WithIPv4Networks(ipv4Networks).
		WithIPv6Networks(ipv6Networks).
		Build()
}

// RenderFRRConfigWith renders frr.conf from the given FRRConfigParams using the FRR template.
func RenderFRRConfigWith(params FRRConfigParams) (string, error) {
	tmpl, err := template.New("frr").Parse(frrTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse frr template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute frr template: %w", err)
	}

	return buf.String(), nil
}
