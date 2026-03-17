package preinstall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

// IBICfgTemplate is the template for the image-based-installation-config.yaml.
const IBICfgTemplate = `apiVersion: v1beta1
kind: ImageBasedInstallationConfig
metadata:
  name: image-based-installation-config
{{- if .Architecture }}
architecture: {{ .Architecture }}
{{- end }}
seedImage: {{ .SeedImage }}
seedVersion: {{ .SeedVersion }}
additionalTrustBundle: |
{{ .AdditionalTrustBundle | indent 2 }}
imageDigestSources:
{{- range .ImageDigestSources }}
  - mirrors:
    {{- range .Mirrors }}
    - {{ . }}
    {{- end }}
    source: {{ .Source }}
{{- end }}
pullSecret: |
{{ .PullSecret | indent 2 }}
installationDisk: "{{ .InstallationDisk }}"
sshKey: "{{ .SSHKey }}"
networkConfig:
{{ .NetworkConfig | indent 2 }}
{{- if .IgnitionConfigOverride }}
ignitionConfigOverride: |
  {{ .IgnitionConfigOverride }}
{{- end }}
{{- if .ExtraPartitionLabel }}
extraPartitionLabel: {{ .ExtraPartitionLabel }}
{{- end }}
`

// ImageDigestSource represents a mirror source.
type ImageDigestSource struct {
	Mirrors []string `yaml:"mirrors"`
	Source  string   `yaml:"source"`
}

// IBIConfigData holds the data needed to render the IBI config template.
type IBIConfigData struct {
	Architecture           string
	SeedImage              string
	SeedVersion            string
	AdditionalTrustBundle  string
	ImageDigestSources     []ImageDigestSource
	PullSecret             string
	InstallationDisk       string
	SSHKey                 string
	NetworkConfig          string
	IgnitionConfigOverride string
	ExtraPartitionLabel    string
}

// GenerateIBIConfig generates the image-based-installation-config.yaml file.
func GenerateIBIConfig(data IBIConfigData, destDir string) error {
	klog.Infof("Generating image-based-installation-config.yaml in %s", destDir)

	// Create a template with a custom indent function
	funcMap := template.FuncMap{
		"indent": func(spaces int, v string) string {
			pad := strings.Repeat(" ", spaces)

			return pad + strings.ReplaceAll(v, "\n", "\n"+pad)
		},
	}

	tmpl, err := template.New("ibi-config").Funcs(funcMap).Parse(IBICfgTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Create the output file
	destPath := filepath.Join(destDir, "image-based-installation-config.yaml")

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	defer file.Close()

	// Execute the template
	err = tmpl.Execute(file, data)
	if err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	klog.Infof("Successfully generated %s", destPath)

	return nil
}

// ParseClusterInstance parses the kustomize output to find the ClusterInstance CR.
func ParseClusterInstance(kustomizeOutput []byte) (map[string]interface{}, error) {
	// Split the multi-document YAML
	docs := strings.Split(string(kustomizeOutput), "---")

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var obj map[string]interface{}

		err := yaml.Unmarshal([]byte(doc), &obj)
		if err != nil {
			continue // Skip invalid docs
		}

		kind, ok := obj["kind"].(string)
		if ok && kind == "ClusterInstance" {
			return obj, nil
		}
	}

	return nil, fmt.Errorf("ClusterInstance not found in kustomize output")
}

// ParseNetworkConfigFromClusterInstance extracts the network configuration from ClusterInstance.
// It returns the network config as YAML string from spec.nodes[0].nodeNetwork.config.
func ParseNetworkConfigFromClusterInstance(clusterInstance map[string]interface{}) (string, error) {
	spec, okSpec := clusterInstance["spec"].(map[string]interface{})
	if !okSpec {
		return "", fmt.Errorf("failed to extract spec from ClusterInstance")
	}

	nodes, okNodes := spec["nodes"].([]interface{})
	if !okNodes || len(nodes) == 0 {
		return "", fmt.Errorf("failed to extract nodes from ClusterInstance spec")
	}

	node, okNode := nodes[0].(map[string]interface{})
	if !okNode {
		return "", fmt.Errorf("failed to extract first node from nodes")
	}

	nodeNetwork, okNetwork := node["nodeNetwork"].(map[string]interface{})
	if !okNetwork {
		return "", fmt.Errorf("failed to extract nodeNetwork from node")
	}

	config, okConfig := nodeNetwork["config"].(map[string]interface{})
	if !okConfig {
		return "", fmt.Errorf("failed to extract config from nodeNetwork")
	}

	// Marshal the config back to YAML
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal network config to YAML: %w", err)
	}

	return string(configYAML), nil
}

// ParseInstallationDiskFromClusterInstance extracts and converts the installation disk path.
// It parses rootDeviceHints from spec.nodes[0] and converts to the appropriate device path.
// Logic matches Ansible: deviceName if present, else /dev/disk/by-id/wwn-<wwn> or nvme-<eui>.
func ParseInstallationDiskFromClusterInstance(clusterInstance map[string]interface{}) (string, error) {
	spec, okSpec := clusterInstance["spec"].(map[string]interface{})
	if !okSpec {
		return "", fmt.Errorf("failed to extract spec from ClusterInstance")
	}

	nodes, okNodes := spec["nodes"].([]interface{})
	if !okNodes || len(nodes) == 0 {
		return "", fmt.Errorf("failed to extract nodes from ClusterInstance spec")
	}

	node, okNode := nodes[0].(map[string]interface{})
	if !okNode {
		return "", fmt.Errorf("failed to extract first node from nodes")
	}

	rootDeviceHints, okHints := node["rootDeviceHints"].(map[string]interface{})
	if !okHints {
		return "", fmt.Errorf("failed to extract rootDeviceHints from node")
	}

	// Check for deviceName first (simplest case)
	if deviceName, okDevice := rootDeviceHints["deviceName"].(string); okDevice {
		return deviceName, nil
	}

	// Check for WWN
	if wwn, okWWN := rootDeviceHints["wwn"].(string); okWWN {
		// Check if it's an NVMe device (EUI format)
		if strings.HasPrefix(wwn, "eui.") {
			return fmt.Sprintf("/dev/disk/by-id/nvme-%s", wwn), nil
		}

		// Standard WWN format - convert to hex if needed
		return fmt.Sprintf("/dev/disk/by-id/wwn-%s", wwn), nil
	}

	return "", fmt.Errorf("no supported rootDeviceHints found (deviceName or wwn)")
}
