package networkpolicy

import (
	"fmt"
	"net"

	"github.com/golang/glog"
	"github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EgressAdditionalOptions additional options for MultiNetworkPolicyEgressRule object.
type EgressAdditionalOptions func(builder *EgressRuleBuilder) (*EgressRuleBuilder, error)

// EgressRuleBuilder provides a struct for EgressRules's object definition.
type EgressRuleBuilder struct {
	// EgressRule definition, used to create the EgressRule object.
	definition *v1beta1.MultiNetworkPolicyEgressRule
	// Used to store latest error message upon defining or mutating EgressRule definition.
	errorMsg string
}

// NewEgressRuleBuilder creates a new instance of EgressRuleBuilder.
func NewEgressRuleBuilder() *EgressRuleBuilder {
	glog.V(100).Infof("Initializing new Egress rule structure")

	// Empty rule allowed.
	builder := &EgressRuleBuilder{
		definition: &v1beta1.MultiNetworkPolicyEgressRule{},
	}

	return builder
}

// WithPortAndProtocol adds port and protocol to Egress rule.
func (builder *EgressRuleBuilder) WithPortAndProtocol(port uint16, protocol corev1.Protocol) *EgressRuleBuilder {
	glog.V(100).Infof("Adding port %d and protocol %s to EgressRule", port, protocol)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if port == 0 {
		glog.V(100).Infof("Port number can not be 0")

		builder.errorMsg = "port number can not be 0"

		return builder
	}

	formattedPort := intstr.FromInt(int(port))

	builder.definition.Ports = append(
		builder.definition.Ports, v1beta1.MultiNetworkPolicyPort{Port: &formattedPort, Protocol: &protocol})

	return builder
}

// WithProtocol appends new item with only protocol to Ports list.
func (builder *EgressRuleBuilder) WithProtocol(protocol corev1.Protocol) *EgressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding protocol %s to EgressRule", protocol)

	if protocol != corev1.ProtocolTCP && protocol != corev1.ProtocolUDP && protocol != corev1.ProtocolSCTP {
		glog.V(100).Infof("invalid protocol argument. Allowed protocols: TCP, UDP & SCTP ")

		builder.errorMsg = "invalid protocol argument. Allowed protocols: TCP, UDP & SCTP"

		return builder
	}

	builder.definition.Ports = append(
		builder.definition.Ports, v1beta1.MultiNetworkPolicyPort{Protocol: &protocol})

	return builder
}

// WithPort appends new item with only port to Ports list.
func (builder *EgressRuleBuilder) WithPort(port uint16) *EgressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding port %d to EgressRule", port)

	if port == 0 {
		glog.V(100).Infof("Cannot set port number to 0")

		builder.errorMsg = "port number cannot be 0"

		return builder
	}

	formattedPort := intstr.FromInt(int(port))

	builder.definition.Ports = append(
		builder.definition.Ports, v1beta1.MultiNetworkPolicyPort{Port: &formattedPort})

	return builder
}

// WithOptions adds generic options to Egress rule.
func (builder *EgressRuleBuilder) WithOptions(options ...EgressAdditionalOptions) *EgressRuleBuilder {
	glog.V(100).Infof("Setting EgressRule additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				glog.V(100).Infof("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// WithPeerPodSelector adds pod selector to Egress rule.
func (builder *EgressRuleBuilder) WithPeerPodSelector(podSelector metav1.LabelSelector) *EgressRuleBuilder {
	glog.V(100).Infof("Adding peer pod selector %v to EgressRule", podSelector)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.definition.To = append(builder.definition.To, v1beta1.MultiNetworkPolicyPeer{PodSelector: &podSelector})

	return builder
}

// WithPeerNamespaceSelector appends new item with only NamespaceSelector into To Peer list.
func (builder *EgressRuleBuilder) WithPeerNamespaceSelector(nsSelector metav1.LabelSelector) *EgressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding peer namespace selector %v to EgressRule", nsSelector)

	builder.definition.To = append(builder.definition.To, v1beta1.MultiNetworkPolicyPeer{NamespaceSelector: &nsSelector})

	return builder
}

// WithCIDR edits last item's IPBlock on Egress/To list or adds new item with only IPBlock into
// Egress/To list if the Egress/To list is empty.
func (builder *EgressRuleBuilder) WithCIDR(cidr string, except ...[]string) *EgressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding peer CIDR %s to Egress Rule", cidr)

	if len(except) != 0 {
		glog.V(100).Infof("Adding CIDR except %v to Egress Rule", except[0])
	}

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		glog.V(100).Infof("Invalid CIDR %s", cidr)

		builder.errorMsg = fmt.Sprintf("invalid CIDR argument %s", cidr)

		return builder
	}

	builder.definition.To = append(builder.definition.To, v1beta1.MultiNetworkPolicyPeer{})

	// Append IPBlock config to the previously added Peer
	builder.definition.To[len(builder.definition.To)-1].IPBlock = &v1beta1.IPBlock{CIDR: cidr}

	if len(except) > 0 {
		builder.definition.To[len(builder.definition.To)-1].IPBlock.Except = except[0]
	}

	return builder
}

// WithPeerPodAndNamespaceSelector appends new item to Egress/To list with PodSelector and NamespaceSelector.
func (builder *EgressRuleBuilder) WithPeerPodAndNamespaceSelector(
	podSelector, nsSelector metav1.LabelSelector) *EgressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding peer pod selector %v namespace selector %v to EgressRule", podSelector, nsSelector)

	builder.definition.To = append(builder.definition.To, v1beta1.MultiNetworkPolicyPeer{
		PodSelector: &podSelector, NamespaceSelector: &nsSelector})

	return builder
}

// WithPeerPodSelectorAndCIDR adds pod selector and CIDR to Egress rule.
func (builder *EgressRuleBuilder) WithPeerPodSelectorAndCIDR(
	podSelector metav1.LabelSelector, cidr string, except ...[]string) *EgressRuleBuilder {
	glog.V(100).Infof("Adding peer pod selector %v to EgressRule", podSelector)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		glog.V(100).Infof("Invalid CIDR %s", cidr)

		builder.errorMsg = fmt.Sprintf("Invalid CIDR argument %s", cidr)

		return builder
	}

	builder.WithPeerPodSelector(podSelector)

	// Append IPBlock config to the previously added rule
	builder.definition.To[len(builder.definition.To)-1].IPBlock = &v1beta1.IPBlock{
		CIDR: cidr,
	}

	if len(except) > 0 {
		builder.definition.To[len(builder.definition.To)-1].IPBlock.Except = except[0]
	}

	return builder
}

// GetEgressRuleCfg returns MultiNetworkPolicyEgressRule.
func (builder *EgressRuleBuilder) GetEgressRuleCfg() (*v1beta1.MultiNetworkPolicyEgressRule, error) {
	glog.V(100).Infof("Returning configuration for egress rule")

	if builder.errorMsg != "" {
		glog.V(100).Infof("Failed to build Egress rule configuration due to %s", builder.errorMsg)

		return nil, fmt.Errorf("%s", builder.errorMsg)
	}

	return builder.definition, nil
}

func (builder *EgressRuleBuilder) validate() (bool, error) {
	objectName := "multiNetworkPolicyEgressRule"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", objectName)

		return false, fmt.Errorf("error: received nil %s builder", objectName)
	}

	if builder.definition == nil {
		glog.V(100).Infof("The %s is undefined", objectName)

		builder.errorMsg = msg.UndefinedCrdObjectErrString(objectName)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", objectName, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
