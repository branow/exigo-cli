package cmd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/branow/exigo-cli/cmd"
	"github.com/branow/exigo-cli/internal/cmdutil"
)

// jsonHandler responds with body and records each request's method, path,
// query, and JSON payload for assertions.
type jsonHandler struct {
	body string

	requests  int
	gotMethod string
	gotPath   string
	gotQuery  map[string][]string
	gotBody   map[string]any
}

func (h *jsonHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests++
	h.gotMethod = r.Method
	h.gotPath = r.URL.Path
	h.gotQuery = r.URL.Query()
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		json.Unmarshal(data, &h.gotBody)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(h.body))
}

func TestAPIHappyPath(t *testing.T) {
	handler := &jsonHandler{body: `{"customers": [{"customerID": 12345, "firstName": "Jane"}], "recordCount": 1}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomers", "-f", "customerID=12345"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if handler.gotMethod != http.MethodGet || handler.gotPath != "/customers" {
		t.Errorf("got %s %s, want GET /customers", handler.gotMethod, handler.gotPath)
	}
	if got := handler.gotQuery["customerID"]; len(got) != 1 || got[0] != "12345" {
		t.Errorf("got query customerID=%v, want [12345]", got)
	}
	if !strings.Contains(out.String(), `"firstName": "Jane"`) {
		t.Errorf("got output %q, want it to contain firstName", out.String())
	}
	if !strings.Contains(out.String(), `"recordCount": 1`) {
		t.Errorf("got output %q, want it to contain recordCount", out.String())
	}
}

func TestAPIFieldValuesAreTyped(t *testing.T) {
	handler := &jsonHandler{body: `{"customerID": 67890, "result": {"status": 0}}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "CreateCustomer",
		"-f", "firstName=Jane",
		"-f", "customerType=1",
		"-f", "mainAddressVerified=true",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if handler.gotMethod != http.MethodPost || handler.gotPath != "/customers" {
		t.Errorf("got %s %s, want POST /customers", handler.gotMethod, handler.gotPath)
	}
	if got := handler.gotBody["firstName"]; got != "Jane" {
		t.Errorf("got body firstName=%v (%T), want the string Jane", got, got)
	}
	if got := handler.gotBody["customerType"]; got != float64(1) {
		t.Errorf("got body customerType=%v (%T), want the JSON number 1", got, got)
	}
	if got := handler.gotBody["mainAddressVerified"]; got != true {
		t.Errorf("got body mainAddressVerified=%v (%T), want the JSON boolean true", got, got)
	}
}

func TestAPIBusinessErrorPrintsPartialResultAndMapsToGenericExitCode(t *testing.T) {
	handler := &jsonHandler{body: `{"customerID": 0, "result": {"status": 1, "errors": ["Email already in use"]}}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "CreateCustomer", "-f", "email=jane@example.com"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for a business-errors response")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitError {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitError)
	}
	if !strings.Contains(out.String(), "Email already in use") {
		t.Errorf("expected the partial result to still be printed, got %q", out.String())
	}
}

func TestAPIUnauthorizedMapsToAuthExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Invalid credentials"}`))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomers"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an HTTP 401 response")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAPINotLoggedIn(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomers"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when no credentials are configured")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAPIUnknownOperationBeatsNotLoggedIn(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "NoSuchOp"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown operation")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d (a typo must not be masked by auth state)", got, cmdutil.ExitValidation)
	}
}

func TestAPIListPrintsOperationsWithoutCredentials(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "--list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), "GetCustomers") {
		t.Errorf("expected the operation list to contain GetCustomers, got %q", out.String()[:200])
	}
}

func TestAPIDescribePrintsFieldsWithoutCredentials(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "CreatePaymentCreditCard", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"POST /payment/creditcard",
		"Request (body)",
		"creditCardNumber", "String", "required", // typed, mandatory field
		"billingName", "optional", // typed, optional field
		"Response", "paymentID",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output missing %q, got %q", want, got)
		}
	}
}

func TestAPIDescribeLabelsGetQueryParameters(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	// Case-insensitive operation names resolve to documented casing.
	root.SetArgs([]string{"api", "getcustomers", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "GetCustomers\n") {
		t.Errorf("want output to start with canonical name GetCustomers, got %q", got)
	}
	if !strings.Contains(got, "Request (query)") || strings.Contains(got, "Request (body)") {
		t.Errorf("want a GET operation described with a query request section, got %q", got)
	}
}

func TestAPIDescribeExpandsNestedTypes(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "CreateOrder", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	// The details field is an array of OrderDetailRequest; its fields must
	// be expanded inline, indented under it.
	if !strings.Contains(got, "details") || !strings.Contains(got, "OrderDetailRequest[]") {
		t.Fatalf("expected a details OrderDetailRequest[] field, got %q", got)
	}
	for _, nested := range []string{"itemCode", "quantity"} {
		if !strings.Contains(got, nested) {
			t.Errorf("expected expanded nested field %q from OrderDetailRequest, got %q", nested, got)
		}
	}
}

func TestAPIDescribeJSON(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "json", "api", "SetAccountCreditCardToken", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var view map[string]any
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("describe -o json is not valid JSON: %v", err)
	}
	if view["operation"] != "SetAccountCreditCardToken" || view["method"] != "PUT" {
		t.Errorf("got operation=%v method=%v, want SetAccountCreditCardToken/PUT", view["operation"], view["method"])
	}
	body, ok := view["body"].([]any)
	if !ok || len(body) == 0 {
		t.Fatalf("want a non-empty body field list in %v", view)
	}
	first, ok := body[0].(map[string]any)
	if !ok || first["name"] != "customerID" || first["type"] != "Int32" || first["required"] != true {
		t.Errorf("first body field = %v, want {name:customerID type:Int32 required:true}", body[0])
	}
}

func TestAPIDescribeJSONIncludesReferencedTypes(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "json", "api", "CreateOrder", "--describe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var view struct {
		Types map[string][]map[string]any `json:"types"`
	}
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("describe -o json is not valid JSON: %v", err)
	}
	def, ok := view.Types["OrderDetailRequest"]
	if !ok || len(def) == 0 {
		t.Fatalf("want a non-empty OrderDetailRequest definition in types, got %v", view.Types)
	}
}

func TestAPIDescribeRejectsFieldCombination(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomers", "--describe", "-f", "customerID=1"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for --describe with --field")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAPIDescribeUnknownOperation(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "NotAnOperation", "--describe"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error describing an uncatalogued operation")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAPIRejectsInvalidOutputFlag(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"-o", "yaml", "api", "--list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for -o yaml")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAPIUnknownOperationMapsToValidationExitCode(t *testing.T) {
	handler := &jsonHandler{body: `{}`}
	server := httptest.NewServer(handler)
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomerz"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for an operation absent from the catalog")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
	if handler.requests != 0 {
		t.Errorf("server was hit %d times, want 0 for an uncatalogued operation", handler.requests)
	}
}
