package helpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"k8s.io/klog/v2"
)

// validatedCloneDestDir returns filepath.Clean(destDir) after rejecting paths that must not be passed to RemoveAll.
func validatedCloneDestDir(destDir string) (cleaned string, err error) {
	cleaned = filepath.Clean(destDir)

	switch cleaned {
	case "", ".", "..":
		return "", fmt.Errorf("refusing to remove unsafe clone destination %q", destDir)
	case "/", `\`:
		return "", fmt.Errorf("refusing to remove filesystem root (%q)", destDir)
	}

	if runtime.GOOS == "windows" {
		v := filepath.VolumeName(cleaned)
		if v != "" {
			afterVol := filepath.ToSlash(cleaned[len(v):])
			if afterVol == "" || afterVol == "/" || afterVol == "/." {
				return "", fmt.Errorf("refusing to remove drive or volume root (%q)", destDir)
			}
		}
	}

	return cleaned, nil
}

// CloneZTPSiteConfigRepo clones the ZTP site config repository with go-git (depth 1, single branch).
func CloneZTPSiteConfigRepo(repoURL, branch, destDir string, insecureSkipTLS bool) error {
	klog.Infof("Cloning ZTP site config repo %s (branch %s) to %s", repoURL, branch, destDir)

	cleaned, err := validatedCloneDestDir(destDir)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(cleaned); statErr == nil {
		if rmErr := os.RemoveAll(cleaned); rmErr != nil {
			return fmt.Errorf("failed to remove existing directory %s: %w", cleaned, rmErr)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("failed to check if directory %s exists: %w", cleaned, statErr)
	}

	_, err = git.PlainClone(cleaned, false, &git.CloneOptions{
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kustomize", "build", siteConfigDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run kustomize: %w, output: %s", err, string(output))
	}

	return output, nil
}
