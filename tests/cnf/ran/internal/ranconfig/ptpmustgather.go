package ranconfig

import (
	"fmt"
	"regexp"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"k8s.io/klog/v2"
)

const ptpMustGatherAnnotation = "operators.openshift.io/must-gather-image"

// majorMinorVersionRegex matches the major and minor portion of a PTP operator version such as "4.22.3".
var majorMinorVersionRegex = regexp.MustCompile(`(\d+)\.(\d+)`)

// ResolvePTPMustGatherImage returns the configured PTP must-gather image or discovers one from the cluster and PTP
// operator version.
func (ranconfig *RANConfig) ResolvePTPMustGatherImage(client *clients.Settings) (string, error) {
	if ranconfig.PTPMustGatherImage != "" {
		return ranconfig.PTPMustGatherImage, nil
	}

	csvList, err := olm.ListClusterServiceVersionWithNamePattern(
		client, "ptp-operator", ranconfig.PtpOperatorNamespace)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to list CSVs: %v, using fallback image", err)

		return formatPTPMustGatherImageForVersion(ranconfig.Spoke1OperatorVersions[ranparam.PTP])
	}

	for _, csv := range csvList {
		if csv.Object == nil {
			continue
		}

		annotations := csv.Object.GetAnnotations()
		if image, ok := annotations[ptpMustGatherAnnotation]; ok && image != "" {
			klog.V(ranparam.LogLevel).Infof("Found must-gather image in CSV annotation: %s", image)

			return image, nil
		}
	}

	klog.V(ranparam.LogLevel).Info("No must-gather annotation found on PTP operator CSV, using fallback image")

	return formatPTPMustGatherImageForVersion(ranconfig.Spoke1OperatorVersions[ranparam.PTP])
}

// formatPTPMustGatherImageForVersion returns the RHEL 8 or RHEL 9 PTP must-gather image for a PTP operator version.
func formatPTPMustGatherImageForVersion(ptpVersion string) (string, error) {
	matches := majorMinorVersionRegex.FindStringSubmatch(ptpVersion)
	if len(matches) < 3 {
		return "", fmt.Errorf("invalid version format: %q", ptpVersion)
	}

	major, minor := matches[1], matches[2]

	atLeast416, err := version.IsVersionStringInRange(matches[0], "4.16.0-0", "")
	if err != nil {
		return "", fmt.Errorf("failed to check if version is at least 4.16: %w", err)
	}

	if !atLeast416 {
		return fmt.Sprintf("registry.redhat.io/openshift4/ptp-must-gather-rhel8:v%s.%s", major, minor), nil
	}

	imageRepository := "openshift4"

	atLeast500, err := version.IsVersionStringInRange(matches[0], "5.0.0-0", "")
	if err != nil {
		return "", fmt.Errorf("failed to check if version is at least 5.0: %w", err)
	}

	if atLeast500 {
		imageRepository = "openshift5"
	}

	return fmt.Sprintf("registry.redhat.io/%s/ptp-must-gather-rhel9:v%s.%s", imageRepository, major, minor), nil
}
