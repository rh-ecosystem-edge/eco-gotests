package ptp

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPTP(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = RANConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "RAN PTP Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("checking that the spoke 1 cluster is present")
	isSpoke1Present := rancluster.AreClustersPresent([]*clients.Settings{Spoke1APIClient})
	Expect(isSpoke1Present).To(BeTrue(), "Spoke 1 cluster must be present for PTP tests")

	By("deploying consumers")
	err := consumer.DeployConsumersOnNodes(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to deploy consumers on nodes with PTP daemons")
})

var _ = AfterSuite(func() {
	By("removing consumers")
	err := consumer.CleanupConsumersOnNodes(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup consumers on nodes with PTP daemons")

	By("cleaning up Prometheus API client resources")
	err = querier.CleanupQuerierResources(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup Prometheus API client resources")
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterSpokeNamespacesToDump, tsparams.ReporterSpokeCRsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, RANConfig.GetReportPath(), RANConfig.TCPrefix)
})
