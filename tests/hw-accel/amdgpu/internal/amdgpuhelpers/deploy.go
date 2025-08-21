package amdgpuhelpers

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/openshift-kni/eco-goinfra/pkg/clients"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/nfd/nfdparams"
)

const (
	timeout = 10 * time.Minute
)

// DeployAllOperators deploys NFD, KMM, and AMD GPU operators using the generic installer.
func DeployAllOperators(apiClient *clients.Settings) error {
	glog.V(amdgpuparams.LogLevel).Info("Deploying all operators")

	operators := []string{"nfd", "kmm", "amdgpu"}
	for _, operator := range operators {
		config := getConfigByName(operator, apiClient)
		if config.Namespace == "" {
			return fmt.Errorf("invalid operator name: %s", operator)
		}

		installer := deploy.NewOperatorInstaller(config)
		err := installer.Install()

		if err != nil {
			return fmt.Errorf("failed to install %s operator: %w", operator, err)
		}

		ready, err := installer.IsReady(timeout)
		if err != nil {
			return fmt.Errorf("%s operator readiness check failed: %w", operator, err)
		}

		if !ready {
			return fmt.Errorf("%s operator is not ready", operator)
		}
	}

	glog.V(amdgpuparams.LogLevel).Info("All operators deployed successfully")

	return nil
}

func getConfigByName(operatorName string, apiClient *clients.Settings) deploy.OperatorInstallConfig {
	switch strings.ToLower(operatorName) {
	case "nfd":
		return deploy.OperatorInstallConfig{
			APIClient:              apiClient,
			Namespace:              nfdparams.NFDNamespace,
			OperatorGroupName:      "nfd-operator-group",
			SubscriptionName:       "nfd-subscription",
			PackageName:            "nfd",
			CatalogSource:          "redhat-operators",
			CatalogSourceNamespace: "openshift-marketplace",
			Channel:                "stable",
			TargetNamespaces:       []string{nfdparams.NFDNamespace},
			LogLevel:               glog.Level(amdgpuparams.LogLevel),
		}
	case "kmm":
		return GetDefaultKMMInstallConfig(apiClient, nil)
	case "amdgpu":
		return GetDefaultAMDGPUInstallConfig(apiClient, nil)
	default:
		return deploy.OperatorInstallConfig{}
	}
}
