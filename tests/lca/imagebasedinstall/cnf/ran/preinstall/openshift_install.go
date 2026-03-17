package preinstall

import (
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"
)

// ExtractOpenshiftInstall extracts the openshift-install binary from the release image.
func ExtractOpenshiftInstall(releaseImage string, destDir string) error {
	klog.Infof("Extracting openshift-install from %s to %s", releaseImage, destDir)

	// oc adm release extract --command=openshift-install --to=<destDir> <releaseImage>
	cmd := exec.Command("oc", "adm", "release", "extract",
		"--command=openshift-install",
		fmt.Sprintf("--to=%s", destDir),
		releaseImage)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to extract openshift-install: %w, output: %s", err, string(output))
	}

	klog.Infof("Successfully extracted openshift-install")

	return nil
}

// GetOpenshiftInstallVersion returns the version of the openshift-install binary.
func GetOpenshiftInstallVersion(binaryPath string) (string, error) {
	cmd := exec.Command(binaryPath, "version")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get openshift-install version: %w", err)
	}

	// Parse output like: openshift-install 4.16.7
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "openshift-install ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}

	return "", fmt.Errorf("could not parse version from output: %s", string(output))
}
