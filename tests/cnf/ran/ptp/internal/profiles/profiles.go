package profiles

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
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
	// ProfileTypeNTPFallback refers to a PTP profile that is configured to fall back to NTP when GNSS sync is lost.
	// This is the same as a GM profile but with chronyd configured.
	ProfileTypeNTPFallback
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

// PullPtpConfig pulls the PtpConfig for the profile referenced by this struct.
func (reference *ProfileReference) PullPtpConfig(client *clients.Settings) (*ptp.PtpConfigBuilder, error) {
	ptpConfig, err := ptp.PullPtpConfig(client, reference.ConfigReference.Name, reference.ConfigReference.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get PtpConfig for reference %v: %w", reference, err)
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

// PullProfile pulls the PTP profile for the profile referenced by this struct. If error is nil, the profile is
// guaranteed to not be nil.
func (profileInfo *ProfileInfo) PullProfile(client *clients.Settings) (*ptpv1.PtpProfile, error) {
	ptpConfig, err := profileInfo.Reference.PullPtpConfig(client)
	if err != nil {
		return nil, fmt.Errorf("failed to pull PTP config for profile %s: %w", profileInfo.Reference.ProfileName, err)
	}

	profileIndex := profileInfo.Reference.ProfileIndex
	if profileIndex < 0 || profileIndex >= len(ptpConfig.Definition.Spec.Profile) {
		return nil, fmt.Errorf("failed to find profile %s at index %d: index out of bounds",
			profileInfo.Reference.ProfileName, profileIndex)
	}

	return &ptpConfig.Definition.Spec.Profile[profileIndex], nil
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

// errPtp4lConfigNotFound indicates that the expected ptp4l.<index>.config file is not present for a profile. This can
// be expected for profiles that only run phc2sys (e.g., HA profiles).
var errPtp4lConfigNotFound = errors.New("ptp4l config file not found")

// SetPortIdentities sets the PortIdentity and ParentPortIdentity for each interface in the profile by querying the
// linuxptp-daemon pod using pmc. The ConfigIndex must be set before calling this method.
func (profileInfo *ProfileInfo) SetPortIdentities(client *clients.Settings, nodeName string) error {
	if profileInfo.ConfigIndex == nil {
		return fmt.Errorf("cannot set port identities for profile %s: ConfigIndex is not set",
			profileInfo.Reference.ProfileName)
	}

	configPath := fmt.Sprintf("/var/run/ptp4l.%d.config", *profileInfo.ConfigIndex)

	configExists, err := ptp4lConfigExists(client, nodeName, configPath)
	if err != nil {
		return fmt.Errorf("failed to check ptp4l config for profile %s on node %s: %w",
			profileInfo.Reference.ProfileName, nodeName, err)
	}

	if !configExists {
		return fmt.Errorf("ptp4l config %s not found for profile %s on node %s: %w",
			configPath, profileInfo.Reference.ProfileName, nodeName, errPtp4lConfigNotFound)
	}

	ifaceToPortIdentity, err := getPortIdentitiesFromPMC(client, nodeName, configPath)
	if err != nil {
		return fmt.Errorf("failed to get port identities for profile %s on node %s: %w",
			profileInfo.Reference.ProfileName, nodeName, err)
	}

	portToParentIdentity, err := getParentPortIdentitiesFromPMC(client, nodeName, configPath)
	if err != nil {
		return fmt.Errorf("failed to get parent port identities for profile %s on node %s: %w",
			profileInfo.Reference.ProfileName, nodeName, err)
	}

	for ifaceName, interfaceInfo := range profileInfo.Interfaces {
		portIdentity, portIdentityFound := ifaceToPortIdentity[ifaceName]
		if !portIdentityFound {
			return fmt.Errorf("port identity not found for interface %s in profile %s on node %s",
				ifaceName, profileInfo.Reference.ProfileName, nodeName)
		}

		interfaceInfo.PortIdentity = portIdentity

		// Look up the parent port identity using the clock identity portion of the port identity. Port
		// identities have the format "clockIdentity-portNumber" (e.g., "507c6f.fffe.5c4c82-1"). The
		// PARENT_DATA_SET response uses "-0" suffix for the clock identity.
		//
		// This does mean that the LISTENING follower port will have an incorrect parent port identity. These
		// tests assume this is not an important detail.
		clockIdentity := getClockIdentityFromPortIdentity(portIdentity)

		parentPortIdentity, parentPortIdentityFound := portToParentIdentity[clockIdentity]
		if !parentPortIdentityFound {
			return fmt.Errorf("parent port identity not found for clock %s (interface %s) in profile %s on node %s",
				clockIdentity, ifaceName, profileInfo.Reference.ProfileName, nodeName)
		}

		interfaceInfo.ParentPortIdentity = parentPortIdentity
	}

	return nil
}

// ptp4lConfigExists checks whether the ptp4l config file exists in /var/run for the provided node.
func ptp4lConfigExists(client *clients.Settings, nodeName, configPath string) (bool, error) {
	command := fmt.Sprintf(`if [ -f %q ]; then echo "true"; else echo "false"; fi`, configPath)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnEmptyOutput(true), ptpdaemon.WithRetryOnError(true))
	if err != nil {
		return false, err
	}

	switch strings.TrimSpace(output) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected output while checking ptp4l config existence: %q", output)
	}
}

// getClockIdentityFromPortIdentity extracts the clock identity from a port identity by replacing the port number suffix
// with "-0". For example, "507c6f.fffe.5c4c82-1" becomes "507c6f.fffe.5c4c82-0".
func getClockIdentityFromPortIdentity(portIdentity string) string {
	lastDash := strings.LastIndex(portIdentity, "-")
	if lastDash == -1 {
		return portIdentity + "-0"
	}

	return portIdentity[:lastDash] + "-0"
}

// getPortIdentitiesFromPMC queries the pmc tool for port properties and returns a map of interface names to their port
// identities.
func getPortIdentitiesFromPMC(
	client *clients.Settings, nodeName, configPath string) (map[iface.Name]string, error) {
	command := fmt.Sprintf(`pmc -u -b 0 -f %s "GET PORT_PROPERTIES_NP"`, configPath)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return nil, fmt.Errorf("failed to execute pmc command: %w", err)
	}

	return parsePortPropertiesOutput(output)
}

