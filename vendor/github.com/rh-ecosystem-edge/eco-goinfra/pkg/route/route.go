package route

import (
	"context"
	"fmt"

	"slices"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

// Builder provides struct for route object containing connection to the cluster and the route definitions.
type Builder struct {
	common.EmbeddableBuilder[routev1.Route, *routev1.Route]
	common.EmbeddableCreator[routev1.Route, Builder, *routev1.Route, *Builder]
	common.EmbeddableDeleteReturner[routev1.Route, Builder, *routev1.Route, *Builder]
}

// AttachMixins attaches the mixins to the builder. This is called automatically when the builder is initialized.
func (builder *Builder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
}

// GetGVK returns the GVK for the Route resource.
func (builder *Builder) GetGVK() schema.GroupVersionKind {
	return routev1.GroupVersion.WithKind("Route")
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname, serviceName string) *Builder {
	klog.V(100).Infof(
		"Initializing new route structure with the following params: name: %s, namespace: %s, serviceName: %s",
		name, nsname, serviceName)

	builder := common.NewNamespacedBuilder[routev1.Route, Builder](apiClient, routev1.AddToScheme, name, nsname)
	if builder.GetError() != nil {
		return builder
	}

	if serviceName == "" {
		klog.V(100).Info("The serviceName of the route is empty")

		builder.SetError(fmt.Errorf("route 'serviceName' cannot be empty"))

		return builder
	}

	builder.Definition.Spec.To = routev1.RouteTargetReference{
		Kind: "Service",
		Name: serviceName,
	}

	return builder
}

// Pull loads existing route from cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	klog.V(100).Infof("Pulling existing route name %s under namespace %s from cluster", name, nsname)

	return common.PullNamespacedBuilder[routev1.Route, Builder](
		context.TODO(), apiClient, routev1.AddToScheme, name, nsname)
}

// WithTargetPortNumber adds a target port to the route by number.
func (builder *Builder) WithTargetPortNumber(port int32) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Adding target port %d to route %s in namespace %s",
		port, builder.Definition.Name, builder.Definition.Namespace)

	if builder.Definition.Spec.Port == nil {
		builder.Definition.Spec.Port = new(routev1.RoutePort)
	}

	builder.Definition.Spec.Port.TargetPort = intstr.IntOrString{IntVal: port}

	return builder
}

// WithTargetPortName adds a target port to the route by name.
func (builder *Builder) WithTargetPortName(portName string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Adding target port %s to route %s in namespace %s",
		portName, builder.Definition.Name, builder.Definition.Namespace)

	if portName == "" {
		klog.V(100).Info("Received empty route portName")

		builder.SetError(fmt.Errorf("route target port name cannot be empty string"))
	}

	if builder.GetError() != nil {
		return builder
	}

	if builder.Definition.Spec.Port == nil {
		builder.Definition.Spec.Port = new(routev1.RoutePort)
	}

	builder.Definition.Spec.Port.TargetPort = intstr.IntOrString{StrVal: portName}

	return builder
}

// WithHostDomain adds a route host domain to the route.
func (builder *Builder) WithHostDomain(hostDomain string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Adding route host domain %s to route %s in namespace %s",
		hostDomain, builder.Definition.Name, builder.Definition.Namespace)

	if hostDomain == "" {
		klog.V(100).Info("Received empty route hostDomain")

		builder.SetError(fmt.Errorf("route host domain cannot be empty string"))

		return builder
	}

	builder.Definition.Spec.Host = hostDomain

	return builder
}

// WithWildCardPolicy adds the specified wildCardPolicy to the route.
func (builder *Builder) WithWildCardPolicy(wildcardPolicy string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Adding wildcardPolicy %s to route %s in namespace %s",
		wildcardPolicy, builder.Definition.Name, builder.Definition.Namespace)

	if !slices.Contains(supportedWildCardPolicies(), wildcardPolicy) {
		klog.V(100).Infof("Received unsupported route wildcardPolicy, expected one of %v", supportedWildCardPolicies())

		builder.SetError(getUnsupportedWildCardPoliciesError())

		return builder
	}

	builder.Definition.Spec.WildcardPolicy = routev1.WildcardPolicyType(wildcardPolicy)

	return builder
}

func supportedWildCardPolicies() []string {
	return []string{
		"Subdomain",
		"None",
	}
}

func getUnsupportedWildCardPoliciesError() error {
	return fmt.Errorf(
		"received unsupported route wildcardPolicy: expected one of %v",
		supportedWildCardPolicies())
}
