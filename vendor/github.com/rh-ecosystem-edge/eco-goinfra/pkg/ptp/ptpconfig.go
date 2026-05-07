package ptp

import (
	"encoding/json"
	"fmt"
	"slices"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// PtpConfigBuilder provides a struct for the PtpConfig resource containing a connection to the cluster and the
// PtpConfig definition.
type PtpConfigBuilder struct {
	// Definition of the PtpConfig used to create the object.
	Definition *ptpv1.PtpConfig
	// Object of the PtpConfig as it is on the cluster.
	Object    *ptpv1.PtpConfig
	apiClient goclient.Client
	errorMsg  string
}

// NewPtpConfigBuilder creates a new instance of a PtpConfig builder.
func NewPtpConfigBuilder(apiClient *clients.Settings, name, nsname string) *PtpConfigBuilder {
	klog.V(100).Infof("Initializing new PtpConfig structure with the following params: name: %s, nsname: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient of the PtpConfig is nil")

		return nil
	}

	err := apiClient.AttachScheme(ptpv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add ptp v1 scheme to client schemes")

		return nil
	}

	builder := &PtpConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &ptpv1.PtpConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the PtpConfig is empty")

		builder.errorMsg = "ptpConfig 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the PtpConfig is empty")

		builder.errorMsg = "ptpConfig 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// intelPluginTypes is the list of Intel plugin types to check for in order.
var intelPluginTypes = []PluginType{PluginTypeE810, PluginTypeE825, PluginTypeE830}

// GetIntelPlugin retrieves an Intel plugin (E810, E825, or E830) from the specified profile in the PtpConfig,
// attempting to unmarshal the raw JSON. The plugin's Type field is set based on which plugin key was found. If the
// profile is not found or no Intel plugin exists, it returns an error.
func (builder *PtpConfigBuilder) GetIntelPlugin(profileName string) (*IntelPlugin, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Unmarshalling Intel plugin from PtpConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if profileName == "" {
		klog.V(100).Info("The profileName is empty")

		return nil, fmt.Errorf("profileName cannot be empty")
	}

	for _, profile := range builder.Definition.Spec.Profile {
		if profile.Name == nil || *profile.Name != profileName {
			continue
		}

		// Check for each Intel plugin type in order
		for _, pluginType := range intelPluginTypes {
			if profile.Plugins == nil {
				continue
			}

			pluginJSON, ok := profile.Plugins[string(pluginType)]
			if !ok || pluginJSON == nil {
				continue
			}

			intelPlugin := &IntelPlugin{}

			err := json.Unmarshal(pluginJSON.Raw, intelPlugin)
			if err != nil {
				klog.V(100).Infof("Failed to unmarshal %s plugin: %v", pluginType, err)

				return nil, err
			}

			intelPlugin.Type = pluginType

			return intelPlugin, nil
		}

		klog.V(100).Infof("No Intel plugin found for profile %s", profileName)

		return nil, fmt.Errorf("ptpProfile %s does not have an Intel plugin", profileName)
	}

	klog.V(100).Infof("Profile %s not found in PtpConfig %s in namespace %s",
		profileName, builder.Definition.Name, builder.Definition.Namespace)

	return nil, fmt.Errorf("ptpProfile %s not found", profileName)
}

// WithIntelPlugin sets an Intel plugin in the specified profile of the PtpConfig, attempting to marshal the plugin
// struct into JSON. The plugin's Type field determines which key (e810, e825, or e830) is used in the Plugins map. If
// the plugin's Type is not set, an error is returned.
func (builder *PtpConfigBuilder) WithIntelPlugin(profileName string, plugin *IntelPlugin) *PtpConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting Intel plugin for PtpConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if profileName == "" {
		klog.V(100).Info("The profileName is empty")

		builder.errorMsg = "cannot set Intel plugin: profileName cannot be empty"

		return builder
	}

	if plugin == nil {
		klog.V(100).Info("Intel plugin is nil")

		builder.errorMsg = "cannot set Intel plugin: plugin is nil"

		return builder
	}

	if plugin.Type == "" {
		klog.V(100).Info("Intel plugin Type is not set")

		builder.errorMsg = "cannot set Intel plugin: plugin Type is not set"

		return builder
	}

	if !slices.Contains(intelPluginTypes, plugin.Type) {
		klog.V(100).Infof("Intel plugin type %s is not supported", plugin.Type)

		builder.errorMsg = fmt.Sprintf("cannot set Intel plugin: plugin type %s is not supported", plugin.Type)

		return builder
	}

	for profileIndex, profile := range builder.Definition.Spec.Profile {
		if profile.Name == nil || *profile.Name != profileName {
			continue
		}

		pluginRaw, err := json.Marshal(plugin)
		if err != nil {
			klog.V(100).Infof("Failed to marshal %s plugin: %v", plugin.Type, err)

			builder.errorMsg = fmt.Sprintf("cannot set Intel plugin: failed to marshal plugin struct: %v", err)

			return builder
		}

		if profile.Plugins == nil {
			profile.Plugins = make(map[string]*apiextensionsv1.JSON)
		}

		// Ensure only one Intel plugin key exists per profile.
		for _, pluginType := range intelPluginTypes {
			delete(profile.Plugins, string(pluginType))
		}

		profile.Plugins[string(plugin.Type)] = &apiextensionsv1.JSON{Raw: pluginRaw}
		builder.Definition.Spec.Profile[profileIndex] = profile

		return builder
	}

	builder.errorMsg = fmt.Sprintf("cannot set Intel plugin: ptpProfile %s does not exist", profileName)

	return builder
}