// portPropertiesRegex matches each PORT_PROPERTIES_NP block in the pmc output. It captures the portIdentity and
// interface name from lines like:
//
//	portIdentity            507c6f.fffe.5c4c88-1
//	interface               ens2f0
var portPropertiesRegex = regexp.MustCompile(
	`(?m)^\s*portIdentity\s+(\S+).*\n(?:.*\n)*?\s*interface\s+(\S+)`)

// parsePortPropertiesOutput parses the output of the pmc GET PORT_PROPERTIES_NP command and returns a map of interface
// names to their port identities.
func parsePortPropertiesOutput(output string) (map[iface.Name]string, error) {
	matches := portPropertiesRegex.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no port identities found in pmc output")
	}

	portIdentities := make(map[iface.Name]string, len(matches))

	for _, match := range matches {
		portIdentity := match[1]
		ifaceName := iface.Name(match[2])
		portIdentities[ifaceName] = portIdentity
	}

	return portIdentities, nil
}

// getParentPortIdentitiesFromPMC queries the pmc tool for parent data set and returns a map of clock identities (port
// identity with "-0" suffix) to their parent port identities.
func getParentPortIdentitiesFromPMC(client *clients.Settings, nodeName, configPath string) (map[string]string, error) {
	command := fmt.Sprintf(`pmc -u -b 0 -f %s "GET PARENT_DATA_SET"`, configPath)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true), ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return nil, fmt.Errorf("failed to execute pmc command: %w", err)
	}

	return parseParentDataSetOutput(output)
}

// parentDataSetRegex matches each PARENT_DATA_SET block in the pmc output. It captures the responding port identity
// from the header line and the parentPortIdentity field. For example:
//
//	507c6f.fffe.5c4c82-0 seq 0 RESPONSE MANAGEMENT PARENT_DATA_SET
//	        parentPortIdentity                    208810.ffff.151f00-8
//
// It is important to note that the parent data set only returns the parent port identity for the active follower. For
// dual follower configurations. As a result, the header line always has a port identity of "-0", not the true port
// identity of the follower.
//
// If one were to want the parent port identity for the LISTENING follower, one would need to grep the logs for the "new
// foreign master" message on startup.
var parentDataSetRegex = regexp.MustCompile(
	`(?m)^\s*(\S+-\d+)\s+seq\s+\d+\s+RESPONSE\s+MANAGEMENT\s+PARENT_DATA_SET\s*\n(?:.*\n)*?\s*parentPortIdentity\s+(\S+)`)

