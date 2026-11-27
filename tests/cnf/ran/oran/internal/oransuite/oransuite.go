package oransuite

import (
	"fmt"
	"path"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/mustgather"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

// Run starts an O-RAN subsuite with the shared setup, reporting, and failure diagnostics. The currentFile must be the
// caller's source file so each subsuite receives a distinct JUnit report path.
func Run(t *testing.T, description, currentFile string, labels []string) {
	t.Helper()

	_, reporterConfig := ginkgo.GinkgoConfiguration()
	reporterConfig.JUnitReport = raninittools.RANConfig.GetJunitReportPath(currentFile)

	registerHooks(currentFile)

	ginkgo.RunSpecs(t, description, ginkgo.Label(labels...), reporterConfig)
}

func registerHooks(currentFile string) {
	ginkgo.BeforeSuite(func() {
		ginkgo.By("checking that the hub cluster is present")

		isHubPresent := rancluster.AreClustersPresent([]*clients.Settings{raninittools.HubAPIClient})
		if !isHubPresent {
			ginkgo.Fail("Hub cluster must be present for O-RAN tests")
		}
	})

	ginkgo.JustAfterEach(func() {
		currentDir, currentFilename := path.Split(currentFile)
		hubReportPath := fmt.Sprintf("%shub_%s", currentDir, currentFilename)
		report := ginkgo.CurrentSpecReport()

		if raninittools.Spoke1APIClient != nil {
			ginkgo.By("collecting k8sreporter for the spoke")
			reporter.ReportIfFailed(
				report, currentFile, tsparams.ReporterSpokeNamespacesToDump, tsparams.ReporterSpokeCRsToDump)
		}

		ginkgo.By("collecting k8sreporter for the hub cluster")
		reporter.ReportIfFailedOnCluster(
			raninittools.RANConfig.HubKubeconfig,
			report,
			hubReportPath,
			tsparams.ReporterHubNamespacesToDump,
			tsparams.ReporterHubCRsToDump)

		ginkgo.By("collecting o2ims must-gather on the hub cluster")
		mustgather.CollectIfFailed(
			report,
			hubReportPath,
			raninittools.RANConfig.HubAPIClient,
			"o2ims",
			mustgather.StaticImage(raninittools.RANConfig.O2IMSMustGatherImage),
		)
	})

	ginkgo.ReportAfterSuite("", func(report ginkgo.Report) {
		reportxml.Create(report, raninittools.RANConfig.GetReportPath(), raninittools.RANConfig.TCPrefix,
			reportxml.WithSuiteProperties(map[string]string{
				"o2ims-must-gather-image": raninittools.RANConfig.O2IMSMustGatherImage,
			}))
	})
}
