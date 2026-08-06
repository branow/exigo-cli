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

// Field is one documented query, body, or response field.
type Field struct {
	// Name is the field name in its documented (wire) casing.
	Name string `json:"name"`
	// Type is the documented value type (e.g. "Int32", "String",
	// "DateTime", "CustomerResponse[]"), empty when not documented.
	Type string `json:"type,omitempty"`
	// Required marks request fields the docs do not mark "Optional". It
	// is meaningful only for query and body fields.
	Required bool `json:"required,omitempty"`
}

// Endpoint describes one operation's REST binding.
type Endpoint struct {
	// Method is the HTTP method, empty when Unavailable.
	Method string `json:"method"`
	// Path is the endpoint path relative to the versioned base URL
	// (e.g. "/orders/calculate").
	Path string `json:"path"`
	// Query lists the documented query parameters (GET operations).
	Query []Field `json:"query"`
	// Body lists the documented request body fields (mutating
	// operations).
	Body []Field `json:"body"`
	// Response lists the documented response fields.
	Response []Field `json:"response"`
	// Unavailable marks operations the API docs list without a REST
	// binding ("Rest call not available for this method yet").
	Unavailable bool `json:"unavailable"`
}

// catalogFile is the on-disk shape of rest-catalog.json: the operations
// keyed by name, plus a registry of complex type definitions used to
// expand nested fields.
type catalogFile struct {
	Operations map[string]Endpoint `json:"operations"`
	Types      map[string][]Field  `json:"types"`
}

var endpoints, canonicalNames, operations, typeDefs = load()

func load() (map[string]Endpoint, map[string]string, []string, map[string][]Field) {
	var file catalogFile
	if err := json.Unmarshal(catalogJSON, &file); err != nil {
		panic("exigoapi/catalog: embedded rest-catalog.json is invalid: " + err.Error())
	}
	byLower := make(map[string]Endpoint, len(file.Operations))
	canonical := make(map[string]string, len(file.Operations))
	names := make([]string, 0, len(file.Operations))
	for name, endpoint := range file.Operations {
		byLower[strings.ToLower(name)] = endpoint
		canonical[strings.ToLower(name)] = name
		names = append(names, name)
	}
	sort.Strings(names)
	types := make(map[string][]Field, len(file.Types))
	for name, fields := range file.Types {
		types[strings.ToLower(name)] = fields
	}
	return byLower, canonical, names, types
}

// Lookup returns the REST endpoint for operation, matching the operation
// name case-insensitively.
func Lookup(operation string) (Endpoint, bool) {
	endpoint, ok := endpoints[strings.ToLower(operation)]
	return endpoint, ok
}

// LookupType returns the field list defining a complex type (e.g.
// "OrderDetailRequest"), matched case-insensitively. It is the registry
// that lets a field typed as another complex type be expanded into its
// own fields. Opaque types (enums, types documented elsewhere) are absent.
func LookupType(name string) ([]Field, bool) {
	fields, ok := typeDefs[strings.ToLower(name)]
	return fields, ok
}

// CanonicalOperation returns the documented casing for operation, matched
// case-insensitively; unknown names pass through unchanged.
func CanonicalOperation(operation string) string {
	if name, ok := canonicalNames[strings.ToLower(operation)]; ok {
		return name
	}
	return operation
}

// CanonicalField returns the documented casing for a query or body field
// name, matched case-insensitively, so users can pass CustomerID for the
// documented customerID. Undocumented names pass through unchanged — the
// catalog samples may be incomplete and the API is the authority.
func (e Endpoint) CanonicalField(name string) string {
	for _, documented := range e.Query {
		if strings.EqualFold(documented.Name, name) {
			return documented.Name
		}
	}
	for _, documented := range e.Body {
		if strings.EqualFold(documented.Name, name) {
			return documented.Name
		}
	}
	return name
}

// Operations returns all catalogued operation names in their documented
// casing, sorted, for listing and shell completion.
func Operations() []string {
	return operations
}
