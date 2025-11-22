package profiles

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PtpProfileType enumerates the supported types of profiles.
type PtpProfileType int

const (
	// ProfileTypeOC refers to a PTP profile with a single interface set to client only. It is an ordinary clock
	// profile.
	ProfileTypeOC PtpProfileType = iota
	// ProfileTypeTwoPortOC refers to a PTP profile with two interfaces set to client only. Only one of these
	// interfaces will be active at a time.
	ProfileTypeTwoPortOC
	// ProfileTypeBC refers to a PTP profile in a boundary clock configuration, i.e., one client interface and one
	// server interface.
	ProfileTypeBC
	// ProfileTypeHA refers to a PTP profile that does not correspond to individual interfaces but indicates other
	// profiles are in a highly available configuration.
	ProfileTypeHA
	// ProfileTypeGM refers to a PTP profile for one NIC with all interfaces set to server only.
	ProfileTypeGM
	// ProfileTypeMultiNICGM refers to a PTP profile for multiple NICs where all interfaces are set to server only.
	// SMA cables are used to synchronize the NICs so they can all act as grand masters.
	ProfileTypeMultiNICGM
)

// PtpClockType enumerates the roles of each interface. It is different from the roles in metrics, which include extra
// runtime values not represented in the profile. The zero value is a client and only serverOnly (or masterOnly) values
// of 1 indicate a server.
type PtpClockType int

const (
	// ClockTypeClient indicates an interface is acting as a follower of time signals. Formerly slave.
	ClockTypeClient PtpClockType = iota
	// ClockTypeServer indicates an interface is acting as a leader of time signals. Formerly master.
	ClockTypeServer
)

// ProfileReference contains the information needed to identify a profile on a cluster.
type ProfileReference struct {
	// ConfigReference is the reference to the PtpConfig object that contains the profile.
	ConfigReference runtimeclient.ObjectKey
	// ProfileIndex is the index of the profile in the PtpConfig object.
	ProfileIndex int
	// ProfileName is the name of the profile. It is not necessary to get the profile directly, but is used as a key
	// when recommending profiles to nodes.
	ProfileName string
}

// PullPtpConfig pulls the PTP config for the profile referenced by this struct.
func (reference *ProfileReference) PullPtpConfig(client *clients.Settings) (*ptp.PtpConfigBuilder, error) {
	ptpConfig, err := ptp.PullPtpConfig(client, reference.ConfigReference.Name, reference.ConfigReference.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get PTP config for reference %v: %w", reference, err)
	}

	return ptpConfig, nil
}

// ProfileInfo contains information about a PTP profile. Since profiles can be readily retrieved from the cluster, it
// only contains information that must be parsed and a reference to the profile on the cluster.
type ProfileInfo struct {
	ProfileType PtpProfileType
	Reference   ProfileReference
	// Interfaces is a map of interface names to a struct holding more detailed information. Values should never be
	// nil.
	Interfaces map[iface.Name]*InterfaceInfo
	// ConfigIndex is the number in the config file for the ptp4l corresponding to this profile. Profiles should
	// have a ptp4l process unless they are HA.
	ConfigIndex *uint
}

// GetInterfacesByClockType returns a slice of InterfaceInfo pointers for each interface in the profile matching the
// provided clockType. Elements are guaranteed not to be nil.
func (profileInfo *ProfileInfo) GetInterfacesByClockType(clockType PtpClockType) []*InterfaceInfo {
	var interfaces []*InterfaceInfo

	for _, interfaceInfo := range profileInfo.Interfaces {
		if interfaceInfo.ClockType == clockType {
			interfaces = append(interfaces, interfaceInfo)
		}
	}

	return interfaces
}

// Clone creates a deep copy of the ProfileInfo instance, including all nested InterfaceInfo structs. This ensures that
// modifications to the cloned ProfileInfo do not affect the original.
func (profileInfo *ProfileInfo) Clone() *ProfileInfo {
	clone := &ProfileInfo{
		ProfileType: profileInfo.ProfileType,
		Reference:   profileInfo.Reference,
		Interfaces:  make(map[iface.Name]*InterfaceInfo),
	}

	if profileInfo.ConfigIndex != nil {
		clone.ConfigIndex = new(uint)
		*clone.ConfigIndex = *profileInfo.ConfigIndex
	}

	for name, interfaceInfo := range profileInfo.Interfaces {
		clone.Interfaces[name] = interfaceInfo.Clone()
	}

	return clone
}

