package healthchecktest

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-kni/eco-gotests/tests/cnf/ran/healthcheck/internal/tsparams"
	_ "github.com/openshift-kni/eco-gotests/tests/cnf/ran/healthcheck/tests"
	. "github.com/openshift-kni/eco-gotests/tests/cnf/ran/internal/raninittools"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestHealthCheck(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = RANConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster Health Check", Label(tsparams.LabelHealthCheckTestCases), reporterConfig)
}
