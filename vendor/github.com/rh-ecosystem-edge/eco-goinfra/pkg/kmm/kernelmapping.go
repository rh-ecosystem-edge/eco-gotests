package kmm

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// KernelMappingBuilder builds kernelMapping struct based on given parameters.
type KernelMappingBuilder struct {
	// Module definition. Used to create a Module object.
	definition *moduleV1Beta1.KernelMapping
	// Used in functions that define or mutate Module definition. errorMsg is processed before the Module
	// object is created.
	errorMsg string
}

// KernelMappingAdditionalOptions additional options for KernelMapping object.
type KernelMappingAdditionalOptions func(builder *KernelMappingBuilder) (*KernelMappingBuilder, error)

// NewRegExKernelMappingBuilder creates new kernel mapping element based on regex.
func NewRegExKernelMappingBuilder(regex string) *KernelMappingBuilder {
	klog.V(100).Infof(
		"Initializing new regex KernelMapping parameter structure with the following regex param: %s", regex)

	builder := &KernelMappingBuilder{
		definition: &moduleV1Beta1.KernelMapping{
			Regexp: regex,
		},
	}

	if regex == "" {
		klog.V(100).Info("The regex of NewRegExKernelMappingBuilder is empty")

		builder.errorMsg = "'regex' parameter can not be empty"

		return builder
	}

	return builder
}

// NewLiteralKernelMappingBuilder create new kernel mapping element based on literal.
func NewLiteralKernelMappingBuilder(literal string) *KernelMappingBuilder {
	klog.V(100).Infof(
		"Initializing new literal KernelMapping parameter structure with following literal param: %s", literal)

	builder := &KernelMappingBuilder{
		definition: &moduleV1Beta1.KernelMapping{
			Literal: literal,
		},
	}

	if literal == "" {
		klog.V(100).Info("The literal of NewLiteralKernelMappingBuilder is empty")

		builder.errorMsg = "'literal' parameter can not be empty"

		return builder
	}

	return builder
}

// BuildKernelMappingConfig returns kernel mapping config if error is not occur.
func (builder *KernelMappingBuilder) BuildKernelMappingConfig() (*moduleV1Beta1.KernelMapping, error) {
	if valid, err := builder.validate(); !valid {
		return nil, fmt.Errorf("error building KernelMappingConfig config due to :%w", err)
	}

	klog.V(100).Infof(
		"Returning the KernelMappingBuilder structure %v", builder.definition)

	return builder.definition, nil
}

// WithContainerImage adds the specified Container Image config to the KernelMapper.
func (builder *KernelMappingBuilder) WithContainerImage(image string) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new Module KernelMapping parameter with container image: %s", image)

	if image == "" {
		klog.V(100).Info("The image of WithContainerImage is empty")

		builder.errorMsg = "'image' parameter can not be empty for KernelMapping"

		return builder
	}

	builder.definition.ContainerImage = image

	return builder
}

// WithBuildArg adds the specified Build Args config to the KernelMapper.
func (builder *KernelMappingBuilder) WithBuildArg(argName, argValue string) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new Module KernelMapping parameter with buildingArgs name: %s, value: %s", argName, argValue)

	if argName == "" {
		klog.V(100).Info("The argName of WithBuildArg is empty")

		builder.errorMsg = "'argName' parameter can not be empty for KernelMapping BuildArg"

		return builder
	}

	if argValue == "" {
		klog.V(100).Info("The argValue of WithBuildArg is empty")

		builder.errorMsg = "'argValue' parameter can not be empty for KernelMapping BuildArg"

		return builder
	}

	builder.addBuild()

	builder.definition.Build.BuildArgs = append(
		builder.definition.Build.BuildArgs, moduleV1Beta1.BuildArg{Name: argName, Value: argValue})

	return builder
}

// WithBuildSecret adds the specified Build Secret config to the KernelMapper.
func (builder *KernelMappingBuilder) WithBuildSecret(secret string) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new Module KernelMapping parameter with BuildSecret %s", secret)

	if secret == "" {
		klog.V(100).Info("The secret of WithBuildSecret is empty")

		builder.errorMsg = "'secret' parameter can not be empty for KernelMapping Secret"

		return builder
	}

	builder.addBuild()

	builder.definition.Build.Secrets = append(
		builder.definition.Build.Secrets, corev1.LocalObjectReference{Name: secret})

	return builder
}

