package helpers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2/types"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"k8s.io/klog/v2"
)

const diagnosticsSSHTimeout = 5 * time.Minute

// spokeJournalUnits lists the systemd units whose full journals are collected
// from the spoke node on test failure.
var spokeJournalUnits = []string{
	tsparams.PreinstallServiceUnit,
	"precache.service",
	"set-ip-address.service",
}

// CollectDiagnosticsIfFailed collects IBI preinstall diagnostics when a test
// fails. Hub-side cluster resources (IDMS, pods, events) are collected by the
// k8sreporter via ReportIfFailed. This function adds spoke-specific artifacts
// that the reporter cannot reach: systemd journals, dmesg, and the IBI config.
func CollectDiagnosticsIfFailed(
	report types.SpecReport,
	testSuite string,
	spokeHost, workDir string,
) {
	if !report.State.Is(types.SpecStateFailureStates) {
		return
	}

	dumpDir := RANConfig.GetDumpFailedTestReportLocation(testSuite)
	if dumpDir == "" {
		klog.V(tsparams.LogLevel).Info("No dump directory configured, skipping IBI diagnostics")

		return
	}

	diagDir := filepath.Join(dumpDir, "ibi-preinstall-diagnostics")
	if err := os.MkdirAll(diagDir, 0o755); err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to create diagnostics dir %s: %v", diagDir, err)

		return
	}

	if spokeHost != "" {
		sshKeyPath := RANConfig.IBIPreinstallConfig.PreinstallSSHKey
		collectSpokeJournals(spokeHost, tsparams.TargetNodeSSHUser, sshKeyPath, diagDir)
	}

	if workDir != "" {
		preserveArtifact(filepath.Join(workDir, ".openshift_install.log"), diagDir)
		preserveArtifact(filepath.Join(workDir, RedactedIBIConfigFilename), diagDir)
	}
}

// collectSpokeJournals retrieves systemd journal logs and dmesg from the spoke node via SSH.
func collectSpokeJournals(host, user, sshKeyPath, destDir string) {
	ctx, cancel := context.WithTimeout(context.TODO(), diagnosticsSSHTimeout)
	defer cancel()

	for _, unit := range spokeJournalUnits {
		cmd := fmt.Sprintf("journalctl -u %s --no-pager 2>&1 || true", unit)

		output, err := SSHExec(ctx, host, user, sshKeyPath, cmd)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof(
				"Could not collect journal for %s from %s: %v", unit, host, err)

			continue
		}

		dest := filepath.Join(destDir, "spoke_journal_"+unit+".log")
		if writeErr := os.WriteFile(dest, []byte(output), 0o644); writeErr != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to write journal to %s: %v", dest, writeErr)
		}
	}

	output, err := SSHExec(ctx, host, user, sshKeyPath, "dmesg --time-format iso 2>&1 | tail -500 || true")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Could not collect dmesg from %s: %v", host, err)
	} else {
		dest := filepath.Join(destDir, "spoke_dmesg.log")
		if writeErr := os.WriteFile(dest, []byte(output), 0o644); writeErr != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to write dmesg to %s: %v", dest, writeErr)
		}
	}
}

// preserveArtifact copies a single file from srcPath into destDir for post-failure inspection.
func preserveArtifact(srcPath, destDir string) {
	src, err := os.Open(srcPath)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Artifact %s not available: %v", srcPath, err)

		return
	}

	defer src.Close()

	destPath := filepath.Join(destDir, filepath.Base(srcPath))

	dst, err := os.Create(destPath)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to create %s: %v", destPath, err)

		return
	}

	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to copy artifact to %s: %v", destPath, err)
	}
}
