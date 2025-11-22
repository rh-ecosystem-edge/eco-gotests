package processes

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// PtpProcess represents one of the linuxptp processes used by the PTP daemon.
type PtpProcess string

const (
	// Ptp4l represents the ptp4l process, which is the main process implementing the PTP protocol.
	Ptp4l PtpProcess = "ptp4l"
	// Phc2sys represents the phc2sys process, which is what sends time from the precision hardware clock (PHC) to
	// the system clock.
	Phc2sys PtpProcess = "phc2sys"
	// Ts2phc represents the ts2phc process, which is what takes timestamps from a GNSS receiver and sends them to
	// the precision hardware clock (PHC).
	//nolint:staticcheck // These executable names are all being treated as single words for constant names.
	Ts2phc PtpProcess = "ts2phc"
)

// ptpProcessRegex is a regular expression that matches the name of any socket or config file in the ptp daemon pod. It
// is setup so that the first capture group is the process name, the second is the config index, and the third is config
// or socket.
var ptpProcessRegex = regexp.MustCompile(`/var/run/(ptp4l|ts2phc|phc2sys)\.(\d+)\.(config|socket)`)

// GetPID returns the PID of a PTP process on a node by executing a pgrep command in the ptp daemon pod. This function
// will return an error if the process is not running.
func GetPID(client *clients.Settings, nodeName string, process PtpProcess) (string, error) {
	command := fmt.Sprintf("pgrep %s", process)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnEmptyOutput(true))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

// WaitForProcessPID waits up to timeout for a PTP process to start running on a node by executing a pgrep command in
// the ptp daemon pod, polling every 3 seconds.
func WaitForProcessPID(
	client *clients.Settings, nodeName string, process PtpProcess, timeout time.Duration) (string, error) {
	var (
		pid string
		err error
	)

	err = wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			// Since GetPID returns an error when the process is not running, we just loop until it does not
			// return an error, or the timeout is reached.
			pid, err = GetPID(client, nodeName, process)

			return err == nil, nil
		})
	if err != nil {
		return "", err
	}

	return pid, nil
}

// GetPtp4lPIDsByRelatedProcess returns the PIDs of all ptp4l processes on a node that are related (or not) to a
// specific PTP process.
//
// The assumption is that there will be potentially multiple ptp4l processes running on a node, but for other PTP
// processes, there can be at most one per node. When related is true, this function finds the single ptp4l process that
// is related to the specified PTP process. When related is false, this function finds all ptp4l processes that are not
// related to the specified PTP process.
//
// This means that in environments with only one PTP profile (and therefore only one ptp4l process), this function
// always returns an empty slice if related is false. This is because then the processes must be related to the one
// ptp4l process.
func GetPtp4lPIDsByRelatedProcess(
	client *clients.Settings, nodeName string, relatedProcess PtpProcess, related bool) ([]string, error) {
	if relatedProcess == Ptp4l {
		return nil, fmt.Errorf("cannot get ptp4l PIDs related or not related to itself")
	}

	getIndexCommand := fmt.Sprintf("pgrep -a %s | grep /var/run | head -n 1", relatedProcess)

	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, getIndexCommand,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnEmptyOutput(true), ptpdaemon.WithRetryDelay(time.Minute))
	if err != nil {
		return nil, fmt.Errorf("failed to get index for process %s: %w", relatedProcess, err)
	}

	matches := ptpProcessRegex.FindAllStringSubmatch(output, -1)
	if len(matches) < 1 {
		return nil, fmt.Errorf("failed to get index for process %s: no matches found: %v", relatedProcess, output)
	}

	grepFlag := ""
	if !related {
		grepFlag = "-v"
	}

	var indices []string

	for _, match := range matches {
		if len(match) > 2 {
			// Index 0 is the entire match, index 1 is the process name, and index 2 is the config index.
			indices = append(indices, match[2])
		}
	}

	grepPattern := fmt.Sprintf("\\/var\\/run\\/ptp4l\\.\\(%s\\).config", strings.Join(indices, "|"))
	getPIDsCommand := fmt.Sprintf("pgrep -a ptp4l | grep -E %s '%s' | cut -d' ' -f1", grepFlag, grepPattern)

	output, err = ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, getPIDsCommand,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnEmptyOutput(true), ptpdaemon.WithRetryDelay(time.Minute))
	if err != nil {
		return nil, err
	}

	pidStrings := strings.Split(strings.TrimSpace(output), "\n")

	return pidStrings, nil
}

// KillProcessByPID kills a process on a node by executing a kill -9 command in the ptp daemon pod. Unlike
// [KillPtpProcess] this function accepts the PID, not the process name.
func KillProcessByPID(client *clients.Settings, nodeName string, pid string) error {
	command := fmt.Sprintf("kill -9 %s", strings.TrimSpace(pid))

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return err
	}

	return nil
}

// KillPtpProcess kills a PTP process on a node by executing a kill command in the ptp daemon pod.
func KillPtpProcess(client *clients.Settings, nodeName string, process PtpProcess) error {
	command := fmt.Sprintf("pkill %s", string(process))

	_, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(client, nodeName, command)
	if err != nil {
		return err
	}

	return nil
}

// KillPtpProcessMultipleTimes kills a PTP process on a node multiple times by executing a pkill command in the ptp
// daemon pod. It waits for 1 second after failed attempts before trying again.
func KillPtpProcessMultipleTimes(client *clients.Settings, nodeName string, process PtpProcess, times int) error {
	for i := 0; i < times; i++ {
		err := KillPtpProcess(client, nodeName, process)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to kill process %s: %v", process, err)

			time.Sleep(1 * time.Second)
		}
	}

	return nil
}
