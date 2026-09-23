//go:build !unit_test

package ptpleap

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// WaitForConfigmapToBeUpdated waits until leap-configmap data for nodeName differs from previousNodeData,
// indicating linuxptp-daemon regenerated the leap file after restart.
func WaitForConfigmapToBeUpdated(nodeName string, previousNodeData string, interval time.Duration,
	timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(
		context.TODO(), interval, timeout, true, func(ctx context.Context) (bool, error) {
			leapConfigMap, err := configmap.Pull(
				RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
			if err != nil {
				klog.Errorf("failed to pull leap configmap %s/%s: %v",
					ranparam.PtpOperatorNamespace, tsparams.LeapConfigmapName, err)

				return false, nil
			}

			if leapConfigMap.Object.Data[nodeName] != previousNodeData {
				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return fmt.Errorf("failed waiting for configmap %s/%s to be updated for node %s: %w",
			ranparam.PtpOperatorNamespace, tsparams.LeapConfigmapName, nodeName, err)
	}

	return nil
}
