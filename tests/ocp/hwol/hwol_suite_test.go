package hwol

import (
	"context"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
	testNamespaceUID     types.UID
	testNamespaceRunID   string
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

var _ = BeforeSuite(func() {
	if HwolOcpConfig == nil {
		Fail("HwolOcpConfig is nil: config init failed (see NewHwolOcpConfig logs)")
	}

	By("Creating a new test namespace with privileged labels")

	suiteConfig, _ := GinkgoConfiguration()
	Expect(suiteConfig.ParallelTotal).To(Equal(1), "HWOL uses shared policy names and must run with one Ginkgo process")

	testNamespaceRunID = string(uuid.NewUUID())
	// A Kubernetes UUID is a valid DNS label suffix; Create still rejects collisions.
	tsparams.TestNamespaceName = tsparams.TestNamespacePrefix + testNamespaceRunID

	testNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   tsparams.TestNamespaceName,
		Labels: map[string]string{tsparams.TestRunLabelKey: testNamespaceRunID},
	}}
	for key, value := range params.PrivilegedNSLabels {
		testNS.Labels[key] = value
	}

	// Direct Create fails on an existing namespace instead of adopting it.
	err := APIClient.Client.Create(context.TODO(), testNS)
	Expect(err).ToNot(HaveOccurred(), "error to create test namespace")
	Expect(testNS.UID).ToNot(BeEmpty(), "created test namespace has no UID")
	testNamespaceUID = testNS.UID
	tsparams.ReporterNamespacesToDump[tsparams.TestNamespaceName] = "other"
	AddReportEntry("hwol-namespace-created", tsparams.TestNamespaceName+" (UID "+string(testNamespaceUID)+")")

	By("Verifying if HWOL tests can be executed on given cluster")

	err = sriovoperator.IsSriovDeployed(APIClient, HwolOcpConfig.OcpHwolOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support HWOL test cases")
})

var _ = AfterSuite(func() {
	if testNamespaceUID == "" {
		return
	}

	By("Retaining the test namespace for a safe ownership review")
	AddReportEntry("hwol-namespace-retained", "Review all namespaced resources before manually deleting "+
		tsparams.TestNamespaceName+" (UID "+string(testNamespaceUID)+")")
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, HwolOcpConfig.GetReportPath(), HwolOcpConfig.TCPrefix)
})
