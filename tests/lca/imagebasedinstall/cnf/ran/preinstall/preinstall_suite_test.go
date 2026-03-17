package preinstall_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPreinstall(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = inittools.GeneralConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "IBI Preinstall Suite", Label("preinstall"), reporterConfig)
}

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, inittools.GeneralConfig.GetReportPath(), inittools.GeneralConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		nil, // No specific namespaces to dump for now
		nil, // No specific CRDs to dump for now
	)
})
