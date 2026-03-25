package helpers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const releaseCLISubprocessTimeout = 2 * time.Minute

// withExtractKubeconfig returns env with a single KUBECONFIG entry pointing at hubKubeconfig when non-empty;
// otherwise returns env unchanged (e.g. container default spoke kubeconfig).
func withExtractKubeconfig(env []string, hubKubeconfig string) []string {
	if hubKubeconfig == "" {
		return env
	}

	out := slices.DeleteFunc(slices.Clone(env), func(s string) bool {
		return strings.HasPrefix(s, "KUBECONFIG=")
	})

	return append(out, "KUBECONFIG="+hubKubeconfig)
}

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

	klog.Infof("Extracting openshift-install and oc from %s to %s using bootstrap %s", releaseImage, destDir, bootstrapOC)

	args := []string{
		"adm", "release", "extract",
		"--command=openshift-install",
		"--command=oc",
		fmt.Sprintf("--to=%s", destDir),
	}

	if registryConfigPath != "" {
		args = append(args, "--registry-config="+registryConfigPath)
	}

	args = append(args, releaseImage)

	ctx, cancel := context.WithTimeout(parentCtx, releaseCLISubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bootstrapOC, args...)
	cmd.Env = withExtractKubeconfig(os.Environ(), hubKubeconfig)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf(
				"release extract timed out after %v: %w, output: %s",
				releaseCLISubprocessTimeout, err, string(output))
		}

		return fmt.Errorf("failed to extract release CLI tools: %w, output: %s", err, string(output))
	}

	klog.Infof("Successfully extracted openshift-install and oc")

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
