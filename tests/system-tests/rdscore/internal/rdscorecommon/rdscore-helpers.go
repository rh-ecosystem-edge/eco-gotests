package rdscorecommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	uncordonNodeInterval = 15 * time.Second
	uncordonNodeTimeout  = 3 * time.Minute

	// Drain operation constants.
	drainNodeTimeout      = 25 * time.Minute // Total drain timeout
	drainNodeGracePeriod  = 600              // Pod termination grace period (10 min)
	drainNodeSkipWait     = 300              // Skip wait for stuck pods (5 min)
	drainNodeRetryTimeout = 2 * time.Minute  // Retry window for transient failures
)

// UncordonNode uncordons a node referenced by nodeToUncordon parameter.
// It retries uncordoning for the specified timeout duration at regular intervals.
func UncordonNode(nodeToUncordon *nodes.Builder, interval, timeout time.Duration) {
	By(fmt.Sprintf("Uncordoning node %q", nodeToUncordon.Definition.Name))

	err := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, true,
		func(context.Context) (bool, error) {
			err := nodeToUncordon.Uncordon()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to uncordon %q: %v", nodeToUncordon.Definition.Name, err)

				return false, nil
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully uncordon %q", nodeToUncordon.Definition.Name)

			return err == nil, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to uncordon %q: %v", nodeToUncordon.Definition.Name, err)
	}
}

// DrainNodeWithRetry drains a node with retry logic for transient failures.
// It configures drain with production-appropriate timeouts and logs drain duration.
func DrainNodeWithRetry(ctx context.Context, nodeToDrain *nodes.Builder) error {
	By(fmt.Sprintf("Draining node %q with timeout=%v",
		nodeToDrain.Definition.Name, drainNodeTimeout))

	// Configure drain with extended timeout
	nodeToDrain.SetDrainHelper(
		true,                 // force: allow standalone pods
		true,                 // ignoreDaemonsets: required for OpenShift
		true,                 // deleteLocalData: required for emptyDir volumes
		drainNodeGracePeriod, // gracePeriod: 10 minutes for clean shutdown
		drainNodeSkipWait,    // skipWaitForDelete: 5 minutes for stuck pods
		drainNodeTimeout,     // timeout: 25 minutes total
	)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Drain configuration - timeout=%v, gracePeriod=%ds, skipWait=%ds",
		drainNodeTimeout, drainNodeGracePeriod, drainNodeSkipWait)

	startTime := time.Now()

	var lastErr error

	// Retry for transient failures (gRPC keepalive timeouts, etc.)
	err := wait.PollUntilContextTimeout(ctx, 15*time.Second,
		drainNodeRetryTimeout, true,
		func(context.Context) (bool, error) {
			lastErr = nodeToDrain.Drain()
			if lastErr == nil {
				duration := time.Since(startTime)
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Successfully drained node %q in %v",
					nodeToDrain.Definition.Name, duration)

				return true, nil
			}

			// Check if error is retryable (gRPC/network issues)
			errorMsg := lastErr.Error()
			if strings.Contains(errorMsg, "keepalive") ||
				strings.Contains(errorMsg, "Unavailable") ||
				strings.Contains(errorMsg, "connection refused") {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Drain failed with retryable error, will retry: %v", lastErr)

				return false, nil // Retry
			}

			// Non-retryable error - fail immediately
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Drain failed with non-retryable error: %v", lastErr)

			return false, lastErr
		})
	if err != nil {
		duration := time.Since(startTime)

		return fmt.Errorf("failed to drain node %q after %v: %w",
			nodeToDrain.Definition.Name, duration, lastErr)
	}

	return lastErr
}
