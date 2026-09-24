// Package iface provides types and utilities for working with network interface names in the PTP test suite. Additional
// helpers are provided too for working with interfaces on running nodes.
package iface

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

// clusterIfaceAliases holds the cluster wide aggregate of the daemons's iface alias API.
var clusterIfaceAliases = make(map[Iface]Alias)

// useLegacyNICNaming is a flag that indicates whether to use the legacy NIC naming system. It is set to true if the PTP
// version is 4.19- and false otherwise.
var useLegacyNICNaming bool

// InitIfaceAliasing initializes the interface naming system. It checks the PTP version and uses:
// * Legacy aliasing system for 4.19 & lower.
// * Modern aliasing system for 4.20 - 4.21
// * API aliasing system for 4.22 & above.
// It returns an error if the version could not be compared.
func InitIfaceAliasing(ptpVersion string, client *clients.Settings) error {
	atLeast422, err := version.IsVersionStringInRange(ptpVersion, "4.22.0-0", "")
	if err != nil {
		return fmt.Errorf("failed to check if PTP version is at least 4.22: %w", err)
	}

	if atLeast422 {
		nodes, err := nodes.List(client)
		if err != nil {
			return err
		}

		for _, node := range nodes {
			nodeIfaceAliases, err := getPtpAliasesAPI(client, node.Definition.Name)
			if err != nil {
				return err
			}

			for iface, alias := range nodeIfaceAliases {
				// The map doesn't associate the ifaces to nodes, as such a duplicate entry is considered a failure.
				_, ok := clusterIfaceAliases[iface]
				if ok {
					return fmt.Errorf("clusterIfaceAliases duplicate iface %s key, node %s", iface, node.Definition.Name)
				}

				clusterIfaceAliases[iface] = alias
			}
		}

		return nil
	}

	atLeast420, err := version.IsVersionStringInRange(ptpVersion, "4.20.0-0", "")
	if err != nil {
		return fmt.Errorf("failed to check if PTP version is at least 4.20: %w", err)
	}

	useLegacyNICNaming = !atLeast420

	return nil
}

func getPtpAliasesAPI(client *clients.Settings, nodeName string) (map[Iface]Alias, error) {
	daemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(client, nodeName)
	if err != nil {
		return nil, err
	}

	addr, stop, err := daemonPod.PortForward(0, 8081)
	if err != nil {
		return nil, fmt.Errorf("failed to port-forward to PTP daemon pod: %w", err)
	}
	defer stop()

	resp, err := http.Get("http://" + addr + "/port-aliases")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch port aliases: %w", err)
	}
	defer resp.Body.Close()

	portAliases := map[Iface]Alias{}
	if err := json.NewDecoder(resp.Body).Decode(&portAliases); err != nil {
		return nil, fmt.Errorf("failed to decode port aliases: %w", err)
	}

	return portAliases, nil
}

// nicNameRegexp is a regular expression that matches the name of a network interface. It is used for generating the NIC
// name from the interface name. It comes from the upstream cloud-event-proxy package.
// https://github.com/redhat-cne/cloud-event-proxy/blob/ca4ced05bcc35e3cfccee41059528a0f315d6c3c/
// plugins/ptp_operator/utils/utils.go#L29.
var nicNameRegexp = regexp.MustCompile(`^(.+?)(\d+)(?:np\d+)?(\..+)?$`)

// Iface represents a network interface name. It provides a number of methods for working with interface names and is
// the canonical way to manipulate interface names in this test suite. The zero value is never valid.
type Iface string

// GetAlias returns the alias associated with the interface based on InitIfaceAliasing logic.
//
// Since this method is an identity operation for NIC names, it will return the special NIC names [ClockRealtime] and
// [Master] unchanged, regardless of the naming system used.
//
// Interface names shorter than 2 characters are considered invalid and will return the zero value.
func (iface Iface) GetAlias() Alias {
	if len(iface) < 2 {
		klog.V(tsparams.LogLevel).Infof("Failed to get NIC name for interface %q: interface name is too short", iface)

		return ""
	}

	if Alias(iface) == ClockRealtime || Alias(iface) == Master {
		return Alias(iface)
	}

	if len(clusterIfaceAliases) > 0 {
		return iface.getAliasAPI()
	}

	if useLegacyNICNaming {
		return iface.getAliasLegacy()
	}

	return iface.getAliasModern()
}

func (iface Iface) getAliasAPI() Alias {
	return clusterIfaceAliases[iface]
}

// getAliasModern returns the NIC name associated with the interface. It follows the same system as the upstream
// cloud-event-proxy package when determining the NIC name. See the upstream function at
// https://github.com/redhat-cne/cloud-event-proxy/blob/ca4ced05bcc35e3cfccee41059528a0f315d6c3c/
// plugins/ptp_operator/utils/utils.go#L23.
//
// This method assumes that the interface name is longer than 2 characters and not one of the special NIC names.
func (iface Iface) getAliasModern() Alias {
	matches := nicNameRegexp.FindStringSubmatch(string(iface))

	// Even if there is no VLAN part, the matches slice will just have an empty string for the VLAN part, not have
	// it missing.
	if len(matches) >= 4 {
		// matches[1] contains the prefix (everything before the last digit sequence)
		// matches[2] contains the digit sequence to replace
		// matches[3] contains the VLAN part (including the dot) or empty string
		return Alias(fmt.Sprintf("%sx%s", matches[1], matches[3]))
	}

	klog.V(tsparams.LogLevel).Infof(
		"Failed to get NIC name for interface %q: interface name does not match known format", iface)

	return ""
}

// getAliasLegacy returns the NIC name for the interface using the legacy NIC naming system. It is applicable to PTP
// operator versions 4.19 and earlier.
//
// The logic is to simply replace the last digit in the interface name with an 'x'. If the interface name has a VLAN,
// the VLAN is preserved.
//
// This method assumes that the interface name is longer than 2 characters and not one of the special NIC names.
func (iface Iface) getAliasLegacy() Alias {
	withoutVLAN, vlan, hasVLAN := strings.Cut(string(iface), ".")
	withoutVLAN = withoutVLAN[:len(withoutVLAN)-1] + "x"

	if hasVLAN {
		return Alias(fmt.Sprintf("%s.%s", withoutVLAN, vlan))
	}

	return Alias(withoutVLAN)
}

// Alias represents a network interface name. It can be derived from [Iface] using the [GetAlias] method. The zero value
// is never valid.
type Alias string

const (
	// ClockRealtime is the name of the NIC representing the realtime clock. It is not actually a NIC but appears as
	// one in some PTP metrics.
	ClockRealtime Alias = "CLOCK_REALTIME"
	// Master is the name of the NIC representing the master clock. It is not actually a NIC but appears as one in
	// some PTP metrics.
	Master Alias = "master"
)

// EnsureIfaceAlias verifies that the NIC name is actually a NIC name. If it is instead an interface name,
// it will be converted to a NIC name. If the NIC name is invalid, the zero value will be returned.
func (nic Alias) EnsureIfaceAlias() Alias {
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
	return Iface(nic).GetAlias()
}

// GroupInterfacesByNIC takes a slice of interface names and and returns a map of NIC names to slices of interface
// names. Invalid interface names are ignored, so not all inputs will be necessarily present in the output. The returned
// map is guaranteed to not be nil and each value is guaranteed to have a length of at least 1.
func GroupInterfacesByNIC(ifaces []Iface) map[Alias][]Iface {
	nicMap := make(map[Alias][]Iface)

	for _, iface := range ifaces {
		nic := iface.GetAlias()
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
