package vcoreconfig

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultVCoreParamsFile path to config file with default vCore parameters.
	PathToDefaultVCoreParamsFile = "./default.yaml"
)

// VCoreConfig type keeps vCore configuration.
type VCoreConfig struct {
	*systemtestsconfig.SystemTestsConfig
	Namespace                   string `yaml:"vcore_default_ns" envconfig:"ECO_SYSTEM_VCORE_NS"`
	OdfMCPName                  string `yaml:"odf_mcp" envconfig:"ECO_SYSTEM_VCORE_ODF_MCP"`
	VCorePpMCPName              string `yaml:"vcore_pp_mcp" envconfig:"ECO_SYSTEM_VCORE_PP_MCP"`
	VCoreCpMCPName              string `yaml:"vcore_cp_mcp" envconfig:"ECO_SYSTEM_VCORE_CP_MCP"`
	Host                        string `yaml:"host" envconfig:"ECO_SYSTEM_VCORE_HOST"`
	User                        string `yaml:"user" envconfig:"ECO_SYSTEM_VCORE_USER"`
	Pass                        string `yaml:"pass" envconfig:"ECO_SYSTEM_VCORE_PASS"`
	MirrorRegistryUser          string `yaml:"mirror_registry_user" envconfig:"ECO_SYSTEM_VCORE_MIRROR_REGISTRY_USER"`
	MirrorRegistryPass          string `yaml:"mirror_registry_pass" envconfig:"ECO_SYSTEM_VCORE_MIRROR_REGISTRY_PASSWORD"`
	CombinedPullSecretFile      string `yaml:"combined_pull_secret" envconfig:"ECO_SYSTEM_VCORE_COMBINED_PULL_SECRET"`
	PrivateKey                  string `yaml:"private_key" envconfig:"ECO_SYSTEM_VCORE_PRIVATE_KEY"`
	RegistryRepository          string `yaml:"registry_repository" envconfig:"ECO_SYSTEM_VCORE_REGISTRY_REPOSITORY"`
	CPUIsolated                 string `yaml:"cpu_isolated" envconfig:"ECO_SYSTEM_VCORE_CPU_ISOLATED"`
	CPUReserved                 string `yaml:"cpu_reserved" envconfig:"ECO_SYSTEM_VCORE_CPU_RESERVED"`
	KubeconfigPath              string `yaml:"kubeconfig_path" envconfig:"ECO_SYSTEM_VCORE_KUBECONFIG"`
	OdfLabel                    string
	VCorePpLabel                string
	VCoreCpLabel                string
	ControlPlaneLabelListOption metav1.ListOptions
	WorkerLabelListOption       metav1.ListOptions
	OdfLabelListOption          metav1.ListOptions
	VCorePpLabelListOption      metav1.ListOptions
	VCoreCpLabelListOption      metav1.ListOptions
	OdfLabelMap                 map[string]string
	VCorePpLabelMap             map[string]string
	VCoreCpLabelMap             map[string]string
}

// NewVCoreConfig returns instance of VCoreConfig config type.
func NewVCoreConfig() *VCoreConfig {
	log.Print("Creating new VCoreConfig struct")

	var vcoreConf VCoreConfig

	vcoreConf.SystemTestsConfig = systemtestsconfig.NewSystemTestsConfig()

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultVCoreParamsFile)

	err := readFile(&vcoreConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&vcoreConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &vcoreConf
}

func readFile(vcoreConfig *VCoreConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&vcoreConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(vcoreConfig *VCoreConfig) error {
	err := envconfig.Process("", vcoreConfig)
	if err != nil {
		return err
	}

	vcoreConfig.OdfLabel = fmt.Sprintf("%s/%s", vcoreConfig.KubernetesRolePrefix, vcoreConfig.OdfMCPName)
	vcoreConfig.VCorePpLabel = fmt.Sprintf("%s/%s", vcoreConfig.KubernetesRolePrefix, vcoreConfig.VCorePpMCPName)
	vcoreConfig.VCoreCpLabel = fmt.Sprintf("%s/%s", vcoreConfig.KubernetesRolePrefix, vcoreConfig.VCoreCpMCPName)
	vcoreConfig.ControlPlaneLabelListOption = metav1.ListOptions{LabelSelector: vcoreConfig.ControlPlaneLabel}
	vcoreConfig.WorkerLabelListOption = metav1.ListOptions{LabelSelector: vcoreConfig.WorkerLabel}
	vcoreConfig.OdfLabelListOption = metav1.ListOptions{LabelSelector: vcoreConfig.OdfLabel}
	vcoreConfig.VCorePpLabelListOption = metav1.ListOptions{LabelSelector: vcoreConfig.VCorePpLabel}
	vcoreConfig.VCoreCpLabelListOption = metav1.ListOptions{LabelSelector: vcoreConfig.VCoreCpLabel}
	vcoreConfig.OdfLabelMap = map[string]string{vcoreConfig.OdfLabel: ""}
	vcoreConfig.VCorePpLabelMap = map[string]string{vcoreConfig.VCorePpLabel: ""}
	vcoreConfig.VCoreCpLabelMap = map[string]string{vcoreConfig.VCoreCpLabel: ""}

	return nil
}