// parseParentDataSetOutput parses the output of the pmc GET PARENT_DATA_SET command and returns a map of clock
// identities to their parent port identities.
func parseParentDataSetOutput(output string) (map[string]string, error) {
	matches := parentDataSetRegex.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no parent port identities found in pmc output")
	}

	parentIdentities := make(map[string]string, len(matches))

	for _, match := range matches {
		clockIdentity := match[1]
		parentPortIdentity := match[2]
		parentIdentities[clockIdentity] = parentPortIdentity
	}

	return parentIdentities, nil
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
		clonedInterface := interfaceInfo.Clone()
		clonedInterface.Profile = clone
		clone.Interfaces[name] = clonedInterface
	}

	return clone
}

// InterfaceInfo contains information about the PTP clock type of an interface, its parent profile, and port identity
// metadata. In the future, it may also contain information about which interface it is connected to.
type InterfaceInfo struct {
	Name               iface.Name
	ClockType          PtpClockType
	PortIdentity       string
	ParentPortIdentity string
	// Parent is a pointer to the InterfaceInfo whose PortIdentity matches this interface's ParentPortIdentity, if any.
	// This will typically be nil because the parent is often a network switch rather than another interface on the node.
	//
	// Note: For some grandmaster configurations, ParentPortIdentity may be set to the clock identity derived from
	// PortIdentity (i.e., the same identity with a "-0" suffix). In those cases, Parent should remain nil rather than
	// becoming self-referential.
	Parent  *InterfaceInfo
	Profile *ProfileInfo
}

