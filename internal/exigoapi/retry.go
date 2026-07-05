package exigoapi

import (
	"context"
	"math/rand"
	"net/http"
	"time"
)

const maxAttempts = 4

// shouldRetry reports whether a request attempt should be retried. GET is
// safe to retry on any network error or retryable status. Mutating methods
// only retry on 429 — the server explicitly refused the request before
// processing it — because a network error or 5xx may have already applied
// the change (e.g. double-creating an order). A completed 2xx response is
// never retried, even if its body reports business-level errors.
func shouldRetry(method string, resp *http.Response, err error) bool {
	if method == http.MethodGet {
		if err != nil {
			return true
		}
		return isRetryableStatus(resp.StatusCode)
	}
	return err == nil && resp.StatusCode == http.StatusTooManyRequests
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
