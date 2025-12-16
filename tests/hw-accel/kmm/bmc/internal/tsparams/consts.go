package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
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

	// SimpleKmodImage represents the simple-kmod kernel module image.
	SimpleKmodImage = "quay.io/ocp-edge-qe/simple-kmod"

	// SimpleKmodModuleName represents the kernel module name.
	SimpleKmodModuleName = "simple-kmod"

	// MachineConfigName represents the name of the MachineConfig created by BMC.
	MachineConfigName = "10-kmod"

	// MachineConfigPoolName represents the target MachineConfigPool.
	MachineConfigPoolName = "worker"
)

var (
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		kmmparams.KmmOperatorNamespace: "kmm",
		BMCTestNamespace:               "bmc",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &bmcV1Beta1.BootModuleConfigList{}},
		{Cr: &corev1.EventList{}},
	}
)
