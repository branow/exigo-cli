package catalog_test

import (
	"net/http"
	"regexp"
	"testing"

	"exigo-cli/internal/exigoapi/catalog"
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
