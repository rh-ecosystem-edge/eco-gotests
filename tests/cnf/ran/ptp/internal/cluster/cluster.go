package cluster

import (
	"fmt"
	"slices"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/processes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/resource"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PtpCluster holds pure PTP cluster-wide domain facts. It holds no K8s/Prometheus connections -- see
// the client package for that.
type PtpCluster struct {
	ptpOperatorVersion clock.PtpOperatorVersion
}

// Node is one node's PTP topology.
type Node struct {
	Clocks []*clock.Clock
}

var ptpCluster *PtpCluster

// Init registers the cluster's own domain facts, once per suite run.
func Init(ptpOperatorVersion clock.PtpOperatorVersion) {
	ptpCluster = &PtpCluster{ptpOperatorVersion: ptpOperatorVersion}
}

// ParsedProfile is one PtpProfile's own parsed facts, plus its correlated HardwareConfig's facts if any.
type ParsedProfile struct {
	Name                   string
	Interfaces             map[iface.Name]*clock.Interface
	Processes              []processes.PtpProcess
	ControllingProfileName string
	PluginTypes            []ptp.PluginType
	HoldoverSource         clock.HoldoverSource
	HardwareConfig         *profiles.ParsedHardwareConfig
	HardwareConfigRef      resource.HardwareConfigReference
}

// parsedResource pairs one profile's own parsed facts with the coordinate needed to find or write the
// real PtpProfile again.
type parsedResource = resource.Resource[resource.ProfileReference, ParsedProfile]

// Resources holds three uncorrelated K8s resource lists, already fetched.
type Resources struct {
	Nodes      []*nodes.Builder
	PtpConfigs []*ptp.PtpConfigBuilder
	HwConfigs  []*ptp.HardwareConfigBuilder
}

// nodeProfiles is one node's own relevant profiles.
type nodeProfiles struct {
	NodeName string
	Profiles []*parsedResource
}

// collectRelevantProfiles returns every node's relevant profiles from already-fetched resources.
func collectRelevantProfiles(resources Resources) ([]nodeProfiles, error) {
	profileRecommends := profiles.GetAllRecommends(resources.PtpConfigs)

	recommendsByNode := make(map[string]map[profiles.ProfileReference]struct{}, len(resources.Nodes))
	uniqueReferences := make(map[profiles.ProfileReference]struct{})

	for _, nodeBuilder := range resources.Nodes {
		recommendsForNode := profiles.GetRecommendsForNode(nodeBuilder.Definition, profileRecommends)
		if len(recommendsForNode) == 0 {
			continue
		}

		recommendsByNode[nodeBuilder.Definition.Name] = recommendsForNode

		for reference := range recommendsForNode {
			uniqueReferences[reference] = struct{}{}
		}
	}

	parsedByReference, err := parseAllProfiles(uniqueReferences, resources.PtpConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PTP profiles: %w", err)
	}

	var relevantByNode []nodeProfiles

	for _, nodeBuilder := range resources.Nodes {
		recommendsForNode, ok := recommendsByNode[nodeBuilder.Definition.Name]
		if !ok {
			continue
		}

		var relevant []*parsedResource

		for reference := range recommendsForNode {
			if parsed, ok := parsedByReference[reference]; ok {
				relevant = append(relevant, parsed)
			}
		}

		if len(relevant) == 0 {
			continue
		}

		relevantByNode = append(relevantByNode, nodeProfiles{NodeName: nodeBuilder.Definition.Name, Profiles: relevant})
	}

	return relevantByNode, nil
}

// parseAllProfiles resolves and parses every unique profile reference, filtering to Clock-shaped profiles.
func parseAllProfiles(
	references map[profiles.ProfileReference]struct{}, ptpConfigList []*ptp.PtpConfigBuilder,
) (map[profiles.ProfileReference]*parsedResource, error) {
	parsed := make(map[profiles.ProfileReference]*parsedResource, len(references))

	for reference := range references {
		configIndex := slices.IndexFunc(ptpConfigList, func(config *ptp.PtpConfigBuilder) bool {
			return runtimeclient.ObjectKeyFromObject(config.Definition) == reference.ConfigReference
		})
		if configIndex == -1 {
			return nil, fmt.Errorf("failed to find PtpConfig for reference %v", reference)
		}

		configProfiles := ptpConfigList[configIndex].Definition.Spec.Profile
		if reference.ProfileIndex < 0 || reference.ProfileIndex >= len(configProfiles) {
			return nil, fmt.Errorf("failed to find profile %s at index %d: index out of bounds",
				reference.ProfileName, reference.ProfileIndex)
		}

		parsedProfile, err := parseProfile(configProfiles[reference.ProfileIndex], reference)
		if err != nil {
			return nil, fmt.Errorf("failed to parse profile %s: %w", reference.ProfileName, err)
		}

		if !isClockCandidate(parsedProfile) {
			continue
		}

		parsed[reference] = parsedProfile
	}

	return parsed, nil
}

// parseProfile parses one raw PtpProfile into cluster's own primitives.
func parseProfile(profile ptpv1.PtpProfile, reference profiles.ProfileReference) (*parsedResource, error) {
	clientFlag := profiles.HasClientFlag(profile.Ptp4lOpts)

	var sections map[string]map[string]string

	if profile.Ptp4lConf != nil && *profile.Ptp4lConf != "" {
		var err error

		sections, err = profiles.GetSectionsFromPtp4lConf(*profile.Ptp4lConf)
		if err != nil {
			return nil, fmt.Errorf("failed to get sections from ptp4lConf: %w", err)
		}
	}

	if globalSection, ok := sections["global"]; ok {
		if globalSection["clientOnly"] == "1" || globalSection["slaveOnly"] == "1" {
			clientFlag = true
		}
	}

	interfaces := make(map[iface.Name]*clock.Interface, len(sections))

	for sectionName, sectionValues := range sections {
		if sectionName == "global" || sectionName == "unicast_master_table" {
			continue
		}

		role := profiles.InterfaceRoleClient

		if !clientFlag && (sectionValues["serverOnly"] == "1" || sectionValues["masterOnly"] == "1") {
			role = profiles.InterfaceRoleServer
		}

		name := iface.Name(sectionName)
		interfaces[name] = &clock.Interface{Name: name, Role: role}
	}

	var pluginTypes []ptp.PluginType

	if profile.Plugins != nil {
		var err error

		pluginTypes, err = profiles.GetPluginTypesFromProfile(&profile)
		if err != nil {
			return nil, fmt.Errorf("failed to get plugin types from profile: %w", err)
		}
	}

	var controllingProfileName string
	if profile.PtpSettings != nil {
		controllingProfileName = profile.PtpSettings["controllingProfile"]
	}

	var name string
	if profile.Name != nil {
		name = *profile.Name
	}

	return &parsedResource{
		Metadata: resource.ProfileReference{
			ConfigReference: resource.ConfigReference{Name: reference.ConfigReference},
			ProfileIndex:    reference.ProfileIndex,
		},
		Data: ParsedProfile{
			Name:                   name,
			Interfaces:             interfaces,
			Processes:              deriveProcesses(profile),
			ControllingProfileName: controllingProfileName,
			PluginTypes:            pluginTypes,
		},
	}, nil
}

// deriveProcesses reports which linuxptp processes a profile's own declared config implies.
func deriveProcesses(profile ptpv1.PtpProfile) []processes.PtpProcess {
	var procs []processes.PtpProcess

	if profile.Ptp4lConf != nil && *profile.Ptp4lConf != "" {
		procs = append(procs, processes.Ptp4l)
	}

	if (profile.Phc2sysOpts != nil && *profile.Phc2sysOpts != "") ||
		(profile.Phc2sysConf != nil && *profile.Phc2sysConf != "") {
		procs = append(procs, processes.Phc2sys)
	}

	if (profile.Ts2PhcOpts != nil && *profile.Ts2PhcOpts != "") ||
		(profile.Ts2PhcConf != nil && *profile.Ts2PhcConf != "") {
		procs = append(procs, processes.Ts2phc)
	}

	return procs
}

// hasProcess reports whether procs contains want.
func hasProcess(procs []processes.PtpProcess, want processes.PtpProcess) bool {
	for _, proc := range procs {
		if proc == want {
			return true
		}
	}

	return false
}

// isClockCandidate reports whether a parsed profile has a shape this package can compose into a Clock:
// exactly one client interface with Ts2phc+Phc2sys (a receiver), or zero client interfaces with
// ControllingProfileName set (a transmitter).
func isClockCandidate(parsed *parsedResource) bool {
	clientCount := 0

	for _, interfaceInfo := range parsed.Data.Interfaces {
		if interfaceInfo.Role == profiles.InterfaceRoleClient {
			clientCount++
		}
	}

	if clientCount == 1 && hasProcess(parsed.Data.Processes, processes.Ts2phc) && hasProcess(parsed.Data.Processes, processes.Phc2sys) {
		return true
	}

	return clientCount == 0 && parsed.Data.ControllingProfileName != ""
}

// associateHoldoverSources derives each profile's own HoldoverSource from its PluginTypes.
// QUIRK: a Carter Flat (e825/e830) plugin's own holdover settings are never valid; only a qualified
// HardwareConfig CR is (the hardware can't expose enough DPLL accuracy otherwise).
// https://docs.okd.io/4.22/networking/advanced_networking/ptp/configuring-ptp.html#nw-ptp-granite-rapids-boundary-clock-overview-without-holdover_configuring-ptp
func associateHoldoverSources(relevantByNode []nodeProfiles, hwConfigs []*ptp.HardwareConfigBuilder) {
	type hwConfigMatch struct {
		parsed *profiles.ParsedHardwareConfig
		ref    resource.HardwareConfigReference
	}

	byRelatedProfile := make(map[string]hwConfigMatch, len(hwConfigs))
	for _, hwConfig := range hwConfigs {
		parsed := profiles.ParseHardwareConfig(hwConfig)
		byRelatedProfile[parsed.RelatedPtpProfileName] = hwConfigMatch{
			parsed: parsed,
			ref:    resource.HardwareConfigReference{Name: runtimeclient.ObjectKeyFromObject(hwConfig.Definition)},
		}
	}

	for _, node := range relevantByNode {
		for _, profile := range node.Profiles {
			if match, ok := byRelatedProfile[profile.Data.Name]; ok && match.parsed.HasHoldoverParameters {
				profile.Data.HoldoverSource = clock.HoldoverSourceHardwareConfig
				profile.Data.HardwareConfig = match.parsed
				profile.Data.HardwareConfigRef = match.ref

				continue
			}

			if slices.Contains(profile.Data.PluginTypes, ptp.PluginTypeE810) {
				profile.Data.HoldoverSource = clock.HoldoverSourcePlugin
			}
		}
	}
}

// buildNodes groups each node's parsed profiles into Clocks -- pairing a transmitter with the receiver
// its own ControllingProfileName names, leaving any unpaired receiver standalone.
func buildNodes(relevantByNode []nodeProfiles) ([]*Node, error) {
	var result []*Node

	for _, node := range relevantByNode {
		byName := make(map[string]*parsedResource, len(node.Profiles))
		for _, profile := range node.Profiles {
			byName[profile.Data.Name] = profile
		}

		consumed := make(map[string]bool, len(node.Profiles))

		var clocks []*clock.Clock

		for _, profile := range node.Profiles {
			if profile.Data.ControllingProfileName == "" {
				continue
			}

			receiver, ok := byName[profile.Data.ControllingProfileName]
			if !ok {
				return nil, fmt.Errorf("transmitter profile %s references unknown receiver %s",
					profile.Data.Name, profile.Data.ControllingProfileName)
			}

			consumed[receiver.Data.Name] = true

			interfaces := make(map[iface.Name]*clock.Interface, len(profile.Data.Interfaces)+len(receiver.Data.Interfaces))
			for name, interfaceInfo := range profile.Data.Interfaces {
				interfaces[name] = interfaceInfo
			}

			for name, interfaceInfo := range receiver.Data.Interfaces {
				interfaces[name] = interfaceInfo
			}

			clocks = append(clocks, &clock.Clock{
				NodeName:                  node.NodeName,
				Interfaces:                interfaces,
				HoldoverEnabled:           receiver.Data.HoldoverSource != clock.HoldoverSourceNone,
				HoldoverSource:            receiver.Data.HoldoverSource,
				ReceivingProfileRef:       receiver.Metadata,
				HoldoverHardwareConfigRef: receiver.Data.HardwareConfigRef,
			})
		}

		for _, profile := range node.Profiles {
			if profile.Data.ControllingProfileName != "" || consumed[profile.Data.Name] {
				continue
			}

			clocks = append(clocks, &clock.Clock{
				NodeName:                  node.NodeName,
				Interfaces:                profile.Data.Interfaces,
				HoldoverEnabled:           profile.Data.HoldoverSource != clock.HoldoverSourceNone,
				HoldoverSource:            profile.Data.HoldoverSource,
				ReceivingProfileRef:       profile.Metadata,
				HoldoverHardwareConfigRef: profile.Data.HardwareConfigRef,
			})
		}

		if len(clocks) > 0 {
			result = append(result, &Node{Clocks: clocks})
		}
	}

	return result, nil
}

// DeriveClocksFromResources derives every Node's PTP topology from already-fetched resources.
func DeriveClocksFromResources(resources Resources) (clock.Clocks, error) {
	relevantByNode, err := collectRelevantProfiles(resources)
	if err != nil {
		return nil, fmt.Errorf("failed to collect PTP profiles: %w", err)
	}

	associateHoldoverSources(relevantByNode, resources.HwConfigs)

	nodes, err := buildNodes(relevantByNode)
	if err != nil {
		return nil, fmt.Errorf("failed to build PTP nodes and clocks: %w", err)
	}

	var clks clock.Clocks

	for _, node := range nodes {
		for _, clk := range node.Clocks {
			if err := clk.DetermineType(); err != nil {
				return nil, err
			}

			clks = append(clks, clk)
		}
	}

	return clks, nil
}

// PtpOperatorVersion reports whether the cluster's own registered PTP operator version falls within
// [minVersion, maxVersion) (an empty maxVersion means no upper bound).
func PtpOperatorVersion(minVersion, maxVersion string) (bool, error) {
	return version.IsVersionStringInRange(string(ptpCluster.ptpOperatorVersion), minVersion, maxVersion)
}
