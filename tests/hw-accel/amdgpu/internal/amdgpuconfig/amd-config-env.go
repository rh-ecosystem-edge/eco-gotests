package amdgpuconfig

import (
	"log"
	"strconv"

	"k8s.io/klog/v2"

	"github.com/kelseyhightower/envconfig"
	amdgpuparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
)

// amdGPUConfigHelper Helps to convert different strings to bool.
type amdGPUConfigHelper struct {
	SkipCleanup        string `envconfig:"ECO_HWACCEL_AMD_SKIP_CLEANUP" split_words:"true" default:"false"`
	SkipCleanupOnError string `envconfig:"ECO_HWACCEL_AMD_SKIP_CLEANUP_ON_ERROR" split_words:"true" default:"false"`
}

// AMDConfig contains environment information related to amd tests.
type AMDConfig struct {
	AMDDriverVersion   string `envconfig:"ECO_HWACCEL_AMD_DRIVER_VERSION"`
	AMDOperatorVersion string `envconfig:"ECO_HWACCEL_AMD_OPERATOR_VERSION"`

	SkipCleanup        bool
	SkipCleanupOnError bool
}

// NewAMDConfig returns instance of AMDConfig type.
func NewAMDConfig() *AMDConfig {
	log.Print("Creating new AMDConfig")

	AMDConfig := new(AMDConfig)

	err := envconfig.Process("eco_hwaccel_amd_", AMDConfig)
	if err != nil {
		log.Printf("failed to instantiate AMDConfig: %v", err)

		return nil
	}

	var configHelper amdGPUConfigHelper

	configHelperErr := envconfig.Process("eco_hwaccel_amd_", &configHelper)
	if configHelperErr != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof(
			"failed to process env vars for amdConfigHelper fields: %v", configHelperErr)

		return nil
	}

	var SkipCleanupConvErr error

	AMDConfig.SkipCleanup, SkipCleanupConvErr = strconv.ParseBool(configHelper.SkipCleanup)
	if SkipCleanupConvErr != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof(
			"failed to convert the string value of 'configHelper.SkipCleanup' "+
				"(type: '%T', value:'%v') to bool: %v",
			configHelper.SkipCleanup, configHelper.SkipCleanup, SkipCleanupConvErr)

		return nil
	}

	var SkipCleanupOnErrConvErr error

	AMDConfig.SkipCleanupOnError, SkipCleanupOnErrConvErr = strconv.ParseBool(configHelper.SkipCleanupOnError)
	if SkipCleanupOnErrConvErr != nil {
		klog.V(amdgpuparams.AMDGPULogLevel).Infof(
			"failed to convert the string value of 'configHelper.SkipCleanupOnError' "+
				"(type: '%T', value:'%v') to bool: %v",
			configHelper.SkipCleanupOnError, configHelper.SkipCleanupOnError, SkipCleanupOnErrConvErr)

		return nil
	}

	return AMDConfig
}
