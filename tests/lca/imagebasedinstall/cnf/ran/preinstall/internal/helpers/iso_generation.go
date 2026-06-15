package helpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"k8s.io/klog/v2"
)

// CreateIBIISO runs openshift-install to create the image-based installation ISO.
// ctx should include a deadline (e.g. via context.WithTimeout) so the subprocess is killed if it hangs.
func CreateIBIISO(ctx context.Context, openshiftInstallPath string, workDir string) (string, error) {
	klog.Infof("Creating IBI ISO using %s in %s", openshiftInstallPath, workDir)

	cmd := exec.CommandContext(ctx, openshiftInstallPath, "image-based", "create", "image", "--dir", workDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create IBI ISO: %w, output: %s", err, string(output))
	}

	isoPath := filepath.Join(workDir, "rhcos-ibi.iso")

	if _, statErr := os.Stat(isoPath); statErr != nil {
		return "", fmt.Errorf("ISO output path %s: %w", isoPath, statErr)
	}

	klog.Infof("Successfully created IBI ISO at %s", isoPath)

	return isoPath, nil
}
