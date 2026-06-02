package ptp

import (
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/nicinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/mustgather"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)

	// savedPtpServiceMonitor holds the original PTP ServiceMonitor state so it can be restored in AfterSuite.
	savedPtpServiceMonitor *monv1.ServiceMonitor
)

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

	By("creating a Prometheus API client")

	prometheusAPI, err := querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

	By("ensuring clocks are locked before testing")

	err = metrics.EnsureClocksAreLocked(prometheusAPI)
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")

	By("updating the PTP ServiceMonitor scrape interval to 1s")

	savedPtpServiceMonitor, err = metrics.UpdatePtpServiceMonitorInterval(RANConfig.Spoke1APIClient, "1s")
	Expect(err).ToNot(HaveOccurred(), "Failed to update PTP ServiceMonitor scrape interval")

	By("deploying consumers")

	err = consumer.DeployConsumersOnNodes(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to deploy consumers on nodes with PTP daemons")
})

var _ = AfterSuite(func() {
	By("restoring the PTP ServiceMonitor")

	if savedPtpServiceMonitor != nil {
		err := metrics.RestorePtpServiceMonitor(RANConfig.Spoke1APIClient, savedPtpServiceMonitor)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore PTP ServiceMonitor")
	}

	By("removing consumers")

	err := consumer.CleanupConsumersOnNodes(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup consumers on nodes with PTP daemons")

	By("cleaning up Prometheus API client resources")

	err = querier.CleanupQuerierResources(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup Prometheus API client resources")
})

var _ = JustAfterEach(func() {
	// If the JustAfterEach runs when the cluster is not reachable, we waste ~4.5 minutes waiting for the
	// k8sreporter to finish timing out. To prevent that case, we poll the cluster to ensure we can list nodes as a
	// reachability check.
	//
	// This still wastes 1 minute, but a persistent unreachable cluster is rare and this fix is good enough to
	// prevent cascading issues in CI due to the entire suite timing out.
	Eventually(func() error {
		_, err := nodes.List(RANConfig.Spoke1APIClient)

		return err
	}).WithTimeout(time.Minute).WithPolling(10*time.Second).Should(Succeed(), "Reachability check to spoke 1 failed")

	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterSpokeNamespacesToDump, tsparams.ReporterSpokeCRsToDump)
	mustgather.MustGatherIfFailed(CurrentSpecReport(), currentFile, RANConfig.Spoke1APIClient)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, RANConfig.GetReportPath(), RANConfig.TCPrefix)

	By("generating network interface information report")

	nicinfoReport, err := nicinfo.GenerateReport(RANConfig.Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to generate network interface information report")

	AddReportEntry("nicinfo", nicinfoReport)
})
