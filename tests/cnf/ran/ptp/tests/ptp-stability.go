package tests

import (
	"context"
	"fmt"
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/nicinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/daemonlogs"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/stability"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
)

var _ = Describe("PTP Stability", Label(tsparams.LabelStability), func() {
	var prometheusAPI prometheusv1.API

	BeforeEach(func() {
		By("creating a Prometheus API client")
		var err error
		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")
		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	AfterEach(func() {
		By("ensuring clocks are locked after testing")
		err := metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 38228 - Measure the PTP Slave Clock Stability leveraging the PTP offset communicated in ptp4l logs
	It("validates PTP stability and offset behavior over configured duration", reportxml.ID("38228"), func() {
		testRanAtLeastOnce := false

		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for _, nodeInfo := range nodeInfoMap {
			By("marking interfaces for nicinfo reporting on node " + nodeInfo.Name)
			for _, profile := range nodeInfo.Profiles {
				nicinfo.Node(nodeInfo.Name).MarkSeqTested(iface.NamesToStringSeq(maps.Keys(profile.Interfaces)))
			}

			testRanAtLeastOnce = true

			By("asserting ptp4l and phc2sys processes are UP on node " + nodeInfo.Name)
			ptp4lProcessStatusQuery := metrics.ProcessStatusQuery{
				Node:    metrics.Equals(nodeInfo.Name),
				Process: metrics.Equals(metrics.ProcessPTP4L),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, ptp4lProcessStatusQuery, metrics.ProcessStatusUp,
				metrics.AssertWithTimeout(5*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert ptp4l process status is UP on node %s", nodeInfo.Name)

			phc2sysProcessStatusQuery := metrics.ProcessStatusQuery{
				Node:    metrics.Equals(nodeInfo.Name),
				Process: metrics.Equals(metrics.ProcessPHC2SYS),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, phc2sysProcessStatusQuery, metrics.ProcessStatusUp,
				metrics.AssertWithTimeout(5*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert phc2sys process status is UP on node %s", nodeInfo.Name)

			By(fmt.Sprintf("collecting daemon logs from node %s for %s", nodeInfo.Name, RANConfig.PtpStabilityDuration))
			collectionResult, err := daemonlogs.CollectDaemonLogs(
				RANConfig.Spoke1APIClient, nodeInfo.Name, RANConfig.PtpStabilityDuration)
			Expect(err).ToNot(HaveOccurred(), "Failed to collect daemon logs on node %s", nodeInfo.Name)

			By("asserting that we collected more log lines than errors on node " + nodeInfo.Name)
			Expect(len(collectionResult.Lines)).To(BeNumerically(">", len(collectionResult.Errors)),
				"collected fewer log lines (%d) than fetch errors (%d); log collection is unreliable",
				len(collectionResult.Lines), len(collectionResult.Errors))

			By("parsing and analyzing collected daemon logs for node " + nodeInfo.Name)
			parsedLogs := daemonlogs.ParseLogs(collectionResult.Lines)
			analysisResult := stability.Analyze(parsedLogs, RANConfig.PtpStabilityThreshold)

			AddReportEntry("ptp_stability_analysis_"+nodeInfo.Name, analysisResult.DiagnosticMessage())

			Expect(analysisResult.Passed).To(BeTrue(), analysisResult.DiagnosticMessage())
		}

		if !testRanAtLeastOnce {
			Skip("Could not find any PTP-capable node for stability test")
		}
	})
})
