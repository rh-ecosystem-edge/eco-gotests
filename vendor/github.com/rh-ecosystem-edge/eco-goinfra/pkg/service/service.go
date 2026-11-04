package service

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

const (
	errEmptyAnnotation            = "annotation can not be empty map"
	failedToSetEmptyIpstackpolicy = "failed to set empty ipStackPolicy"
)

// AdditionalOptions additional options for service object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// Builder provides struct for service object containing connection to the cluster and the service definitions.
type Builder struct {
	common.EmbeddableBuilder[corev1.Service, *corev1.Service]
	common.EmbeddableWithOptions[corev1.Service, Builder, *corev1.Service, *Builder, AdditionalOptions]
	common.EmbeddableCreator[corev1.Service, Builder, *corev1.Service, *Builder]
	common.EmbeddableDeleter[corev1.Service, *corev1.Service]
	common.EmbeddableUpdater[corev1.Service, Builder, *corev1.Service, *Builder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *Builder) AttachMixins() {
	builder.EmbeddableWithOptions.SetBase(builder)
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
	builder.EmbeddableUpdater.SetBase(builder)
}

// GetGVK returns the Service GVK for this builder.
func (builder *Builder) GetGVK() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Service")
}

// NewBuilder creates a new instance of Builder
// Default type of service is ClusterIP
// Use WithNodePort() for setting the NodePort type.
func NewBuilder(
	apiClient *clients.Settings,
	name string,
	nsname string,
	selector map[string]string,
	servicePort corev1.ServicePort) *Builder {
	klog.V(100).Infof(
		"Initializing new service structure with the following params: %s, %s", name, nsname)

	builder := common.NewNamespacedBuilder[corev1.Service, Builder](apiClient, corev1.AddToScheme, name, nsname)
	builder.Definition.Spec = corev1.ServiceSpec{
		Selector: selector,
		Ports:    []corev1.ServicePort{servicePort},
	}

	return builder
}

// WithNodePort redefines the service with NodePort service type.
func (builder *Builder) WithNodePort() *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	builder.Definition.Spec.Type = "NodePort"

	if len(builder.Definition.Spec.Ports) < 1 {
		builder.SetError(fmt.Errorf("service does not have the available ports"))

		return builder
	}

	builder.Definition.Spec.Ports[0].NodePort = builder.Definition.Spec.Ports[0].Port

	return builder
}

// Pull loads an existing service into Builder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	return common.PullNamespacedBuilder[corev1.Service, Builder](
		context.TODO(), apiClient, corev1.AddToScheme, name, nsname)
}

// WithExternalTrafficPolicy redefines the service with ServiceExternalTrafficPolicy type.
//
//nolint:staticcheck // Removing deprecated types would break consumers of eco-goinfra
func (builder *Builder) WithExternalTrafficPolicy(policyType corev1.ServiceExternalTrafficPolicyType) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof(
		"Defining service's with ExternalTrafficPolicy: %v", policyType)

	if policyType == "" {
		klog.V(100).Infof(
			"Failed to set ExternalTrafficPolicy on service %s in namespace %s. "+
				"policyType can not be empty",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.SetError(fmt.Errorf("ExternalTrafficPolicy can not be empty"))

		return builder
	}

	builder.Definition.Spec.Type = "LoadBalancer"
	builder.Definition.Spec.ExternalTrafficPolicy = policyType

	return builder
}

// WithLabels redefines the service with label.
func (builder *Builder) WithLabels(labels map[string]string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Defining service's with labels: %v", labels)

	if len(labels) == 0 {
		klog.V(100).Info("labels can not be empty")

		builder.SetError(fmt.Errorf("labels can not be empty"))

		return builder
	}

	for key := range labels {
		if key == "" {
			klog.V(100).Info("The 'labels' key cannot be empty")

			builder.SetError(fmt.Errorf("can not apply a labels with an empty key"))

			return builder
		}
	}

	builder.Definition.Labels = labels

	return builder
}

// WithSelector redefines the service with Selector.
func (builder *Builder) WithSelector(selector map[string]string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Defining service's with selector: %v", selector)

	if len(selector) == 0 {
		klog.V(100).Info("selector can not be empty")

		builder.SetError(fmt.Errorf("selector can not be empty"))

		return builder
	}

	for key := range selector {
		if key == "" {
			klog.V(100).Info("The 'selector' key cannot be empty")

			builder.SetError(fmt.Errorf("can not apply a selector with an empty key"))

			return builder
		}
	}

	builder.Definition.Spec.Selector = selector

	return builder
}

// WithAnnotation redefines the service with Annotation type.
func (builder *Builder) WithAnnotation(annotation map[string]string) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Defining service's Annotation to %v", annotation)

	if len(annotation) == 0 {
		klog.V(100).Infof(
			"Failed to set Annotation on service %s in namespace %s. "+
				"Service Annotation can not be empty",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.SetError(fmt.Errorf("%s", errEmptyAnnotation))

		return builder
	}

	for key := range annotation {
		if key == "" {
			klog.V(100).Info("The 'annotation' key cannot be empty")

			builder.SetError(fmt.Errorf("can not apply a annotation with an empty key"))

			return builder
		}
	}

	builder.Definition.Annotations = annotation

	return builder
}

// WithIPFamily redefines the service with IPFamilies type.
//
//nolint:staticcheck // Removing deprecated types would break consumers of eco-goinfra
func (builder *Builder) WithIPFamily(ipFamily []corev1.IPFamily, ipStackPolicy corev1.IPFamilyPolicyType) *Builder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	klog.V(100).Infof("Defining service's IPFamily: %v and IPFamilyPolicy: %v", ipFamily, ipStackPolicy)

	if ipFamily == nil {
		klog.V(100).Infof("Failed to set empty ipFamily on service %s in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.SetError(fmt.Errorf("failed to set empty ipFamily"))

		return builder
	}

	if ipStackPolicy == "" {
		klog.V(100).Infof("Failed to set empty ipStackPolicy on service %s in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.SetError(fmt.Errorf("%s", failedToSetEmptyIpstackpolicy))

		return builder
	}

	builder.Definition.Spec.IPFamilies = ipFamily
	builder.Definition.Spec.IPFamilyPolicy = &ipStackPolicy

	return builder
}

// DefineServicePort helper for creating a Service with a ServicePort.
func DefineServicePort(port, targetPort int32, protocol corev1.Protocol) (*corev1.ServicePort, error) {
	klog.V(100).Infof(
		"Defining ServicePort with port %d and targetport %d", port, targetPort)

	if !isValidPort(port) {
		return nil, fmt.Errorf("invalid port number")
	}

	if !isValidPort(targetPort) {
		return nil, fmt.Errorf("invalid target port number")
	}

	return &corev1.ServicePort{
		Protocol: protocol,
		Port:     port,
		TargetPort: intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: targetPort,
		},
	}, nil
}

// GetGVR returns service's GroupVersionResource which could be used for Clean function.
func GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
}

// isValidPort checks if a port is valid.
func isValidPort(port int32) bool {
	if (port > 0) && (port < 65535) {
		return true
	}

	return false
}
