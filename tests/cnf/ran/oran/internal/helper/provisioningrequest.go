package helper

import (
	"fmt"
	"os"
	"sync"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

// provisioningRequestFile is a minimal decode target for site-config ProvisioningRequest YAML. Only the fields needed
// to construct test ProvisioningRequests and extract spoke hostnames are parsed.
type provisioningRequestFile struct {
	Spec struct {
		TemplateName       string         `yaml:"templateName"`
		TemplateVersion    string         `yaml:"templateVersion"`
		TemplateParameters map[string]any `yaml:"templateParameters"`
	} `yaml:"spec"`
}

var (
	prTemplateOnce   sync.Once
	cachedPRTemplate *provisioningRequestFile
	cachedPRSource   string
	errCachedPR      error
)

// loadProvisioningRequestTemplate loads and caches the ProvisioningRequest YAML from
// ECO_CNF_RAN_PROVISIONING_REQUEST_URL (preferred for Jenkins/container runs) or
// ECO_CNF_RAN_PROVISIONING_REQUEST_PATH.
func loadProvisioningRequestTemplate() (*provisioningRequestFile, error) {
	prTemplateOnce.Do(func() {
		switch {
		case RANConfig.OranProvisioningRequestURL != "":
			cachedPRSource = RANConfig.OranProvisioningRequestURL

			klog.V(tsparams.LogLevel).Infof("Loading ProvisioningRequest template from URL %s", cachedPRSource)

			data, err := fetchYAMLFromURL(RANConfig.OranProvisioningRequestURL, RANConfig.SkipTLSVerify)
			if err != nil {
				errCachedPR = err

				return
			}

			cachedPRTemplate, errCachedPR = parseProvisioningRequestYAML(data, cachedPRSource)
		case RANConfig.OranProvisioningRequestPath != "":
			cachedPRSource = RANConfig.OranProvisioningRequestPath

			klog.V(tsparams.LogLevel).Infof("Loading ProvisioningRequest template from path %s", cachedPRSource)

			data, err := os.ReadFile(RANConfig.OranProvisioningRequestPath)
			if err != nil {
				errCachedPR = fmt.Errorf("read ProvisioningRequest file %s: %w", cachedPRSource, err)

				return
			}

			cachedPRTemplate, errCachedPR = parseProvisioningRequestYAML(data, cachedPRSource)
		default:
			errCachedPR = fmt.Errorf(
				"ECO_CNF_RAN_PROVISIONING_REQUEST_URL or ECO_CNF_RAN_PROVISIONING_REQUEST_PATH must be set")
		}
	})

	return cachedPRTemplate, errCachedPR
}

// parseProvisioningRequestYAML unmarshals ProvisioningRequest YAML and validates that templateName and
// templateParameters are present.
func parseProvisioningRequestYAML(data []byte, source string) (*provisioningRequestFile, error) {
	var parsed provisioningRequestFile

	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse ProvisioningRequest from %s: %w", source, err)
	}

	if parsed.Spec.TemplateName == "" {
		return nil, fmt.Errorf("ProvisioningRequest from %s has empty spec.templateName", source)
	}

	if parsed.Spec.TemplateParameters == nil {
		return nil, fmt.Errorf("ProvisioningRequest from %s has nil spec.templateParameters", source)
	}

	return &parsed, nil
}

// GetClusterTemplateName returns the ClusterTemplate base name from spec.templateName in the loaded
// ProvisioningRequest YAML.
func GetClusterTemplateName() (string, error) {
	prTemplate, err := loadProvisioningRequestTemplate()
	if err != nil {
		return "", err
	}

	return prTemplate.Spec.TemplateName, nil
}

// GetExtraManifestsName returns the expected extra-manifests ConfigMap name for the resolved ClusterTemplate.
func GetExtraManifestsName() (string, error) {
	clusterTemplateName, err := GetClusterTemplateName()
	if err != nil {
		return "", err
	}

	return clusterTemplateName + "-extra-manifest-1", nil
}

// GetSpokeHostnames returns the expected spoke hostnames extracted from the loaded ProvisioningRequest template's
// clusterInstanceParameters (supporting both nodeGroups for MNO and nodes for SNO).
func GetSpokeHostnames() ([]string, error) {
	prTemplate, err := loadProvisioningRequestTemplate()
	if err != nil {
		return nil, err
	}

	ciParamsRaw, hasParams := prTemplate.Spec.TemplateParameters[tsparams.ClusterInstanceParamsKey]
	if !hasParams {
		return nil, fmt.Errorf("ProvisioningRequest from %s has no %s in templateParameters",
			cachedPRSource, tsparams.ClusterInstanceParamsKey)
	}

	ciMap, isMap := ciParamsRaw.(map[string]any)
	if !isMap {
		return nil, fmt.Errorf("ProvisioningRequest from %s: %s is not a map",
			cachedPRSource, tsparams.ClusterInstanceParamsKey)
	}

	return extractHostnames(ciMap, cachedPRSource)
}

