package tsparams

import (
	"time"

	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	performanceprofileV2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

const (
	// LabelSuite represents acc label that can be used for test cases selection.
	LabelSuite = "accelerator"
	// TestNamespaceName acc namespace where all test cases are performed.
	TestNamespaceName = "accelerator-tests"
	// Acc100DeviceID represents the device id of the acc100.
	Acc100DeviceID = "0d5c"
	// Acc100ResourceName represents the resource name of the acc100.
	Acc100ResourceName = "intel.com/intel_fec_acc100"
	// Acc100EnvVar represents the env variable of the acc100.
	Acc100EnvVar = "PCIDEVICE_INTEL_COM_INTEL_FEC_ACC100"
	// TotalNumberBbdevTests represents the total number of bbdev tests.
	TotalNumberBbdevTests = 33
	// ExpectedNumberBbdevTestsPassedForAcc100 represents the expected number of bbdev tests passed.
	ExpectedNumberBbdevTestsPassedForAcc100 = 27
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// WaitTimeout represents timeout for the most ginkgo Eventually functions.
	WaitTimeout = 3 * time.Minute
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &performanceprofileV2.PerformanceProfileList{}},
		{Cr: &mcfgv1.MachineConfigPoolList{}},
	}
	// OperatorNamespace represents the namespace of the fec operator.
	OperatorNamespace = "vran-acceleration-operators"
	// DaemonsetNames represents the names of the daemonsets to check for fec operator.
	DaemonsetNames = []string{"accelerator-discovery", "sriov-device-plugin", "sriov-fec-daemonset"}
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		"openshift-performance-addon-operator": "performance",
		TestNamespaceName:                      "other",
	}
)

// VFIOToken represents the vfiotoken struct.
type VFIOToken struct {
	Extra struct {
		VfioToken string `json:"VFIO_TOKEN"`
	} `json:"extra"`
}
