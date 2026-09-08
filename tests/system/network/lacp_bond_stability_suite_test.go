package network

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system/network/internal/lacpparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/system/network/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestLACPBondStability(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = GeneralConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "LACP Bond Stability Suite", Label(lacpparams.Labels...), reporterConfig)
}

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, map[string]string{}, nil)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, GeneralConfig.GetReportPath(), GeneralConfig.TCPrefix)
})
