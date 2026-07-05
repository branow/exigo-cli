// Package exigoapi is a client for the Exigo SOAP API: envelope
// construction with a custom ApiAuthentication header, retry with backoff
// on transient transport failures, and decoding of both business-level
// Errors[] responses and envelope-level SOAP faults.
package exigoapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"exigo-cli/internal/credentials"
)

// DefaultEndpoint is the production Exigo SOAP endpoint, overridable per
// profile for sandbox hosts.
const DefaultEndpoint = "https://api.exigo.com/3.0/ExigoApi.asmx"

const (
	targetNamespace = "http://api.exigo.com/"
	requestTimeout  = 30 * time.Second
)

// Client invokes named Exigo SOAP operations. It is an interface so
// commands can be tested against a fake implementation instead of a real
// network call.
type Client interface {
	Call(ctx context.Context, operation string, fields map[string]string) (*Result, error)
}

// HTTPClient is the real Client implementation, posting SOAP envelopes to
// a single tenant's endpoint with credentials injected via the
// ApiAuthentication SOAP header (not HTTP Basic auth — this API treats a
// Basic-auth'd request as if the operation doesn't exist).
type HTTPClient struct {
	endpoint   string
	creds      credentials.Credentials
	httpClient *http.Client
}

// New returns an HTTPClient for endpoint, authenticating every call with
// creds via the ApiAuthentication SOAP header.
func New(endpoint string, creds credentials.Credentials) *HTTPClient {
	return &HTTPClient{
		endpoint:   endpoint,
		creds:      creds,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// Call invokes operation with fields as its request body's child elements.
// It retries transient transport failures with backoff; a completed
// response is never retried, even if it reports business-level Errors[].
func (c *HTTPClient) Call(ctx context.Context, operation string, fields map[string]string) (*Result, error) {
	envelope := buildEnvelope(c.creds, operation, fields)

	resp, err := c.doWithRetry(ctx, operation, envelope)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result, err := parseResponse(operation, body)
	if err != nil {
		var fault *Fault
		if !errors.As(err, &fault) && isRetryableStatus(resp.StatusCode) {
			return nil, &Unavailable{StatusCode: resp.StatusCode}
		}
	}
	return result, err
}

func (c *HTTPClient) doWithRetry(ctx context.Context, operation string, envelope []byte) (*http.Response, error) {
	var resp *http.Response
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err = c.attempt(ctx, operation, envelope)
		lastAttempt := attempt == maxAttempts-1
		if !shouldRetry(resp, err) || lastAttempt {
			return resp, err
		}
		if resp != nil {
			resp.Body.Close()
		}
		if werr := wait(ctx, attempt); werr != nil {
			return nil, werr
		}
	}
	return resp, err
}

func (c *HTTPClient) attempt(ctx context.Context, operation string, envelope []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", fmt.Sprintf("%q", targetNamespace+operation))
	return c.httpClient.Do(req)
}
