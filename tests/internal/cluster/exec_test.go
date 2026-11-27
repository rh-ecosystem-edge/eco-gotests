//go:build unit_test

package cluster

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	executil "k8s.io/client-go/util/exec"
)

func TestDefaultCommandOptions(t *testing.T) {
	t.Parallel()

	options := defaultCommandOptions()

	assert.Equal(t, uint(4), options.attempts)
	assert.Zero(t, options.timeout)
	assert.Equal(t, 10*time.Second, options.retryDelay)
	assert.Nil(t, options.retryIf)
	assert.True(t, options.retryOnUnavailable)
}

func TestOverlayCommandOptions(t *testing.T) {
	t.Parallel()

	t.Run("ignores nil options", func(t *testing.T) {
		t.Parallel()

		options := overlayCommandOptions(nil)

		assert.Equal(t, defaultCommandOptions(), options)
	})

	t.Run("applies options in order", func(t *testing.T) {
		t.Parallel()

		options := overlayCommandOptions(
			WithRetries(0),
			WithRetries(3),
			WithTimeout(5*time.Second),
			WithRetryDelay(2*time.Second),
			WithRetryOnUnavailable(false),
		)

		assert.Equal(t, uint(4), options.attempts)
		assert.Equal(t, 5*time.Second, options.timeout)
		assert.Equal(t, 2*time.Second, options.retryDelay)
		assert.False(t, options.retryOnUnavailable)
	})
}

func TestWithRetries(t *testing.T) {
	t.Parallel()

	t.Run("zero retries means one attempt", func(t *testing.T) {
		t.Parallel()

		options := overlayCommandOptions(WithRetries(0))

		assert.Equal(t, uint(1), options.attempts)
	})

	t.Run("retries add to the initial attempt", func(t *testing.T) {
		t.Parallel()

		options := overlayCommandOptions(WithRetries(3))

		assert.Equal(t, uint(4), options.attempts)
	})
}

func TestWithTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		timeout     time.Duration
		wantTimeout time.Duration
	}{
		{name: "positive timeout is applied", timeout: 5 * time.Second, wantTimeout: 5 * time.Second},
		{name: "zero timeout is ignored", timeout: 0},
		{name: "negative timeout is ignored", timeout: -time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			options := overlayCommandOptions(WithTimeout(test.timeout))

			assert.Equal(t, test.wantTimeout, options.timeout)
		})
	}
}

func TestWithRetryDelay(t *testing.T) {
	t.Parallel()

	options := overlayCommandOptions(WithRetryDelay(2 * time.Second))

	assert.Equal(t, 2*time.Second, options.retryDelay)
}

func TestWithRetryIf(t *testing.T) {
	t.Parallel()

	t.Run("uses a single predicate", func(t *testing.T) {
		t.Parallel()

		options := overlayCommandOptions(WithRetryIf(func(result CommandResult) bool {
			shouldRetry := result.ExitCode == 1

			return shouldRetry
		}))

		assert.True(t, options.retryIf(CommandResult{ExitCode: 1}))
		assert.False(t, options.retryIf(CommandResult{ExitCode: 0}))
	})

	t.Run("combines predicates with OR", func(t *testing.T) {
		t.Parallel()

		firstPredicateCalled := false
		secondPredicateCalled := false
		options := overlayCommandOptions(
			WithRetryIf(func(CommandResult) bool {
				firstPredicateCalled = true

				return false
			}),
			WithRetryIf(func(CommandResult) bool {
				secondPredicateCalled = true

				return true
			}),
		)

		assert.True(t, options.retryIf(CommandResult{}))
		assert.True(t, firstPredicateCalled)
		assert.True(t, secondPredicateCalled)
	})
}

func TestWithRetryOnEmptyStdout(t *testing.T) {
	t.Parallel()

	options := overlayCommandOptions(WithRetryOnEmptyStdout())

	assert.True(t, options.retryIf(CommandResult{Stdout: " \n\t"}))
	assert.False(t, options.retryIf(CommandResult{Stdout: " output \n"}))
}

func TestWithRetryOnUnavailable(t *testing.T) {
	t.Parallel()

	t.Run("defaults to enabled", func(t *testing.T) {
		t.Parallel()

		assert.True(t, overlayCommandOptions().retryOnUnavailable)
	})

	t.Run("can be disabled", func(t *testing.T) {
		t.Parallel()

		assert.False(t, overlayCommandOptions(WithRetryOnUnavailable(false)).retryOnUnavailable)
	})
}

func TestHandleExecutionError(t *testing.T) {
	t.Parallel()

	executionFailure := errors.New("connection failed")
	tests := []struct {
		name           string
		stdout         string
		stderr         string
		executionError error
		wantResult     *CommandResult
		wantError      error
	}{
		{
			name:   "success",
			stdout: "command output",
			stderr: "command warning",
			wantResult: &CommandResult{
				Stdout: "command output",
				Stderr: "command warning",
			},
		},
		{
			name:           "nonzero exit",
			stdout:         "partial output",
			stderr:         "command failed",
			executionError: executil.CodeExitError{Err: errors.New("exit status 7"), Code: 7},
			wantResult: &CommandResult{
				Stdout:   "partial output",
				Stderr:   "command failed",
				ExitCode: 7,
			},
		},
		{
			name:           "execution error",
			executionError: executionFailure,
			wantError:      executionFailure,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stdout := bytes.NewBufferString(test.stdout)
			stderr := bytes.NewBufferString(test.stderr)

			result, err := handleExecutionError(stdout, stderr, test.executionError)

			if test.wantError != nil {
				require.Error(t, err)
				require.Nil(t, result)
				assert.ErrorIs(t, err, test.wantError)
				assert.ErrorContains(t, err, "failed to execute command")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.wantResult, result)
		})
	}
}
