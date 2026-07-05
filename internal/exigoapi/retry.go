package exigoapi

import (
	"context"
	"math/rand"
	"net/http"
	"time"
)

const maxAttempts = 4

// shouldRetry reports whether a request attempt should be retried: any
// network error (including the timeouts research associates with
// IP-allowlist rejections) or a 5xx response. ASMX SOAP faults are
// typically also carried on a 5xx status, so a transient fault gets
// retried here at the transport layer; a completed 2xx response is never
// retried even if its body reports business-level Errors[].
func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	return isRetryableStatus(resp.StatusCode)
}

// isRetryableStatus reports whether status looks transient rather than a
// deterministic business/protocol failure.
func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// wait blocks for an exponentially increasing, jittered backoff before the
// next retry attempt, returning early if ctx is cancelled.
func wait(ctx context.Context, attempt int) error {
	const base = 200 * time.Millisecond
	backoff := base * time.Duration(1<<attempt)
	jitter := time.Duration(rand.Int63n(int64(base)))
	select {
	case <-time.After(backoff + jitter):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
