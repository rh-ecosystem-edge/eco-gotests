package nhcparams

import (
	"time"

	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{rhwaparams.Label, Label}

	// OperatorDeploymentName represents NHC deployment name.
	OperatorDeploymentName = "node-healthcheck-controller-manager"

	// OperatorControllerPodLabel is how the controller pod is labeled.
	OperatorControllerPodLabel = "node-healthcheck-operator"

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		rhwaparams.RhwaOperatorNs: rhwaparams.RhwaOperatorNs,
		AppNamespace:              AppNamespace,
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
	}

	// NhcGVR is the GroupVersionResource for NodeHealthCheck resources.
	NhcGVR = schema.GroupVersionResource{
		Group:    "remediation.medik8s.io",
		Version:  "v1alpha1",
		Resource: "nodehealthchecks",
	}

	// NodeReadyTimeout is how long to wait for a node Ready condition change.
	NodeReadyTimeout = 2 * time.Minute

	// NHCObserveTimeout is how long to wait for NHC to mark a node unhealthy.
	NHCObserveTimeout = 3 * time.Minute

	// SNRFenceTimeout is how long to wait for SNR to fence the node.
	SNRFenceTimeout = 5 * time.Minute

	// RescheduleTimeout is how long to wait for the pod to reschedule.
	RescheduleTimeout = 5 * time.Minute

	// DeploymentTimeout is how long to wait for a deployment to become ready.
	DeploymentTimeout = 5 * time.Minute

	// DeletionTimeout is how long to wait for a deletion to apply.
	DeletionTimeout = 5 * time.Minute

	// NodeRecoveryTimeout is how long to wait for a node to become Ready after power-on.
	NodeRecoveryTimeout = 25 * time.Minute

	// PollingInterval is the default polling interval for Eventually blocks.
	PollingInterval = 10 * time.Second

	// BMCTimeout is the Redfish operation timeout.
	BMCTimeout = 6 * time.Minute
)
