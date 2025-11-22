package version

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var inputStringRegex = regexp.MustCompile(`(\d+)\.(\d+)`)

// IsVersionStringInRange checks if a version string is between a specified min and max value, inclusive. All the string
// inputs to this function should contain dot separated positive intergers, e.g. "1.0.0" or "4.10". Each string input
// must contain at least two dot separarted integers but may also contain more, though only the first two are compared.
// Digits are compared per position, so 4.Y is not less than 5.0 if Y > 0.
func IsVersionStringInRange(version, minimum, maximum string) (bool, error) {
	versionDigits := getInputIntegers(version)
	minimumDigits := getInputIntegers(minimum)
	maximumDigits := getInputIntegers(maximum)

	minInvalid := minimumDigits == nil
	if minInvalid {
		// Only accept invalid empty strings
		if minimum != "" {
			return false, fmt.Errorf("invalid minimum provided: '%s'", minimum)
		}

		// Assume the minimum digits are [0,0] for later comparison
		minimumDigits = []int{0, 0}
	}

	maxInvalid := maximumDigits == nil
	if maxInvalid {
		// Only accept invalid empty strings
		if maximum != "" {
			return false, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}

		// Assume the maximum digits are [math.MaxInt, math.MaxInt] for later comparison
		maximumDigits = []int{math.MaxInt, math.MaxInt}
	}

	// If the version was not valid then we return whether the maximum is empty.
	if versionDigits == nil {
		return maximum == "", nil
	}

	// Otherwise the versions were valid so compare the digits
	for i := 0; i < 2; i++ {
		// The version digit should be between the minimum and maximum, inclusive.
		if versionDigits[i] < minimumDigits[i] || versionDigits[i] > maximumDigits[i] {
			return false, nil
		}
	}

	// At the end if we never returned then all the digits were in valid range
	return true, nil
}

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

// getInputIntegers returns the first two dot-separated integers in the input string. A nil return value indicates a
// malformed input string.
func getInputIntegers(input string) []int {
	digits := inputStringRegex.FindStringSubmatch(input)
	if digits == nil {
		return nil
	}

	var integers []int

	for _, digit := range digits[1:] {
		integer, err := strconv.Atoi(digit)
		if err != nil {
			// Since we have already validated these are digits
			return nil
		}

		integers = append(integers, integer)
	}

	return integers
}
