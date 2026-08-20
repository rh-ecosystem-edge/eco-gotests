//go:build unit_test

package client

import (
	"fmt"
	"os"
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	ptpv2alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v2alpha1"
	"sigs.k8s.io/yaml"
)

func TestGnrdFixturesUnmarshal(t *testing.T) {
	var cfg ptpv1.PtpConfig

	data, err := os.ReadFile("testdata/PtpConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("PtpConfig name=%s profiles=%d\n", cfg.Name, len(cfg.Spec.Profile))

	for _, p := range cfg.Spec.Profile {
		var pluginKeys []string
		for k := range p.Plugins {
			pluginKeys = append(pluginKeys, k)
		}

		fmt.Printf("  profile=%s plugins=%v ptpSettings=%v\n", *p.Name, pluginKeys, p.PtpSettings)
	}

	var hwConfig ptpv2alpha1.HardwareConfig

	hwData, err := os.ReadFile("testdata/HardwareConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if err := yaml.Unmarshal(hwData, &hwConfig); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("HardwareConfig name=%s relatedPtpProfileName=%s\n",
		hwConfig.Name, hwConfig.Spec.RelatedPtpProfileName)

	if hwConfig.Spec.Profile.ClockChain == nil {
		t.Fatal("ClockChain is nil")
	}

	for _, s := range hwConfig.Spec.Profile.ClockChain.Structure {
		if s.DPLL.HoldoverParameters == nil {
			continue
		}

		fmt.Printf("  subsystem=%s holdoverParameters=%+v\n", s.Name, *s.DPLL.HoldoverParameters)
	}

	// Sanity: confirm ptp.HardwareConfigBuilder wraps the same Definition shape used elsewhere
	// (e.g. profiles.ParseHardwareConfig), not a divergent local unmarshal shortcut.
	builder := &ptp.HardwareConfigBuilder{}
	builder.SetDefinition(&hwConfig)

	if builder.GetDefinition().Spec.RelatedPtpProfileName != "01-bc-tr" {
		t.Fatalf("unexpected RelatedPtpProfileName: %s", builder.GetDefinition().Spec.RelatedPtpProfileName)
	}
}
