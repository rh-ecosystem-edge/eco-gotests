package vcoreparams

import (
	"github.com/openshift-kni/k8sreporter"
	securityv1 "github.com/openshift/api/security/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsparams"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{systemtestsparams.Label, Label}

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{"vcore-test": "vcore-test"}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
	}

	// LocalImageRegistry represents the local registry.
	LocalImageRegistry = "image-registry.openshift-image-registry.svc:5000"

	// PossibleWorkerNodeRoles list of the possible worker roles.
	PossibleWorkerNodeRoles = []string{"odf", "control-plane-worker", "user-plane-worker"}

	// CpMCSelector the control-plane-worker nodes selector value.
	CpMCSelector = "machineconfiguration.openshift.io/role: control-plane-worker"

	// PpMCSelectorKey the user-plane-worker mc selector key.
	PpMCSelectorKey = "machineconfiguration.openshift.io/role: user-plane-worker"

	// OdfMCSelector the odf nodes selector value.
	OdfMCSelector = map[string]string{"machineconfiguration.openshift.io/role": "odf"}

	// CpNodesSelector the cp nodes selector value.
	CpNodesSelector = map[string]string{"node-role.kubernetes.io/control-plane-worker": ""}

	// PpNodesSelector the user-plane-worker nodes selector value.
	PpNodesSelector = map[string]string{"node-role.kubernetes.io/user-plane-worker": ""}

	// OdfNodesSelector the odf nodes selector value.
	OdfNodesSelector = map[string]string{"node-role.kubernetes.io/odf": ""}

	// NetworkPoliciesNamespaces list of the possible worker roles.
	NetworkPoliciesNamespaces = []string{"amfmme1", "nrf1", "nssf1", "smf1", "upf1"}

	// NetworkPolicyType the policyType value.
	NetworkPolicyType = netv1.PolicyTypeIngress

	// NetworkPolicyMonitoringNamespaceSelectorMatchLabels the namespaceSelector value.
	NetworkPolicyMonitoringNamespaceSelectorMatchLabels = map[string]string{
		"network.openshift.io/policy-group": "monitoring",
	}

	// CpSccAllowCapabilities control-plane-worker scc AllowCapabilites value.
	CpSccAllowCapabilities = []corev1.Capability{"SYS_PTRACE", "SYS_ADMIN", "NET_ADMIN", "NET_RAW", "NET_BIND_SERVICE"}

	// CpSccGroups control-plane-worker scc Groups value.
	CpSccGroups = []string{"system:authenticated"}

	// CpSccDropCapabilities control-plane-worker scc DropCapabilites value.
	CpSccDropCapabilities = []corev1.Capability{"KILL", "MKNOD"}

	// CpSccVolumes control-plane-worker scc Volumes value.
	CpSccVolumes = []securityv1.FSType{"configMap", "downwardAPI", "emptyDir",
		"persistentVolumeClaim", "projected", "secret"}
)
