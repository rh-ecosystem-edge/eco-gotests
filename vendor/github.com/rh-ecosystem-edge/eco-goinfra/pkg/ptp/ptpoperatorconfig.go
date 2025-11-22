package ptp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	// PtpOperatorConfigName is the name of the PtpOperatorConfig singleton.
	PtpOperatorConfigName = "default"
	// PtpOperatorConfigNamespace is the namespace for the PtpOperatorConfig singleton.
	PtpOperatorConfigNamespace = "openshift-ptp"
)

// PtpOperatorConfigBuilder provides a struct for the PtpOperatorConfig resource.
type PtpOperatorConfigBuilder struct {
	// Definition of the PtpOperatorConfig used to create the object.
	Definition *ptpv1.PtpOperatorConfig
	// Object of the PtpOperatorConfig as it is on the cluster.
	Object    *ptpv1.PtpOperatorConfig
	apiClient runtimeclient.Client
	errorMsg  string
}

// PullPtpOperatorConfig pulls the existing PtpOperatorConfig singleton resource.
func PullPtpOperatorConfig(apiClient *clients.Settings) (*PtpOperatorConfigBuilder, error) {
	klog.V(100).Infof("Pulling existing PtpOperatorConfig %s from namespace %s",
		PtpOperatorConfigName, PtpOperatorConfigNamespace)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("ptpOperatorConfig 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(ptpv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add PtpOperatorConfig scheme to client schemes")

		return nil, err
	}

	builder := PtpOperatorConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &ptpv1.PtpOperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PtpOperatorConfigName,
				Namespace: PtpOperatorConfigNamespace,
			},
		},
	}

	if !builder.Exists() {
		klog.V(100).Infof("PtpOperatorConfig %s does not exist in namespace %s",
			PtpOperatorConfigName, PtpOperatorConfigNamespace)

		return nil, fmt.Errorf("ptpOperatorConfig object %s does not exist in namespace %s",
			PtpOperatorConfigName, PtpOperatorConfigNamespace)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get returns the PtpOperatorConfig object if found.
func (builder *PtpOperatorConfigBuilder) Get() (*ptpv1.PtpOperatorConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting PtpOperatorConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	ptpOpConfig := &ptpv1.PtpOperatorConfig{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, ptpOpConfig)
	if err != nil {
		return nil, err
	}

	return ptpOpConfig, nil
}

// Exists checks whether the PtpOperatorConfig exists on the cluster.
func (builder *PtpOperatorConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if PtpOperatorConfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to get PtpOperatorConfig %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return false
	}

	return true
}

// Update changes the existing PtpOperatorConfig resource on the cluster.
func (builder *PtpOperatorConfigBuilder) Update() (*PtpOperatorConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Updating PtpOperatorConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("PtpOperatorConfig %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent ptpOperatorConfig")
	}

	// Preserve the existing resource version to avoid update conflicts
	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithEventConfig sets the PtpEventConfig for the PtpOperatorConfig. It validates that TransportHost is a valid URL and
// ApiVersion is either "1.0" or starts with "2." if provided.
func (builder *PtpOperatorConfigBuilder) WithEventConfig(eventConfig ptpv1.PtpEventConfig) *PtpOperatorConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting PtpEventConfig for PtpOperatorConfig %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if eventConfig.TransportHost != "" {
		_, err := url.Parse(eventConfig.TransportHost)
		if err != nil {
			builder.errorMsg = fmt.Sprintf("invalid TransportHost for PtpEventConfig: %v", err)

			return builder
		}
	}

	if eventConfig.ApiVersion != "" && eventConfig.ApiVersion != "1.0" &&
		!strings.HasPrefix(eventConfig.ApiVersion, "2.") {
		builder.errorMsg = "invalid ApiVersion for PtpEventConfig: must be 1.0 or start with 2."

		return builder
	}

	builder.Definition.Spec.EventConfig = &eventConfig

	return builder
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *PtpOperatorConfigBuilder) validate() (bool, error) {
	resourceCRD := "ptpOperatorConfig"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s definition is uninitialized", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
