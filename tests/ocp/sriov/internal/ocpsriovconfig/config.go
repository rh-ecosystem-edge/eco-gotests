package sriovconfig

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/internal/ocpconfig"
	"gopkg.in/yaml.v2"
)

const (
	// PathToDefaultOcpSriovParamsFile path to config file with default ocp sriov parameters.
	PathToDefaultOcpSriovParamsFile = "./default.yaml"
)

// SriovOcpConfig type keeps sriov configuration.
type SriovOcpConfig struct {
	*ocpconfig.OcpConfig
	OcpWorkerLabel            string `yaml:"ocp_worker_label" envconfig:"ECO_OCP_SRIOV_WORKER_LABEL"`
	OcpSriovOperatorNamespace string `yaml:"sriov_operator_namespace" envconfig:"ECO_OCP_SRIOV_OPERATOR_NAMESPACE"`
	OcpSriovTestContainer     string `yaml:"ocp_sriov_test_container" envconfig:"ECO_OCP_SRIOV_TEST_CONTAINER"`
	SriovInterfaces           string `envconfig:"ECO_OCP_SRIOV_SRIOV_INTERFACE_LIST"`
}

// NewSriovOcpConfig returns instance of SriovConfig config type.
func NewSriovOcpConfig() *SriovOcpConfig {
	log.Print("Creating new SriovOcpConfig struct")

	var sriovOcpConf SriovOcpConfig

	sriovOcpConf.OcpConfig = ocpconfig.NewOcpConfig()

	if sriovOcpConf.OcpConfig == nil {
		log.Print("Error to initialize OcpConfig")

		return nil
	}

	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	confFile := filepath.Join(baseDir, PathToDefaultOcpSriovParamsFile)

	err := readFile(&sriovOcpConf, confFile)
	if err != nil {
		log.Printf("Error to read config file %s", confFile)

		return nil
	}

	err = readEnv(&sriovOcpConf)
	if err != nil {
		log.Print("Error to read environment variables")

		return nil
	}

	return &sriovOcpConf
}

func readFile(sriovOcpConfig *SriovOcpConfig, cfgFile string) error {
	openedCfgFile, err := os.Open(cfgFile)
	if err != nil {
		return err
	}

	defer func() {
		_ = openedCfgFile.Close()
	}()

	decoder := yaml.NewDecoder(openedCfgFile)

	err = decoder.Decode(sriovOcpConfig)
	if err != nil {
		return err
	}

	return nil
}

func readEnv(sriovOcpConfig *SriovOcpConfig) error {
	err := envconfig.Process("", sriovOcpConfig)
	if err != nil {
		return err
	}

	return nil
}

// GetJunitReportPath returns full path to the junit report file with timestamp.
// Format: {reportsDir}/sriov_suite_test_{timestamp}_junit.xml
func (cfg *SriovOcpConfig) GetJunitReportPath(file string) string {
	reportFileName := filepath.Base(file)
	reportFileName = reportFileName[:len(reportFileName)-len(filepath.Ext(reportFileName))]
	
	// Add timestamp to filename: YYYYMMDD_HHMMSS
	timestamp := time.Now().Format("20060102_150405")
	
	return fmt.Sprintf("%s_%s_junit.xml", filepath.Join(cfg.ReportsDirAbsPath, reportFileName), timestamp)
}

// GetReportPath returns full path to the reportxml file with timestamp.
// Format: {reportsDir}/report_{timestamp}_testrun.xml
func (cfg *SriovOcpConfig) GetReportPath() string {
	if !cfg.EnableReport {
		return ""
	}
	
	// Add timestamp to filename: YYYYMMDD_HHMMSS
	timestamp := time.Now().Format("20060102_150405")
	
	return fmt.Sprintf("%s_%s_testrun.xml", filepath.Join(cfg.ReportsDirAbsPath, "report"), timestamp)
}