// InterfaceInfo contains information about the PTP clock type of an interface. In the future, it may also contain
// information about which interface it is connected to.
type InterfaceInfo struct {
	Name               iface.Name
	ClockType          PtpClockType
	PortIdentity       string
	ParentPortIdentity string
}

// Clone creates a deep copy of the InterfaceInfo instance.
func (interfaceInfo *InterfaceInfo) Clone() *InterfaceInfo {
	return &InterfaceInfo{
		Name:               interfaceInfo.Name,
		ClockType:          interfaceInfo.ClockType,
		PortIdentity:       interfaceInfo.PortIdentity,
		ParentPortIdentity: interfaceInfo.ParentPortIdentity,
	}
}

// GetInterfacesNames returns a slice of interface names for the provided slice of InterfaceInfo pointers.
func GetInterfacesNames(interfaces []*InterfaceInfo) []iface.Name {
	names := make([]iface.Name, 0, len(interfaces))

	for _, interfaceInfo := range interfaces {
		names = append(names, interfaceInfo.Name)
	}

	return names
}

// ProfileCounts records the number of profiles of each type. It is provided as a map rather than a struct to allow
// indexing using the profile type.
type ProfileCounts map[PtpProfileType]uint

// NodeInfo contains all the PTP config-related information for a single node. Common operations are provided as methods
// on this type to avoid the need to aggregate and query nested data.
type NodeInfo struct {
	// Name is the name of the node resource this struct is associated to.
	Name string
	// Counts records the number of each profile type recommended to this node. It will never be nil when this
	// struct is returned from a function in this package.
	Counts ProfileCounts
	// Profiles contains a list of information structs corresponding to each profile that is recommended to this
	// node. Elements should never be nil.
	Profiles []*ProfileInfo
}

// GetInterfacesByClockType returns a slice of InterfaceInfo pointers for each interface across all profiles on this
// node matching the provided clockType. Elements are guaranteed not to be nil.
func (nodeInfo *NodeInfo) GetInterfacesByClockType(clockType PtpClockType) []*InterfaceInfo {
	var nodeInterfaces []*InterfaceInfo

	for _, profileInfo := range nodeInfo.Profiles {
		nodeInterfaces = append(nodeInterfaces, profileInfo.GetInterfacesByClockType(clockType)...)
	}

	return nodeInterfaces
}

// GetProfilesByType returns a slice of ProfileInfo pointers for each profile on this node matching the provided
// profileType. Elements are guaranteed not to be nil.
func (nodeInfo *NodeInfo) GetProfilesByType(profileType PtpProfileType) []*ProfileInfo {
	var nodeProfiles []*ProfileInfo

	for _, profileInfo := range nodeInfo.Profiles {
		if profileInfo.ProfileType == profileType {
			nodeProfiles = append(nodeProfiles, profileInfo)
		}
	}

	return nodeProfiles
}

// GetProfileByName returns the ProfileInfo for the profile with the provided name. It returns nil if no profile is
// found.
func (nodeInfo *NodeInfo) GetProfileByName(name string) *ProfileInfo {
	for _, profileInfo := range nodeInfo.Profiles {
		if profileInfo.Reference.ProfileName == name {
			return profileInfo
		}
	}

	return nil
}

// GetProfileByConfigPath returns the ProfileInfo for the profile with the provided config path. The config path will be
// relative to /var/run, so it should be something like ptp4l.0.config. This function makes the assumption that the
// first line of the file contains the profile name.
func (nodeInfo *NodeInfo) GetProfileByConfigPath(
	client *clients.Settings, nodeName string, path string) (*ProfileInfo, error) {
	// The config file will begin with something like:
	//  #profile: slave1
	command := fmt.Sprintf("cat /var/run/%s | head -1 | cut -d' ' -f2", path)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile by config path %s on node %s: %w", path, nodeName, err)
	}

	profileName := strings.TrimSpace(output)
	profile := nodeInfo.GetProfileByName(profileName)

	if profile == nil {
		return nil, fmt.Errorf("profile %s not found on node %s", profileName, nodeInfo.Name)
	}

	return profile, nil
}
