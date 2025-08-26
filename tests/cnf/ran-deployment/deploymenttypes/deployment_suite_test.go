package deployment

import (
	"fmt"
	"path"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/deploymenttypes/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/deploymenttypes/tests"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestDeployment(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = RANConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "RAN Deployment Types Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = JustAfterEach(func() {
	var (
		currentDir, currentFilename = path.Split(currentFile)
		hubReportPath               = fmt.Sprintf("%shub_%s", currentDir, currentFilename)
		spoke2ReportPath            = fmt.Sprintf("%sspoke2_%s", currentDir, currentFilename)
		report                      = CurrentSpecReport()
	)

	if Spoke1APIClient != nil && Spoke1APIClient.KubeconfigPath != "" {
		reporter.ReportIfFailed(
			report,
			currentFile,
			map[string]string{},
			tsparams.ReporterSpokeCRsToDump)
	}

	if HubAPIClient != nil && HubAPIClient.KubeconfigPath != "" {
		reporter.ReportIfFailedOnCluster(
			HubAPIClient.KubeconfigPath,
			report,
			hubReportPath,
			map[string]string{},
			tsparams.ReporterHubCRsToDump)
	}

	if Spoke2APIClient != nil && Spoke2APIClient.KubeconfigPath != "" {
		reporter.ReportIfFailedOnCluster(
			Spoke2APIClient.KubeconfigPath,
			report,
			spoke2ReportPath,
			map[string]string{},
			tsparams.ReporterSpokeCRsToDump)
	}
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, RANConfig.GetReportPath(), RANConfig.TCPrefix)
})
