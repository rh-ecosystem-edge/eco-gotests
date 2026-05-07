package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/openshift-kni/lifecycle-agent/internal/common"
	"github.com/samber/lo"
)

// NmAddr represents an IP address in nmstate output.
type NmAddr struct {
	IP           string `json:"ip"`
	PrefixLength int    `json:"prefix-length"`
}

// NmIPConf represents IPv4/IPv6 configuration for an interface.
type NmIPConf struct {
	Enabled  bool     `json:"enabled"`
	DHCP     bool     `json:"dhcp"`
	Autoconf bool     `json:"autoconf,omitempty"`
	Address  []NmAddr `json:"address"`
}

// NmIf represents a network interface in nmstate output.
type NmIf struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	IPv4   NmIPConf `json:"ipv4"`
	IPv6   NmIPConf `json:"ipv6"`
	Bridge NmBridge `json:"bridge,omitempty"`
	VLAN   *NmVLAN  `json:"vlan,omitempty"`
}

// NmRoute represents a route entry in nmstate output.
type NmRoute struct {
	Destination      string `json:"destination"`
	NextHopAddress   string `json:"next-hop-address"`
	NextHopInterface string `json:"next-hop-interface"`
}

// NmRoutes represents running and configured routes in nmstate output.
type NmRoutes struct {
	Running []NmRoute `json:"running"`
	Config  []NmRoute `json:"config"`
}

// NmDNSList represents a list of DNS servers in nmstate output.
type NmDNSList struct {
	Server []string `json:"server"`
}

// NmDNS represents DNS resolver configuration in nmstate output.
type NmDNS struct {
	Running NmDNSList `json:"running"`
	Config  NmDNSList `json:"config"`
}

// NmState is the top-level nmstate JSON structure.
type NmState struct {
	Interfaces  []NmIf   `json:"interfaces"`
	Routes      NmRoutes `json:"routes"`
	DNSResolver NmDNS    `json:"dns-resolver"`
}

// NmBridge represents bridge configuration for an interface.
type NmBridge struct {
	Port []struct {
		Name string `json:"name"`
	} `json:"port"`
}

// NmVLAN represents VLAN configuration for an interface.
type NmVLAN struct {
	BaseIface string `json:"base-iface"`
	ID        int    `json:"id"`
}

// ParseNmstate parses nmstate JSON output into NmState.
func ParseNmstate(output string) (NmState, error) {
	var state NmState
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		return state, fmt.Errorf("failed to parse nmstate JSON: %w", err)
	}
	return state, nil
}

// ExtractDNSServers returns the ordered DNS server list from nmstate
func ExtractDNSServers(state NmState) []string {
	return state.DNSResolver.Running.Server
}

func findDefaultGateway(state NmState, bridgeName, destination string) *string {
	for _, rt := range state.Routes.Running {
		if rt.Destination == destination && rt.NextHopInterface == bridgeName {
			return &rt.NextHopAddress
		}
	}

	return nil
}

// FindDefaultGateways searches nmstate routes to find default IPv4 and IPv6 gateways
// for the given bridge name and default route destinations.
func FindDefaultGateways(state NmState, bridgeName, defaultRouteV4, defaultRouteV6 string) (string, string) {
	gw4 := findDefaultGateway(state, bridgeName, defaultRouteV4)
	gw6 := findDefaultGateway(state, bridgeName, defaultRouteV6)

	return lo.FromPtr(gw4), lo.FromPtr(gw6)
}

// ExtractBrExVLANID inspects the bridge uplink port; if it's a VLAN interface, returns its VLAN ID.
func ExtractBrExVLANID(state NmState) (*int, error) {
	uplink, err := ExtractBrExUplinkName(state)
	if err != nil {
		return nil, err
	}

	for _, intf := range state.Interfaces {
		if intf.Name == uplink && intf.Type == "vlan" &&
			intf.VLAN != nil {
			return &intf.VLAN.ID, nil
		}
	}

	return nil, nil
}

// ExtractBrExUplinkName returns the uplink port name connected to the given bridge
// (excluding the bridge internal and patch ports).
func ExtractBrExUplinkName(state NmState) (string, error) {
	bridgeIf, err := getBrExBridgeInterface(state)
	if err != nil {
		return "", err
	}

	for _, p := range bridgeIf.Bridge.Port {
		if !strings.Contains(p.Name, "patch") &&
			p.Name != common.OvsBridgeExternalName {
			return p.Name, nil
		}
	}

	return "", fmt.Errorf(
		"%s uplink interface not found",
		common.OvsBridgeExternalName,
	)
}

func getBrExBridgeInterface(state NmState) (*NmIf, error) {
	for _, intf := range state.Interfaces {
		if intf.Name == common.OvsBridgeExternalName &&
			intf.Type == common.OvsBridgeInterfaceType {
			return &intf, nil
		}
	}

	return nil, fmt.Errorf(
		"%s bridge interface not found",
		common.OvsBridgeExternalName,
	)
}

func GetBrExInterface(state NmState) (*NmIf, error) {
	for _, intf := range state.Interfaces {
		if intf.Name == common.OvsBridgeExternalName &&
			intf.Type == common.OvsInterfaceType {
			return &intf, nil
		}
	}

	return nil, fmt.Errorf(
		"%s interface not found",
		common.OvsBridgeExternalName,
	)
}

// FindMatchingCIDR returns the first CIDR from the list that matches the given IP's
// IP family (IPv4 vs IPv6). If none is found, returns an empty string.
func FindMatchingCIDR(ipStr string, cidrs []string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}
	isV4 := ip.To4() != nil
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil || n == nil {
			continue
		}
		if (n.IP.To4() != nil) != isV4 {
			continue
		}
		return c
	}
	return ""
}
