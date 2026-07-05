package cmdutil_test

import (
	"fmt"
	"testing"

	"exigo-cli/internal/cmdutil"
	"exigo-cli/internal/exigoapi"
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
		{"auth fault", &exigoapi.Fault{Code: "soap:Client", String: "Login failed for user"}, cmdutil.ExitAuth},
		{"whitelist fault", &exigoapi.Fault{Code: "soap:Server", String: "Request blocked: IP address not allowed"}, cmdutil.ExitAuth},
		{"generic fault", &exigoapi.Fault{Code: "soap:Client", String: "Malformed request"}, cmdutil.ExitError},
		{"not found business error", &exigoapi.BusinessError{Operation: "GetCustomer", Errors: []string{"Customer not found"}}, cmdutil.ExitNotFound},
		{"generic business error", &exigoapi.BusinessError{Operation: "CreateCustomer", Errors: []string{"Email already in use"}}, cmdutil.ExitError},
		{"unavailable after retries", &exigoapi.Unavailable{StatusCode: 503}, cmdutil.ExitRateLimited},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cmdutil.ExitCode(c.err); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
