package version

import (
	"errors"
	"fmt"
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
	if len(ztpVersion) < 2 || ztpVersion[0] != 'v' {
		return "", fmt.Errorf("unexpected ztp-site-generate version tag %q", ztpVersion)
	}

	return ztpVersion[1:], nil
}

// GetZTPSiteGenerateImage returns the image used for the ztp-site-generate init container. It takes this from the Argo
// CD resource.
func GetZTPSiteGenerateImage(client *clients.Settings) (string, error) {
	gitops, err := argocd.Pull(client, ranparam.OpenshiftGitOpsNamespace, ranparam.OpenshiftGitOpsNamespace)
	if err != nil {
		return "", err
	}

	// argocd.Pull can return a nil Definition when Get fails with a non-NotFound error (Exists treats that as
	// present). Guard so hub config init logs an error instead of panicking.
	if gitops == nil || gitops.Definition == nil {
		return "", errors.New("argocd object is nil after pull; cannot read ztp-site-generate image")
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

// IsVersionStringInRange reports whether version satisfies minimum <= version < maximum using SemVer 2.0
// (github.com/Masterminds/semver/v3).
//
// minimum: empty means no lower bound. For a lower bound that must include pre-releases of X.Y.0 (e.g. OCP
// 4.20.0-20251212...), pass X.Y.0-0 — the lowest pre-release of X.Y.0. Plain X.Y.0 excludes pre-releases of X.Y.0.
//
// maximum: empty means no upper bound. Otherwise maximum is exclusive: the interval is minimum <= v < maximum
// (half-open). Non-empty bounds use the same rules as version: trim an optional leading v, then parse with
// github.com/Masterminds/semver/v3. Callers must pass semver-compatible strings (e.g. lower bound "4.16.0-0",
// exclusive upper "4.21.0-0").
//
// Empty or whitespace-only version is an explicit error (false, error) fail-closed. This prevents unknown
// versions (e.g. empty Spoke1OCPVersion when GetOCPVersion fails) from being considered supported when maximum==""
// legacy no-upper-bound gates. Empty minimum/maximum still mean unbounded — only the value under test (version)
// is fail-closed on empty. Callers should treat the error as an explicit failure signal.
//
// Any other unparsable non-empty version string is treated as out-of-range (false, nil) for backward
// compatibility with legacy call sites that pass non-semver operator tags. Empty version is the only
// value-under-test case that returns an error.
func IsVersionStringInRange(version, minimum, maximum string) (bool, error) {
	var minV *semver.Version

	if minimum != "" {
		parsed, err := semver.NewVersion(trimSemverVPrefix(minimum))
		if err != nil {
			return false, fmt.Errorf("invalid minimum provided: '%s'", minimum)
		}

		minV = parsed
	}

	var maxExclusive *semver.Version

	if maximum != "" {
		parsed, err := semver.NewVersion(trimSemverVPrefix(maximum))
		if err != nil {
			return false, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}

		maxExclusive = parsed
	}

	if strings.TrimSpace(version) == "" {
		return false, fmt.Errorf("version string is empty")
	}

	parsedVersion, err := semver.NewVersion(trimSemverVPrefix(version))
	if err != nil {
		klog.V(ranparam.LogLevel).Infof(
			"IsVersionStringInRange: unparsable version %q treated as out-of-range (false, nil): %v", version, err)

		return false, nil
	}

	if minV != nil && parsedVersion.LessThan(minV) {
		return false, nil
	}

	if maxExclusive != nil && !parsedVersion.LessThan(maxExclusive) {
		return false, nil
	}

	return true, nil
}

func trimSemverVPrefix(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "v")
}
