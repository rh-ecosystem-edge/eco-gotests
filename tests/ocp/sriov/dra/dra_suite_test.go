package dra

import (
	"runtime"
	"testing"

	sriovdrav1 "github.com/k8snetworkplumbingwg/dra-driver-sriov/pkg/api/sriovdra/v1alpha1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/dra/tests"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	resourcev1 "k8s.io/api/resource/v1"
)

func TestDRA(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	_, config := GinkgoConfiguration()
	config.JUnitReport = SriovOcpConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "SR-IOV DRA lifecycle", Label(tsparams.LabelSuite, tsparams.LabelDRA), config)
}

var _ = BeforeSuite(func() {
	if !SriovOcpConfig.DRAEnable {
		Skip("DRA lifecycle tests require explicit ECO_OCP_SRIOV_DRA_ENABLE=true")
	}

	Expect(SriovOcpConfig.DRANode).NotTo(BeEmpty(), "ECO_OCP_SRIOV_DRA_NODE is required")
	Expect(SriovOcpConfig.DRAPF).NotTo(BeEmpty(), "ECO_OCP_SRIOV_DRA_PF is required")
})

var _ = ReportAfterSuite("DRA XML report", func(report Report) {
	reportxml.Create(report, SriovOcpConfig.GetReportPath(), SriovOcpConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	_, currentFile, _, _ := runtime.Caller(0)
	reporter.ReportIfFailed(CurrentSpecReport(), currentFile,
		map[string]string{SriovOcpConfig.OcpSriovOperatorNamespace: SriovOcpConfig.OcpSriovOperatorNamespace},
		[]k8sreporter.CRData{
			{Cr: &sriovv1.SriovOperatorConfigList{}}, {Cr: &sriovv1.SriovNetworkNodePolicyList{}},
			{Cr: &sriovv1.SriovNetworkNodeStateList{}}, {Cr: &resourcev1.DeviceClassList{}},
			{Cr: &resourcev1.ResourceSliceList{}}, {Cr: &sriovdrav1.DeviceAttributesList{}},
			{Cr: &sriovdrav1.SriovResourcePolicyList{}},
		})
})
