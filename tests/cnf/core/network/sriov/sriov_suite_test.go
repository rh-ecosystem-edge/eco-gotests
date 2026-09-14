package sriov

import (
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
)

var (
	_, currentFile, _, _ = runtime.Caller(0)
	testNS               = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
)

func TestLB(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = NetConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "sriov", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Creating test namespace with privileged labels")

	for key, value := range params.PrivilegedNSLabels {
		testNS.WithLabel(key, value)
	}

	_, err := testNS.Create()
	Expect(err).ToNot(HaveOccurred(), "error to create test namespace")

	By("Verifying if sriov tests can be executed on given cluster")

	err = sriovoperator.IsSriovDeployed(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")

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
	clusterTopology := "multi-node"

	if isSNO, err := cluster.IsSNOCluster(APIClient); err != nil {
		clusterTopology = "unknown"
	} else if isSNO {
		clusterTopology = "sno"
	}

	reportxml.Create(report, NetConfig.GetReportPath(), NetConfig.TCPrefix,
		reportxml.WithSuiteProperties(map[string]string{
			"sriov-interface-list":               NetConfig.SriovInterfaces,
			"sriov-operator-namespace":           NetConfig.SriovOperatorNamespace,
			"cnf-net-test-container":             NetConfig.CnfNetTestContainer,
			"dpdk-test-container":                NetConfig.DpdkTestContainer,
			"cnf-mcp-label":                      NetConfig.CnfMcpLabel,
			"worker-label":                       NetConfig.WorkerLabelEnvVar,
			"vlan":                               NetConfig.VLAN,
			"native-vlan":                        NetConfig.NativeVLAN,
			"cluster-vlan":                       NetConfig.ClusterVlan,
			"switch-interfaces":                  NetConfig.SwitchInterfaces,
			"switch-lags":                        NetConfig.SwitchLagNames,
			"pf-status-relay-operator-namespace": NetConfig.PFStatusRelayOperatorNamespace,
			"prometheus-operator-namespace":      NetConfig.PrometheusOperatorNamespace,
			"cluster-topology":                   clusterTopology,
		}))
})
