package helpers

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/internal/tsparams"
)

// CloneZTPSiteConfigRepo clones the ZTP site config repository with go-git (depth 1, single branch).
func CloneZTPSiteConfigRepo(repoURL, branch, destDir string, insecureSkipTLS bool) error {
	destDir = filepath.Clean(destDir)
	klog.V(tsparams.LogLevel).Infof("Cloning ZTP site config repo %s (branch %s) to %s", repoURL, branch, destDir)

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

	klog.V(tsparams.LogLevel).Infof("Successfully cloned ZTP site config repo")

	return nil
}

// RunKustomize runs kustomize build on the specified directory.
func RunKustomize(siteConfigDir string) ([]byte, error) {
	klog.V(tsparams.LogLevel).Infof("Running kustomize build on %s", siteConfigDir)

	ctx, cancel := context.WithTimeout(context.TODO(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kustomize", "build", siteConfigDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run kustomize: %w, output: %s", err, string(output))
	}

	return output, nil
}
