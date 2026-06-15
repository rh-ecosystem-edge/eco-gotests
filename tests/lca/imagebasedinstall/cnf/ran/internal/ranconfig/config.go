package ranconfig

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/internal/ranparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/internal/ibiconfig"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

const (
	// PathToDefaultRanParamsFile path to config file with default ran test parameters.
	PathToDefaultRanParamsFile = "./default.yaml"
)

// RanConfig holds configuration for IBI CNF (ran) workflows such as preinstall.
type RanConfig struct {
	*ibiconfig.IBIConfig

	HubKubeConfig    string `yaml:"hub_kubeconfig" envconfig:"ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG"`
	SeedImage        string `yaml:"seed_image" envconfig:"ECO_LCA_IBI_SEED_IMAGE"`
	SiteConfigRepo   string `yaml:"siteconfig_repo" envconfig:"ECO_LCA_IBI_SITECONFIG_REPO"`
	SiteConfigBranch string `yaml:"siteconfig_branch" envconfig:"ECO_LCA_IBI_SITECONFIG_BRANCH"`
	ReleaseImage     string `yaml:"release_image" envconfig:"ECO_LCA_IBI_RELEASE_IMAGE"`
	ProvisioningHost string `yaml:"provisioning_host" envconfig:"ECO_LCA_IBI_PROVISIONING_HOST"`
	ProvisioningUser string `yaml:"provisioning_user" envconfig:"ECO_LCA_IBI_PROVISIONING_USER"`

	// ProvisioningSSHKey is the private key path for SSH to the provisioning host.
	ProvisioningSSHKey string `yaml:"provisioning_ssh_key" envconfig:"ECO_LCA_IBI_PROVISIONING_SSH_KEY"`

	// ProvisioningSSHDir is the directory containing the default private key (id_rsa or id_ed25519).
	ProvisioningSSHDir string `yaml:"provisioning_ssh_dir" envconfig:"ECO_LCA_IBI_PROVISIONING_SSH_DIR"`

	// BMC credentials are env-only (not unmarshaled from YAML).
	BMCUsername string `envconfig:"ECO_LCA_IBI_BMC_USERNAME"`
	BMCPassword string `envconfig:"ECO_LCA_IBI_BMC_PASSWORD"`

	SiteConfigKustomizePath string `yaml:"siteconfig_kustomize_path" envconfig:"ECO_LCA_IBI_SITECONFIG_KUSTOMIZE_PATH"`

	// SiteConfigGitSkipTLS skips TLS verification when cloning the siteconfig repo (go-git).
	SiteConfigGitSkipTLS bool `yaml:"siteconfig_git_skip_tls" envconfig:"ECO_LCA_IBI_SITECONFIG_GIT_SKIP_TLS"` //nolint:lll // long envconfig tag

	// ISOHTTPBaseURL is the HTTP base for the live ISO (e.g. http://host:8080/images), no trailing slash.
	ISOHTTPBaseURL string `yaml:"iso_http_base_url" envconfig:"ECO_LCA_IBI_ISO_HTTP_BASE_URL"`

	RemoteISOPath string `yaml:"remote_iso_path" envconfig:"ECO_LCA_IBI_REMOTE_ISO_PATH"`

	PreinstallNodeSSHUser string `yaml:"preinstall_node_ssh_user" envconfig:"ECO_LCA_IBI_PREINSTALL_NODE_SSH_USER"`

	PreinstallWaitTimeoutSeconds int `yaml:"preinstall_wait_timeout_seconds" envconfig:"ECO_LCA_IBI_PREINSTALL_WAIT_TIMEOUT_SECONDS"` //nolint:lll // long envconfig tag

	ExtraPartitionLabel string `yaml:"extra_partition_label" envconfig:"ECO_LCA_IBI_EXTRA_PARTITION_LABEL"`

	// BootstrapOC is the host oc binary for `oc adm release extract` when that path exists.
	BootstrapOC string `yaml:"bootstrap_oc" envconfig:"ECO_LCA_IBI_BOOTSTRAP_OC"`
}

// NewRanConfig returns a RanConfig loaded from default.yaml and the environment.
func NewRanConfig() *RanConfig {
	klog.V(ranparams.RANLogLevel).Info("Creating new RanConfig struct")

	var ranConfig RanConfig

	ranConfig.IBIConfig = ibiconfig.NewIBIConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	configFile := filepath.Join(baseDir, PathToDefaultRanParamsFile)

	err := readFile(&ranConfig, configFile)
	if err != nil {
		klog.V(ranparams.RANLogLevel).Infof("Error reading config file %s: %v", configFile, err)
	}

	err = envconfig.Process("", &ranConfig)
	if err != nil {
		klog.V(ranparams.RANLogLevel).Infof("Error reading environment variables: %v", err)

		return nil
	}

	return &ranConfig
}

func readFile(ranConfig *RanConfig, configFile string) error {
	openedConfigFile, err := os.Open(configFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedConfigFile.Close()
	}()

	decoder := yaml.NewDecoder(openedConfigFile)

	return decoder.Decode(ranConfig)
}

