// Package exigoapi is a client for the Exigo REST API: operation-name
// routing via the embedded REST catalog, HTTP Basic auth in the API's
// login@company form, retry with backoff on transient transport failures,
// and decoding of both business-level result errors and HTTP-level
// failures.
package exigoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/branow/exigo-cli/internal/credentials"
	"github.com/branow/exigo-cli/internal/exigoapi/catalog"
)

const requestTimeout = 30 * time.Second

// DefaultEndpoint returns the production REST base URL for a company: the
// API routes per tenant via a company-prefixed hostname. Overridable per
// profile for sandbox hosts.
func DefaultEndpoint(company string) string {
	return fmt.Sprintf("https://%s-api.exigo.com/3.0", strings.ToLower(company))
}

// Client invokes named Exigo API operations. It is an interface so
// commands can be tested against a fake implementation instead of a real
// network call.
type Client interface {
	Call(ctx context.Context, operation string, fields map[string]any) (*Result, error)
}

// HTTPClient is the real Client implementation, sending JSON requests to a
// single tenant's REST base URL with credentials as HTTP Basic auth
// (username login@company — the REST surface, unlike SOAP, authenticates
// at the transport level).
type HTTPClient struct {
	baseURL    string
	creds      credentials.Credentials
	httpClient *http.Client
}

// New returns an HTTPClient for the versioned REST base URL (e.g.
// "https://acme-api.exigo.com/3.0"), authenticating every call with creds.
func New(baseURL string, creds credentials.Credentials) *HTTPClient {
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		creds:      creds,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// Call routes operation through the REST catalog and invokes its endpoint:
// fields become query parameters for GET operations and the JSON request
// body otherwise. Transient transport failures retry with backoff (only
// 429 for mutating methods — a 5xx may have already applied the change); a
// completed response is never retried, even if it reports business errors.
func (c *HTTPClient) Call(ctx context.Context, operation string, fields map[string]any) (*Result, error) {
	endpoint, ok := catalog.Lookup(operation)
	if !ok {
		return nil, &UnknownOperationError{Operation: operation}
	}
	if endpoint.Unavailable {
		return nil, &UnsupportedOperationError{Operation: operation}
	}

	requestURL, body, err := buildRequest(c.baseURL, endpoint, fields)
	if err != nil {
		return nil, err
	}

	resp, err := c.doWithRetry(ctx, endpoint.Method, requestURL, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseResponse(operation, resp.StatusCode, responseBody)
}

// buildRequest maps fields onto the endpoint: the query string for GET,
// a JSON body for everything else. Field names are normalized to the
// documented casing so CustomerID and customerID are interchangeable.
func buildRequest(baseURL string, endpoint catalog.Endpoint, fields map[string]any) (string, []byte, error) {
	requestURL := baseURL + endpoint.Path

	if endpoint.Method == http.MethodGet {
		if len(fields) == 0 {
			return requestURL, nil, nil
		}
		query := url.Values{}
		for name, value := range fields {
			query.Set(endpoint.CanonicalField(name), queryValue(value))
		}
		return requestURL + "?" + query.Encode(), nil, nil
	}

	payload := make(map[string]any, len(fields))
	for name, value := range fields {
		payload[endpoint.CanonicalField(name)] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	return requestURL, body, nil
}

// queryValue renders a field value as a query-parameter string: strings
// pass through raw, everything else as its JSON encoding, so an array
// field survives as "[1,2]" rather than Go's "[1 2]".
func queryValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func (c *HTTPClient) doWithRetry(ctx context.Context, method, requestURL string, body []byte) (*http.Response, error) {
	var resp *http.Response
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err = c.attempt(ctx, method, requestURL, body)
		lastAttempt := attempt == maxAttempts-1
		if !shouldRetry(method, resp, err) || lastAttempt {
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

func (c *HTTPClient) attempt(ctx context.Context, method, requestURL string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.creds.LoginName+"@"+c.creds.Company, c.creds.Password)
	return c.httpClient.Do(req)
}
