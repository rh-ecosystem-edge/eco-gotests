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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/tests"
	subscriber "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-subscriber"
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

	By("deploying the subscriber for alarm notifications")
	subscriberDomain := RANConfig.GetAppsURL(tsparams.SubscriberSubdomain)
	err := subscriber.Deploy(HubAPIClient, tsparams.SubscriberNamespace, subscriberDomain, "")
	Expect(err).ToNot(HaveOccurred(), "Failed to deploy subscriber")
})

var _ = AfterSuite(func() {
	By("cleaning up the subscriber deployment")
	err := subscriber.Cleanup(HubAPIClient, tsparams.SubscriberNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to cleanup subscriber")
})

var _ = JustAfterEach(func() {
	var (
		currentDir, currentFilename = path.Split(currentFile)
		hubReportPath               = fmt.Sprintf("%shub_%s", currentDir, currentFilename)
		report                      = CurrentSpecReport()
	)

	if Spoke1APIClient != nil {
		reporter.ReportIfFailed(
			report, currentFile, tsparams.ReporterSpokeNamespacesToDump, tsparams.ReporterSpokeCRsToDump)
	}

	reporter.ReportIfFailedOnCluster(
		RANConfig.HubKubeconfig,
		report,
		hubReportPath,
		tsparams.ReporterHubNamespacesToDump,
		tsparams.ReporterHubCRsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, RANConfig.GetReportPath(), RANConfig.TCPrefix)
})
