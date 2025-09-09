package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// 	MasterPodIPv4Address is the IP address of the external Pod.
	MasterPodIPv4Address = "172.16.0.1"
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		TestNamespaceName: TestNamespaceName,
	}

	// TestNamespaceName security-tests namespace where all test cases are performed.
	TestNamespaceName = "security-tests"
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigList{}},
	}
)
