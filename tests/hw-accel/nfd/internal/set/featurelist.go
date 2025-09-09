package set

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"gopkg.in/yaml.v2"
)

// Config worker-config.
type Config struct {
	Sources Sources `yaml:"sources"`
}

// CPUConfig cpu feature config.
type CPUConfig struct {
	CPUID struct {
		AttributeBlacklist []string `yaml:"attributeBlacklist,omitempty"`
		AttributeWhitelist []string `yaml:"attributeWhitelist,omitempty"`
	} `yaml:"cpuid,omitempty"`
}

// PCIDevice pci config.
type PCIDevice struct {
	DeviceClassWhitelist []string `yaml:"deviceClassWhitelist,omitempty"`
	DeviceLabelFields    []string `yaml:"deviceLabelFields,omitempty"`
}

// Sources contains all sources.
type Sources struct {
	CPU    *CPUConfig    `yaml:"cpu,omitempty"`
	PCI    []PCIDevice   `yaml:"pci,omitempty"`
	USB    []interface{} `yaml:"usb,omitempty"`
	Custom []interface{} `yaml:"custom,omitempty"`
}

// CPUConfigLabels set cpu blacklist/whitelist using the new NFD CR utilities.
func CPUConfigLabels(apiClient *clients.Settings,
	blackListLabels,
	whiteListLabels []string,
	enableTopology bool,
	namespace,
	image string) {
	var cfg Config

	if cfg.Sources.CPU == nil {
		cfg.Sources.CPU = &CPUConfig{}
	}

	cfg.Sources.CPU.CPUID.AttributeBlacklist = append(cfg.Sources.CPU.CPUID.AttributeBlacklist, blackListLabels...)
	cfg.Sources.CPU.CPUID.AttributeWhitelist = append(cfg.Sources.CPU.CPUID.AttributeWhitelist, whiteListLabels...)

	modifiedCPUYAML, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	nfdCRUtils := deploy.NewNFDCRUtils(apiClient, namespace, nfdparams.NfdInstance)

	nfdConfig := deploy.NFDCRConfig{

		EnableTopology: enableTopology,
		Image:          image,
	}

	err = nfdCRUtils.DeployNFDCRWithWorkerConfig(nfdConfig, string(modifiedCPUYAML))
	if err != nil {
		panic(err)
	}
}
