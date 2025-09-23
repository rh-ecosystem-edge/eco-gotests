package route

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/strings/slices"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides struct for route object containing connection to the cluster and the route definitions.
type Builder struct {
	// Route definition. Used to create a route object
	Definition *routev1.Route
	// Created route object
	Object *routev1.Route
	// Used in functions that define or mutate the route definition.
	// errorMsg is processed before the route object is created
	errorMsg  string
	apiClient goclient.Client
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, nsname, serviceName string) *Builder {
	glog.V(100).Infof(
		"Initializing new route structure with the following params: name: %s, namespace: %s, serviceName: %s",
		name, nsname, serviceName)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is nil")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: routev1.RouteSpec{
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: serviceName,
				},
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the route is empty")

		builder.errorMsg = "route 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the route is empty")

		builder.errorMsg = "route 'nsname' cannot be empty"

		return builder
	}

	if serviceName == "" {
		glog.V(100).Infof("The serviceName of the route is empty")

		builder.errorMsg = "route 'serviceName' cannot be empty"

		return builder
	}

	return builder
}

// Pull loads existing route from cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	glog.V(100).Infof("Pulling existing route name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the route is empty")

		return nil, fmt.Errorf("route 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the route is empty")

		return nil, fmt.Errorf("route 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("route object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// WithTargetPortNumber adds a target port to the route by number.
func (builder *Builder) WithTargetPortNumber(port int32) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding target port %d to route %s in namespace %s",
		port, builder.Definition.Name, builder.Definition.Namespace)

	if builder.Definition.Spec.Port == nil {
		builder.Definition.Spec.Port = new(routev1.RoutePort)
	}

	builder.Definition.Spec.Port.TargetPort = intstr.IntOrString{IntVal: port}

	return builder
}

// WithTargetPortName adds a target port to the route by name.
func (builder *Builder) WithTargetPortName(portName string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding target port %s to route %s in namespace %s",
		portName, builder.Definition.Name, builder.Definition.Namespace)

	if portName == "" {
		glog.V(100).Infof("Received empty route portName")

		builder.errorMsg = "route target port name cannot be empty string"
	}

	if builder.errorMsg != "" {
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
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding route host domain %s to route %s in namespace %s",
		hostDomain, builder.Definition.Name, builder.Definition.Namespace)

	if hostDomain == "" {
		glog.V(100).Infof("Received empty route hostDomain")

		builder.errorMsg = "route host domain cannot be empty string"

		return builder
	}

	builder.Definition.Spec.Host = hostDomain

	return builder
}

// WithWildCardPolicy adds the specified wildCardPolicy to the route.
func (builder *Builder) WithWildCardPolicy(wildcardPolicy string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Adding wildcardPolicy %s to route %s in namespace %s",
		wildcardPolicy, builder.Definition.Name, builder.Definition.Namespace)

	if !slices.Contains(supportedWildCardPolicies(), wildcardPolicy) {
		glog.V(100).Infof("Received unsupported route wildcardPolicy, supported policies: %v", supportedWildCardPolicies())

		builder.errorMsg = fmt.Sprintf("received unsupported route wildcardPolicy: supported policies %v",
			supportedWildCardPolicies())

		return builder
	}

	builder.Definition.Spec.WildcardPolicy = routev1.WildcardPolicyType(wildcardPolicy)

	return builder
}

// Exists checks whether the given route exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof(
		"Checking if route %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns route object if found.
func (builder *Builder) Get() (*routev1.Route, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Getting route %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	route := &routev1.Route{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, route)
	if err != nil {
		return nil, err
	}

	return route, nil
}

// Create makes a route according to the route definition and stores the created object in the route builder.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the route %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes the route object and resets the builder object.
func (builder *Builder) Delete() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the route %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("Route %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("cannot delete route: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "Route"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}

func supportedWildCardPolicies() []string {
	return []string{
		"Subdomain",
		"None",
	}
}
