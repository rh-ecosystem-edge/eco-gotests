package amdgpunfd

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/openshift-kni/eco-goinfra/pkg/clients"
	"github.com/openshift-kni/eco-goinfra/pkg/nfd"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpucommon"
	"github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"
)

// CreateAMDGPUFeatureRule creates an NFD FeatureRule for advanced AMD GPU detection and labeling.
func CreateAMDGPUFeatureRule(apiClient *clients.Settings) error {
	glog.V(amdgpuparams.LogLevel).Infof("Creating NFD FeatureRule for enhanced AMD GPU detection")

	featureRuleBuilder := nfd.NewNodeFeatureRuleBuilderFromObjectString(apiClient, getAMDGPUFeatureRuleYAML())
	if featureRuleBuilder == nil {
		return fmt.Errorf("failed to create NodeFeatureRule builder")
	}

	if featureRuleBuilder.Exists() {
		glog.V(amdgpuparams.LogLevel).Info("AMD GPU FeatureRule already exists")

		return nil
	}

	_, err := featureRuleBuilder.Create()
	if err != nil {
		return handleFeatureRuleCreationError(err)
	}

	glog.V(amdgpuparams.LogLevel).Info("Successfully created AMD GPU FeatureRule")
	glog.V(amdgpuparams.LogLevel).Info("This will enhance AMD GPU node detection and labeling via NFD")

	return nil
}

// getAMDGPUFeatureRuleYAML returns the YAML configuration for AMD GPU NodeFeatureRule.
func getAMDGPUFeatureRuleYAML() string {
	return `
	[
{
  "apiVersion": "nfd.openshift.io/v1alpha1",
  "kind": "NodeFeatureRule",
  "metadata": {
    "name": "amd-gpu-feature-rule",
    "namespace": "openshift-amd-gpu"
  },
  "spec": {
    "rules": [
      {
        "name": "amd.gpu.device",
        "labels": {
          "amd.com/gpu": "true",
          "feature.node.kubernetes.io/amd-gpu": "true"
        },
        "matchFeatures": [
          {
            "feature": "pci.device",
            "matchExpressions": {
              "vendor": {
                "op": "In",
                "value": [
                  "1002"
                ]
              },
              "device": {
                "op": "In",
                "value": [
                  "75a3",
                  "75a0",
                  "74a5",
                  "74a0",
                  "74a1",
                  "74a9",
                  "74bd",
                  "740f",
                  "7408",
                  "740c",
                  "738c",
                  "738e"
                ]
              }
            }
          }
        ]
      },
      {
        "name": "amd.gpu.class",
        "labels": {
          "feature.node.kubernetes.io/amd-gpu-class": "true"
        },
        "matchFeatures": [
          {
            "feature": "pci.device",
            "matchExpressions": {
              "vendor": {
                "op": "In",
                "value": [
                  "1002"
                ]
              },
              "class": {
                "op": "In",
                "value": [
                  "030000",
                  "030200",
                  "038000"
                ]
              }
            }
          }
        ]
      }
    ]
  }
}
]
		`
}

// handleFeatureRuleCreationError handles errors during FeatureRule creation.
func handleFeatureRuleCreationError(err error) error {
	glog.V(amdgpuparams.LogLevel).Infof("Error creating AMD GPU FeatureRule: %v", err)

	if amdgpucommon.IsCRDNotAvailable(err) {
		featureRuleYAML := getAMDGPUFeatureRuleYAML()

		glog.V(amdgpuparams.LogLevel).Info("NFD FeatureRule CRD not available - manual creation required")
		glog.V(amdgpuparams.LogLevel).Infof("AMD GPU FeatureRule YAML:\n%s", featureRuleYAML)

		return fmt.Errorf("NFD FeatureRule CRD not available, manual creation required")
	}

	return err
}