// Clone creates a deep copy of the InterfaceInfo instance. The Profile pointer is only shallow copied, however, since
// it forms a circular reference.
func (interfaceInfo *InterfaceInfo) Clone() *InterfaceInfo {
	return &InterfaceInfo{
		Name:               interfaceInfo.Name,
		ClockType:          interfaceInfo.ClockType,
		PortIdentity:       interfaceInfo.PortIdentity,
		ParentPortIdentity: interfaceInfo.ParentPortIdentity,
		// Parent is intentionally not cloned. It is a derived linkage that should be recomputed on the cloned graph
		// (e.g., by calling NodeInfo.LinkInterfacesByPortIdentities()).
		Parent:  nil,
		Profile: interfaceInfo.Profile,
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

// NodeInfo contains all the PtpConfig-related information for a single node. Common operations are provided as methods
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

// LinkInterfacesByPortIdentities attempts to link interfaces on the node together based on port identity metadata. It
// will first clear any existing linkage to ensure the result reflects the current identities.
//
// For each interface on the node, this method checks whether its ParentPortIdentity matches the PortIdentity of any
// other interface on the node. If so, it sets the interface's Parent pointer.
//
// This is a best-effort linkage:
//   - If no match is found (common when the parent is a switch), Parent is left nil.
//   - It is guaranteed that no self-referential links will be created.
func (nodeInfo *NodeInfo) LinkInterfacesByPortIdentities() {
	for _, profileInfo := range nodeInfo.Profiles {
		for _, interfaceInfo := range profileInfo.Interfaces {
			interfaceInfo.Parent = nil
		}
	}

	// Index all interfaces by PortIdentity. Port identities are expected to uniquely map to an interface.
	portIdentityToIface := make(map[string]*InterfaceInfo)

	for _, profileInfo := range nodeInfo.Profiles {
		for _, interfaceInfo := range profileInfo.Interfaces {
			if interfaceInfo.PortIdentity == "" {
				continue
			}

			portIdentityToIface[interfaceInfo.PortIdentity] = interfaceInfo
		}
	}

	// Link Parent pointers when there is a unique match.
	for _, profileInfo := range nodeInfo.Profiles {
		for _, interfaceInfo := range profileInfo.Interfaces {
			// If either identity is missing, skip matching for this interface.
			if interfaceInfo.PortIdentity == "" || interfaceInfo.ParentPortIdentity == "" {
				continue
			}

			parent, parentFound := portIdentityToIface[interfaceInfo.ParentPortIdentity]
			if !parentFound {
				// In many cases, the parent is not part of the node. This is expected.
				continue
			}

			// Although it should not happen, we prevent self-referential links.
			if parent == interfaceInfo {
				continue
			}

			interfaceInfo.Parent = parent
		}
	}
}

// SetPortIdentitiesAndLink sets the PortIdentity and ParentPortIdentity for all interfaces across all profiles on this
// node by querying the linuxptp-daemon pod using pmc, then links interfaces together based on their port identities.
// This method will call [SetConfigIndices] to set the ConfigIndex for each profile on the node, so config indices can
// be unset when calling this method.
func (nodeInfo *NodeInfo) SetPortIdentitiesAndLink(client *clients.Settings) error {
	err := nodeInfo.SetConfigIndices(client)
	if err != nil {
		return fmt.Errorf("failed to set config indices on node %s: %w", nodeInfo.Name, err)
	}

	for _, profileInfo := range nodeInfo.Profiles {
		err := profileInfo.SetPortIdentities(client, nodeInfo.Name)
		if err != nil {
			// We expect to see some profiles where there is a config index, but no ptp4l config file. HA
			// profiles are one such example, as they only run phc2sys. We skip these profiles and focus
			// only on those with a ptp4l config file.
			if errors.Is(err, errPtp4lConfigNotFound) {
				klog.V(tsparams.LogLevel).Infof(
					"Skipping port identity lookup for profile %s on node %s: %v",
					profileInfo.Reference.ProfileName, nodeInfo.Name, err)

				continue
			}

			return fmt.Errorf("failed to set port identities for profile %s: %w", profileInfo.Reference.ProfileName, err)
		}
	}

	nodeInfo.LinkInterfacesByPortIdentities()

	return nil
}

// GetProfilesByTypes returns a slice of ProfileInfo pointers for each profile on this node matching any of the provided
// profileTypes. Returned elements are guaranteed not to be nil.
func (nodeInfo *NodeInfo) GetProfilesByTypes(profileTypes ...PtpProfileType) []*ProfileInfo {
	var nodeProfiles []*ProfileInfo

	for _, profileInfo := range nodeInfo.Profiles {
		if slices.Contains(profileTypes, profileInfo.ProfileType) {
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

// SetConfigIndices sets the ConfigIndex for each profile on the node by inspecting the config files present in the
// linuxptp-daemon pod. It relies on the first line of each config file containing the profile name and maps profile
// names to the index embedded in the file name (e.g. ptp4l.0.config).
func (nodeInfo *NodeInfo) SetConfigIndices(client *clients.Settings) error {
	const configIndexCommand = `for f in /var/run/*.config; do ` +
		`b=$(basename "$f"); idx=${b%%.config}; idx=${idx##*.}; ` +
		`profile=$(awk 'NR==1 {print $NF}' "$f"); echo "$profile $idx"; done`

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeInfo.Name, configIndexCommand,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnEmptyOutput(true), ptpdaemon.WithRetryOnError(true))
	if err != nil {
		return fmt.Errorf("failed to gather config indices on node %s: %w", nodeInfo.Name, err)
	}

	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput == "" {
		return fmt.Errorf("failed to find PTP config files on node %s", nodeInfo.Name)
	}

	profileIndices := make(map[string]uint)

	for line := range strings.SplitSeq(trimmedOutput, "\n") {
		fields := strings.Fields(line)

		if len(fields) != 2 {
			return fmt.Errorf("unexpected output %q while parsing config indices on node %s", line, nodeInfo.Name)
		}

		profileName := fields[0]
		indexStr := fields[1]

		indexValue, err := strconv.ParseUint(indexStr, 10, 0)
		if err != nil {
			return fmt.Errorf("failed to parse config index from %s on node %s: %w", indexStr, nodeInfo.Name, err)
		}

		index := uint(indexValue)

		existingIndex, ok := profileIndices[profileName]
		if ok && existingIndex != index {
			return fmt.Errorf(
				"profile %s mapped to multiple config indices (%d and %d) on node %s",
				profileName, existingIndex, index, nodeInfo.Name)
		}

		profileIndices[profileName] = index
	}

	for _, profile := range nodeInfo.Profiles {
		index, ok := profileIndices[profile.Reference.ProfileName]
		if !ok {
			return fmt.Errorf("config index not found for profile %s on node %s",
				profile.Reference.ProfileName, nodeInfo.Name)
		}

		profile.ConfigIndex = ptr.To(index)
	}

	return nil
}
