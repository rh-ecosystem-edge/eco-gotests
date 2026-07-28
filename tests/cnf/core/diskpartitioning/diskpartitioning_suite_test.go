package diskpartitioningtest

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/diskpartitioning/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/diskpartitioning/tests"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreinittools"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestDiskPartitioning(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = CoreConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Disk Partitioning", Label(tsparams.Labels...), reporterConfig)
}