// WithBuildImageRegistryTLS adds the specified ImageRegistryTLS config to the KernelMapper Build.
func (builder *KernelMappingBuilder) WithBuildImageRegistryTLS(insecure, skipTLSVerify bool) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new Module KernelMapping parameter with BuildImageRegistryTLS %t, value: %t",
		insecure, skipTLSVerify)

	builder.addBuild()
	builder.definition.Build.BaseImageRegistryTLS.Insecure = insecure
	builder.definition.Build.BaseImageRegistryTLS.InsecureSkipTLSVerify = skipTLSVerify

	return builder
}

// WithBuildDockerCfgFile adds the specified DockerCfgFil config to the KernelMapper Build.
func (builder *KernelMappingBuilder) WithBuildDockerCfgFile(name string) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating new Module KernelMapping parameter with DockerCfgFile %s, ", name)

	if name == "" {
		klog.V(100).Info("The name of WithBuildDockerCfgFile is empty")

		builder.errorMsg = "'name' parameter can not be empty for KernelMapping Docker file"

		return builder
	}

	builder.addBuild()
	builder.definition.Build.DockerfileConfigMap = &corev1.LocalObjectReference{Name: name}

	return builder
}

// WithSign adds the specified Sign config to the KernelMapper.
func (builder *KernelMappingBuilder) WithSign(certSecret, keySecret string, fileToSign []string) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new Module KernelMapping parameter with Sign. CertSecret: %s, KeySecret: %s, fileToSign: %v",
		certSecret, keySecret, fileToSign)

	if certSecret == "" {
		klog.V(100).Info("The certSecret of WithSign is empty")

		builder.errorMsg = "'certSecret' parameter can not be empty for KernelMapping Sign"

		return builder
	}

	if keySecret == "" {
		klog.V(100).Info("The keySecret of WithSign is empty")

		builder.errorMsg = "'keySecret' parameter can not be empty for KernelMapping Sign"

		return builder
	}

	if len(fileToSign) < 1 {
		klog.V(100).Info("The fileToSign of WithSign is empty")

		builder.errorMsg = "'fileToSign' parameter can not be empty for KernelMapping Sign"

		return builder
	}

	builder.definition.Sign = &moduleV1Beta1.Sign{
		CertSecret:  &corev1.LocalObjectReference{Name: certSecret},
		KeySecret:   &corev1.LocalObjectReference{Name: keySecret},
		FilesToSign: fileToSign,
	}

	return builder
}

// RegistryTLS adds the specified RegistryTLS to the KernelMapper.
func (builder *KernelMappingBuilder) RegistryTLS(insecure, skipTLSVerify bool) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating new Module KernelMapping parameter with RegistryTLS. Insecure: %t, InsecureSkipTLSVerify: %t",
		insecure, skipTLSVerify)

	builder.definition.RegistryTLS = &moduleV1Beta1.TLSOptions{}
	builder.definition.RegistryTLS.Insecure = insecure
	builder.definition.RegistryTLS.InsecureSkipTLSVerify = skipTLSVerify

	return builder
}

// WithInTreeModuleToRemove adds the module to be removed to KernelMapper.
func (builder *KernelMappingBuilder) WithInTreeModuleToRemove(existingModule string) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating new Module KernelMapping with inTreeModuleToRemove: %v", existingModule)

	if existingModule == "" {
		klog.V(100).Info("The 'existingModule' is empty")

		builder.errorMsg = "'existingModule' parameter can not be empty for KernelMapping inTreeModuleToRemove"

		return builder
	}

	builder.definition.InTreeModulesToRemove = []string{existingModule}

	return builder
}

// WithOptions creates KernelMapping with generic mutation options.
func (builder *KernelMappingBuilder) WithOptions(options ...KernelMappingAdditionalOptions) *KernelMappingBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting KernelMapping additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

func (builder *KernelMappingBuilder) addBuild() {
	if builder.definition.Build == nil {
		builder.definition.Build = &moduleV1Beta1.Build{}
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *KernelMappingBuilder) validate() (bool, error) {
	resourceCRD := "KernelMapping"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
