package oran

import (
	"fmt"
	"path"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/mustgather"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestORAN(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = RANConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "RAN O-RAN Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("checking that the hub cluster is present")

	isHubPresent := rancluster.AreClustersPresent([]*clients.Settings{HubAPIClient})
	Expect(isHubPresent).To(BeTrue(), "Hub cluster must be present for O-RAN tests")
})

var _ = JustAfterEach(func() {
	var (
		currentDir, currentFilename = path.Split(currentFile)
		hubReportPath               = fmt.Sprintf("%shub_%s", currentDir, currentFilename)
		report                      = CurrentSpecReport()
	)

	if Spoke1APIClient != nil {
		By("collecting k8sreporter for the spoke")
		reporter.ReportIfFailed(
			report, currentFile, tsparams.ReporterSpokeNamespacesToDump, tsparams.ReporterSpokeCRsToDump)
	}

	By("collecting k8sreporter for the hub cluster")
	reporter.ReportIfFailedOnCluster(
		RANConfig.HubKubeconfig,
		report,
		hubReportPath,
		tsparams.ReporterHubNamespacesToDump,
		tsparams.ReporterHubCRsToDump)

	By("collecting o2ims must-gather on the hub cluster")
	mustgather.CollectIfFailed(
		report,
		hubReportPath,
		RANConfig.HubAPIClient,
		"o2ims",
		mustgather.StaticImage(RANConfig.O2IMSMustGatherImage),
	)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, RANConfig.GetReportPath(), RANConfig.TCPrefix,
		reportxml.WithSuiteProperties(map[string]string{
			"o2ims-must-gather-image": RANConfig.O2IMSMustGatherImage,
		}))
})
