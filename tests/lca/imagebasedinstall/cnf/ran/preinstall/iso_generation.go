package preinstall

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"k8s.io/klog/v2"
)

// CreateIBIISO runs openshift-install to create the image-based installation ISO.
func CreateIBIISO(openshiftInstallPath string, workDir string) (string, error) {
	klog.Infof("Creating IBI ISO using %s in %s", openshiftInstallPath, workDir)

	// openshift-install image-based create image --dir <workDir>
	cmd := exec.Command(openshiftInstallPath, "image-based", "create", "image", "--dir", workDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create IBI ISO: %w, output: %s", err, string(output))
	}

	isoPath := filepath.Join(workDir, "rhcos-ibi.iso")

	// Verify the ISO was actually created
	if _, err := os.Stat(isoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("ISO file was not found at expected path %s after successful command execution", isoPath)
	}

	klog.Infof("Successfully created IBI ISO at %s", isoPath)

	return isoPath, nil
}
