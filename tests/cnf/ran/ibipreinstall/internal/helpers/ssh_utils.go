package helpers

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
)

const sshSubprocessTimeout = 3 * time.Minute

// SSHExec executes a command on a remote host via SSH.
func SSHExec(parentCtx context.Context, host, user, sshKeyPath, command string) (string, error) {
	klog.V(tsparams.LogLevel).Infof("Executing on %s@%s: %s", user, host, command)

	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		"-i", sshKeyPath,
		fmt.Sprintf("%s@%s", user, host),
		command,
	}

	ctx, cancel := context.WithTimeout(parentCtx, sshSubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return string(output), fmt.Errorf(
				"ssh timed out after %v: %w, output: %s", sshSubprocessTimeout, err, string(output))
		}

		return string(output), fmt.Errorf("ssh failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}
