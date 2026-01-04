package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	bmcV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(kmmparams.Labels, LabelSuite)
)

const (
	// LabelSuite represents the label for the BMC test suite.
	LabelSuite = "bmc"

	// BMCTestName represents the name of the BootModuleConfig for testing.
	BMCTestName = "bmc"

	// BMCTestNamespace represents the namespace for the BMC test.
	BMCTestNamespace = "default"

	// MachineConfigName represents the name of the MachineConfig created by BMC.
	MachineConfigName = "10-kmod"

	// BMCInTreeRemoveName represents the name of the BMC for inTreeModulesToRemove test.
	BMCInTreeRemoveName = "bmc-remove"

	// MachineConfigInTreeRemoveName represents the MachineConfig name for inTreeModulesToRemove test.
	MachineConfigInTreeRemoveName = "12-kmod-remove"

	// IbIpoibModuleName represents the InfiniBand IPoIB kernel module to be removed.
	IbIpoibModuleName = "ib_ipoib"
)

var (
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		kmmparams.KmmOperatorNamespace: "openshift-kmm",
		BMCTestNamespace:               "bmc",
		"NodesNamespace":               "default",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &bmcV1Beta1.BootModuleConfigList{}},
		{Cr: &mcv1.MachineConfigList{}},
		{Cr: &mcv1.MachineConfigPoolList{}},
		{Cr: &operatorv1.MachineConfigurationList{}},
		{Cr: &corev1.EventList{}},
	}
)
