package ptp

import (
	"fmt"
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

func ubloxFromProfileName(pc *ptpv1.PtpConfig, profileName string) (string, bool) {
	for i := range pc.Spec.Profile {
		p := &pc.Spec.Profile[i]
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

		return nodeLabels[k] == v
	}

	_, ok := nodeLabels[rule]

	return ok
}

func pickUbloxFromPtpConfig(pc *ptpv1.PtpConfig, nodeName string, nodeLabels map[string]string) (string, bool) {
	for _, m := range pc.Status.MatchList {
		if m.NodeName == nil || m.Profile == nil {
			continue
		}

		if *m.NodeName != nodeName {
			continue
		}

		if v, ok := ubloxFromProfileName(pc, *m.Profile); ok {
			return v, true
		}
	}

	for _, rec := range pc.Spec.Recommend {
		if rec.Profile == nil {
			continue
		}

		for _, rule := range rec.Match {
			if rule.NodeName != nil && *rule.NodeName == nodeName {
				if v, ok := ubloxFromProfileName(pc, *rec.Profile); ok {
					return v, true
				}
			}
		}
	}

	for _, rec := range pc.Spec.Recommend {
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

			if v, ok := ubloxFromProfileName(pc, *rec.Profile); ok {
				return v, true
			}
		}
	}

	return "", false
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
		pc := cfg.Object
		if pc == nil {
			pc = cfg.Definition
		}

		if v, ok := pickUbloxFromPtpConfig(pc, nodeName, nodeLabels); ok {

			return v, nil
		}
	}

	return "", fmt.Errorf(
		"no PtpConfig profile with %s/%s/%s plugin applies to node %s",
		PluginNameE810, PluginNameE825, PluginNameE830, nodeName,
	)
}
