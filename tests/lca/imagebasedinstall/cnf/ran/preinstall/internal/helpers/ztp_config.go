package helpers

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"k8s.io/klog/v2"
)

// CloneZTPSiteConfigRepo clones the ZTP site config repository with go-git (depth 1, single branch).
func CloneZTPSiteConfigRepo(repoURL, branch, destDir string, insecureSkipTLS bool) error {
	klog.Infof("Cloning ZTP site config repo %s (branch %s) to %s", repoURL, branch, destDir)

	if _, err := os.Stat(destDir); err == nil {
		err = os.RemoveAll(destDir)
		if err != nil {
			return fmt.Errorf("failed to remove existing directory %s: %w", destDir, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if directory %s exists: %w", destDir, err)
	}

	_, err := git.PlainClone(destDir, false, &git.CloneOptions{
		URL:             repoURL,
		Tags:            git.NoTags,
		ReferenceName:   plumbing.NewBranchReferenceName(branch),
		Depth:           1,
		SingleBranch:    true,
		InsecureSkipTLS: insecureSkipTLS,
	})
	if err != nil {
		return fmt.Errorf("git clone: %w", err)
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