// GetPluginType returns the Intel plugin type (e810, e825, or e830) for the specified profile, if one exists. This is a
// lightweight check that does not unmarshal the plugin data.
func (builder *PtpConfigBuilder) GetPluginType(profileName string) (PluginType, error) {
	if valid, err := builder.validate(); !valid {
		return "", err
	}

	klog.V(100).Infof("Getting Intel plugin type from PtpConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if profileName == "" {
		klog.V(100).Info("The profileName is empty")

		return "", fmt.Errorf("profileName cannot be empty")
	}

	for _, profile := range builder.Definition.Spec.Profile {
		if profile.Name == nil || *profile.Name != profileName {
			continue
		}

		for _, pluginType := range intelPluginTypes {
			if profile.Plugins == nil {
				continue
			}

			pluginJSON, ok := profile.Plugins[string(pluginType)]
			if !ok || pluginJSON == nil {
				continue
			}

			return pluginType, nil
		}

		klog.V(100).Infof("No Intel plugin found for profile %s", profileName)

		return "", fmt.Errorf("ptpProfile %s does not have an Intel plugin", profileName)
	}

	klog.V(100).Infof("Profile %s not found in PtpConfig %s in namespace %s",
		profileName, builder.Definition.Name, builder.Definition.Namespace)

	return "", fmt.Errorf("ptpProfile %s not found", profileName)
}

// PullPtpConfig pulls an existing PtpConfig into a Builder struct.
func PullPtpConfig(apiClient *clients.Settings, name, nsname string) (*PtpConfigBuilder, error) {
	klog.V(100).Infof("Pulling existing PtpConfig %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("ptpConfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(ptpv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add PtpConfig scheme to client schemes")

		return nil, err
	}

	builder := &PtpConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &ptpv1.PtpConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the PtpConfig is empty")

		return nil, fmt.Errorf("ptpConfig 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the PtpConfig is empty")

		return nil, fmt.Errorf("ptpConfig 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		klog.V(100).Infof("The PtpConfig %s does not exist in namespace %s", name, nsname)

		return nil, fmt.Errorf("ptpConfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the PtpConfig object if found.
func (builder *PtpConfigBuilder) Get() (*ptpv1.PtpConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Getting PtpConfig object %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	ptpConfig := &ptpv1.PtpConfig{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, ptpConfig)
	if err != nil {
		return nil, err
	}

	return ptpConfig, nil
}

// Exists checks whether the given PtpConfig exists on the cluster.
func (builder *PtpConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof(
		"Checking if PtpConfig %s exists in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to get PtpConfig %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return false
	}

	return true
}

// Create makes a PtpConfig on the cluster if it does not already exist.
func (builder *PtpConfigBuilder) Create() (*PtpConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Creating PtpConfig %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if builder.Exists() {
		return builder, nil
	}

	err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Update changes the existing PtpConfig resource on the cluster, failing if it does not exist or cannot be updated.
func (builder *PtpConfigBuilder) Update() (*PtpConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Updating PtpConfig %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof(
			"PtpConfig %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent ptpConfig")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes a PtpConfig from the cluster if it exists.
func (builder *PtpConfigBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof(
		"Deleting PtpConfig %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof(
			"PtpConfig %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Object)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *PtpConfigBuilder) validate() (bool, error) {
	resourceCRD := "ptpConfig"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is uninitialized", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
