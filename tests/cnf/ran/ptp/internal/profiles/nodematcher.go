package profiles

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// GetNodeInfoMap retrieves a map of node names to NodeInfo structs for all nodes in the cluster that have PTP profiles
// recommended to them. The returned map is guaranteed to be non-nil if there is no error. Each node gets its own unique
// ProfileInfo instances, even if the same profile is recommended to multiple nodes.
//
// The algorithm for determining recommendations is described in the PTP operator code:
// https://github.com/openshift/ptp-operator/blob/main/controllers/recommend.go. It assumes that profile names are
// unique.
func GetNodeInfoMap(client *clients.Settings) (map[string]*NodeInfo, error) {
	nodeList, err := nodes.List(client)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes while building NodeInfo map: %w", err)
	}

	ptpConfigList, err := ptp.ListPtpConfigs(client)
	if err != nil {
		return nil, fmt.Errorf("failed to list PTP configs while building NodeInfo map: %w", err)
	}

	allRecommends := getAllRecommends(ptpConfigList)
	ptpProfileInfos := make(map[ProfileReference]*ProfileInfo)
	nodeInfoMap := make(map[string]*NodeInfo)

	for _, nodeBuilder := range nodeList {
		recommendsForNode := getRecommendsForNode(nodeBuilder.Definition, allRecommends)
		if len(recommendsForNode) == 0 {
			klog.V(tsparams.LogLevel).Infof("No PTP recommends found for node %s", nodeBuilder.Definition.Name)

			continue
		}

		nodeInfo := &NodeInfo{
			Name: nodeBuilder.Definition.Name,
		}

		for reference := range recommendsForNode {
			profileInfo, err := getMemoizedAndClonedProfileInfo(reference, ptpConfigList, ptpProfileInfos)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Failed to get profile info for reference %v: %v", reference, err)

				continue
			}

			nodeInfo.Profiles = append(nodeInfo.Profiles, profileInfo)
		}

		nodeInfo.Counts = getProfileCounts(nodeInfo.Profiles)
		nodeInfoMap[nodeBuilder.Definition.Name] = nodeInfo
	}

	return nodeInfoMap, nil
}

// recommendWithProfileReference is a helper struct that combines a PTP recommend with a reference to the profile it
// applies to. Since all recommends are aggregated from all PTP configs, this struct is used to keep track of the
// profile reference along with the recommend itself.
type recommendWithProfileReference struct {
	recommend ptpv1.PtpRecommend
	reference ProfileReference
}

// getAllRecommends returns the recommend structures for all PTP configs provided and sorts by priority in ascending
// order. Recommends without profiles, priorities, or matches are filtered out. Recommends that do not match any profile
// in the config are also filtered out.
func getAllRecommends(configs []*ptp.PtpConfigBuilder) []recommendWithProfileReference {
	var recommends []recommendWithProfileReference

	for _, config := range configs {
		for _, recommend := range config.Definition.Spec.Recommend {
			if recommend.Profile == nil || recommend.Priority == nil || len(recommend.Match) == 0 {
				continue
			}

			profileIndex := slices.IndexFunc(config.Definition.Spec.Profile, func(profile ptpv1.PtpProfile) bool {
				return profile.Name != nil && *profile.Name == *recommend.Profile
			})

			// If the recommend does not match any profile, it is ignored.
			if profileIndex == -1 {
				continue
			}

			reference := ProfileReference{
				ConfigReference: runtimeclient.ObjectKeyFromObject(config.Definition),
				ProfileIndex:    profileIndex,
				ProfileName:     *recommend.Profile,
			}
			recommends = append(recommends, recommendWithProfileReference{
				recommend: recommend,
				reference: reference,
			})
		}
	}

	slices.SortFunc(recommends, func(a, b recommendWithProfileReference) int {
		// We already filtered out nil pointers, so we can safely dereference.
		return cmp.Compare(*a.recommend.Priority, *b.recommend.Priority)
	})

	return recommends
}

// getRecommendsForNode matches the recommends to a node based on the match criteria. The provided recommends are
// assumed to be sorted in ascending order by priority, with non-nil priorities and profile names. It returns a set of
// profile references that are recommended for the node. The returned map is guaranteed to not be nil.
//
// The algorithm is the same as the one used in the PTP operator:
//   - Loop through all recommends in order and ignore those that do not match the node.
//   - The lowest priority that matches the node is used to determine which profiles are recommended.
//   - If a recommend with a different priority is found, the loop is broken and no further recommends are considered.
func getRecommendsForNode(
	node *corev1.Node, allRecommends []recommendWithProfileReference) map[ProfileReference]struct{} {
	profiles := make(map[ProfileReference]struct{})
	priority := int64(-1)

	for _, recommend := range allRecommends {
		if !nodeMatches(node, recommend.recommend) {
			continue
		}

		if priority >= 0 && *recommend.recommend.Priority != priority {
			break
		}

		priority = *recommend.recommend.Priority
		profiles[recommend.reference] = struct{}{}
	}

	return profiles
}

// nodeMatches checks if the node matches the criteria in the recommend. It returns true if the node matches any of the
// matches for the recommend.
func nodeMatches(node *corev1.Node, recommend ptpv1.PtpRecommend) bool {
	for _, match := range recommend.Match {
		if match.NodeName != nil && *match.NodeName == node.Name {
			return true
		}

		if match.NodeLabel != nil {
			if _, ok := node.Labels[*match.NodeLabel]; ok {
				return true
			}
		}
	}

	return false
}

// getMemoizedAndClonedProfileInfo retrieves a ProfileInfo for the given reference from the provided PTP configs. If the
// profile has already been parsed and saved in ptpProfileInfos, a clone of it is returned. Otherwise, the profile is
// parsed from the PTP config, saved in ptpProfileInfos, and a clone is returned. The returned ProfileInfo pointer is
// guaranteed to not be nil if there is no error. Each call returns a unique ProfileInfo instance to ensure
// modifications don't affect other nodes.
func getMemoizedAndClonedProfileInfo(
	reference ProfileReference,
	allConfigs []*ptp.PtpConfigBuilder,
	ptpProfileInfos map[ProfileReference]*ProfileInfo) (*ProfileInfo, error) {
	if profileInfo, ok := ptpProfileInfos[reference]; ok {
		return profileInfo.Clone(), nil
	}

	configIndex := slices.IndexFunc(allConfigs, func(config *ptp.PtpConfigBuilder) bool {
		return runtimeclient.ObjectKeyFromObject(config.Definition) == reference.ConfigReference
	})
	if configIndex == -1 {
		return nil, fmt.Errorf("failed to find PTP config for reference %v", reference)
	}

	profiles := allConfigs[configIndex].Definition.Spec.Profile
	if reference.ProfileIndex < 0 || reference.ProfileIndex >= len(profiles) {
		return nil, fmt.Errorf("failed to find profile %s at index %d: index out of bounds",
			reference.ProfileName, reference.ProfileIndex)
	}

	profile := profiles[reference.ProfileIndex]

	profileInfo, err := parsePtpProfile(profile, reference)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PTP profile %s: %w", reference.ProfileName, err)
	}

	ptpProfileInfos[reference] = profileInfo

	return profileInfo.Clone(), nil
}

// getProfileCounts creates a ProfileCounts map from a slice of ProfileInfo pointers. It does not verify the ProfileType
// for a ProfileInfo is valid.
func getProfileCounts(profiles []*ProfileInfo) ProfileCounts {
	counts := make(ProfileCounts)

	for _, profile := range profiles {
		counts[profile.ProfileType]++
	}

	return counts
}
