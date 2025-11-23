package reporter

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/onsi/ginkgo/v2/types"
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

var (
	pathToPodExecLogs = "/tmp/pod_exec_logs.log"
	// generalCfg holds the configuration for reporter operations.
	generalCfg *config.GeneralConfig
)

// init initializes the reporter configuration.
func init() {
	// Skip loading config if running unit tests
	if os.Getenv("UNIT_TEST") == "true" {
		return
	}

	generalCfg = config.NewConfig()
}

// SetGeneralConfig allows overriding the default configuration.
func SetGeneralConfig(cfg *config.GeneralConfig) {
	generalCfg = cfg
}

func newReporter(
	reportPath,
	kubeconfig string,
	namespacesToDump map[string]string,
	apiScheme func(scheme *runtime.Scheme) error,
	cRDs []k8sreporter.CRData) (*k8sreporter.KubernetesReporter, error) {
	nsToDumpFilter := func(ns string) bool {
		_, found := namespacesToDump[ns]

		return found
	}

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		err := os.MkdirAll(reportPath, 0755)
		if err != nil {
			return nil, err
		}
	}

	res, err := k8sreporter.New(kubeconfig, apiScheme, nsToDumpFilter, reportPath, cRDs...)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// ReportIfFailed dumps requested cluster CRs if TC is failed to the given directory.
func ReportIfFailed(
	report types.SpecReport,
	testSuite string,
	nSpaces map[string]string,
	cRDs []k8sreporter.CRData) {
	ReportIfFailedOnCluster("", report, testSuite, nSpaces, cRDs)
}

// ReportIfFailedOnCluster dumps the requested cluster CRs on the cluster specified by kubeconfig if TC is failed to the
// given directory.
func ReportIfFailedOnCluster(
	kubeconfig string,
	report types.SpecReport,
	testSuite string,
	nSpaces map[string]string,
	cRDs []k8sreporter.CRData) {
	if !types.SpecStateFailureStates.Is(report.State) {
		return
	}

	// If no config is available, skip dumping
	if generalCfg == nil {
		klog.V(100).Infof("No reporter configuration available, skipping dump for test: %s", testSuite)

		return
	}

	dumpDir := generalCfg.GetDumpFailedTestReportLocation(testSuite)

	if dumpDir != "" {
		reporter, err := newReporter(dumpDir, kubeconfig, nSpaces, setReporterSchemes, cRDs)
		if err != nil {
			klog.Fatalf("Failed to create log reporter due to %s", err)
		}

		tcReportFolderName := strings.ReplaceAll(report.FullText(), " ", "_")

		// Workaround for the fact we are unable to pass a context to specify a logger for the client used by
		// the reporter. Otherwise, we get megabytes of verbose logging.
		_ = flag.Set("v", "0")

		reporter.Dump(report.RunTime, tcReportFolderName)

		_ = flag.Set("v", generalCfg.VerboseLevel)

		_, podExecLogsFName := path.Split(pathToPodExecLogs)

		err = moveFile(
			pathToPodExecLogs, path.Join(generalCfg.ReportsDirAbsPath, tcReportFolderName, podExecLogsFName))
		if err != nil {
			klog.Fatalf("Failed to move pod exec logs %s to report folder: %s", pathToPodExecLogs, err)
		}
	}

	err := removeFile(pathToPodExecLogs)
	if err != nil {
		klog.Fatal(err.Error())
	}
}

func moveFile(sourcePath, destPath string) error {
	_, err := os.Stat(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %w", err)
	}

	defer func() {
		_ = inputFile.Close()
	}()

	outputFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("couldn't open dest file: %w", err)
	}

	defer func() {
		_ = outputFile.Close()
	}()

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return fmt.Errorf("writing to output file failed: %w", err)
	}

	return nil
}

func removeFile(fPath string) error {
	if _, err := os.Stat(fPath); err == nil {
		err := os.Remove(fPath)
		if err != nil {
			return fmt.Errorf("failed to remove pod exec logs from %s: %w", fPath, err)
		}
	}

	return nil
}
