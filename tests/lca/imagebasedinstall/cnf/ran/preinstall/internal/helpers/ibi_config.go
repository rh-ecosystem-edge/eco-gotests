package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	assistedv1 "github.com/openshift/assisted-service/api/v1beta1"
	"github.com/openshift/installer/pkg/types"
	"github.com/openshift/installer/pkg/types/imagebased"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// IBIConfigData holds values used to build imagebased.InstallationConfig.
type IBIConfigData struct {
	Architecture           string
	SeedImage              string
	SeedVersion            string
	AdditionalTrustBundle  string
	ImageDigestSources     []types.ImageDigestSource
	PullSecret             string
	InstallationDisk       string
	SSHKey                 string
	NetworkConfig          *assistedv1.NetConfig
	IgnitionConfigOverride string
	ExtraPartitionLabel    string
}

// NormalizeIgnitionConfigOverrideForIBI validates ignition JSON and returns a single-line compact form,
// matching Ansible `ignition_config | to_json` for use in image-based-installation-config.yaml.
func NormalizeIgnitionConfigOverrideForIBI(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	var buf bytes.Buffer

	err := json.Compact(&buf, []byte(trimmed))
	if err != nil {
		return "", fmt.Errorf("ignitionConfigOverride is not valid JSON: %w", err)
	}

	return buf.String(), nil
}

// GenerateIBIConfig writes image-based-installation-config.yaml using openshift/installer API types.
func GenerateIBIConfig(data IBIConfigData, destDir string) error {
	klog.Infof("Generating image-based-installation-config.yaml in %s", destDir)

	ignition := strings.TrimSpace(data.IgnitionConfigOverride)
	if ignition != "" {
		normalized, err := NormalizeIgnitionConfigOverrideForIBI(ignition)
		if err != nil {
			return fmt.Errorf("ignitionConfigOverride: %w", err)
		}

		ignition = normalized
	}

	cfg := imagebased.InstallationConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: imagebased.ImageBasedConfigVersion,
			Kind:       "ImageBasedInstallationConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "image-based-installation-config",
		},
		AdditionalTrustBundle: data.AdditionalTrustBundle,
		PullSecret:            data.PullSecret,
		InstallationDisk:      data.InstallationDisk,
		SSHKey:                data.SSHKey,
		SeedImage:             data.SeedImage,
		SeedVersion:           data.SeedVersion,
		NetworkConfig:         data.NetworkConfig,
		ImageDigestSources:    data.ImageDigestSources,
	}

	if data.Architecture != "" {
		cfg.Architecture = data.Architecture
	}

	if ignition != "" {
		cfg.IgnitionConfigOverride = ignition
	}

	if data.ExtraPartitionLabel != "" {
		cfg.ExtraPartitionLabel = data.ExtraPartitionLabel
	}

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal InstallationConfig: %w", err)
	}

	destPath := filepath.Join(destDir, "image-based-installation-config.yaml")

	err = os.WriteFile(destPath, out, 0o600)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	klog.Infof("Successfully generated %s", destPath)

	return nil
}

// ParseClusterInstance parses kustomize multi-doc YAML and returns the ClusterInstance CR.
func ParseClusterInstance(kustomizeOutput []byte) (*siteconfigv1alpha1.ClusterInstance, error) {
	docs := strings.Split(string(kustomizeOutput), "---")

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var clusterInst siteconfigv1alpha1.ClusterInstance

		err := yaml.Unmarshal([]byte(doc), &clusterInst)
		if err != nil {
			continue
		}

		if clusterInst.Kind == "ClusterInstance" {
			return &clusterInst, nil
		}
	}

	return nil, fmt.Errorf("ClusterInstance not found in kustomize output")
}

// SeedVersionFromSeedImage derives seedVersion from seedImage for ImageBasedInstallationConfig (Ansible ibi_ocp_build).
// Digest-pinned refs (anything after "@", e.g. "@sha256:...") are ignored when extracting a tag: only the repository
// side of "@" is considered. The tag is the substring after the last ':' only when that ':' follows the last '/'
// (so digest hex after sha256 is not used as seedVersion, and host:port before the path is not mistaken for a tag).
func SeedVersionFromSeedImage(seedImage string) string {
	ref := strings.TrimSpace(seedImage)
	if ref == "" {
		return ""
	}

	if i := strings.Index(ref, "@"); i >= 0 {
		ref = strings.TrimSpace(ref[:i])
	}

	if ref == "" {
		return ""
	}

	lastSlash := strings.LastIndex(ref, "/")

	lastColon := strings.LastIndex(ref, ":")
	if lastColon <= lastSlash {
		return ""
	}

	return ref[lastColon+1:]
}
