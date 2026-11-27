package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/remotecommand"
	executil "k8s.io/client-go/util/exec"
	"k8s.io/klog/v2"
)

// LogLevel is the log level for the cluster package.
const LogLevel klog.Level = 90

// CommandResult is the result of a command execution. It contains the stdout, stderr, and exit code of the command.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// commandOptions are the options for a command execution. They are used to configure the command execution behavior.
type commandOptions struct {
	// attempts is the total number of attempts to execute the command. It includes the initial attempt and the
	// retries. It must be at least 1.
	attempts uint
	// timeout is the timeout for the command execution. It applies to a single invocation of the command. To have a
	// timeout be inclusive of retries, a context with a timeout should be passed.
	timeout time.Duration
	// retryDelay is the delay between retries.
	retryDelay time.Duration
	// retryIf is the function to determine if the command should be retried. If a command executes, retryIf is
	// called with the result. If retryIf returns true, the command will be retried.
	retryIf func(CommandResult) bool
	// retryOnUnavailable is whether to retry the command if it could not be executed.
	retryOnUnavailable bool
}

// defaultCommandOptions returns the default command options. These will be used unless an option specifically overrides
// them.
//
// The default is 4 attempts, which corresponds to the initial attempt and 3 retries. The default retry delay is 10
// seconds. These defaults are chosen to be reasonable and match what is commonly used by [ExecCommandOnSNOWithRetries].
//
// By default, retryIf is nil and retryOnUnavailable is true. This means the command will be retried if it could not be
// executed, but eagerly returns the result if the command could be executed.
func defaultCommandOptions() *commandOptions {
	return &commandOptions{
		attempts:           4,
		timeout:            0,
		retryDelay:         10 * time.Second,
		retryIf:            nil,
		retryOnUnavailable: true,
	}
}

// overlayCommandOptions overlays the default command options with the provided options. The last option takes
// precedence.
//
// If an option is nil, it is ignored.
func overlayCommandOptions(options ...CommandOption) *commandOptions {
	opts := defaultCommandOptions()

	for _, option := range options {
		if option != nil {
			option(opts)
		}
	}

	return opts
}

// CommandOption is a function that can be used to configure the command execution behavior.
type CommandOption func(*commandOptions)

// WithRetries sets the number of retries to execute the command. If provided with n retries, the command will be
// executed up to n+1 times. It defaults to 3 retries.
//
// These retries are shared between all types of retries, including [WithRetryIf] and [WithRetryOnUnavailable].
func WithRetries(retries uint) CommandOption {
	return func(o *commandOptions) {
		o.attempts = retries + 1
	}
}

// WithTimeout sets the timeout for the command execution. It applies only to an individual execution of the command.
// Retry delays, pod fetches, etc. are not included in the timeout.
//
// If a broader timeout is desired, a context with a timeout should be passed to the execution functions.
func WithTimeout(timeout time.Duration) CommandOption {
	if timeout <= 0 {
		return nil
	}

	return func(o *commandOptions) {
		o.timeout = timeout
	}
}

// WithRetryDelay sets the delay between retries. It defaults to 10 seconds.
func WithRetryDelay(retryDelay time.Duration) CommandOption {
	return func(o *commandOptions) {
		o.retryDelay = retryDelay
	}
}

// WithRetryIf sets the function to determine if the command should be retried. If a command executes, retryIf is called
// with the result. If retryIf returns true, the command will be retried.
//
// RetryIf consumes the same retry pool as [WithRetryOnUnavailable].
//
// This option may be provided multiple times, in which case the command will be retried if any of the functions return
// true.
func WithRetryIf(retryIf func(CommandResult) bool) CommandOption {
	return func(o *commandOptions) {
		if o.retryIf == nil {
			o.retryIf = retryIf
		} else {
			existing := o.retryIf
			o.retryIf = func(result CommandResult) bool {
				return existing(result) || retryIf(result)
			}
		}
	}
}

// WithRetryOnEmptyStdout sets whether to retry the command if the stdout, trimmed of whitespace, is empty. It is a
// special case of [WithRetryIf]. See that documentation for more details.
func WithRetryOnEmptyStdout() CommandOption {
	return WithRetryIf(func(result CommandResult) bool {
		return strings.TrimSpace(result.Stdout) == ""
	})
}

// WithRetryOnUnavailable sets whether to retry the command if it could not be executed. In practice, this means any
// error other than [executil.ExitError]. It is strongly recommended to leave the default value of true.
//
// The motivation for this type of retry is in case the MCO daemon pod is restarting or the API server is temporarily
// unavailable. In these cases, it may be useful to distinguish between a command that failed (e.g. a nonzero exit code)
// and a command that was not executed (e.g. the node is not reachable).
//
// Retrying on unavailable shares the retry pool with [WithRetryIf]. This is why the default retries is not zero.
func WithRetryOnUnavailable(retryOnUnavailable bool) CommandOption {
	return func(o *commandOptions) {
		o.retryOnUnavailable = retryOnUnavailable
	}
}

