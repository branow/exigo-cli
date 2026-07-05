package catalog_test

import (
	"net/http"
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
