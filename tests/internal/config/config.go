package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultParamsFile path to config file with default parameters.
	PathToDefaultParamsFile = "./default.yaml"
)

// GeneralConfig type keeps general configuration.
type GeneralConfig struct {
	ReportsDirAbsPath         string `yaml:"reports_dump_dir" envconfig:"ECO_REPORTS_DUMP_DIR"`
	VerboseLevel              string `yaml:"verbose_level" envconfig:"ECO_VERBOSE_LEVEL"`
	DumpFailedTests           bool   `yaml:"dump_failed_tests" envconfig:"ECO_DUMP_FAILED_TESTS"`
	EnableReport              bool   `yaml:"enable_report" envconfig:"ECO_ENABLE_REPORT"`
	DryRun                    bool   `yaml:"dry_run" envconfig:"ECO_DRY_RUN"`
	SSHKeyPath                string `envconfig:"ECO_SSH_KEY_PATH"`
	SSHUser                   string `yaml:"ssh_user" envconfig:"ECO_SSH_USER"`
	KubernetesRolePrefix      string `yaml:"kubernetes_role_prefix" envconfig:"ECO_KUBERNETES_ROLE_PREFIX"`
	WorkerLabelEnvVar         string `yaml:"worker_label" envconfig:"ECO_WORKER_LABEL"`
	WorkerLabel               string
	ControlPlaneLabel         string `yaml:"control_plane_label" envconfig:"ECO_CONTROL_PLANE_LABEL"`
	TCPrefix                  string `yaml:"tc_prefix" envconfig:"ECO_TC_PREFIX"`
	MCONamespace              string `yaml:"mco_namespace" envconfig:"ECO_MCO_NAMESPACE"`
	LoggingOperatorNamespace  string `yaml:"logging_operator_namespace" envconfig:"ECO_LOGGING_OPERATOR_NAMESPACE"`
	MCOConfigDaemonName       string `yaml:"mco_config_daemon_name" envconfig:"ECO_MCO_CONFIG_DAEMON_NAME"`
	SriovOperatorNamespace    string `yaml:"sriov_operator_namespace" envconfig:"ECO_SRIOV_OPERATOR_NAMESPACE"`
	NMStateOperatorNamespace  string `yaml:"nmstate_operator_namespace" envconfig:"ECO_NMSTATE_OPERATOR_NAMESPACE"`
	SriovFecOperatorNamespace string `yaml:"sriov_fec_operator_namespace" envconfig:"ECO_SRIOV_FEC_OPERATOR_NAMESPACE"`
	WorkerLabelMap            map[string]string
	ControlPlaneLabelMap      map[string]string
}

// NewConfig returns instance of GeneralConfig config type.
func NewConfig() *GeneralConfig {
	log.Print("Creating new GeneralConfig struct")

	var conf GeneralConfig

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultParamsFile)

	err := readFile(&conf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&conf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	err = deployReportDir(conf.ReportsDirAbsPath)
	if err != nil {
		log.Printf("Error to deploy report directory %s due to %s", conf.ReportsDirAbsPath, err.Error())

		return nil
	}

	return &conf
}

// GetJunitReportPath returns full path to the junit report file.
func (cfg *GeneralConfig) GetJunitReportPath(file string) string {
	reportFileName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))

	return fmt.Sprintf("%s_junit.xml", filepath.Join(cfg.ReportsDirAbsPath, reportFileName))
}

// GetReportPath returns full path to the reportxml file.
func (cfg *GeneralConfig) GetReportPath() string {
	reportFileName := strings.TrimSuffix(filepath.Base("report"), filepath.Ext(filepath.Base("report")))

	if !cfg.EnableReport {
		return ""
	}

	return fmt.Sprintf("%s_testrun.xml", filepath.Join(cfg.ReportsDirAbsPath, reportFileName))
}

// GetDumpFailedTestReportLocation returns destination file for failed tests logs.
func (cfg *GeneralConfig) GetDumpFailedTestReportLocation(file string) string {
	if cfg.DumpFailedTests {
		if _, err := os.Stat(cfg.ReportsDirAbsPath); os.IsNotExist(err) {
			err := os.MkdirAll(cfg.ReportsDirAbsPath, 0744)
			if err != nil {
				log.Fatalf("panic: Failed to create report dir due to %s", err)
			}
		}

		dumpFileName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(filepath.Base(file)))

		return filepath.Join(cfg.ReportsDirAbsPath, fmt.Sprintf("failed_%s", dumpFileName))
	}

	return ""
}

func readFile(cfg *GeneralConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(&cfg)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(cfg *GeneralConfig) error {
	err := envconfig.Process("", cfg)
	if err != nil {
		return err
	}

	cfg.WorkerLabel = fmt.Sprintf("%s/%s", cfg.KubernetesRolePrefix, cfg.WorkerLabelEnvVar)
	cfg.ControlPlaneLabel = fmt.Sprintf("%s/%s", cfg.KubernetesRolePrefix, cfg.ControlPlaneLabel)
	cfg.WorkerLabelMap = map[string]string{cfg.WorkerLabel: ""}
	cfg.ControlPlaneLabelMap = map[string]string{cfg.ControlPlaneLabel: ""}

	return nil
}

func deployReportDir(dirName string) error {
	_, err := os.Stat(dirName)

	if os.IsNotExist(err) {
		return os.MkdirAll(dirName, 0777)
	}

	return err
}
