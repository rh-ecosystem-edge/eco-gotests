package ptp

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	infraptp "github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// UbloxProtocolFromPlugins maps Intel PtpConfig plugin entries to ubxtool -P revision.
func UbloxProtocolFromPlugins(plugins map[string]*apiextensions.JSON) (string, bool) {
	if plugins == nil {
		return "", false
	}

	if _, ok := plugins[PluginNameE825]; ok {
		return UbloxProtocolE825E830, true
	}

	if _, ok := plugins[PluginNameE830]; ok {
		return UbloxProtocolE825E830, true
	}

	if _, ok := plugins[PluginNameE810]; ok {
		return UbloxProtocolE810, true
	}

	return "", false
}

func ubloxFromProfileName(ptpConfig *ptpv1.PtpConfig, profileName string) (string, bool) {
	for i := range ptpConfig.Spec.Profile {
		p := &ptpConfig.Spec.Profile[i]
		if p.Name == nil || *p.Name != profileName {
			continue
		}

		ver, ok := UbloxProtocolFromPlugins(p.Plugins)

		return ver, ok
	}

	return "", false
}

func nodeLabelMatches(rule string, nodeLabels map[string]string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return false
	}

	if idx := strings.Index(rule, "="); idx >= 0 {
		k := strings.TrimSpace(rule[:idx])
		v := strings.TrimSpace(rule[idx+1:])
		actual, ok := nodeLabels[k]

		return ok && actual == v
	}

	_, ok := nodeLabels[rule]

	return ok
}

func recommendPriority(rec ptpv1.PtpRecommend) int64 {
	if rec.Priority == nil {
		return math.MaxInt64
	}

	return *rec.Priority
}

func sortedRecommendations(ptpConfig *ptpv1.PtpConfig) []ptpv1.PtpRecommend {
	recs := make([]ptpv1.PtpRecommend, len(ptpConfig.Spec.Recommend))
	copy(recs, ptpConfig.Spec.Recommend)
	sort.Slice(recs, func(i, j int) bool {
		return recommendPriority(recs[i]) < recommendPriority(recs[j])
	})

	return recs
}

func pickFromStatusMatchList(ptpConfig *ptpv1.PtpConfig, nodeName string) (string, bool) {
	for _, match := range ptpConfig.Status.MatchList {
		if match.NodeName == nil || match.Profile == nil {
			continue
		}

		if *match.NodeName != nodeName {
			continue
		}

		if v, ok := ubloxFromProfileName(ptpConfig, *match.Profile); ok {
			return v, true
		}
	}

	return "", false
}

func pickFromRecommendByNodeName(
	ptpConfig *ptpv1.PtpConfig,
	nodeName string,
	recs []ptpv1.PtpRecommend,
) (string, bool) {
	for _, rec := range recs {
		if rec.Profile == nil {
			continue
		}

		for _, rule := range rec.Match {
			if rule.NodeName != nil && *rule.NodeName == nodeName {
				if v, ok := ubloxFromProfileName(ptpConfig, *rec.Profile); ok {
					return v, true
				}
			}
		}
	}

	return "", false
}

func pickFromRecommendByNodeLabel(
	ptpConfig *ptpv1.PtpConfig,
	nodeLabels map[string]string,
	recs []ptpv1.PtpRecommend,
) (string, bool) {
	for _, rec := range recs {
		if rec.Profile == nil {
			continue
		}

		for _, rule := range rec.Match {
			if rule.NodeLabel == nil {
				continue
			}

			if !nodeLabelMatches(*rule.NodeLabel, nodeLabels) {
				continue
			}

			if v, ok := ubloxFromProfileName(ptpConfig, *rec.Profile); ok {
				return v, true
			}
		}
	}

	return "", false
}

func pickUbloxFromPtpConfig(ptpConfig *ptpv1.PtpConfig, nodeName string, nodeLabels map[string]string) (string, bool) {
	if v, ok := pickFromStatusMatchList(ptpConfig, nodeName); ok {
		return v, true
	}

	recs := sortedRecommendations(ptpConfig)
	if v, ok := pickFromRecommendByNodeName(ptpConfig, nodeName, recs); ok {
		return v, true
	}

	return pickFromRecommendByNodeLabel(ptpConfig, nodeLabels, recs)
}

// GetUbloxProtocolVersion returns ubxtool -P based on the PtpConfig profile applied to nodeName.
func GetUbloxProtocolVersion(apiClient *clients.Settings, nodeName string) (string, error) {
	ptpConfigs, err := infraptp.ListPtpConfigs(apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to list PtpConfigs: %w", err)
	}

	nodeBuilder, err := nodes.Pull(apiClient, nodeName)
	if err != nil {
		return "", fmt.Errorf("failed to pull node %s: %w", nodeName, err)
	}

	nodeLabels := nodeBuilder.Object.Labels

	for _, cfg := range ptpConfigs {
		ptpConfig := cfg.Object
		if ptpConfig == nil {
			ptpConfig = cfg.Definition
		}

		if ptpConfig == nil {
			continue
		}

		if v, ok := pickUbloxFromPtpConfig(ptpConfig, nodeName, nodeLabels); ok {
			return v, nil
		}
	}

	return "", fmt.Errorf(
		"no PtpConfig profile with %s/%s/%s plugin applies to node %s",
		PluginNameE810, PluginNameE825, PluginNameE830, nodeName,
	)
}
