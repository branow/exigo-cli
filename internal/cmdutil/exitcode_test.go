package cmdutil_test

import (
	"fmt"
	"testing"

	"github.com/branow/exigo-cli/internal/cmdutil"
	"github.com/branow/exigo-cli/internal/exigoapi"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, cmdutil.ExitSuccess},
		{"generic", fmt.Errorf("boom"), cmdutil.ExitError},
		{"cancelled", cmdutil.ErrCancelled, cmdutil.ExitCancelled},
		{"not logged in", cmdutil.ErrNotLoggedIn, cmdutil.ExitAuth},
		{"validation", &cmdutil.ValidationError{Message: "bad flag"}, cmdutil.ExitValidation},
		{"http 401", &exigoapi.HTTPError{StatusCode: 401, Message: "Invalid credentials"}, cmdutil.ExitAuth},
		{"http 403", &exigoapi.HTTPError{StatusCode: 403, Message: "Forbidden"}, cmdutil.ExitAuth},
		{"http 404", &exigoapi.HTTPError{StatusCode: 404, Message: "Not found"}, cmdutil.ExitNotFound},
		{"http 429", &exigoapi.HTTPError{StatusCode: 429, Message: "Too many requests"}, cmdutil.ExitRateLimited},
		{"http 500", &exigoapi.HTTPError{StatusCode: 500, Message: "Server error"}, cmdutil.ExitError},
		{"not found business error", &exigoapi.BusinessError{Operation: "GetCustomers", Errors: []string{"Customer not found"}}, cmdutil.ExitNotFound},
		{"generic business error", &exigoapi.BusinessError{Operation: "CreateCustomer", Errors: []string{"Email already in use"}}, cmdutil.ExitError},
		{"unavailable after retries", &exigoapi.UnavailableError{StatusCode: 503}, cmdutil.ExitRateLimited},
		{"unknown operation", &exigoapi.UnknownOperationError{Operation: "GetCustomerz"}, cmdutil.ExitValidation},
		{"unsupported operation", &exigoapi.UnsupportedOperationError{Operation: "ProcessTransaction"}, cmdutil.ExitValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cmdutil.ExitCode(c.err); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