// extractHostnames reads hostnames from a clusterInstanceParameters map. It checks for nodeGroups (MNO) first, then
// falls back to nodes (SNO).
func extractHostnames(ciParams map[string]any, source string) ([]string, error) {
	if nodeGroups, ok := ciParams["nodeGroups"]; ok {
		return extractHostnamesFromNodeGroups(nodeGroups, source)
	}

	if nodes, ok := ciParams["nodes"]; ok {
		return extractHostnamesFromNodes(nodes, source)
	}

	return nil, fmt.Errorf(
		"ProvisioningRequest from %s: clusterInstanceParameters has neither nodeGroups nor nodes", source)
}

// extractHostnamesFromNodeGroups extracts hostnames from the MNO nodeGroups structure:
// nodeGroups[].nodes[].hostName.
func extractHostnamesFromNodeGroups(nodeGroupsRaw any, source string) ([]string, error) {
	nodeGroups, ok := nodeGroupsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("ProvisioningRequest from %s: nodeGroups is not a list", source)
	}

	var hostnames []string

	for groupIdx, groupRaw := range nodeGroups {
		group, isMap := groupRaw.(map[string]any)
		if !isMap {
			return nil, fmt.Errorf("ProvisioningRequest from %s: nodeGroups[%d] is not a map", source, groupIdx)
		}

		groupHostnames, err := extractHostnamesFromNodes(group["nodes"], source)
		if err != nil {
			return nil, fmt.Errorf("ProvisioningRequest from %s: nodeGroups[%d]: %w", source, groupIdx, err)
		}

		hostnames = append(hostnames, groupHostnames...)
	}

	if len(hostnames) == 0 {
		return nil, fmt.Errorf("ProvisioningRequest from %s: nodeGroups contain no nodes", source)
	}

	return hostnames, nil
}

// extractHostnamesFromNodes extracts hostnames from a nodes list: nodes[].hostName.
func extractHostnamesFromNodes(nodesRaw any, source string) ([]string, error) {
	nodes, ok := nodesRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("ProvisioningRequest from %s: nodes is not a list", source)
	}

	hostnames := make([]string, 0, len(nodes))

	for nodeIdx, nodeRaw := range nodes {
		node, isMap := nodeRaw.(map[string]any)
		if !isMap {
			return nil, fmt.Errorf("ProvisioningRequest from %s: nodes[%d] is not a map", source, nodeIdx)
		}

		hostname, hasName := node["hostName"].(string)
		if !hasName || hostname == "" {
			return nil, fmt.Errorf("ProvisioningRequest from %s: nodes[%d] has no hostName", source, nodeIdx)
		}

		hostnames = append(hostnames, hostname)
	}

	return hostnames, nil
}

// buildSecondaryCIParams deep-copies the clusterInstanceParameters from the loaded ProvisioningRequest and replaces
// clusterName and all hostnames with synthetic values for the secondary ProvisioningRequest.
func buildSecondaryCIParams(prTemplate *provisioningRequestFile) (map[string]any, error) {
	ciParamsRaw, hasParams := prTemplate.Spec.TemplateParameters[tsparams.ClusterInstanceParamsKey]
	if !hasParams {
		return nil, fmt.Errorf("ProvisioningRequest template has no %s", tsparams.ClusterInstanceParamsKey)
	}

	copied, err := deepCopyViaYAML(ciParamsRaw)
	if err != nil {
		return nil, fmt.Errorf("deep-copy clusterInstanceParameters: %w", err)
	}

	ciParams, isMap := copied.(map[string]any)
	if !isMap {
		return nil, fmt.Errorf("clusterInstanceParameters is not a map after deep-copy")
	}

	ciParams["clusterName"] = tsparams.TestName2

	counter := 0

	if nodeGroups, isSlice := ciParams["nodeGroups"].([]any); isSlice {
		for _, groupRaw := range nodeGroups {
			group, isGroupMap := groupRaw.(map[string]any)
			if !isGroupMap {
				continue
			}

			nodes, isNodeSlice := group["nodes"].([]any)
			if !isNodeSlice {
				continue
			}

			for _, nodeRaw := range nodes {
				node, isNodeMap := nodeRaw.(map[string]any)
				if !isNodeMap {
					continue
				}

				node["hostName"] = fmt.Sprintf("%s-%d", tsparams.TestName2, counter)
				counter++
			}
		}
	} else if nodes, isSlice := ciParams["nodes"].([]any); isSlice {
		for _, nodeRaw := range nodes {
			node, isNodeMap := nodeRaw.(map[string]any)
			if !isNodeMap {
				continue
			}

			node["hostName"] = fmt.Sprintf("%s-%d", tsparams.TestName2, counter)
			counter++
		}
	}

	return ciParams, nil
}

// deepCopyViaYAML round-trips a value through YAML marshal/unmarshal to produce a deep copy.
func deepCopyViaYAML(v any) (any, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	var out any

	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}

	return out, nil
}
