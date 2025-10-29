package profiles

import (
	"bufio"
	"fmt"
	"strings"

	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
)

// configSections is a map of section names to their key-value pairs. It represents the format used by ptp4l and ts2phc.
type configSections = map[string]map[string]string

// parsePtpProfile parses the PTP profile and the ptp4l information to get the interfaces and their types before making
// a determination on the profile type. Maps in the parsedPtp4lConf struct are guaranteed to not be nil when returned.
func parsePtpProfile(profile ptpv1.PtpProfile, reference ProfileReference) (*ProfileInfo, error) {
	profileInfo := &ProfileInfo{
		Reference: reference,
	}
	clientFlag := hasClientFlag(profile.Ptp4lOpts)

	var (
		err           error
		ptp4lSections = make(configSections)
	)

	if profile.Ptp4lConf != nil && *profile.Ptp4lConf != "" {
		ptp4lSections, err = getSectionsFromPtp4lConf(*profile.Ptp4lConf)
		if err != nil {
			return nil, fmt.Errorf("failed to get sections from ptp4lConf: %w", err)
		}
	}

	profileInfo.Interfaces = getInterfacesFromPtp4lSections(clientFlag, ptp4lSections)

	if profile.Interface != nil && *profile.Interface != "" {
		ifaceName := iface.Name(*profile.Interface)
		if _, ok := profileInfo.Interfaces[ifaceName]; !ok {
			profileInfo.Interfaces[ifaceName] = &InterfaceInfo{
				Name: ifaceName,
				// If the interface is not set in the config file, it cannot be server only.
				ClockType: ClockTypeClient,
			}
		}
	}

	profileInfo.ProfileType, err = determineProfileType(profileInfo.Interfaces, profile)
	if err != nil {
		return nil, fmt.Errorf("failed to determine profile type: %w", err)
	}

	return profileInfo, nil
}

// getSectionsFromPtp4lConf parses the ptp4l configuration file and returns a map of sections and their key-value pairs.
func getSectionsFromPtp4lConf(ptp4lConf string) (configSections, error) {
	var currentSectionName string

	sections := make(configSections)
	scanner := bufio.NewScanner(strings.NewReader(ptp4lConf))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Ignore empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Lines with text between brackets are considered section names.
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && len(line) > 2 {
			currentSectionName = line[1 : len(line)-1]

			if _, ok := sections[currentSectionName]; !ok {
				sections[currentSectionName] = make(map[string]string)
			}

			continue
		}

		// If the first section has not been found yet, skip the line.
		if currentSectionName == "" {
			continue
		}

		// This is not a section name, so it should be a key-value pair, separated by a space.
		keyValue := strings.SplitN(line, " ", 2)
		if len(keyValue) < 2 {
			continue
		}

		sections[currentSectionName][keyValue[0]] = keyValue[1]
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ptp4l configuration: %w", err)
	}

	return sections, nil
}

// getInterfacesFromPtp4lSections extracts the interfaces and their clock types from the ptp4l configuration sections.
// The provided clientFlag indicates whether the clientOnly command line flag is set in ptp4lOpts. The returned map is
// guaranteed to not be nil.
func getInterfacesFromPtp4lSections(clientFlag bool, sections configSections) map[iface.Name]*InterfaceInfo {
	interfaces := make(map[iface.Name]*InterfaceInfo)

	// Setting clientOnly in the global section is equivalent to setting it as a command line flag, meaning all
	// interfaces are client only.
	if globalSection, ok := sections["global"]; ok && globalSection != nil {
		// slaveOnly is deprecated but still used and supported by ptp4l.
		if globalSection["clientOnly"] == "1" || globalSection["slaveOnly"] == "1" {
			clientFlag = true
		}
	}

	for sectionName, sectionValues := range sections {
		if sectionName == "global" || sectionName == "unicast_master_table" {
			continue
		}

		var clockType PtpClockType

		switch {
		case clientFlag:
			clockType = ClockTypeClient
		// masterOnly is deprecated but still used and supported by ptp4l, similar to slaveOnly.
		case sectionValues["serverOnly"] == "1" || sectionValues["masterOnly"] == "1":
			clockType = ClockTypeServer
		default:
			clockType = ClockTypeClient
		}

		ifaceName := iface.Name(sectionName)
		interfaces[ifaceName] = &InterfaceInfo{
			Name:      ifaceName,
			ClockType: clockType,
		}
	}

	return interfaces
}

// determineProfileType determines the PTP profile type based on the number of interfaces and their clock types.
// Additionally, it makes use of ts2phc settings to determine if the profile is GM or MultiNICGM. An error is returned
// if the profile type cannot be determined.
func determineProfileType(interfaces map[iface.Name]*InterfaceInfo, profile ptpv1.PtpProfile) (PtpProfileType, error) {
	// If the profile has ts2phc.master set to 1, it means there is a time source and the profile is a GM profile.
	// If there is also ts2phc.master set to 0, it means there is another NIC acting as a time sink, so it is a
	// multi-NIC GM profile.
	if profile.Ts2PhcConf != nil && strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 1") {
		if strings.Contains(*profile.Ts2PhcConf, "ts2phc.master 0") {
			return ProfileTypeMultiNICGM, nil
		}

		return ProfileTypeGM, nil
	}

	// If the profile has PtpSettings and haProfiles is set, it must be a highly available profile.
	if profile.PtpSettings != nil && profile.PtpSettings["haProfiles"] != "" {
		return ProfileTypeHA, nil
	}

	// The remaining profile types are determined based on the number of interfaces and their clock types.
	numInterfaces := len(interfaces)
	numClientInterfaces := 0
	numServerInterfaces := 0

	for _, interfaceInfo := range interfaces {
		switch interfaceInfo.ClockType {
		case ClockTypeClient:
			numClientInterfaces++
		case ClockTypeServer:
			numServerInterfaces++
		}
	}

	switch {
	// If the profile has one interface and one client interface, return ProfileTypeOC.
	case numInterfaces == 1 && numClientInterfaces == 1:
		return ProfileTypeOC, nil
	// If the profile has two interfaces and two client interfaces, return ProfileTypeTwoPortOC.
	case numInterfaces == 2 && numClientInterfaces == 2:
		return ProfileTypeTwoPortOC, nil
	// If the profile has at least two interfaces and only one client interface, return ProfileTypeBC.
	case numInterfaces >= 2 && numClientInterfaces == 1:
		return ProfileTypeBC, nil
	// All other profile types are considered unsupported.
	default:
		return 0, fmt.Errorf("unable to determine PTP profile type based on defined rules")
	}
}

// hasClientFlag checks if the ptp4lOpts string contains any client-only flags. Though the reference PTP profiles use
// only `-s`, this function supports all possible client-only flags that ptp4l supports.
func hasClientFlag(ptp4lOpts *string) bool {
	if ptp4lOpts == nil {
		return false
	}

	// Checking for client flags requires splitting fields to ensure that -s is the entire option, not part of
	// --summary_interval, for example.
	fields := strings.Fields(*ptp4lOpts)

	//nolint:varnamelen // i for an index is a well-established convention.
	for i, field := range fields {
		if field == "-s" {
			return true
		}

		if field == "--clientOnly=1" || field == "--slaveOnly=1" {
			return true
		}

		// In addition to the single-field flags, ptp4l also supports space-separated flags with a value. For
		// these we must check that there is a next field and that it is 1.
		if i+1 >= len(fields) {
			continue
		}

		if (field == "--clientOnly" || field == "--slaveOnly") && fields[i+1] == "1" {
			return true
		}
	}

	return false
}
