package catalog_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/branow/exigo-cli/internal/exigoapi/catalog"
)

func TestLookupMatchesCaseInsensitively(t *testing.T) {
	for _, name := range []string{"GetCustomers", "getcustomers", "GETCUSTOMERS", "getCustomers"} {
		endpoint, ok := catalog.Lookup(name)
		if !ok {
			t.Errorf("Lookup(%q) not found, want the GetCustomers endpoint", name)
			continue
		}
		if endpoint.Method != http.MethodGet || endpoint.Path != "/customers" {
			t.Errorf("Lookup(%q) = %s %s, want GET /customers", name, endpoint.Method, endpoint.Path)
		}
	}
}

func TestLookupUnknownOperation(t *testing.T) {
	if _, ok := catalog.Lookup("NoSuchOperation"); ok {
		t.Error("Lookup(NoSuchOperation) found an endpoint, want not found")
	}
}

func TestLookupUnavailableOperation(t *testing.T) {
	endpoint, ok := catalog.Lookup("ProcessTransaction")
	if !ok {
		t.Fatal("Lookup(ProcessTransaction) not found, want a catalogued entry")
	}
	if !endpoint.Unavailable {
		t.Error("ProcessTransaction should be marked Unavailable (no REST binding)")
	}
}

func TestCanonicalField(t *testing.T) {
	getCustomers, ok := catalog.Lookup("GetCustomers")
	if !ok {
		t.Fatal("Lookup(GetCustomers) not found")
	}
	createCustomer, ok := catalog.Lookup("CreateCustomer")
	if !ok {
		t.Fatal("Lookup(CreateCustomer) not found")
	}

	cases := []struct {
		name     string
		endpoint catalog.Endpoint
		field    string
		want     string
	}{
		{"documented query casing kept", getCustomers, "customerID", "customerID"},
		{"query field recased", getCustomers, "CustomerID", "customerID"},
		{"query field recased from lowercase", getCustomers, "customerid", "customerID"},
		{"documented body casing kept", createCustomer, "firstName", "firstName"},
		{"body field recased", createCustomer, "FIRSTNAME", "firstName"},
		{"undocumented name passes through", getCustomers, "someFutureParam", "someFutureParam"},
		{"undocumented casing preserved", createCustomer, "SomeFutureParam", "SomeFutureParam"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.endpoint.CanonicalField(c.field); got != c.want {
				t.Errorf("CanonicalField(%q) = %q, want %q", c.field, got, c.want)
			}
		})
	}
}

// Every available endpoint must have a known method and a plain URL path.
// Guards against generation artifacts leaking into the embedded catalog —
// a doc-crawl bug once glued the sample's "Authorization:" header line
// onto 16 paths.
func TestCataloguedEndpointsAreWellFormed(t *testing.T) {
	pathShape := regexp.MustCompile(`^(/[A-Za-z0-9._-]+)+$`)
	methods := map[string]bool{
		http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
		http.MethodPatch: true, http.MethodDelete: true,
	}
	for _, name := range catalog.Operations() {
		endpoint, _ := catalog.Lookup(name)
		if endpoint.Unavailable {
			continue
		}
		if !methods[endpoint.Method] {
			t.Errorf("operation %s has unknown method %q", name, endpoint.Method)
		}
		if !pathShape.MatchString(endpoint.Path) {
			t.Errorf("operation %s has a malformed path %q", name, endpoint.Path)
		}
	}
}

// findField returns the field named name (case-insensitively) from fields.
func findField(fields []catalog.Field, name string) (catalog.Field, bool) {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return catalog.Field{}, false
}

func TestEndpointFieldsCarryTypeAndRequired(t *testing.T) {
	create, _ := catalog.Lookup("CreatePaymentCreditCard")
	cases := []struct {
		field        string
		wantType     string
		wantRequired bool
	}{
		{"amount", "Decimal", true},
		{"orderID", "Int32", true},
		{"creditCardNumber", "String", true},
		{"billingName", "String", false},
		{"memo", "String", false},
	}
	for _, c := range cases {
		f, ok := findField(create.Body, c.field)
		if !ok {
			t.Errorf("CreatePaymentCreditCard body missing field %q", c.field)
			continue
		}
		if f.Type != c.wantType || f.Required != c.wantRequired {
			t.Errorf("field %q = {type:%q required:%v}, want {type:%q required:%v}",
				c.field, f.Type, f.Required, c.wantType, c.wantRequired)
		}
	}

	// GET query parameters carry types too.
	getCustomers, _ := catalog.Lookup("GetCustomers")
	if f, ok := findField(getCustomers.Query, "customerID"); !ok || f.Type != "Int32" {
		t.Errorf("GetCustomers query customerID = %+v, want type Int32", f)
	}
}

// Every documented request field must carry a type; the build joins types
// from api-catalog by field name, so an empty type across the board signals
// a broken join (e.g. a moved or renamed source directory).
func TestRequestFieldsCarryTypes(t *testing.T) {
	for _, name := range catalog.Operations() {
		endpoint, _ := catalog.Lookup(name)
		for _, f := range append(append([]catalog.Field{}, endpoint.Query...), endpoint.Body...) {
			if f.Type == "" {
				t.Errorf("operation %s request field %q has no type", name, f.Name)
			}
		}
	}
}

func TestLookupTypeResolvesComplexTypes(t *testing.T) {
	// A field typed as a complex type must be resolvable to its definition
	// so describe can expand it; opaque types (enums) resolve to nothing.
	fields, ok := catalog.LookupType("OrderDetailRequest")
	if !ok || len(fields) == 0 {
		t.Fatalf("LookupType(OrderDetailRequest) = %v, %v; want a non-empty definition", fields, ok)
	}
	if _, ok := findField(fields, "quantity"); !ok {
		t.Errorf("OrderDetailRequest definition missing the quantity field: %+v", fields)
	}
	// Case-insensitive, matching how field types are compared.
	if _, ok := catalog.LookupType("orderdetailrequest"); !ok {
		t.Error("LookupType should match type names case-insensitively")
	}
	if _, ok := catalog.LookupType("NotAType"); ok {
		t.Error("LookupType returned a definition for an unknown type")
	}
}

func TestCatalogLoadsAllOperations(t *testing.T) {
	names := catalog.Operations()
	if got, want := len(names), 252; got != want {
		t.Errorf("got %d catalogued operations, want %d", got, want)
	}
	for _, name := range names {
		if _, ok := catalog.Lookup(name); !ok {
			t.Errorf("Operations() lists %q but Lookup cannot find it", name)
		}
	}
}
