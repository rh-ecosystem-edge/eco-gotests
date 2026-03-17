package preinstall

import (
	"fmt"
	"os/exec"

	"k8s.io/klog/v2"
)

// SCPToProvisioningHost copies a file to the provisioning host using scp.
func SCPToProvisioningHost(srcPath, destPath, host, user, sshKeyPath string) error {
	klog.Infof("Copying %s to %s@%s:%s", srcPath, user, host, destPath)

	// scp -i <sshKeyPath> -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <srcPath> <user>@<host>:<destPath>
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}

	if sshKeyPath != "" {
		args = append(args, "-i", sshKeyPath)
	}

	args = append(args, srcPath, fmt.Sprintf("%s@%s:%s", user, host, destPath))

	cmd := exec.Command("scp", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to scp file: %w, output: %s", err, string(output))
	}

	klog.Infof("Successfully copied file to provisioning host")

	return nil
}

// SSHExecOnProvisioningHost executes a command on the provisioning host via SSH.
func SSHExecOnProvisioningHost(host, user, sshKeyPath, command string) (string, error) {
	klog.Infof("Executing command on %s@%s: %s", user, host, command)

	// ssh -i <sshKeyPath> -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null <user>@<host> '<command>'
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}

	if sshKeyPath != "" {
		args = append(args, "-i", sshKeyPath)
	}

	args = append(args, fmt.Sprintf("%s@%s", user, host), command)

	cmd := exec.Command("ssh", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to execute ssh command: %w, output: %s", err, string(output))
	}

	return string(output), nil
}
