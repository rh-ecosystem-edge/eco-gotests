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

// IngressAdditionalOptions additional options for MultiNetworkPolicyIngressRule object.
type IngressAdditionalOptions func(builder *IngressRuleBuilder) (*IngressRuleBuilder, error)

// IngressRuleBuilder provides a struct for IngressRules's object definition.
type IngressRuleBuilder struct {
	// IngressRule definition, used to create the IngressRule object.
	definition *v1beta1.MultiNetworkPolicyIngressRule
	// Used to store latest error message upon defining or mutating IngressRule definition.
	errorMsg string
}

// NewIngressRuleBuilder creates a new instance of IngressRuleBuilder.
func NewIngressRuleBuilder() *IngressRuleBuilder {
	glog.V(100).Infof("Initializing new Ingress rule structure")

	builder := &IngressRuleBuilder{
		definition: &v1beta1.MultiNetworkPolicyIngressRule{},
	}

	return builder
}

// WithPortAndProtocol adds port and protocol to Ingress rule.
func (builder *IngressRuleBuilder) WithPortAndProtocol(port uint16, protocol corev1.Protocol) *IngressRuleBuilder {
	glog.V(100).Infof("Adding port %d protocol %s to IngressRule", port, protocol)

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
func (builder *IngressRuleBuilder) WithProtocol(protocol corev1.Protocol) *IngressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding protocol %s to IngressRule", protocol)

	if protocol != corev1.ProtocolTCP && protocol != corev1.ProtocolUDP && protocol != corev1.ProtocolSCTP {
		glog.V(100).Infof("invalid protocol argument")

		builder.errorMsg = "invalid protocol argument. Allowed protocols: TCP, UDP & SCTP"

		return builder
	}

	builder.definition.Ports = append(
		builder.definition.Ports, v1beta1.MultiNetworkPolicyPort{Protocol: &protocol})

	return builder
}

// WithPort appends new item with only port to Ports list.
func (builder *IngressRuleBuilder) WithPort(port uint16) *IngressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding port %d to IngressRule", port)

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

// WithOptions adds generic options to Ingress rule.
func (builder *IngressRuleBuilder) WithOptions(options ...IngressAdditionalOptions) *IngressRuleBuilder {
	glog.V(100).Infof("Setting IngressRule additional options")

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

// WithPeerPodSelector adds peer pod selector to Ingress rule.
func (builder *IngressRuleBuilder) WithPeerPodSelector(podSelector metav1.LabelSelector) *IngressRuleBuilder {
	glog.V(100).Infof("Adding peer pod selector %v to Ingress Rule", podSelector)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.definition.From = append(
		builder.definition.From, v1beta1.MultiNetworkPolicyPeer{
			PodSelector: &podSelector,
		})

	return builder
}

// WithPeerNamespaceSelector appends new item with only NamespaceSelector to From Peer list.
func (builder *IngressRuleBuilder) WithPeerNamespaceSelector(nsSelector metav1.LabelSelector) *IngressRuleBuilder {
	glog.V(100).Infof("Adding peer namespace selector %v to IngressRule", nsSelector)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.definition.From = append(builder.definition.From,
		v1beta1.MultiNetworkPolicyPeer{NamespaceSelector: &nsSelector})

	return builder
}

// WithCIDR adds CIDR to Ingress rule.
func (builder *IngressRuleBuilder) WithCIDR(cidr string, except ...[]string) *IngressRuleBuilder {
	glog.V(100).Infof("Adding peer CIDR %s to Ingress Rule", cidr)

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		glog.V(100).Infof("Invalid CIDR %s", cidr)

		builder.errorMsg = fmt.Sprintf("Invalid CIDR argument %s", cidr)

		return builder
	}

	if len(except) > 0 {
		glog.V(100).Infof("Adding CIDR except %s to Ingress Rule", except[0])
	}

	builder.definition.From = append(builder.definition.From, v1beta1.MultiNetworkPolicyPeer{})

	// Append IPBlock config to the previously added rule
	builder.definition.From[len(builder.definition.From)-1].IPBlock = &v1beta1.IPBlock{
		CIDR: cidr,
	}

	if len(except) > 0 {
		builder.definition.From[len(builder.definition.From)-1].IPBlock.Except = except[0]
	}

	return builder
}

// WithPeerPodAndNamespaceSelector appends new item to Ingress/From list with PodSelector and NamespaceSelector.
func (builder *IngressRuleBuilder) WithPeerPodAndNamespaceSelector(
	podSelector, nsSelector metav1.LabelSelector) *IngressRuleBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding peer pod selector %v namespace selector %v to IngressRule", podSelector, nsSelector)

	builder.definition.From = append(builder.definition.From, v1beta1.MultiNetworkPolicyPeer{
		PodSelector: &podSelector, NamespaceSelector: &nsSelector})

	return builder
}

// WithPeerPodSelectorAndCIDR adds port and protocol,CIDR to Ingress rule.
func (builder *IngressRuleBuilder) WithPeerPodSelectorAndCIDR(
	podSelector metav1.LabelSelector, cidr string, except ...[]string) *IngressRuleBuilder {
	glog.V(100).Infof("Adding peer pod selector %v and CIDR %s to IngressRule", podSelector, cidr)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.WithPeerPodSelector(podSelector)
	builder.WithCIDR(cidr, except...)

	return builder
}

// GetIngressRuleCfg returns MultiNetworkPolicyIngressRule.
func (builder *IngressRuleBuilder) GetIngressRuleCfg() (*v1beta1.MultiNetworkPolicyIngressRule, error) {
	glog.V(100).Infof("Returning configuration for ingress rule")

	if builder.errorMsg != "" {
		glog.V(100).Infof("Failed to build Ingress rule configuration due to %s", builder.errorMsg)

		return nil, fmt.Errorf("%s", builder.errorMsg)
	}

	return builder.definition, nil
}

func (builder *IngressRuleBuilder) validate() (bool, error) {
	objectName := "multiNetworkPolicyIngressRule"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", objectName)

		return false, fmt.Errorf("error: received nil %s builder", objectName)
	}

	if builder.definition == nil {
		glog.V(100).Infof("The %s is undefined", objectName)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(objectName))
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", objectName, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