// ExecOnSNO executes a command on a single node cluster. It begins by discovering the node to execute the command on.
// If there is not exactly one node, an error will be returned. Otherwise, it behaves the same as [ExecOnNode].
func ExecOnSNO(
	ctx context.Context,
	client *clients.Settings,
	command string,
	options ...CommandOption,
) (*CommandResult, error) {
	clusterNodes, err := nodes.List(client, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes while executing command on SNO: %w", err)
	}

	if len(clusterNodes) != 1 {
		return nil, fmt.Errorf("expected exactly one node while executing command on SNO, found %d", len(clusterNodes))
	}

	// We use the exported function here to make it explicit that the behavior is the same, even though we could
	// call the unexported [execOnNode] directly.
	return ExecOnNode(ctx, client, clusterNodes[0].Definition.Name, command, options...)
}

// ExecOnNode executes a command on a specific node. Options are applied in the order they are provided.
//
// Note that this function will not return an error if the command returns a non-zero exit code, unless [WithRetryIf] is
// used to modify that behavior. Callers must check the result exit code to verify the command execution was successful.
//
// There are three possible return value pairs:
//
//   - (nil, non-nil): The command coult not be executed, possibly after retries or the context finished.
//   - (non-nil, nil): The command was executed, with a potentially non-zero exit code.
//   - (non-nil, non-nil): The command was executed and on its last attempt was rejected by [WithRetryIf].
//     It then ran out of attempts or the context finished.
//
// The default behavior with retries is such that callers should expect a nil error unless something consistently
// prevented the command from being executed (e.g. the node is not reachable). Callers should then check the result exit
// code to verify the command execution was successful.
func ExecOnNode(
	ctx context.Context,
	client *clients.Settings,
	nodeName string,
	command string,
	options ...CommandOption,
) (*CommandResult, error) {
	return execOnNode(ctx, client, nodeName, command, overlayCommandOptions(options...))
}

// ExecOnNodes executes a command on all nodes matching the selector. The options are global and applied in the order
// they are provided.
//
// Per-node, the behavior is the same as [ExecOnNode]. Consult the documentation there for more details.
//
// This function will first list all nodes matching the selector, and then execute the command on each node in sequence.
// If this first listing fails, an error will be returned. If no nodes are found, an error will be returned, so the
// function never vacuously succeeds. Unless the function fails in this stage, the result map will be non-nil.
//
// Once nodes are found, the command is executed on each node in sequence. Unless the context finishes during the
// sequence, the command will be attempted on all nodes. Results are accumulated into a map, keyed by node name (not
// hostname). Errors are accumulated using [errors.Join]. Both results and the accumulated errors will be returned.
func ExecOnNodes(
	ctx context.Context,
	client *clients.Settings,
	selector metav1.ListOptions,
	command string,
	options ...CommandOption,
) (map[string]CommandResult, error) {
	overlayedOptions := overlayCommandOptions(options...)

	matchingNodes, err := nodes.List(client, selector)
	if err != nil {
		return nil, fmt.Errorf("failed to exec on nodes: %w", err)
	}

	if len(matchingNodes) == 0 {
		return nil, fmt.Errorf("no matching nodes found")
	}

	klog.V(LogLevel).Infof("Found %d matching nodes on which to execute command %s", len(matchingNodes), command)

	var (
		accumulatedErrors []error
		results           = make(map[string]CommandResult)
	)

	for _, node := range matchingNodes {
		select {
		case <-ctx.Done():
			accumulatedErrors = append(accumulatedErrors,
				fmt.Errorf("context finished while executing command on nodes: %w", ctx.Err()))

			return results, errors.Join(accumulatedErrors...)
		default:
		}

		klog.V(LogLevel).Infof("Attempting to execute command %s on node %s", command, node.Definition.Name)

		result, err := execOnNode(ctx, client, node.Definition.Name, command, overlayedOptions)
		if result != nil {
			// Since result may be non-nil even if err is non-nil, we always add the result to the results
			// map if it's available.
			results[node.Definition.Name] = *result
		}

		if err != nil {
			accumulatedErrors = append(accumulatedErrors,
				fmt.Errorf("command execution on matching node %s failed: %w", node.Definition.Name, err))
		}
	}

	return results, errors.Join(accumulatedErrors...)
}

