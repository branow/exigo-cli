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
	var fault *exigoapi.Fault
	if errors.As(err, &fault) {
		return exitCodeForFault(fault)
	}
	var businessErr *exigoapi.BusinessError
	if errors.As(err, &businessErr) {
		return exitCodeForBusinessError(businessErr)
	}
	var unavailable *exigoapi.Unavailable
	if errors.As(err, &unavailable) {
		return ExitRateLimited
	}
	return ExitError
}

// exitCodeForFault classifies a SOAP envelope-level fault. The API's exact
// fault vocabulary is unconfirmed (see docs/DESIGN.md), so this is a
// best-effort keyword match against the one documented real-world fault
// pattern (auth/credential/IP-allowlist failures), not a verified
// taxonomy.
func exitCodeForFault(f *exigoapi.Fault) int {
	if mentionsAny(f.Code+" "+f.String, "auth", "credential", "login", "whitelist", "ip address") {
		return ExitAuth
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
