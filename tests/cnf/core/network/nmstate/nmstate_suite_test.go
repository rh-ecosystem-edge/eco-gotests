package nmstate

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/nmstate/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/nmstate/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
	testNS               = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
)

func TestNMState(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = NetConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "NMState", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Creating test namespace with privileged labels")

	for key, value := range params.PrivilegedNSLabels {
		testNS.WithLabel(key, value)
	}

	_, err := testNS.Create()
	Expect(err).ToNot(HaveOccurred(), "error to create test namespace")

	By("Verifying if nmstate tests can be executed on given cluster")

	err = verifyNMStateOperatorDeployed()
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support nmstate test cases")

	By("Pulling test images on cluster before running test cases")

	err = cluster.PullTestImageOnNodes(APIClient, NetConfig.WorkerLabel, NetConfig.CnfNetTestContainer, 300)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull test image on nodes")
})

var _ = AfterSuite(func() {
	By("Deleting test namespace")

	err := testNS.DeleteAndWait(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "error to delete test namespace")
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(report, NetConfig.GetReportPath(), NetConfig.TCPrefix)
})

func verifyNMStateOperatorDeployed() error {
	nmstateNS := namespace.NewBuilder(APIClient, NetConfig.NMStateOperatorNamespace)
	if !nmstateNS.Exists() {
		return fmt.Errorf("NMState namespace %s doesn't exist", nmstateNS.Definition.Name)
	}

	nmstateOperatorDeployment, err := deployment.Pull(
		APIClient, "nmstate-operator", NetConfig.NMStateOperatorNamespace)
	if err != nil {
		return fmt.Errorf("error to pull nmstate-operator deployment from the cluster: %w", err)
	}

	if !nmstateOperatorDeployment.IsReady(30 * time.Second) {
		return fmt.Errorf("nmstate-operator deployment is not in ready state")
	}

	return nil
}
