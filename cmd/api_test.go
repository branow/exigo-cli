package cmd_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"exigo-cli/cmd"
	"exigo-cli/internal/cmdutil"
)

func soapActionOperation(r *http.Request) string {
	action := strings.Trim(r.Header.Get("SOAPAction"), `"`)
	idx := strings.LastIndex(action, "/")
	if idx == -1 {
		return action
	}
	return action[idx+1:]
}

func soapSuccessEnvelope(operation string, fields map[string]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`)
	b.WriteString("<" + operation + `Response xmlns="http://api.exigo.com/">`)
	b.WriteString("<" + operation + "Result>")
	for k, v := range fields {
		b.WriteString("<" + k + ">" + v + "</" + k + ">")
	}
	b.WriteString("<Errors />")
	b.WriteString("</" + operation + "Result>")
	b.WriteString("</" + operation + "Response>")
	b.WriteString("</soap:Body></soap:Envelope>")
	return b.String()
}

func soapFaultEnvelope(code, message string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault>` +
		`<faultcode>` + code + `</faultcode><faultstring>` + message + `</faultstring>` +
		`</soap:Fault></soap:Body></soap:Envelope>`
}

func TestAPIHappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op := soapActionOperation(r)
		if op != "GetCustomer" {
			t.Errorf("got operation %q, want GetCustomer", op)
		}
		w.Write([]byte(soapSuccessEnvelope(op, map[string]string{"CustomerID": "12345", "FirstName": "Jane"})))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomer", "-f", "CustomerID=12345"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), `"CustomerID": "12345"`) {
		t.Errorf("got output %q, want it to contain CustomerID", out.String())
	}
	if !strings.Contains(out.String(), `"FirstName": "Jane"`) {
		t.Errorf("got output %q, want it to contain FirstName", out.String())
	}
}

func TestAPIFieldSentAsRequestElement(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Write([]byte(soapSuccessEnvelope("GetCustomer", nil)))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomer", "-f", "CustomerID=12345"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(gotBody, "<CustomerID>12345</CustomerID>") {
		t.Errorf("request body missing field element: %s", gotBody)
	}
}

func TestAPIBusinessErrorMapsToGenericExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>
<CreateCustomerResponse xmlns="http://api.exigo.com/"><CreateCustomerResult>
<Errors><string>Email already in use</string></Errors>
</CreateCustomerResult></CreateCustomerResponse>
</soap:Body></soap:Envelope>`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "CreateCustomer", "-f", "Email=jane@example.com"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for a business Errors[] response")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitError {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitError)
	}
	if !strings.Contains(out.String(), "Email already in use") {
		t.Errorf("expected the partial result to still be printed, got %q", out.String())
	}
}

func TestAPIFaultMapsToAuthExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(soapFaultEnvelope("soap:Client", "Authentication failed: invalid credentials")))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, server.URL)
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomer"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error for a SOAP fault")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAPINotLoggedIn(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"api", "GetCustomer"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error when no credentials are configured")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}
