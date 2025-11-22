package servicemesh

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	istiov2 "maistra.io/api/core/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ControlPlaneBuilder provides a struct for serviceMeshControlPlane object from the cluster and
// a serviceMeshControlPlane definition.
type ControlPlaneBuilder struct {
	// serviceMeshControlPlane definition, used to create the serviceMeshControlPlane object.
	Definition *istiov2.ServiceMeshControlPlane
	// Created serviceMeshControlPlane object.
	Object *istiov2.ServiceMeshControlPlane
	// Used in functions that define or mutate serviceMeshControlPlane definition. errorMsg is processed
	// before the serviceMeshControlPlane object is created.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewControlPlaneBuilder method creates new instance of builder.
func NewControlPlaneBuilder(apiClient *clients.Settings, name, nsname string) *ControlPlaneBuilder {
	klog.V(100).Infof("Initializing new ControlPlaneBuilder structure with the following "+
		"params: name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(istiov2.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add istiov2 scheme to client schemes")

		return nil
	}

	builder := &ControlPlaneBuilder{
		apiClient: apiClient.Client,
		Definition: &istiov2.ServiceMeshControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the serviceMeshControlPlane is empty")

		builder.errorMsg = "serviceMeshControlPlane 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the serviceMeshControlPlane is empty")

		builder.errorMsg = "serviceMeshControlPlane 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// WithAllAddonsDisabled disables all addons to the serviceMeshControlPlane.
func (builder *ControlPlaneBuilder) WithAllAddonsDisabled() *ControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	enablement := false

	klog.V(100).Infof(
		"Creating serviceMeshControlPlane %s in namespace %s with the all addons disabled",
		builder.Definition.Name, builder.Definition.Namespace)

	disableAllAddons := istiov2.AddonsConfig{
		Prometheus: &istiov2.PrometheusAddonConfig{
			Enablement: istiov2.Enablement{Enabled: &enablement},
		},
		Grafana: &istiov2.GrafanaAddonConfig{
			Enablement: istiov2.Enablement{Enabled: &enablement},
		},
		Kiali: &istiov2.KialiAddonConfig{
			Enablement: istiov2.Enablement{Enabled: &enablement},
		},
		ThreeScale: &istiov2.ThreeScaleAddonConfig{
			Enablement: istiov2.Enablement{Enabled: &enablement},
		},
	}

	builder.Definition.Spec.Addons = &disableAllAddons

	return builder
}

// WithGrafanaAddon adds grafana addon to the serviceMeshControlPlane.
func (builder *ControlPlaneBuilder) WithGrafanaAddon(
	enablement bool,
	grafanaInstallConfig *istiov2.GrafanaInstallConfig,
	address string) *ControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating serviceMeshControlPlane %s in namespace %s with the Grafana addons defined: enablement %v, "+
			"grafanaInstallConfig %v, address %s", builder.Definition.Name, builder.Definition.Namespace,
		enablement, grafanaInstallConfig, address)

	if enablement {
		if grafanaInstallConfig == nil {
			klog.V(100).Info("The grafanaInstallConfig of the Grafana addon is empty")

			builder.errorMsg = "the Grafana addon 'grafanaInstallConfig' cannot be empty when Grafana addon is enabled"

			return builder
		}

		if address == "" {
			klog.V(100).Info("The address of the Grafana addon is empty")

			builder.errorMsg = "the Grafana addon 'address' cannot be empty when Grafana addon is enabled"

			return builder
		}
	}

	addonConfig := &istiov2.GrafanaAddonConfig{
		Enablement: istiov2.Enablement{
			Enabled: &enablement,
		},
		Install: grafanaInstallConfig,
		Address: &address,
	}

	if builder.Definition.Spec.Addons == nil {
		builder.Definition.Spec.Addons = new(istiov2.AddonsConfig)
	}

	builder.Definition.Spec.Addons.Grafana = addonConfig

	return builder
}

// WithJaegerAddon adds jaeger addon to the serviceMeshControlPlane.
func (builder *ControlPlaneBuilder) WithJaegerAddon(
	name string,
	jaegerInstallConfig *istiov2.JaegerInstallConfig) *ControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating serviceMeshControlPlane %s in namespace %s with the JaegerAddonConfig defined",
		builder.Definition.Name, builder.Definition.Namespace)

	if name == "" {
		klog.V(100).Info("The name of the Jaeger addon is empty")

		builder.errorMsg = "the Jaeger addon 'name' cannot be empty"

		return builder
	}

	if jaegerInstallConfig == nil {
		klog.V(100).Info("The jaegerInstallConfig of the Jaeger addon is empty")

		builder.errorMsg = "the Jaeger addon 'jaegerInstallConfig' cannot be empty"

		return builder
	}

	addonConfig := &istiov2.JaegerAddonConfig{
		Name:    name,
		Install: jaegerInstallConfig,
	}

	if builder.Definition.Spec.Addons == nil {
		builder.Definition.Spec.Addons = new(istiov2.AddonsConfig)
	}

	builder.Definition.Spec.Addons.Jaeger = addonConfig

	return builder
}

