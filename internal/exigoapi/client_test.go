package exigoapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/branow/exigo-cli/internal/credentials"
	"github.com/branow/exigo-cli/internal/exigoapi"
)

func testCreds() credentials.Credentials {
	return credentials.Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"}
}

func TestCallGETSendsQueryAndBasicAuth(t *testing.T) {
	var gotMethod, gotPath string
	var gotQuery map[string][]string
	var gotUser, gotPassword string
	var gotAuthOK bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotUser, gotPassword, gotAuthOK = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"customers": [{"customerID": 12345, "firstName": "Jane"}], "recordCount": 1}`))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())
	result, err := client.Call(context.Background(), "GetCustomers", map[string]any{"CustomerID": 12345})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("got method %q, want GET", gotMethod)
	}
	if gotPath != "/customers" {
		t.Errorf("got path %q, want /customers", gotPath)
	}
	if got := gotQuery["customerID"]; len(got) != 1 || got[0] != "12345" {
		t.Errorf("got query customerID=%v, want [12345] (field name canonicalized to documented casing)", got)
	}
	if _, present := gotQuery["CustomerID"]; present {
		t.Error("query should not contain the uncanonicalized CustomerID parameter")
	}
	if !gotAuthOK {
		t.Fatal("request had no decodable Basic Authorization header")
	}
	if want := "alice@ACME"; gotUser != want {
		t.Errorf("got Basic auth username %q, want %q", gotUser, want)
	}
	if want := "s3cret"; gotPassword != want {
		t.Errorf("got Basic auth password %q, want %q", gotPassword, want)
	}

	if got := result.Fields["recordCount"]; got != float64(1) {
		t.Errorf("got recordCount %v (%T), want 1", got, got)
	}
	customers, ok := result.Fields["customers"].([]any)
	if !ok || len(customers) != 1 {
		t.Fatalf("got customers %v, want a one-element array", result.Fields["customers"])
	}
	customer, ok := customers[0].(map[string]any)
	if !ok || customer["firstName"] != "Jane" {
		t.Errorf("got customer %v, want firstName Jane", customers[0])
	}
	if len(result.Errors) != 0 {
		t.Errorf("got Errors %v, want none", result.Errors)
	}
}

func TestCallPOSTSendsCanonicalizedJSONBody(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"customerID": 67890, "result": {"status": 0}}`))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())
	result, err := client.Call(context.Background(), "CreateCustomer", map[string]any{
		"FirstName":    "Jane",
		"LASTNAME":     "Doe",
		"customerType": 1,
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("got method %q, want POST", gotMethod)
	}
	if gotPath != "/customers" {
		t.Errorf("got path %q, want /customers", gotPath)
	}
	if want := "application/json"; gotContentType != want {
		t.Errorf("got Content-Type %q, want %q", gotContentType, want)
	}

	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("request body is not JSON: %v\nbody: %s", err, gotBody)
	}
	want := map[string]any{"firstName": "Jane", "lastName": "Doe", "customerType": float64(1)}
	for key, wantValue := range want {
		if got := payload[key]; got != wantValue {
			t.Errorf("got body field %s=%v (%T), want %v", key, got, got, wantValue)
		}
	}
	for _, stale := range []string{"FirstName", "LASTNAME"} {
		if _, present := payload[stale]; present {
			t.Errorf("body should not contain the uncanonicalized field %q: %s", stale, gotBody)
		}
	}

	if got := result.Fields["customerID"]; got != float64(67890) {
		t.Errorf("got customerID %v, want 67890", got)
	}
}

func TestCallDecodesBusinessErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "top-level errors array",
			body: `{"errors": ["Email is already in use", "Invalid state code"]}`,
			want: []string{"Email is already in use", "Invalid state code"},
		},
		{
			name: "result object errors array",
			body: `{"customerID": 0, "result": {"status": 1, "errors": ["Email is already in use"]}}`,
			want: []string{"Email is already in use"},
		},
		{
			name: "result object error string on non-success status",
			body: `{"result": {"status": 1, "error": "Customer not found"}}`,
			want: []string{"Customer not found"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(c.body))
			}))
			defer server.Close()

			client := exigoapi.New(server.URL, testCreds())
			result, err := client.Call(context.Background(), "CreateCustomer", nil)

			var businessErr *exigoapi.BusinessError
			if !errors.As(err, &businessErr) {
				t.Fatalf("got error %v, want *exigoapi.BusinessError", err)
			}
			if businessErr.Operation != "CreateCustomer" {
				t.Errorf("got operation %q, want CreateCustomer", businessErr.Operation)
			}
			if len(businessErr.Errors) != len(c.want) {
				t.Fatalf("got errors %v, want %v", businessErr.Errors, c.want)
			}
			for i := range c.want {
				if businessErr.Errors[i] != c.want[i] {
					t.Errorf("got errors %v, want %v", businessErr.Errors, c.want)
				}
			}
			if result == nil {
				t.Fatal("expected a non-nil partial Result alongside the BusinessError")
			}
			if len(result.Errors) != len(c.want) {
				t.Errorf("got Result.Errors %v, want %v", result.Errors, c.want)
			}
		})
	}
}

func TestCallSuccessStatusMessageIsNotABusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result": {"status": 0, "message": "Customer created"}}`))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())
	result, err := client.Call(context.Background(), "CreateCustomer", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Errorf("got Errors %v, want none for a success-status message", result.Errors)
	}
}

func TestCallDecodes401AsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Invalid credentials"}`))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())
	result, err := client.Call(context.Background(), "GetCustomers", nil)

	var httpErr *exigoapi.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("got error %v, want *exigoapi.HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d, want 401", httpErr.StatusCode)
	}
	if want := "Invalid credentials"; httpErr.Message != want {
		t.Errorf("got message %q, want %q", httpErr.Message, want)
	}
	if result != nil {
		t.Errorf("got result %v, want nil for a non-2xx response", result)
	}
}

func TestCallGETRetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"recordCount": 0}`))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "GetCustomers", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("got %d attempts, want 3", got)
	}
}

func TestCallGETGivesUpAfterMaxAttempts(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "GetCustomers", nil)

	var unavailable *exigoapi.UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("got error %v, want *exigoapi.UnavailableError", err)
	}
	if unavailable.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want 503", unavailable.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 4 {
		t.Errorf("got %d attempts, want 4", got)
	}
}

func TestCallPOSTDoesNotRetryOn5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())
	_, err := client.Call(context.Background(), "CreateCustomer", map[string]any{"firstName": "Jane"})

	var unavailable *exigoapi.UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("got error %v, want *exigoapi.UnavailableError", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("got %d attempts, want exactly 1 (mutating calls must not retry on 5xx)", got)
	}
}

func TestCallRejectsUncataloguedOperationsWithoutRequest(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, testCreds())

	t.Run("unknown operation", func(t *testing.T) {
		result, err := client.Call(context.Background(), "GetCustomerz", nil)
		var unknownErr *exigoapi.UnknownOperationError
		if !errors.As(err, &unknownErr) {
			t.Fatalf("got error %v, want *exigoapi.UnknownOperationError", err)
		}
		if unknownErr.Operation != "GetCustomerz" {
			t.Errorf("got operation %q, want GetCustomerz", unknownErr.Operation)
		}
		if result != nil {
			t.Errorf("got result %v, want nil", result)
		}
	})

	t.Run("catalogued without REST binding", func(t *testing.T) {
		result, err := client.Call(context.Background(), "ProcessTransaction", nil)
		var unsupportedErr *exigoapi.UnsupportedOperationError
		if !errors.As(err, &unsupportedErr) {
			t.Fatalf("got error %v, want *exigoapi.UnsupportedOperationError", err)
		}
		if unsupportedErr.Operation != "ProcessTransaction" {
			t.Errorf("got operation %q, want ProcessTransaction", unsupportedErr.Operation)
		}
		if result != nil {
			t.Errorf("got result %v, want nil", result)
		}
	})

	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("server was hit %d times, want 0 (operation routing must fail before any HTTP request)", got)
	}
}
