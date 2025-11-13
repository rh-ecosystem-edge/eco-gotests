package profiles

import (
	"fmt"
	"regexp"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"k8s.io/utils/ptr"
)

// holdoverFlagRegexp is a regular expression that matches the holdover flag in the ts2phc command line options. It
// contains two capture groups: the delimiter of an equals sign or whitespace, and the holdover timeout in seconds.
//
// Examples of valid matches:
//   - --ts2phc.holdover=60
//   - --ts2phc.holdover 60
var holdoverFlagRegexp = regexp.MustCompile(`--ts2phc\.holdover(=|\s+)(\d+)`)

// UpdateTS2PHCHoldover updates the holdover timeout for the ts2phc process in the PTP profile. It returns the old
// profile or an error if the update fails.
//
// Updates are always done through changing the command line options, since these will override any configuration in the
// global section of the configuration file.
//
// The returned old profile is deep copied, so will not be modified by this function.
func UpdateTS2PHCHoldover(
	client *clients.Settings, profile *ProfileInfo, newHoldoverSeconds uint64) (*ptpv1.PtpProfile, error) {
	ptpConfig, err := profile.Reference.PullPtpConfig(client)
	if err != nil {
		return nil, fmt.Errorf("failed to pull PtpConfig for profile %s: %w", profile.Reference.ProfileName, err)
	}

	profileIndex := profile.Reference.ProfileIndex
	if profileIndex < 0 || profileIndex >= len(ptpConfig.Definition.Spec.Profile) {
		return nil, fmt.Errorf("failed to get profile %s at index %d: index out of bounds",
			profile.Reference.ProfileName, profileIndex)
	}

	pulledProfile := &ptpConfig.Definition.Spec.Profile[profileIndex]
	oldProfile := pulledProfile.DeepCopy()

	newHoldoverFlag := fmt.Sprintf("--ts2phc.holdover=%d", newHoldoverSeconds)

	switch {
	case pulledProfile.Ts2PhcOpts == nil:
		pulledProfile.Ts2PhcOpts = ptr.To(newHoldoverFlag)
	case holdoverFlagRegexp.MatchString(*pulledProfile.Ts2PhcOpts):
		*pulledProfile.Ts2PhcOpts = holdoverFlagRegexp.ReplaceAllString(
			*pulledProfile.Ts2PhcOpts, newHoldoverFlag)
	default:
		*pulledProfile.Ts2PhcOpts += fmt.Sprintf(" %s", newHoldoverFlag)
	}

	_, err = ptpConfig.Update()
	if err != nil {
		return nil, fmt.Errorf("failed to update PtpConfig for profile %s: %w", profile.Reference.ProfileName, err)
	}

	return oldProfile, nil
}
