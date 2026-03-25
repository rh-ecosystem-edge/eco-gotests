package helpers

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"k8s.io/klog/v2"
)

const (
	// scpSubprocessTimeout bounds how long a file copy to the provisioning host may run (e.g. large ISO).
	scpSubprocessTimeout = 30 * time.Minute
	// sshSubprocessTimeout bounds each remote ssh invocation (e.g. journalctl tail in a wait loop).
	sshSubprocessTimeout = 3 * time.Minute
)

// SCPToProvisioningHost copies a file to the provisioning host using scp.
// parentCtx is used with an internal deadline (see scpSubprocessTimeout) so the transfer cannot hang indefinitely.
func SCPToProvisioningHost(parentCtx context.Context, srcPath, destPath, host, user, sshKeyPath string) error {
	klog.Infof("Copying %s to %s@%s:%s", srcPath, user, host, destPath)

	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}

	if sshKeyPath != "" {
		args = append(args, "-i", sshKeyPath)
	}

	args = append(args, srcPath, fmt.Sprintf("%s@%s:%s", user, host, destPath))

	ctx, cancel := context.WithTimeout(parentCtx, scpSubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "scp", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("scp timed out after %v: %w, output: %s", scpSubprocessTimeout, err, string(output))
		}

		return fmt.Errorf("failed to scp file: %w, output: %s", err, string(output))
	}

	klog.Infof("Successfully copied file to provisioning host")

	return nil
}

// SSHExecOnProvisioningHost executes a command on the provisioning host via SSH.
// parentCtx is used with an internal deadline (see sshSubprocessTimeout) so the session cannot hang.
func SSHExecOnProvisioningHost(parentCtx context.Context, host, user, sshKeyPath, command string) (string, error) {
	klog.Infof("Executing command on %s@%s: %s", user, host, command)

	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}

	if sshKeyPath != "" {
		args = append(args, "-i", sshKeyPath)
	}

	args = append(args, fmt.Sprintf("%s@%s", user, host), command)

	ctx, cancel := context.WithTimeout(parentCtx, sshSubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return string(output), fmt.Errorf(
				"ssh command timed out after %v: %w, output: %s",
				sshSubprocessTimeout, err, string(output))
		}

		return string(output), fmt.Errorf("failed to execute ssh command: %w, output: %s", err, string(output))
	}

	return string(output), nil
}
