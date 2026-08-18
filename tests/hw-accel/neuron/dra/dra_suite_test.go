package dra

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/tests"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestDRA(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = GeneralConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Neuron DRA Suite", reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Setting up Neuron DRA test suite")
})

var _ = AfterSuite(func() {
	By("Tearing down Neuron DRA test suite")
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, GeneralConfig.GetReportPath(), GeneralConfig.TCPrefix)
})
