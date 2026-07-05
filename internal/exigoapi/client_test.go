package exigoapi_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"exigo-cli/internal/credentials"
	"exigo-cli/internal/exigoapi"
)

const successEnvelope = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <GetCustomerResponse xmlns="http://api.exigo.com/">
      <GetCustomerResult>
        <CustomerID>12345</CustomerID>
        <FirstName>Jane</FirstName>
        <Errors />
      </GetCustomerResult>
    </GetCustomerResponse>
  </soap:Body>
</soap:Envelope>`

func businessErrorEnvelope(messages ...string) string {
	var errs strings.Builder
	for _, m := range messages {
		errs.WriteString("<string>" + m + "</string>")
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <CreateCustomerResponse xmlns="http://api.exigo.com/">
      <CreateCustomerResult>
        <Errors>` + errs.String() + `</Errors>
      </CreateCustomerResult>
    </CreateCustomerResponse>
  </soap:Body>
</soap:Envelope>`
}

const faultEnvelope = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <soap:Fault>
      <faultcode>soap:Server</faultcode>
      <faultstring>Server was unable to process request. Login failed.</faultstring>
    </soap:Fault>
  </soap:Body>
</soap:Envelope>`

func TestCallSendsExpectedEnvelopeAndHeaders(t *testing.T) {
	var gotSOAPAction, gotContentType string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSOAPAction = r.Header.Get("SOAPAction")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(successEnvelope))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, credentials.Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"})
	result, err := client.Call(context.Background(), "GetCustomer", map[string]string{"CustomerID": "12345"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if want := `"http://api.exigo.com/GetCustomer"`; gotSOAPAction != want {
		t.Errorf("got SOAPAction %q, want %q", gotSOAPAction, want)
	}
	if want := "text/xml; charset=utf-8"; gotContentType != want {
		t.Errorf("got Content-Type %q, want %q", gotContentType, want)
	}

	body := string(gotBody)
	for _, want := range []string{
		`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">`,
		`<ApiAuthentication xmlns="http://api.exigo.com/">`,
		`<LoginName>alice</LoginName>`,
		`<Password>s3cret</Password>`,
		`<Company>ACME</Company>`,
		`<GetCustomer xmlns="http://api.exigo.com/">`,
		`<CustomerID>12345</CustomerID>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("request body missing %q\nfull body: %s", want, body)
		}
	}
	if strings.Contains(body, "Identity") || strings.Contains(body, "Signature") {
		t.Errorf("request body should not include speculative auth fields: %s", body)
	}

	if result.Fields["CustomerID"] != "12345" {
		t.Errorf("got CustomerID %v, want 12345", result.Fields["CustomerID"])
	}
	if result.Fields["FirstName"] != "Jane" {
		t.Errorf("got FirstName %v, want Jane", result.Fields["FirstName"])
	}
	if len(result.Errors) != 0 {
		t.Errorf("got Errors %v, want none", result.Errors)
	}
}

func TestCallDecodesBusinessErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(businessErrorEnvelope("Email is already in use", "Invalid state code")))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, credentials.Credentials{})
	result, err := client.Call(context.Background(), "CreateCustomer", nil)

	var businessErr *exigoapi.BusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("got error %v, want *exigoapi.BusinessError", err)
	}
	if businessErr.Operation != "CreateCustomer" {
		t.Errorf("got operation %q, want CreateCustomer", businessErr.Operation)
	}
	want := []string{"Email is already in use", "Invalid state code"}
	if len(businessErr.Errors) != len(want) || businessErr.Errors[0] != want[0] || businessErr.Errors[1] != want[1] {
		t.Errorf("got errors %v, want %v", businessErr.Errors, want)
	}
	if result == nil {
		t.Fatal("expected a non-nil Result alongside the BusinessError")
	}
}

func TestCallDecodesSingleBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(businessErrorEnvelope("Customer not found")))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, credentials.Credentials{})
	_, err := client.Call(context.Background(), "CreateCustomer", nil)

	var businessErr *exigoapi.BusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("got error %v, want *exigoapi.BusinessError", err)
	}
	if len(businessErr.Errors) != 1 || businessErr.Errors[0] != "Customer not found" {
		t.Errorf("got errors %v, want [Customer not found]", businessErr.Errors)
	}
}

func TestCallDecodesSOAPFault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(faultEnvelope))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, credentials.Credentials{})
	_, err := client.Call(context.Background(), "GetCustomer", nil)

	var fault *exigoapi.Fault
	if !errors.As(err, &fault) {
		t.Fatalf("got error %v, want *exigoapi.Fault", err)
	}
	if fault.Code != "soap:Server" {
		t.Errorf("got fault code %q, want soap:Server", fault.Code)
	}
	if !strings.Contains(fault.String, "Login failed") {
		t.Errorf("got fault string %q, want it to mention Login failed", fault.String)
	}
}

func TestCallRetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(successEnvelope))
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, credentials.Credentials{})
	_, err := client.Call(context.Background(), "GetCustomer", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("got %d attempts, want 3", got)
	}
}

func TestCallGivesUpAfterMaxAttempts(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := exigoapi.New(server.URL, credentials.Credentials{})
	_, err := client.Call(context.Background(), "GetCustomer", nil)
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if got := atomic.LoadInt32(&attempts); got != 4 {
		t.Errorf("got %d attempts, want 4", got)
	}
}
