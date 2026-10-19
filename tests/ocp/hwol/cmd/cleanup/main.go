// Command cleanup removes and verifies HWOL suite resources after a test run.
//
// It is deliberately separate from Ginkgo so CI can invoke it from an
// unconditional post step when a Ginkgo worker or launcher is interrupted.
package main

import (
	"fmt"
	"os"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/hwolenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
)

func main() {
	if err := cleanup(); err != nil {
		fmt.Fprintf(os.Stderr, "HWOL post-run cleanup failed: %v\n", err)
		os.Exit(1)
	}
}

func cleanup() error {
	if HwolOcpConfig == nil {
		return fmt.Errorf("HWOL config initialization failed")
	}

	devices, err := HwolOcpConfig.GetHwolDevices()
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		return fmt.Errorf("no HWOL devices configured")
	}

	// Re-run the read-only ownership checks immediately before cleanup. Do not
	// delete fixed-name objects if an administrator has claimed the PF or MCP
	// since the test started.
	if err := hwolenv.ValidateHwolPreconditions(
		HwolOcpConfig.OcpHwolOperatorNamespace, HwolOcpConfig.MCPLabel, devices[0],
	); err != nil {
		return fmt.Errorf("refusing post-run cleanup after preflight revalidation: %w", err)
	}

	testNamespace := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
	if err := testNamespace.DeleteAndWait(tsparams.DefaultTimeout); err != nil {
		return fmt.Errorf("failed to delete HWOL test namespace: %w", err)
	}

	if err := hwolenv.CleanupHwolResources(
		HwolOcpConfig.OcpHwolOperatorNamespace,
		HwolOcpConfig.MCPLabel,
		devices[0].InterfaceName,
		tsparams.CleanupWaitTimeout,
		tsparams.DefaultStableDuration,
	); err != nil {
		if HwolOcpConfig.RecoverOnCleanupFailure {
			fmt.Fprintf(os.Stderr, "HWOL post-run cleanup failed; attempting opt-in reboot recovery: %v\n", err)
			recoveryErr := hwolenv.RecoverHwolCleanup(
				HwolOcpConfig.OcpHwolOperatorNamespace,
				HwolOcpConfig.MCPLabel,
				devices[0].InterfaceName,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultStableDuration,
			)
			if recoveryErr != nil {
				return fmt.Errorf("HWOL cleanup failed: %w; reboot recovery also failed: %v", err, recoveryErr)
			}
			return fmt.Errorf("HWOL cleanup failed: %w; reboot recovery succeeded", err)
		}
		return err
	}

	return nil
}
