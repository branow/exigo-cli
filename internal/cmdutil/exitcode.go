package cmdutil

import (
	"errors"
	"strings"

	"exigo-cli/internal/exigoapi"
)

// Exit codes, per docs/DESIGN.md's exit-code table.
const (
	ExitSuccess     = 0
	ExitError       = 1
	ExitCancelled   = 2
	ExitValidation  = 3
	ExitAuth        = 4
	ExitNotFound    = 5
	ExitRateLimited = 6
)

// ExitCode maps a command error to the process exit code documented in
// docs/DESIGN.md, giving main.go one place to translate errors into
// process behavior.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, ErrCancelled) {
		return ExitCancelled
	}
	if errors.Is(err, ErrNotLoggedIn) {
		return ExitAuth
	}
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return ExitValidation
	}
	var httpErr *exigoapi.HTTPError
	if errors.As(err, &httpErr) {
		return exitCodeForHTTP(httpErr)
	}
	var businessErr *exigoapi.BusinessError
	if errors.As(err, &businessErr) {
		return exitCodeForBusinessError(businessErr)
	}
	var unavailable *exigoapi.UnavailableError
	if errors.As(err, &unavailable) {
		return ExitRateLimited
	}
	var unknownOp *exigoapi.UnknownOperationError
	var unsupportedOp *exigoapi.UnsupportedOperationError
	if errors.As(err, &unknownOp) || errors.As(err, &unsupportedOp) {
		return ExitValidation
	}
	return ExitError
}

// exitCodeForHTTP classifies a non-2xx REST response by its status code.
func exitCodeForHTTP(e *exigoapi.HTTPError) int {
	switch e.StatusCode {
	case 401, 403:
		return ExitAuth
	case 404:
		return ExitNotFound
	case 429:
		return ExitRateLimited
	}
	return ExitError
}

// exitCodeForBusinessError classifies a decoded Errors[] response. The
// error-message vocabulary is unconfirmed, so this only makes the one
// low-risk inference the exit-code table calls out by name: a message
// mentioning "not found" maps to ExitNotFound.
func exitCodeForBusinessError(e *exigoapi.BusinessError) int {
	if mentionsAny(strings.Join(e.Errors, " "), "not found") {
		return ExitNotFound
	}
	return ExitError
}

func mentionsAny(haystack string, needles ...string) bool {
	haystack = strings.ToLower(haystack)
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
