package helpers

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/internal/tsparams"
)

const releaseCLISubprocessTimeout = 2 * time.Minute

// ExtractOpenshiftInstall extracts openshift-install and oc from the release image into destDir using bootstrapOC.
// parentCtx bounds how long the oc adm release extract subprocess may run (2m internal timeout).
//
// hubKubeconfig should be the mounted hub kubeconfig path (e.g. /clusterconfigs/auth/kubeconfig) so the extract
// subprocess does not use the container default spoke KUBECONFIG.
//
// registryConfigPath, when non-empty, is passed to oc as --registry-config (typically a file containing the hub
// pull-secret .dockerconfigjson body) so `oc adm release extract` can authenticate to mirrored registries.
func ExtractOpenshiftInstall(
	parentCtx context.Context,
	releaseImage, destDir, hubKubeconfig, registryConfigPath, bootstrapOC string,
) error {
	if strings.TrimSpace(bootstrapOC) == "" {
		return fmt.Errorf("bootstrap oc path is empty")
	}

	klog.V(tsparams.LogLevel).Infof(
		"Extracting openshift-install and oc from %s to %s using bootstrap %s", releaseImage, destDir, bootstrapOC)

	args := []string{
		"adm", "release", "extract",
		"--command=openshift-install",
		"--command=oc",
		fmt.Sprintf("--to=%s", destDir),
	}

	if hubKubeconfig != "" {
		args = append(args, "--kubeconfig="+hubKubeconfig)
	}

	if registryConfigPath != "" {
		args = append(args, "--registry-config="+registryConfigPath)
	}

	args = append(args, releaseImage)

	ctx, cancel := context.WithTimeout(parentCtx, releaseCLISubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bootstrapOC, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf(
				"release extract timed out after %v: %w, output: %s",
				releaseCLISubprocessTimeout, err, string(output))
		}

		return fmt.Errorf("failed to extract release CLI tools: %w, output: %s", err, string(output))
	}

	klog.V(tsparams.LogLevel).Infof("Successfully extracted openshift-install and oc")

	return nil
}

// GetOpenshiftInstallVersion returns the version of the openshift-install binary.
func GetOpenshiftInstallVersion(parentCtx context.Context, binaryPath string) (string, error) {
	ctx, cancel := context.WithTimeout(parentCtx, releaseCLISubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "version")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf(
				"openshift-install version timed out after %v: %w",
				releaseCLISubprocessTimeout, err)
		}

		return "", fmt.Errorf("failed to get openshift-install version: %w", err)
	}

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
