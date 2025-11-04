package utils

import (
	"net/http"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

// RetryMiddleware pass this into your client config (with .Wrap()) to make it retriable
func RetryMiddleware(logger logr.Logger) func(rt http.RoundTripper) http.RoundTripper {
	logger.Info("Setting up retry middleware")
	return func(rt http.RoundTripper) http.RoundTripper {
		return &retryRoundTripper{
			transport: rt,
			backoff:   getBackoff(),
			log:       logger.WithName("retry-middleware"),
		}
	}
}

// retryRoundTripper include anything here that's useful during middleware processing
type retryRoundTripper struct {
	transport http.RoundTripper
	backoff   wait.Backoff
	log       logr.Logger
}

// getBackoff configured for API outages where we require additional time to recover before being served.
// this exponential with max wait time about ~3mins
func getBackoff() wait.Backoff {
	return wait.Backoff{
		Steps:    45,
		Duration: 10 * time.Millisecond,
		Factor:   1.2,
		Jitter:   0.1,
	}
}

// isRetriable validates if an error is worth retrying
func isRetriable(err error) bool {
	return apierrors.IsInternalError(err) || apierrors.IsServiceUnavailable(err) || net.IsConnectionRefused(err)
}

// RoundTrip implements RoundTripper to do retries.
// This helpful especially when running in SNO,
// where API server maybe down and retries are needed to eventually be successful,
// This allows all our client calls to have this "retry" feature without explicitly wrapping them
func (r *retryRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if errRetryOnError := retry.OnError(r.backoff, isRetriable, func() error {
		resp, err = r.transport.RoundTrip(req)
		if err != nil && isRetriable(err) {
			r.log.Info("DEBUG: retrying", "req.method", req.Method, "req.url", req.URL.String(), "error", err)
		}
		return err //nolint:wrapcheck
	}); errRetryOnError != nil {
		r.log.Info("DEBUG: retrying failed", "error", errRetryOnError)
	}

	return resp, err //nolint:wrapcheck
}
