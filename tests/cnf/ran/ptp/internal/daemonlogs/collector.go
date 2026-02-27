package daemonlogs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// CollectionResult is the output of a long-window daemon log collection.
type CollectionResult struct {
	// NodeName is the name of the node that the logs were collected from.
	NodeName string
	// StartedAt is the time the collection started.
	StartedAt time.Time
	// EndedAt is the time the collection ended.
	EndedAt time.Time
	// TempFilePath is the path to the temporary file containing the collected log lines, one per line. The caller
	// is responsible for removing this file when it is no longer needed.
	TempFilePath string
	// CollectedLineCount is the total number of non-empty log lines written to the temp file.
	CollectedLineCount int
	// Errors is the errors that occurred while collecting the logs.
	Errors []error
}

// CollectDaemonLogs collects linuxptp daemon logs for a single node for the provided duration. It polls every 10
// seconds for the full duration. Some log lines may be duplicated, but they will never be skipped. Collected lines are
// streamed to a temporary file to keep memory usage bounded regardless of duration. The returned pointer is non-nil if
// and only if error is nil.
//
// In the returned CollectionResult, the StartedAt and EndedAt times are the time the collection process started and
// ended, not necessarily the time the first and last log lines were collected. The caller is responsible for removing
// CollectionResult.TempFilePath when it is no longer needed.
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

	tempFile, err := os.CreateTemp("", "ptp-daemon-logs-*.log")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for daemon logs: %w", err)
	}

	startTime := time.Now()
	lastFetchTime := startTime

	result := CollectionResult{
		NodeName:     nodeName,
		StartedAt:    startTime,
		TempFilePath: tempFile.Name(),
	}

	var collectionErrors []error

	pollErr := wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, duration, true, func(ctx context.Context) (bool, error) {
			localFetchTime := time.Now()

			linesWritten, fetchErr := appendLogsSince(tempFile, client, nodeName, lastFetchTime)
			if fetchErr != nil {
				klog.V(tsparams.LogLevel).Infof("Error collecting daemon logs from node %s: %v", nodeName, fetchErr)

				collectionErrors = append(collectionErrors, fetchErr)
			} else {
				lastFetchTime = localFetchTime

				result.CollectedLineCount += linesWritten
			}

			return false, nil
		})

	closeErr := tempFile.Close()

	if pollErr != nil && !errors.Is(pollErr, context.DeadlineExceeded) {
		_ = os.Remove(tempFile.Name())

		return nil, fmt.Errorf("unexpected error collecting daemon logs on node %s: %w", nodeName, pollErr)
	}

	if closeErr != nil {
		_ = os.Remove(tempFile.Name())

		return nil, fmt.Errorf("failed to close temp file %s: %w", tempFile.Name(), closeErr)
	}

	result.EndedAt = time.Now()
	result.Errors = collectionErrors

	return &result, nil
}

// appendLogsSince fetches daemon log lines produced after lastFetchTime and appends them to dest. It returns the number
// of non-empty lines written and any error encountered during the fetch.
func appendLogsSince(
	dest io.Writer, client *clients.Settings, nodeName string, lastFetchTime time.Time,
) (int, error) {
	daemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(client, nodeName)
	if err != nil {
		return 0, fmt.Errorf("failed to get PTP daemon pod on node %s: %w", nodeName, err)
	}

	logs, err := daemonPod.GetLogsWithOptions(&corev1.PodLogOptions{
		SinceTime: &metav1.Time{Time: lastFetchTime},
		Container: ranparam.PtpContainerName,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get logs from node %s since %s: %w", nodeName, lastFetchTime, err)
	}

	return writeNonEmptyLines(dest, string(logs))
}

// writeNonEmptyLines splits raw on newlines, writes each non-empty line (terminated by newline) to dest, and returns
// the count of lines written.
func writeNonEmptyLines(dest io.Writer, raw string) (int, error) {
	lines := strings.Split(raw, "\n")
	written := 0

	for _, line := range lines {
		if line == "" {
			continue
		}

		if _, err := io.WriteString(dest, line+"\n"); err != nil {
			return written, fmt.Errorf("failed to write log line to temp file: %w", err)
		}

		written++
	}

	return written, nil
}
