package await

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// OperatorUpgrade awaits operator upgrade to semver version.
func OperatorUpgrade(apiClient *clients.Settings, versionRegex string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			regex := regexp.MustCompile(versionRegex)

			csv, err := olm.ListClusterServiceVersionWithNamePattern(apiClient, "nfd",
				nfdparams.NFDNamespace)

			for _, csvResource := range csv {
				klog.V(nfdparams.LogLevel).Infof("CSV: %s, Version: %s, Status: %s",
					csvResource.Object.Spec.DisplayName, csvResource.Object.Spec.Version, csvResource.Object.Status.Phase)
			}

			for _, csvResource := range csv {
				csvVersion := csvResource.Object.Spec.Version.String()
				matched := regex.MatchString(csvVersion)

				klog.V(nfdparams.LogLevel).Infof("csvVersion %v is matched:%v with regex%v", csvVersion, matched, versionRegex)

				if matched {
					return csvResource.Object.Status.Phase == "Succeeded", nil
				}
			}

			if err == nil {
				err = fmt.Errorf("csv with version pattern %v not found", versionRegex)
			}

			return false, err
		})
}
