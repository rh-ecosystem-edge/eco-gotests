// Package iface provides types and utilities for working with network interface names in the PTP test suite. Additional
// helpers are provided too for working with interfaces on running nodes.
package iface

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
)

// nicNameRegexp is a regular expression that matches the name of a network interface. It is used for generating the NIC
// name from the interface name. It comes from the upstream cloud-event-proxy package.
// https://github.com/redhat-cne/cloud-event-proxy/blob/ca4ced05bcc35e3cfccee41059528a0f315d6c3c/
// plugins/ptp_operator/utils/utils.go#L29.
var nicNameRegexp = regexp.MustCompile(`^(.+?)(\d+)(?:np\d+)?(\..+)?$`)

// Name represents a network interface name. It provides a number of methods for working with interface names and is the
// canonical way to manipulate interface names in this test suite. The zero value is never valid.
type Name string

// GetNIC returns the NIC name associated with the interface. It follows the same system as the upstream
// cloud-event-proxy package when determining the NIC name. See the upstream function at
// https://github.com/redhat-cne/cloud-event-proxy/blob/ca4ced05bcc35e3cfccee41059528a0f315d6c3c/
// plugins/ptp_operator/utils/utils.go#L23.
//
// Since this method is an identity operation for NIC names, it will return the special NIC names [ClockRealtime] and
// [Master] unchanged.
func (iface Name) GetNIC() NICName {
	if len(iface) < 2 {
		glog.V(tsparams.LogLevel).Infof("Failed to get NIC name for interface %q: interface name is too short", iface)

		return ""
	}

	if NICName(iface) == ClockRealtime || NICName(iface) == Master {
		return NICName(iface)
	}

	matches := nicNameRegexp.FindStringSubmatch(string(iface))

	// Even if there is no VLAN part, the matches slice will just have an empty string for the VLAN part, not have
	// it missing.
	if len(matches) >= 4 {
		// matches[1] contains the prefix (everything before the last digit sequence)
		// matches[2] contains the digit sequence to replace
		// matches[3] contains the VLAN part (including the dot) or empty string
		return NICName(fmt.Sprintf("%sx%s", matches[1], matches[3]))
	}

	glog.V(tsparams.LogLevel).Infof(
		"Failed to get NIC name for interface %q: interface name does not match known format", iface)

	return ""
}

// NICName represents a network interface name. It can be derived from [Name] using the [GetNIC] method. The zero value
// is never valid.
type NICName string

const (
	// ClockRealtime is the name of the NIC representing the realtime clock. It is not actually a NIC but appears as
	// one in some PTP metrics.
	ClockRealtime NICName = "CLOCK_REALTIME"
	// Master is the name of the NIC representing the master clock. It is not actually a NIC but appears as one in
	// some PTP metrics.
	Master NICName = "master"
)

// EnsureNIC verifies that the NIC name is actually a NIC name. If it is instead an interface name, it will be converted
// to a NIC name. If the NIC name is invalid, the zero value will be returned.
func (nic NICName) EnsureNIC() NICName {
	withoutVLAN, _, _ := strings.Cut(string(nic), ".")

	// Since we guarantee a zero value is returned on invalid NIC names, we need an extra case for withoutVLAN being
	// empty.
	if withoutVLAN == "" {
		return ""
	}

	if withoutVLAN[len(withoutVLAN)-1] == 'x' {
		return nic
	}

	// Since [GetNIC] handles the case of [ClockRealtime] and [Master], we can just call it here rather than
	// duplicating the special case.
	return Name(nic).GetNIC()
}

// GroupInterfacesByNIC takes a slice of interface names and and returns a map of NIC names to slices of interface
// names. Invalid interface names are ignored, so not all inputs will be necessarily present in the output. The returned
// map is guaranteed to not be nil and each value is guaranteed to have a length of at least 1.
func GroupInterfacesByNIC(ifaces []Name) map[NICName][]Name {
	nicMap := make(map[NICName][]Name)

	for _, iface := range ifaces {
		nic := iface.GetNIC()
		if nic == "" {
			continue
		}

		nicMap[nic] = append(nicMap[nic], iface)
	}

	return nicMap
}

// InterfaceState represents the state of a network interface.
type InterfaceState string

const (
	// InterfaceStateUp represents the up state of an interface.
	InterfaceStateUp InterfaceState = "up"
	// InterfaceStateDown represents the down state of an interface.
	InterfaceStateDown InterfaceState = "down"
)
