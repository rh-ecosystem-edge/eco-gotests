package tsparams

import (
	"time"

	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	performanceprofileV2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// WaitTimeout represents timeout for the most ginkgo Eventually functions.
	WaitTimeout = 3 * time.Minute
	// RetryInterval represents retry interval for the most ginkgo Eventually functions.
	RetryInterval = 3 * time.Second
	// MCOWaitTimeout represent timeout for mco operations.
	MCOWaitTimeout = 60 * time.Minute
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