// execOnNode executes a command on a specific node, handling retries. It treats [executil.ExitError] as a successful
// command execution, and the returned error will be nil.
//
// When retryOnUnavailable is true, the command will be retried on any other error than [executil.ExitError]. Similarly,
// if it is false, it will return an error immediately.
//
// When retryIf is not nil, the command will be retried if retryIf returns true. This is the only way a successful
// command execution will cause retries.
//
// If the attempts are exhausted, an error will always be returned. If the last attempt was a successful command
// execution, but retryIf returned true, the result will be non-nil too. In any other case, the result will be nil.
func execOnNode(
	ctx context.Context,
	client *clients.Settings,
	nodeName string,
	command string,
	options *commandOptions,
) (*CommandResult, error) {
	var (
		mcoDaemonPod *pod.Builder
		lastResult   *CommandResult
		err          error
	)

	for attempt := range options.attempts {
		if err := ctx.Err(); err != nil {
			return lastResult, fmt.Errorf("context finished while executing command on node %s: %w", nodeName, err)
		}

		if attempt > 0 {
			select {
			case <-ctx.Done():
				return lastResult, fmt.Errorf("context finished while waiting for retry delay: %w", ctx.Err())
			case <-time.After(options.retryDelay):
			}
		}

		lastResult = nil

		mcoDaemonPod, err = getMCODaemonPodOnNode(client, nodeName)
		if err != nil {
			if options.retryOnUnavailable {
				klog.V(LogLevel).Infof("Failed to execute command on node %s: %v, attempt %d of %d",
					nodeName, err, attempt, options.attempts)

				continue
			}

			return nil, fmt.Errorf("failed to execute command on node %s: %w", nodeName, err)
		}

		var (
			stdout         bytes.Buffer
			stderr         bytes.Buffer
			ctxWithTimeout = ctx
			cancel         context.CancelFunc
		)

		if options.timeout > 0 {
			ctxWithTimeout, cancel = context.WithTimeout(ctx, options.timeout)
		}

		// We intentionally let TTY be false. This confers a few advantages in the context of our
		// non-interactive execution:
		//
		//  - It prevents the carriage return (\r) from being included in the output, unless a command explicitly prints it.
		//  - It prevents ANSI escapes from being included in the output.
		//  - It allows stdout and stderr to be captured as separate buffers.
		err = mcoDaemonPod.ExecCommandWithContext(ctxWithTimeout, corev1.PodExecOptions{
			Command: []string{"nsenter", "--mount=/proc/1/ns/mnt", "--", "sh", "-c", command},
			Stdout:  true,
			Stderr:  true,
		}, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})

		if cancel != nil {
			cancel()
		}

		lastResult, err = handleExecutionError(&stdout, &stderr, err)
		if err != nil {
			if options.retryOnUnavailable {
				klog.V(LogLevel).Infof("Failed to execute command on node %s: %v, attempt %d of %d",
					nodeName, err, attempt, options.attempts)

				continue
			}

			return nil, err
		}

		if options.retryIf != nil && lastResult != nil && options.retryIf(*lastResult) {
			klog.V(LogLevel).Infof("Retrying command on node %s based on retryIf, attempt %d of %d",
				nodeName, attempt, options.attempts)

			continue
		}

		return lastResult, nil
	}

	if lastResult != nil {
		return lastResult, fmt.Errorf("ran out of attempts to execute command on node %s", nodeName)
	}

	return lastResult, fmt.Errorf("ran out of attempts to execute command on node %s: %w", nodeName, err)
}

// handleExecutionError handles the error from a command execution. It returns a CommandResult and an error. The error
// will be nil if the command executed successfully, or if it was an [executil.ExitError]. The CommandResult will be nil
// if any other error occurred.
func handleExecutionError(stdout, stderr *bytes.Buffer, err error) (*CommandResult, error) {
	result := CommandResult{}

	if exitError, ok := errors.AsType[executil.ExitError](err); ok {
		result.ExitCode = exitError.ExitStatus()
	} else if err != nil {
		return nil, fmt.Errorf("failed to execute command: %w", err)
	}

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	return &result, nil
}

// getMCODaemonPodOnNode gets the MCO daemon pod on a specific node. It returns an error unless there is exactly one
// eligible pod.
//
// Eligible pods are those that are running and have the label "k8s-app" set to the MCO daemon name, spec.nodeName
// matches the node name, and status.phase is Running.
func getMCODaemonPodOnNode(client *clients.Settings, nodeName string) (*pod.Builder, error) {
	listOptions := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.nodeName": nodeName,
			"status.phase":  string(corev1.PodRunning),
		}).String(),
		LabelSelector: labels.SelectorFromSet(labels.Set{"k8s-app": GeneralConfig.MCOConfigDaemonName}).String(),
	}

	mcoDaemonPodList, err := pod.List(client, GeneralConfig.MCONamespace, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCO daemon pods on node %s: %w", nodeName, err)
	}

	if len(mcoDaemonPodList) == 0 {
		return nil, fmt.Errorf("no MCO daemon pods found on node %s", nodeName)
	}

	if len(mcoDaemonPodList) > 1 {
		return nil, fmt.Errorf("multiple MCO daemon pods found on node %s", nodeName)
	}

	return mcoDaemonPodList[0], nil
}
