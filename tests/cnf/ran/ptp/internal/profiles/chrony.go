package profiles

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"k8s.io/utils/ptr"
)

// ReplaceChronydServers replaces the servers in the chronyd configuration for the provided profile with the provided
// new server. It will also add iburst to the new server. It returns the old profile or an error if the update fails.
//
// The returned old profile is deep copied, so will not be modified by this function.
func ReplaceChronydServers(
	client *clients.Settings, profileReference ProfileReference, newServer string) (*ptpv1.PtpProfile, error) {
	ptpConfig, err := profileReference.PullPtpConfig(client)
	if err != nil {
		return nil, fmt.Errorf("failed to pull PtpConfig for profile %s: %w", profileReference.ProfileName, err)
	}

	profileIndex := profileReference.ProfileIndex
	if profileIndex < 0 || profileIndex >= len(ptpConfig.Definition.Spec.Profile) {
		return nil, fmt.Errorf("failed to get profile %s at index %d: index out of bounds",
			profileReference.ProfileName, profileIndex)
	}

	pulledProfile := &ptpConfig.Definition.Spec.Profile[profileIndex]
	oldProfile := pulledProfile.DeepCopy()

	if pulledProfile.ChronydConf == nil {
		pulledProfile.ChronydConf = ptr.To(fmt.Sprintf("server %s iburst", newServer))
	} else {
		*pulledProfile.ChronydConf = removeServerLinesFromChronydConfig(*pulledProfile.ChronydConf)
		*pulledProfile.ChronydConf += fmt.Sprintf("\nserver %s iburst", newServer)
	}

	_, err = ptpConfig.Update()
	if err != nil {
		return nil, fmt.Errorf("failed to update PtpConfig for profile %s: %w", profileReference.ProfileName, err)
	}

	return oldProfile, nil
}

// removeServerLinesFromChronydConfig removes all server lines from the chronyd configuration.
func removeServerLinesFromChronydConfig(config string) string {
	var newLines strings.Builder

	for line := range strings.Lines(config) {
		if strings.HasPrefix(strings.TrimSpace(line), "server") {
			continue
		}

		newLines.WriteString(line)
		newLines.WriteRune('\n')
	}

	return newLines.String()
}
