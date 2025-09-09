package upgradeparams

import (
	"fmt"

	"github.com/openshift-kni/k8sreporter"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/accel/internal/accelparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{accelparams.Label, Label}
	// DeploymentName is the name of the test workload.
	DeploymentName = "test-workload"
	// TestNamespaceName is the namespace where the workload is deployed.
	TestNamespaceName = "accel-upgrade-workload-ns"
	// ContainerLabelsMap labels in an map used when creating the workload container.
	ContainerLabelsMap = map[string]string{"app": DeploymentName}
	// ContainerLabelsStr labels in a str used when creating the workload container.
	ContainerLabelsStr = fmt.Sprintf("%s=%s", "app", DeploymentName)
	// ServicePort is the workload service port.
	ServicePort int32 = 8080
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{"test-workload": "test-workload",
		"accel-upgrade-workload-ns": "accel-upgrade-workload-ns"}
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
		{Cr: &configv1.ClusterOperatorList{}},
		{Cr: &configv1.ClusterVersionList{}},
	}
	trueFlag  = true
	falseFlag = false
	// DefaultSC is the default security context for the containers.
	DefaultSC = &corev1.SecurityContext{
		AllowPrivilegeEscalation: &falseFlag,
		RunAsNonRoot:             &trueFlag,
		SeccompProfile: &corev1.SeccompProfile{
			Type: "RuntimeDefault",
		},
	}
)
