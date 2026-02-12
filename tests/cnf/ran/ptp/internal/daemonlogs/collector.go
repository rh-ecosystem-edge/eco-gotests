package daemonlogs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const collectorPollInterval = 10 * time.Second

// CollectDaemonLogs collects linuxptp daemon logs for a single node for the provided duration. The returned pointer is
// non-nil if and only if error is nil.
func CollectDaemonLogs(client *clients.Settings, nodeName string, duration time.Duration) (*CollectionResult, error) {
	if client == nil {
		return nil, fmt.Errorf("cannot collect daemon logs with nil client")
	}

	if nodeName == "" {
		return nil, fmt.Errorf("cannot collect daemon logs with empty node name")
	}

	if duration <= 0 {
		return nil, fmt.Errorf("cannot collect daemon logs with non-positive duration: %s", duration)
	}

	startTime := time.Now()
	lastFetchTime := startTime

	result := CollectionResult{
		NodeName:  nodeName,
		StartedAt: startTime,
	}

	var collectionErrors []error

	err := wait.PollUntilContextTimeout(
		context.TODO(), collectorPollInterval, duration, true, func(ctx context.Context) (bool, error) {
			lines, localFetchTime, fetchErr := collectLinesSince(client, nodeName, lastFetchTime)
			if fetchErr != nil {
				klog.V(tsparams.LogLevel).Infof("Error collecting daemon logs from node %s: %v", nodeName, fetchErr)

				collectionErrors = append(collectionErrors, fetchErr)
			} else {
				lastFetchTime = localFetchTime

				result.Lines = append(result.Lines, lines...)
			}

			return false, nil
		})

	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("unexpected error collecting daemon logs on node %s: %w", nodeName, err)
	}

	result.EndedAt = time.Now()
	result.Errors = collectionErrors

	return &result, nil
}

// collectLinesSince fetches daemon log lines produced after lastFetchTime from the PTP daemon pod on the given node. It
// returns the lines, the local time just before the fetch was issued (for use as the next sinceTime), and an error if
// the fetch failed.
func collectLinesSince(
	client *clients.Settings, nodeName string, lastFetchTime time.Time) ([]string, time.Time, error) {
	localFetchTime := time.Now()

	daemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(client, nodeName)
	if err != nil {
		return nil, localFetchTime, fmt.Errorf("failed to get PTP daemon pod on node %s: %w", nodeName, err)
	}

	logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
		SinceTime: &metav1.Time{Time: lastFetchTime},
		Container: ranparam.PtpContainerName,
	})
	if err != nil {
		return nil, localFetchTime, fmt.Errorf("failed to get logs from node %s since %s: %w", nodeName, lastFetchTime, err)
	}

	logLines := splitAndTrimLogLines(string(logs))

	return logLines, localFetchTime, nil
}

// splitAndTrimLogLines splits a raw log string on newlines and discards empty lines.
func splitAndTrimLogLines(logs string) []string {
	lines := strings.Split(logs, "\n")
	filteredLines := make([]string, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		filteredLines = append(filteredLines, line)
	}

	return filteredLines
}