// ValidateMandatory returns an error listing any mandatory preinstall settings that are unset.
func (c *RanConfig) ValidateMandatory() error {
	if c == nil {
		return fmt.Errorf("ran config is nil")
	}

	var missing []string

	if c.HubKubeConfig == "" {
		missing = append(missing, "ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG")
	}

	if c.SeedImage == "" {
		missing = append(missing, "ECO_LCA_IBI_SEED_IMAGE")
	}

	if c.SiteConfigRepo == "" {
		missing = append(missing, "ECO_LCA_IBI_SITECONFIG_REPO")
	}

	if c.SiteConfigBranch == "" {
		missing = append(missing, "ECO_LCA_IBI_SITECONFIG_BRANCH")
	}

	if c.ReleaseImage == "" {
		missing = append(missing, "ECO_LCA_IBI_RELEASE_IMAGE")
	}

	if c.ProvisioningHost == "" {
		missing = append(missing, "ECO_LCA_IBI_PROVISIONING_HOST")
	}

	if c.BMCUsername == "" {
		missing = append(missing, "ECO_LCA_IBI_BMC_USERNAME")
	}

	if c.BMCPassword == "" {
		missing = append(missing, "ECO_LCA_IBI_BMC_PASSWORD")
	}

	if c.SiteConfigKustomizePath == "" {
		missing = append(missing, "siteconfig_kustomize_path (default.yaml or ECO_LCA_IBI_SITECONFIG_KUSTOMIZE_PATH)")
	}

	if c.ISOHTTPBaseURL == "" {
		missing = append(missing, "ECO_LCA_IBI_ISO_HTTP_BASE_URL (or iso_http_base_url in default.yaml)")
	}

	if c.RemoteISOPath == "" {
		missing = append(missing, "remote_iso_path (default.yaml or ECO_LCA_IBI_REMOTE_ISO_PATH)")
	}

	if c.ProvisioningUser == "" {
		missing = append(missing, "provisioning_user (default.yaml or ECO_LCA_IBI_PROVISIONING_USER)")
	}

	if c.PreinstallNodeSSHUser == "" {
		missing = append(missing, "preinstall_node_ssh_user (default.yaml or ECO_LCA_IBI_PREINSTALL_NODE_SSH_USER)")
	}

	if c.PreinstallWaitTimeoutSeconds <= 0 {
		missing = append(missing,
			"preinstall_wait_timeout_seconds (default.yaml or ECO_LCA_IBI_PREINSTALL_WAIT_TIMEOUT_SECONDS)")
	}

	if c.ProvisioningSSHKey == "" && c.ProvisioningSSHDir == "" {
		missing = append(missing, "ECO_LCA_IBI_PROVISIONING_SSH_KEY or ECO_LCA_IBI_PROVISIONING_SSH_DIR")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	return nil
}

// ResolveBootstrapOCPath returns the oc binary for `oc adm release extract`.
// Order: BootstrapOC from config (yaml + env) if that path exists and is a file, then oc on PATH.
// A configured path that does not exist is logged and ignored so PATH fallback can run.
func (c *RanConfig) ResolveBootstrapOCPath() (string, error) {
	if c != nil {
		if bootstrapPath := strings.TrimSpace(c.BootstrapOC); bootstrapPath != "" {
			bootstrapFileInfo, err := os.Stat(bootstrapPath)
			if err != nil {
				if os.IsNotExist(err) {
					klog.Warningf("bootstrap oc %q does not exist, falling back to oc on PATH", bootstrapPath)
				} else {
					return "", fmt.Errorf("bootstrap oc %q: %w", bootstrapPath, err)
				}
			} else {
				if bootstrapFileInfo.IsDir() {
					return "", fmt.Errorf("bootstrap oc %q is a directory", bootstrapPath)
				}

				return bootstrapPath, nil
			}
		}
	}

	path, err := exec.LookPath("oc") //nolint:gosec // PATH lookup for oc
	if err != nil {
		return "", fmt.Errorf(
			"bootstrap oc not found: set bootstrap_oc in default.yaml or ECO_LCA_IBI_BOOTSTRAP_OC, or install oc on PATH (%w)",
			err)
	}

	return path, nil
}

// EffectiveProvisioningSSHKey returns the SSH private key path for the provisioning host.
func (c *RanConfig) EffectiveProvisioningSSHKey() string {
	if c == nil {
		return ""
	}

	if c.ProvisioningSSHKey != "" {
		return c.ProvisioningSSHKey
	}

	dir := c.ProvisioningSSHDir

	for _, name := range []string{"id_rsa", "id_ed25519"} {
		p := filepath.Join(dir, name)

		fi, err := os.Stat(p)
		if err == nil && !fi.IsDir() {
			return p
		}
	}

	return filepath.Join(dir, "id_rsa")
}

// EffectivePreinstallWait returns the max wait duration loaded from configuration (no hard-coded default).
func (c *RanConfig) EffectivePreinstallWait() time.Duration {
	if c == nil {
		return 0
	}

	return time.Duration(c.PreinstallWaitTimeoutSeconds) * time.Second
}

// ISOArtifactURL builds the full URL for a file under ISOHTTPBaseURL (e.g. rhcos-ibi.iso).
func (c *RanConfig) ISOArtifactURL(filename string) string {
	base := strings.TrimSuffix(c.ISOHTTPBaseURL, "/")

	return fmt.Sprintf("%s/%s", base, strings.TrimPrefix(filename, "/"))
}
