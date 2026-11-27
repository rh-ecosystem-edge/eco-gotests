package hwol

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/tests"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
	testNS               = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
)

func TestHWOL(t *testing.T) {
	if HwolOcpConfig == nil {
		t.Fatal("HwolOcpConfig is nil: config init failed (see NewHwolOcpConfig logs)")
	}

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = HwolOcpConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "hwol", Label(tsparams.Labels...), reporterConfig)
}

// HWOL setup changes shared cluster state and creates fixed-name resources. It must
// run only once: a normal BeforeSuite runs in every Ginkgo parallel process and
// causes those processes to race on the image-pull pod and test namespace.
var _ = SynchronizedBeforeSuite(func() []byte {
	if HwolOcpConfig == nil {
		Fail("HwolOcpConfig is nil: config init failed (see NewHwolOcpConfig logs)")
	}

	By("Creating test namespace with privileged labels")

	for key, value := range params.PrivilegedNSLabels {
		testNS.WithLabel(key, value)
	}

	_, err := testNS.Create()
	Expect(err).ToNot(HaveOccurred(), "error to create test namespace")

	By("Verifying if HWOL tests can be executed on given cluster")

	err = sriovoperator.IsSriovDeployed(APIClient, HwolOcpConfig.OcpHwolOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support HWOL test cases")

	By("Pulling test images on cluster before running test cases")

	err = cluster.PullTestImageOnNodes(APIClient, HwolOcpConfig.WorkerLabel, HwolOcpConfig.OcpHwolTestContainer, 300)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull test image on nodes")

	return nil
}, func([]byte) {})

// Match synchronized setup with a single final namespace cleanup. This prevents
// parallel workers from racing to delete the fixed-name namespace.
var _ = SynchronizedAfterSuite(func() {}, func() {
	By("Deleting test namespace")

	err := testNS.DeleteAndWait(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "error to delete test namespace")
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, HwolOcpConfig.GetReportPath(), HwolOcpConfig.TCPrefix)
})
