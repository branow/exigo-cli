// Package catalog maps Exigo operation names to their REST endpoints. The
// embedded catalog is generated from the crawled API docs by
// scripts/build-rest-catalog.mjs; regenerate it there rather than editing
// rest-catalog.json by hand.
package catalog

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed rest-catalog.json
var catalogJSON []byte

// Endpoint describes one operation's REST binding.
type Endpoint struct {
	// Method is the HTTP method, empty when Unavailable.
	Method string `json:"method"`
	// Path is the endpoint path relative to the versioned base URL
	// (e.g. "/orders/calculate").
	Path string `json:"path"`
	// Query lists the documented query parameter names (GET operations).
	Query []string `json:"query"`
	// Body lists the documented request body field names (mutating
	// operations).
	Body []string `json:"body"`
	// Unavailable marks operations the API docs list without a REST
	// binding ("Rest call not available for this method yet").
	Unavailable bool `json:"unavailable"`
}

var endpoints, operations = load()

func load() (map[string]Endpoint, []string) {
	var raw map[string]Endpoint
	if err := json.Unmarshal(catalogJSON, &raw); err != nil {
		panic("exigoapi/catalog: embedded rest-catalog.json is invalid: " + err.Error())
	}
	byLower := make(map[string]Endpoint, len(raw))
	names := make([]string, 0, len(raw))
	for name, endpoint := range raw {
		byLower[strings.ToLower(name)] = endpoint
		names = append(names, name)
	}
	sort.Strings(names)
	return byLower, names
}

// Lookup returns the REST endpoint for operation, matching the operation
// name case-insensitively.
func Lookup(operation string) (Endpoint, bool) {
	endpoint, ok := endpoints[strings.ToLower(operation)]
	return endpoint, ok
}

// CanonicalField returns the documented casing for a query or body field
// name, matched case-insensitively, so users can pass CustomerID for the
// documented customerID. Undocumented names pass through unchanged — the
// catalog samples may be incomplete and the API is the authority.
func (e Endpoint) CanonicalField(name string) string {
	for _, documented := range e.Query {
		if strings.EqualFold(documented, name) {
			return documented
		}
	}
	for _, documented := range e.Body {
		if strings.EqualFold(documented, name) {
			return documented
		}
	}
	return name
}

// Operations returns all catalogued operation names in their documented
// casing, sorted, for listing and shell completion.
func Operations() []string {
	return operations
}
