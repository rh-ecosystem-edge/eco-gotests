package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// GetOCPVersion uses the cluster version on a given cluster to find the latest OCP version, returning the desired
// version if the latest version could not be found.
func GetOCPVersion(client *clients.Settings) (string, error) {
	clusterVersion, err := cluster.GetOCPClusterVersion(client)
	if err != nil {
		return "", err
	}

	// Workaround for an issue in eco-goinfra where builder.Object is nil even when Pull returns a nil error.
	if clusterVersion.Object == nil {
		return "", fmt.Errorf("failed to get ClusterVersion object")
	}

	histories := clusterVersion.Object.Status.History
	for i := len(histories) - 1; i >= 0; i-- {
		if histories[i].State == configv1.CompletedUpdate {
			return histories[i].Version, nil
		}
	}

	klog.V(ranparam.LogLevel).Info("No completed cluster version found in history, returning desired version")

	return clusterVersion.Object.Status.Desired.Version, nil
}

// GetClusterName extracts the cluster name from provided kubeconfig, assuming there's one cluster in the kubeconfig.
func GetClusterName(kubeconfigPath string) (string, error) {
	rawConfig, _ := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		}).RawConfig()

	for _, cluster := range rawConfig.Clusters {
		// Get a cluster name by parsing it from the server hostname. Expects the url to start with
		// `https://api.cluster-name.` so splitting by `.` gives the cluster name.
		splits := strings.Split(cluster.Server, ".")
		clusterName := splits[1]

		klog.V(ranparam.LogLevel).Infof("cluster name %s found for kubeconfig at %s", clusterName, kubeconfigPath)

		return clusterName, nil
	}

	return "", fmt.Errorf("could not get cluster name for kubeconfig at %s", kubeconfigPath)
}

// GetOperatorVersionFromCsv returns operator version from csv, or an empty string if no CSV for the provided operator
// is found.
func GetOperatorVersionFromCsv(client *clients.Settings, operatorName, operatorNamespace string) (string, error) {
	csv, err := olm.ListClusterServiceVersion(client, operatorNamespace)
	if err != nil {
		return "", err
	}

	for _, csv := range csv {
		if strings.Contains(csv.Object.Name, operatorName) {
			return csv.Object.Spec.Version.String(), nil
		}
	}

	return "", fmt.Errorf("could not find version for operator %s in namespace %s", operatorName, operatorNamespace)
}

// GetZTPVersionFromArgoCd is used to fetch the version of the ztp-site-generate init container.
func GetZTPVersionFromArgoCd(client *clients.Settings, name, namespace string) (string, error) {
	containerImage, err := GetZTPSiteGenerateImage(client)
	if err != nil {
		return "", err
	}

	colonSplit := strings.Split(containerImage, ":")
	ztpVersion := colonSplit[len(colonSplit)-1]

	if ztpVersion == "latest" {
		klog.V(ranparam.LogLevel).Info("ztp-site-generate version tag was 'latest', returning empty version")

		return "", nil
	}

	// The format here will be like vX.Y.Z so we need to remove the v at the start.
	return ztpVersion[1:], nil
}

// GetZTPSiteGenerateImage returns the image used for the ztp-site-generate init container. It takes this from the Argo
// CD resource.
func GetZTPSiteGenerateImage(client *clients.Settings) (string, error) {
	gitops, err := argocd.Pull(client, ranparam.OpenshiftGitOpsNamespace, ranparam.OpenshiftGitOpsNamespace)
	if err != nil {
		return "", err
	}

	for _, container := range gitops.Definition.Spec.Repo.InitContainers {
		// Match both the `ztp-site-generator` and `ztp-site-generate` images since which one matches is version
		// dependent.
		if strings.Contains(container.Image, "ztp-site-gen") {
			return container.Image, nil
		}
	}

	return "", errors.New("unable to identify ZTP site generate image")
}

