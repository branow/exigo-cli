package exigoapi

import (
	"fmt"
	"strings"
)

// Fault is a decoded SOAP 1.1 envelope-level fault: a transport or
// authentication-level failure, distinct from a business-logic Errors[]
// response returned by a completed call.
type Fault struct {
	Code   string
	String string
}

func (f *Fault) Error() string {
	return fmt.Sprintf("soap fault (%s): %s", f.Code, f.String)
}

// BusinessError is a decoded {Operation}Result.Errors[] array: the
// operation completed successfully at the transport level but reported
// one or more business-logic errors.
type BusinessError struct {
	Operation string
	Errors    []string
}

func (e *BusinessError) Error() string {
	return fmt.Sprintf("%s reported errors: %s", e.Operation, strings.Join(e.Errors, "; "))
}

// Unavailable indicates the server kept returning a retryable-looking
// status (429/5xx) even after retries were exhausted, with a response body
// that wasn't a decodable SOAP envelope explaining why.
type Unavailable struct {
	StatusCode int
}

func (e *Unavailable) Error() string {
	return fmt.Sprintf("Exigo API unavailable after retries (HTTP %d)", e.StatusCode)
}
