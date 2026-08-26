package define

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
)

// DRAModuleSpec builds a ModuleSpec with DRA configuration using the builder pattern.
func DRAModuleSpec(apiClient *clients.Settings, name, namespace, image,
	kmodName, serviceAccountName string, nodeSelector map[string]string,
	deviceClassNames []string) (moduleV1Beta1.ModuleSpec, error) {
	kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
		WithContainerImage(image).
		BuildKernelMappingConfig()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	moduleLoaderCfg, err := kmm.NewModLoaderContainerBuilder(kmodName).
		WithKernelMapping(kernelMapping).
		BuildModuleLoaderContainerCfg()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	draCfg, err := kmm.NewDRAContainerBuilder(kmmparams.DRADriverImage).
		WithCommand([]string{"dra-example-kubeletplugin"}).
		GetDRAContainerConfig()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	builder := kmm.NewModuleBuilder(apiClient, name, namespace).
		WithNodeSelector(nodeSelector).
		WithModuleLoaderContainer(moduleLoaderCfg).
		WithDRAContainer(draCfg).
		WithDRADriverName(kmmparams.DRADriverName).
		WithDRAServiceAccount(serviceAccountName)

	for _, dc := range deviceClassNames {
		builder = builder.WithDRADeviceClass(moduleV1Beta1.DeviceClassSpec{Name: dc})
	}

	return builder.BuildModuleSpec()
}

// DRAAndDevicePluginModuleSpec builds a ModuleSpec with both DRA and DevicePlugin
// configured, which should be rejected by the webhook.
func DRAAndDevicePluginModuleSpec(apiClient *clients.Settings, name, namespace, image,
	kmodName, serviceAccountName, devicePluginImage string,
	nodeSelector map[string]string) (moduleV1Beta1.ModuleSpec, error) {
	kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
		WithContainerImage(image).
		BuildKernelMappingConfig()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	moduleLoaderCfg, err := kmm.NewModLoaderContainerBuilder(kmodName).
		WithKernelMapping(kernelMapping).
		BuildModuleLoaderContainerCfg()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	draCfg, err := kmm.NewDRAContainerBuilder(kmmparams.DRADriverImage).
		WithCommand([]string{"dra-example-kubeletplugin"}).
		GetDRAContainerConfig()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	dpCfg, err := kmm.NewDevicePluginContainerBuilder(devicePluginImage).
		GetDevicePluginContainerConfig()
	if err != nil {
		return moduleV1Beta1.ModuleSpec{}, err
	}

	return kmm.NewModuleBuilder(apiClient, name, namespace).
		WithNodeSelector(nodeSelector).
		WithModuleLoaderContainer(moduleLoaderCfg).
		WithDRAContainer(draCfg).
		WithDRADriverName(kmmparams.DRADriverName).
		WithDRAServiceAccount(serviceAccountName).
		WithDevicePluginContainer(dpCfg).
		BuildModuleSpec()
}