// WithKialiAddon adds kiali addons to the serviceMeshControlPlane.
func (builder *ControlPlaneBuilder) WithKialiAddon(
	enablement bool,
	name string,
	kialiInstallConfig *istiov2.KialiInstallConfig) *ControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating serviceMeshControlPlane %s in namespace %s with the KialiAddonConfig defined",
		builder.Definition.Name, builder.Definition.Namespace)

	if enablement {
		if kialiInstallConfig == nil {
			klog.V(100).Info("The kialiInstallConfig of the Kiali addon is empty")

			builder.errorMsg = "the Kiali addon 'kialiInstallConfig' cannot be empty when Kiali addon is enabled"

			return builder
		}

		if name == "" {
			klog.V(100).Info("The name of the Kiali addon is empty")

			builder.errorMsg = "the Kiali addon 'name' cannot be empty when Kiali addon is enabled"

			return builder
		}
	}

	addonConfig := &istiov2.KialiAddonConfig{
		Enablement: istiov2.Enablement{
			Enabled: &enablement,
		},
		Name:    name,
		Install: kialiInstallConfig,
	}

	if builder.Definition.Spec.Addons == nil {
		builder.Definition.Spec.Addons = new(istiov2.AddonsConfig)
	}

	builder.Definition.Spec.Addons.Kiali = addonConfig

	return builder
}

// WithPrometheusAddon adds prometheus addons to the serviceMeshControlPlane.
func (builder *ControlPlaneBuilder) WithPrometheusAddon(
	enablement bool,
	scrape bool,
	metricsExpiryDuration string,
	address string,
	prometheusInstallConfig *istiov2.PrometheusInstallConfig) *ControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating serviceMeshControlPlane %s in namespace %s with the PrometheusAddonConfig defined",
		builder.Definition.Name, builder.Definition.Namespace)

	if enablement {
		if prometheusInstallConfig == nil {
			klog.V(100).Info("The prometheusInstallConfig of the Prometheus addon is empty")

			builder.errorMsg = "the Prometheus addon 'prometheusInstallConfig' cannot " +
				"be empty when Prometheus addon is enabled"

			return builder
		}

		if metricsExpiryDuration == "" {
			klog.V(100).Info("The metricsExpiryDuration of the Prometheus addon is empty")

			builder.errorMsg = "the Prometheus addon 'metricsExpiryDuration' cannot " +
				"be empty when Prometheus addon is enabled"

			return builder
		}

		if address == "" {
			klog.V(100).Info("The address of the Prometheus addon is empty")

			builder.errorMsg = "the Prometheus addon 'address' cannot be empty when Prometheus addon is enabled"

			return builder
		}
	}

	addonConfig := &istiov2.PrometheusAddonConfig{
		Enablement: istiov2.Enablement{
			Enabled: &enablement,
		},
		MetricsExpiryDuration: metricsExpiryDuration,
		Scrape:                &scrape,
		Install:               prometheusInstallConfig,
		Address:               &address,
	}

	if builder.Definition.Spec.Addons == nil {
		builder.Definition.Spec.Addons = new(istiov2.AddonsConfig)
	}

	builder.Definition.Spec.Addons.Prometheus = addonConfig

	return builder
}

// WithGatewaysEnablement adds gateway enablement to the serviceMeshControlPlane.
func (builder *ControlPlaneBuilder) WithGatewaysEnablement(enablement bool) *ControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating serviceMeshControlPlane %s in namespace %s with enabled Gateways",
		builder.Definition.Name, builder.Definition.Namespace)

	gatewaysConfig := istiov2.Enablement{Enabled: &enablement}

	if builder.Definition.Spec.Gateways == nil {
		builder.Definition.Spec.Gateways = &istiov2.GatewaysConfig{}
	}

	builder.Definition.Spec.Gateways.Enablement = gatewaysConfig

	return builder
}

// PullControlPlane retrieves an existing serviceMeshControlPlane object from the cluster.
func PullControlPlane(apiClient *clients.Settings, name, nsname string) (*ControlPlaneBuilder, error) {
	klog.V(100).Infof(
		"Pulling serviceMeshControlPlane object name %s in namespace: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("serviceMeshControlPlane 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(istiov2.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add istiov2 scheme to client schemes")

		return nil, err
	}

	builder := &ControlPlaneBuilder{
		apiClient: apiClient.Client,
		Definition: &istiov2.ServiceMeshControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the serviceMeshControlPlane is empty")

		return nil, fmt.Errorf("serviceMeshControlPlane 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the serviceMeshControlPlane is empty")

		return nil, fmt.Errorf("serviceMeshControlPlane 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("serviceMeshControlPlane object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches existing serviceMeshControlPlane from cluster.
func (builder *ControlPlaneBuilder) Get() (*istiov2.ServiceMeshControlPlane, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Pulling existing serviceMeshControlPlane with name %s in namespace %s from cluster",
		builder.Definition.Name, builder.Definition.Namespace)

	servicemeshcontrolplane := &istiov2.ServiceMeshControlPlane{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, servicemeshcontrolplane)
	if err != nil {
		return nil, err
	}

	builder.Object = servicemeshcontrolplane

	return servicemeshcontrolplane, nil
}

// Create makes a serviceMeshControlPlane in the cluster and stores the created object in struct.
func (builder *ControlPlaneBuilder) Create() (*ControlPlaneBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the serviceMeshControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes serviceMeshControlPlane from a cluster.
func (builder *ControlPlaneBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the serviceMeshControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("The serviceMeshControlPlane %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete serviceMeshControlPlane %s in namespace %s due to %w",
			builder.Definition.Name, builder.Definition.Namespace, err)
	}

	builder.Object = nil

	return nil
}

// Update renovates the existing serviceMeshControlPlane object with serviceMeshControlPlane definition in builder.
func (builder *ControlPlaneBuilder) Update(force bool) (*ControlPlaneBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating serviceMeshControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("serviceMeshControlPlane", builder.Definition.Name, builder.Definition.Namespace))

			err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("serviceMeshControlPlane", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks whether the given serviceMeshControlPlane exists.
func (builder *ControlPlaneBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if serviceMeshControlPlane %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ControlPlaneBuilder) validate() (bool, error) {
	resourceCRD := "ServiceMeshControlPlane"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The builder %s has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
