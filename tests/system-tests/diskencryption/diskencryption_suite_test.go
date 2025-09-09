package diskencryption

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestTPM2(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = SystemTestsTestConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)

	RunSpecs(t, "RAN tpm2 tests", Label(tsparams.Labels...), reporterConfig)
}
