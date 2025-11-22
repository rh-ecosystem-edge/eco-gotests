package sriov

import (
	"fmt"

	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"k8s.io/klog/v2"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/strings/slices"
)

const (
	sriovOperatorConfigName = "default"
)

var (
	// allowedDisablePlugins represents all allowed plugins that could be disabled.
	allowedDisablePlugins = []string{"mellanox"}
)

// OperatorConfigBuilder provides a struct for SriovOperatorConfig object from the cluster and
// a SriovOperatorConfig definition.
type OperatorConfigBuilder struct {
	// SriovOperatorConfig definition, used to create the SriovOperatorConfig object.
	Definition *srIovV1.SriovOperatorConfig
	// Created SriovOperatorConfig object.
	Object *srIovV1.SriovOperatorConfig
	// api client to interact with the cluster.
	apiClient runtimeClient.Client
	errorMsg  string
}

// NewOperatorConfigBuilder creates new instance of OperatorConfigBuilder.
func NewOperatorConfigBuilder(apiClient *clients.Settings, nsname string) *OperatorConfigBuilder {
	klog.V(100).Infof(
		"Initializing new OperatorConfigBuilder structure with the following params: namespace: %s", nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriovv1 scheme to client schemes")

		return nil
	}

	builder := &OperatorConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovOperatorConfig{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      sriovOperatorConfigName,
				Namespace: nsname,
			},
		},
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovOperatorConfig is empty")

		builder.errorMsg = "SriovOperatorConfig 'nsname' is empty"

		return builder
	}

	return builder
}

// Create generates SriovOperatorConfig in a cluster and stores the created object in struct.
func (builder *OperatorConfigBuilder) Create() (*OperatorConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the SriovOperatorConfig in namespace %s", builder.Definition.Namespace)

	if !builder.Exists() {
		err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err != nil {
			klog.V(100).Info("Failed to create the SriovOperatorConfig")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// PullOperatorConfig loads an existing SriovOperatorConfig into OperatorConfigBuilder struct.
func PullOperatorConfig(apiClient *clients.Settings, nsname string) (*OperatorConfigBuilder, error) {
	klog.V(100).Infof("Pulling existing default SriovOperatorConfig: %s", sriovOperatorConfigName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("SriovOperatorConfig 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriovv1 scheme to client schemes")

		return nil, err
	}

	builder := &OperatorConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovOperatorConfig{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      sriovOperatorConfigName,
				Namespace: nsname,
			},
		},
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovOperatorConfig is empty")

		return nil, fmt.Errorf("SriovOperatorConfig 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("SriovOperatorConfig object %s does not exist in namespace %s",
			sriovOperatorConfigName, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns CatalogSource object if found.
func (builder *OperatorConfigBuilder) Get() (*srIovV1.SriovOperatorConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collecting SriovOperatorConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	operatorConfig := &srIovV1.SriovOperatorConfig{}

	err := builder.apiClient.Get(logging.DiscardContext(),
		runtimeClient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		operatorConfig)
	if err != nil {
		klog.V(100).Infof("SriovOperatorConfig object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return operatorConfig, nil
}

// Exists checks whether the given SriovOperatorConfig exists.
func (builder *OperatorConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if SriovOperatorConfig %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithInjector configures enableInjector in the SriovOperatorConfig.
func (builder *OperatorConfigBuilder) WithInjector(enable bool) *OperatorConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Configuring enableInjector %t to SriovOperatorConfig object %s",
		enable, builder.Definition.Name,
	)

	builder.Definition.Spec.EnableInjector = enable

	return builder
}

// WithOperatorWebhook configures enableOperatorWebhook in the SriovOperatorConfig.
func (builder *OperatorConfigBuilder) WithOperatorWebhook(enable bool) *OperatorConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Configuring WithOperatorWebhook %t to SriovOperatorConfig object %s",
		enable, builder.Definition.Name,
	)

	builder.Definition.Spec.EnableOperatorWebhook = enable

	return builder
}

// WithConfigDaemonNodeSelector configures configDaemonNodeSelector in the SriovOperatorConfig.
func (builder *OperatorConfigBuilder) WithConfigDaemonNodeSelector(
	configDaemonNodeSelector map[string]string) *OperatorConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Configuring configDaemonNodeSelector %s in SriovOperatorConfig object %s in namespace %s",
		configDaemonNodeSelector, builder.Definition.Name, builder.Definition.Namespace,
	)

	if len(configDaemonNodeSelector) == 0 {
		klog.V(100).Info("The 'configDaemonNodeSelector' of the SriovOperatorConfig is empty")

		builder.errorMsg = "can not apply empty configDaemonNodeSelector"

		return builder
	}

	for selectorKey := range configDaemonNodeSelector {
		if selectorKey == "" {
			klog.V(100).Info("The 'configDaemonNodeSelector' selectorKey cannot be empty")

			builder.errorMsg = "can not apply configDaemonNodeSelector with an empty selectorKey value"

			return builder
		}
	}

	builder.Definition.Spec.ConfigDaemonNodeSelector = configDaemonNodeSelector

	return builder
}

// WithDisablePlugins configures disablePlugins in the SriovOperatorConfig.
func (builder *OperatorConfigBuilder) WithDisablePlugins(plugins []string) *OperatorConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Configuring disablePlugins %v in SriovOperatorConfig object %s",
		plugins, builder.Definition.Name,
	)

	var pluginSlice srIovV1.PluginNameSlice

	for _, plugin := range plugins {
		if !slices.Contains(allowedDisablePlugins, plugin) {
			klog.V(100).Infof("error to add plugin %s, allowed modes are %v", plugin, allowedDisablePlugins)

			builder.errorMsg = "invalid plugin parameter"

			return builder
		}

		pluginSlice = append(pluginSlice, srIovV1.PluginNameValue(plugin))
	}

	builder.Definition.Spec.DisablePlugins = pluginSlice

	return builder
}

// RemoveDisablePlugins deletes disablePlugins in the SriovOperatorConfig.
func (builder *OperatorConfigBuilder) RemoveDisablePlugins() *OperatorConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Removing disablePlugins in SriovOperatorConfig object %s", builder.Definition.Name)
	builder.Definition.Spec.DisablePlugins = nil

	return builder
}

// Update renovates the existing SriovOperatorConfig object with the new definition in builder.
func (builder *OperatorConfigBuilder) Update() (*OperatorConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the SriovOperatorConfig object %s",
		builder.Definition.Name,
	)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes SriovOperatorConfig object from a cluster.
func (builder *OperatorConfigBuilder) Delete() (*OperatorConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the SriovOperatorConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace,
	)

	if !builder.Exists() {
		klog.V(100).Infof("SriovOperatorConfig %s namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete SriovOperatorConfig: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *OperatorConfigBuilder) validate() (bool, error) {
	resourceCRD := "SriovOperatorConfig"

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
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