// IsVersionStringInRange reports whether version satisfies minimum <= version < maximumUpper using SemVer 2.0
// (github.com/Masterminds/semver/v3).
//
// minimum: empty means no lower bound. For a lower bound that must include pre-releases of X.Y.0 (e.g. OCP
// 4.20.0-20251212...), pass X.Y.0-0 — the lowest pre-release of X.Y.0. Plain X.Y.0 excludes pre-releases of X.Y.0.
//
// maximum: empty means no upper bound. Otherwise maximum is exclusive: the interval is minimum <= v < maximum
// (half-open). Prefer an explicit exclusive bound such as "4.18.0-0" (all 4.16.z and 4.17.z, not 4.18.0+). As a
// shorthand only, a plain two-segment "X.Y" (no prerelease/build suffix) still means "below the next minor", i.e.
// exclusive upper X.(Y+1).0-0 — same as passing that explicit version.
//
// If version is not a valid semver string and maximum is empty, the function returns (true, nil) for compatibility
// with legacy call sites (e.g. empty or non-semver operator tags with no upper bound). Callers that require a parsed
// version should validate the string separately or pass a non-empty maximum so the result is (false, nil).
func IsVersionStringInRange(version, minimum, maximum string) (bool, error) {
	minV, err := parseMinimumBound(minimum)
	if err != nil {
		return false, err
	}

	maxExclusive, err := parseMaximumExclusiveUpper(maximum)
	if err != nil {
		return false, err
	}

	v, err := semver.NewVersion(trimSemverVPrefix(version))
	if err != nil {
		return maximum == "", nil
	}

	if minV != nil && v.LessThan(minV) {
		return false, nil
	}

	if maxExclusive != nil && !v.LessThan(maxExclusive) {
		return false, nil
	}

	return true, nil
}

func trimSemverVPrefix(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "v")
}

func parseMinimumBound(minimum string) (*semver.Version, error) {
	if minimum == "" {
		return nil, nil
	}

	s := trimSemverVPrefix(minimum)
	coerced, err := coerceSemverCore(s)
	if err != nil {
		return nil, fmt.Errorf("invalid minimum provided: '%s'", minimum)
	}

	parsed, err := semver.NewVersion(coerced)
	if err != nil {
		return nil, fmt.Errorf("invalid minimum provided: '%s'", minimum)
	}

	return parsed, nil
}

// parseMaximumExclusiveUpper returns the first version not allowed: valid versions satisfy v < upperExclusive.
func parseMaximumExclusiveUpper(maximum string) (*semver.Version, error) {
	if maximum == "" {
		return nil, nil
	}

	s := trimSemverVPrefix(maximum)
	core, tail := splitSemverCore(s)
	parts := strings.Split(core, ".")
	for _, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}
	}

	switch len(parts) {
	case 0, 1:
		return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
	case 2:
		if tail != "" {
			return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}

		maj, _ := strconv.ParseUint(parts[0], 10, 64)
		min, _ := strconv.ParseUint(parts[1], 10, 64)

		upper, err := semver.NewVersion(fmt.Sprintf("%d.%d.0-0", maj, min+1))
		if err != nil {
			return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}

		return upper, nil
	default:
		full := core
		if tail != "" {
			full = core + tail
		}

		exclusive, err := semver.NewVersion(full)
		if err != nil {
			return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}

		return exclusive, nil
	}
}

func splitSemverCore(s string) (core, tail string) {
	for i, r := range s {
		if r == '-' || r == '+' {
			return s[:i], s[i:]
		}
	}

	return s, ""
}

func coerceSemverCore(s string) (string, error) {
	core, tail := splitSemverCore(s)
	parts := strings.Split(core, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("need at least major.minor")
	}

	for _, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 64); err != nil {
			return "", err
		}
	}

	for len(parts) < 3 {
		parts = append(parts, "0")
	}

	out := strings.Join(parts, ".")
	if tail != "" {
		out += tail
	}

	return out, nil
}
