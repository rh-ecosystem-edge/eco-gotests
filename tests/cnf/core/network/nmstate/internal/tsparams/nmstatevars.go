package tsparams

import (
	"time"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	nmstateV1 "github.com/nmstate/kubernetes-nmstate/api/v1"
	nmstateV1beta1 "github.com/nmstate/kubernetes-nmstate/api/v1beta1"
	"github.com/openshift-kni/k8sreporter"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	corev1 "k8s.io/api/core/v1"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(netparam.Labels, LabelSuite)
	// DefaultTimeout represents the default timeout for most of Eventually/PollImmediate functions.
	DefaultTimeout = 300 * time.Second
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &mcfgv1.MachineConfigPoolList{}},
		{Cr: &nmstateV1.NMStateList{}},
		{Cr: &nmstateV1.NodeNetworkConfigurationPolicyList{}},
		{Cr: &nmstateV1beta1.NodeNetworkStateList{}},
		{Cr: &nmstateV1beta1.NodeNetworkConfigurationEnactmentList{}},
		{Cr: &nadv1.NetworkAttachmentDefinitionList{}, Namespace: &NetConfig.NMStateOperatorNamespace},
		{Cr: &sriovv1.SriovNetworkNodePolicyList{}},
		{Cr: &corev1.NodeList{}},
	}
	// ReporterNamespacesToDump tells to the reporter what namespaces to dump.
	ReporterNamespacesToDump = map[string]string{
		NetConfig.NMStateOperatorNamespace: NetConfig.NMStateOperatorNamespace,
		TestNamespaceName:                  "other",
	}
)
