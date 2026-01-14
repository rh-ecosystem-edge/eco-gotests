package profiles

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SavePtpConfigs returns a list of all PtpConfigs in the cluster.
func SavePtpConfigs(client *clients.Settings) ([]*ptp.PtpConfigBuilder, error) {
	ptpConfigList, err := ptp.ListPtpConfigs(client)
	if err != nil {
		return nil, fmt.Errorf("failed to list PtpConfigs: %w", err)
	}

	return ptpConfigList, nil
}

// RestoreProfileToConfig updates the profile referenced by the provided ProfileReference with the provided profile. It
// returns true if the profile was changed, false otherwise. It returns an error if the restore fails.
func RestoreProfileToConfig(
	client *clients.Settings, profileReference ProfileReference, profile *ptpv1.PtpProfile) (bool, error) {
	if profile == nil {
		return false, fmt.Errorf("profile cannot be nil")
	}

	ptpConfig, err := profileReference.PullPtpConfig(client)
	if err != nil {
		return false, fmt.Errorf("failed to pull PtpConfig for profile %s: %w", profileReference.ProfileName, err)
	}

	profileIndex := profileReference.ProfileIndex
	if profileIndex < 0 || profileIndex >= len(ptpConfig.Definition.Spec.Profile) {
		return false, fmt.Errorf("failed to get profile %s at index %d: index out of bounds",
			profileReference.ProfileName, profileIndex)
	}

	if reflect.DeepEqual(ptpConfig.Definition.Spec.Profile[profileIndex], *profile) {
		return false, nil
	}

	ptpConfig.Definition.Spec.Profile[profileIndex] = *profile

	_, err = ptpConfig.Update()
	if err != nil {
		return false, fmt.Errorf("failed to update PtpConfig for profile %s: %w", profileReference.ProfileName, err)
	}

	return true, nil
}

// RestorePtpConfigs restores the PtpConfigs from the list to the cluster. It first checks if the PtpConfig has changed
// using reflect.DeepEqual on the spec then updating if necessary. It collects all the errors and returns them
// as a single error. It returns a list of profile references that were changed.
func RestorePtpConfigs(client *clients.Settings, ptpConfigList []*ptp.PtpConfigBuilder) ([]*ProfileReference, error) {
	var (
		changedProfiles []*ProfileReference
		errs            []error
	)

	for _, ptpConfig := range ptpConfigList {
		changedProfilesForConfig, err := listChangedProfilesInConfig(ptpConfig)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to list changed profiles in PtpConfig %s in namespace %s: %w",
				ptpConfig.Definition.Name, ptpConfig.Definition.Namespace, err))

			continue
		}

		if len(changedProfilesForConfig) == 0 {
			continue
		}

		_, err = ptpConfig.Update()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to update PtpConfig %s in namespace %s: %w",
				ptpConfig.Definition.Name, ptpConfig.Definition.Namespace, err))

			continue
		}

		changedProfiles = append(changedProfiles, changedProfilesForConfig...)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to restore PtpConfigs: %w", errors.Join(errs...))
	}

	return changedProfiles, nil
}

// listChangedProfilesInConfig lists the profiles in the PtpConfig that have changed. It considers all the profiles in
// the original PtpConfig and then uses reflect.DeepEqual to compare each to the latest version of the PtpConfig. It
// returns a list of profile references that were changed.
//
// This function will return an error if a profile is not found in the latest PtpConfig.
func listChangedProfilesInConfig(ptpConfig *ptp.PtpConfigBuilder) ([]*ProfileReference, error) {
	latestPtpConfig, err := ptpConfig.Get()
	if err != nil {
		if runtimeclient.IgnoreNotFound(err) == nil {
			// If the PtpConfig is not found, it means it was deleted, so we return an empty list of changed profiles.
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get latest version of PtpConfig: %w", err)
	}

	var changedProfiles []*ProfileReference

	for _, profile := range ptpConfig.Definition.Spec.Profile {
		if profile.Name == nil {
			return nil, fmt.Errorf("profile name is nil")
		}

		index := slices.IndexFunc(latestPtpConfig.Spec.Profile, func(p ptpv1.PtpProfile) bool {
			return p.Name != nil && *p.Name == *profile.Name
		})
		if index == -1 {
			// restoring profile that was deleted from config
			changedProfiles = append(changedProfiles, &ProfileReference{
				ConfigReference: runtimeclient.ObjectKeyFromObject(ptpConfig.Definition),
				ProfileIndex:    len(latestPtpConfig.Spec.Profile), // Append at end
				ProfileName:     *profile.Name,
			})

			ptpConfig.Definition.Spec.Profile = append(
				ptpConfig.Definition.Spec.Profile,
				profile,
			)

			continue
		}

		if reflect.DeepEqual(profile, latestPtpConfig.Spec.Profile[index]) {
			continue
		}

		changedProfiles = append(changedProfiles, &ProfileReference{
			ConfigReference: runtimeclient.ObjectKeyFromObject(ptpConfig.Definition),
			ProfileIndex:    index,
			ProfileName:     *profile.Name,
		})
	}

	return changedProfiles, nil
}

// RemoveProfileFromConfig removes a profile from a PtpConfig by deleting it from the profile array. The
// profile is identified by the provided ProfileReference.
func RemoveProfileFromConfig(
	client *clients.Settings,
	profileReference ProfileReference,
) error {
	ptpConfig, err := profileReference.PullPtpConfig(client)
	if err != nil {
		return fmt.Errorf("failed to pull PtpConfig %s: %w",
			profileReference.ConfigReference.Name, err)
	}

	profileIndex := profileReference.ProfileIndex

	// Validate index is in bounds
	if profileIndex < 0 || profileIndex >= len(ptpConfig.Definition.Spec.Profile) {
		return fmt.Errorf("profile %s at index %d is out of bounds (config has %d profiles)",
			profileReference.ProfileName, profileIndex, len(ptpConfig.Definition.Spec.Profile))
	}

	// Verify the profile name matches (safety check)
	profileToRemove := ptpConfig.Definition.Spec.Profile[profileIndex]
	if profileToRemove.Name == nil || *profileToRemove.Name != profileReference.ProfileName {
		return fmt.Errorf("profile name mismatch at index %d: expected %s, got %v",
			profileIndex, profileReference.ProfileName,
			func() string {
				if profileToRemove.Name == nil {
					return "<nil>"
				}

				return *profileToRemove.Name
			}())
	}

	// Remove the profile from the slice
	ptpConfig.Definition.Spec.Profile = append(
		ptpConfig.Definition.Spec.Profile[:profileIndex],
		ptpConfig.Definition.Spec.Profile[profileIndex+1:]...,
	)

	// Update the config in the cluster
	_, err = ptpConfig.Update()
	if err != nil {
		return fmt.Errorf("failed to update PtpConfig after removing profile %s: %w",
			profileReference.ProfileName, err)
	}

	return nil
}
