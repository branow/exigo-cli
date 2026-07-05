package exigoapi

import (
	"fmt"
	"strings"
)

// HTTPError is a non-2xx REST response: a transport, authentication, or
// routing-level failure, distinct from a business-logic error reported by
// a completed call.
type HTTPError struct {
	StatusCode int
	// Message is the response body's error text, when one was decodable.
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Exigo API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("Exigo API returned HTTP %d: %s", e.StatusCode, e.Message)
}

// BusinessError is a completed response whose result reports one or more
// business-logic errors: the call succeeded at the transport level but the
// operation itself failed.
type BusinessError struct {
	Operation string
	Errors    []string
}

func (e *BusinessError) Error() string {
	return fmt.Sprintf("%s reported errors: %s", e.Operation, strings.Join(e.Errors, "; "))
}

// UnavailableError indicates the server returned a retryable-looking
// status (429/5xx) — after retries were exhausted for GET, or immediately
// for mutating methods, which are not retried on 5xx.
type UnavailableError struct {
	StatusCode int
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("Exigo API unavailable (HTTP %d)", e.StatusCode)
}

// UnknownOperationError is a request for an operation name absent from the
// embedded REST catalog — almost always a typo, caught before any network
// call.
type UnknownOperationError struct {
	Operation string
}

func (e *UnknownOperationError) Error() string {
	return fmt.Sprintf("unknown operation %q (run exigo api --list to see all operations)", e.Operation)
}

// UnsupportedOperationError is a catalogued operation the Exigo API docs
// list without a REST binding ("Rest call not available for this method
// yet").
type UnsupportedOperationError struct {
	Operation string
}

func (e *UnsupportedOperationError) Error() string {
	return fmt.Sprintf("operation %q has no REST endpoint yet in the Exigo API", e.Operation)
}
