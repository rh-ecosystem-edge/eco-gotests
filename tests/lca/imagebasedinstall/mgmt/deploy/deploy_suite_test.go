package deploy_test

import (
	"testing"

	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/deploy/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/deploy/tests"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/seedimage"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestDeploy(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = MGMTConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Deploy Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	seedClusterInfo, err := seedimage.GetContent(APIClient, MGMTConfig.SeedImage)
	Expect(err).NotTo(HaveOccurred(), "error getting seed image info")

	MGMTConfig.SeedClusterInfo = seedClusterInfo
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, MGMTConfig.GetReportPath(), MGMTConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})
