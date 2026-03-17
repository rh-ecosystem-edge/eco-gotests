package preinstall

import (
	"fmt"
	"os"
	"os/exec"

	"k8s.io/klog/v2"
)

// CloneZTPSiteConfigRepo clones the ZTP site config repository.
func CloneZTPSiteConfigRepo(repoURL, branch, destDir string) error {
	klog.Infof("Cloning ZTP site config repo %s (branch %s) to %s", repoURL, branch, destDir)

	// Clean up destination directory if it exists
	if _, err := os.Stat(destDir); err == nil {
		err = os.RemoveAll(destDir)
		if err != nil {
			return fmt.Errorf("failed to remove existing directory %s: %w", destDir, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if directory %s exists: %w", destDir, err)
	}

	// We use git command line directly as go-git might have issues with some internal auth setups
	// and to closely match the ansible git module behavior
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", branch, "--single-branch", repoURL, destDir)

	// If we need to skip TLS verify (like in deploymenttypes)
	cmd.Env = append(os.Environ(), "GIT_SSL_NO_VERIFY=true")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to clone repo: %w, output: %s", err, string(output))
	}

	klog.Infof("Successfully cloned ZTP site config repo")

	return nil
}

// RunKustomize runs kustomize build on the specified directory.
func RunKustomize(siteConfigDir string) ([]byte, error) {
	klog.Infof("Running kustomize build on %s", siteConfigDir)

	cmd := exec.Command("kustomize", "build", siteConfigDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run kustomize: %w, output: %s", err, string(output))
	}

	return output, nil
}
