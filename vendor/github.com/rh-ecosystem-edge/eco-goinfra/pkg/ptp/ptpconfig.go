package ptp

import (
	"context"
	"encoding/json"
	"fmt"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
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

// GetE810Plugin retrieves the E810 plugin from the specified profile in the PtpConfig, attempting to unmarshal the raw
// JSON. If the profile or plugin is not found, it returns an error.
func (builder *PtpConfigBuilder) GetE810Plugin(profileName string) (*E810Plugin, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Unmarshalling E810 plugin from PtpConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	for _, profile := range builder.Definition.Spec.Profile {
		if profile.Name == nil || *profile.Name != profileName || profile.Plugins == nil {
			continue
		}

		if plugin, ok := profile.Plugins["e810"]; ok && plugin != nil {
			e810Plugin := &E810Plugin{}

			err := json.Unmarshal(plugin.Raw, e810Plugin)
			if err != nil {
				klog.V(100).Infof("Failed to unmarshal E810 plugin: %v", err)

				return nil, err
			}

			return e810Plugin, nil
		}

		klog.V(100).Infof("E810 plugin not found for profile %s", profileName)

		return nil, fmt.Errorf("ptpProfile %s does not have E810 plugin", profileName)
	}

	klog.V(100).Infof("Profile %s not found in PtpConfig %s in namespace %s",
		profileName, builder.Definition.Name, builder.Definition.Namespace)

	return nil, fmt.Errorf("ptpProfile %s not found", profileName)
}

// WithE810Plugin sets the E810 plugin in the specified profile of the PtpConfig, attempting to marshal the plugin
// struct into JSON.
func (builder *PtpConfigBuilder) WithE810Plugin(profileName string, e810Plugin *E810Plugin) *PtpConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting E810 plugin for PtpConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	for profileIndex, profile := range builder.Definition.Spec.Profile {
		if profile.Name == nil || *profile.Name != profileName {
			continue
		}

		e810PluginRaw, err := json.Marshal(e810Plugin)
		if err != nil {
			klog.V(100).Infof("Failed to marshal E810 plugin: %v", err)

			builder.errorMsg = fmt.Sprintf("cannot set E810 plugin: failed to marshal plugin struct: %v", err)

			return builder
		}

		if profile.Plugins == nil {
			profile.Plugins = make(map[string]*apiextensionsv1.JSON)
		}

		profile.Plugins["e810"] = &apiextensionsv1.JSON{Raw: e810PluginRaw}
		builder.Definition.Spec.Profile[profileIndex] = profile

		return builder
	}

	builder.errorMsg = fmt.Sprintf("cannot set E810 plugin: ptpProfile %s does not exist", profileName)

	return builder
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

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
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

	err := builder.apiClient.Create(context.TODO(), builder.Definition)
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

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
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

	err := builder.apiClient.Delete(context.TODO(), builder.Object)
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
