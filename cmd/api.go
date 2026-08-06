package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/branow/exigo-cli/internal/cmdutil"
	"github.com/branow/exigo-cli/internal/exigoapi"
	"github.com/branow/exigo-cli/internal/exigoapi/catalog"
	"github.com/branow/exigo-cli/internal/output"
)

// newAPICmd builds "exigo api", a generic invoker for any of the Exigo
// REST API's catalogued operations (see internal/exigoapi/catalog/),
// without the CLI needing a schema-specific subcommand per operation.
func newAPICmd(f *cmdutil.Factory) *cobra.Command {
	var fields []string
	var inputFile string
	var listOperations bool
	var describe bool

	cmd := &cobra.Command{
		Use:   "api <Operation>",
		Short: "Invoke a named Exigo REST API operation",
		Long: `Invoke a named Exigo REST API operation against the active profile's
endpoint (e.g. GetCustomers, CreateCustomer, UpdateCustomerExtended).
The operation name routes to its REST endpoint via the embedded catalog:
fields become query parameters for GET operations and the JSON request
body otherwise. --list prints every catalogued operation name.

-f/--field values parse as JSON when they look like it (numbers, booleans,
null, arrays, objects) and as plain strings otherwise. --input reads a
full JSON object from a file (or "-" for stdin) — use it for requests with
nested values, like an order's detail lines — and cannot be combined with
-f/--field.

--describe fully documents an operation: its HTTP method, path, and request
and response fields — each with its type and, for request fields, whether it
is required, with nested complex types expanded inline (no credentials or
network needed) — so you know what to pass instead of guessing. Add -o json
for a {name, type, required} shape per field plus a types map defining the
referenced complex types.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  exigo api GetCustomers -f customerID=12345
  exigo api CreateCustomer -f firstName=Jane -f lastName=Doe -f email=jane@example.com
  exigo api CreateOrder --input order.json
  exigo api CreatePaymentCreditCard --describe
  exigo api --list`,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return catalog.Operations(), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if listOperations {
				if len(args) != 0 {
					return &cmdutil.ValidationError{Message: "--list cannot be combined with an operation name"}
				}
				return runAPIList(f)
			}
			if len(args) != 1 {
				return &cmdutil.ValidationError{Message: "an operation name is required (run exigo api --list to see all operations)"}
			}
			if describe {
				if len(fields) > 0 || inputFile != "" {
					return &cmdutil.ValidationError{Message: "--describe cannot be combined with --field or --input"}
				}
				return runAPIDescribe(f, args[0])
			}
			return runAPI(cmd.Context(), f, args[0], fields, inputFile)
		},
	}

	cmd.Flags().StringArrayVarP(&fields, "field", "f", nil, "request field as key=value (repeatable)")
	cmd.Flags().StringVar(&inputFile, "input", "", `read request fields as a JSON object from a file ("-" for stdin)`)
	cmd.Flags().BoolVar(&listOperations, "list", false, "list all catalogued operation names")
	cmd.Flags().BoolVar(&describe, "describe", false, "print the operation's method, path, and documented fields")
	return cmd
}

// runAPIList prints every catalogued operation name, one per line (or as a
// JSON array with -o json), needing no credentials or network access.
func runAPIList(f *cmdutil.Factory) error {
	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, catalog.Operations())
	}
	for _, name := range catalog.Operations() {
		fmt.Fprintln(f.IOStreams.Out, name)
	}
	return nil
}

// runAPIDescribe prints an operation's REST binding and documented field
// names from the embedded catalog, needing no credentials or network. It
// answers "which fields does this operation take?" so users compose calls
// instead of guessing.
func runAPIDescribe(f *cmdutil.Factory, operation string) error {
	endpoint, ok := catalog.Lookup(operation)
	if !ok {
		return &exigoapi.UnknownOperationError{Operation: operation}
	}
	name := catalog.CanonicalOperation(operation)

	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, describeView(name, endpoint))
	}

	out := f.IOStreams.Out
	if endpoint.Unavailable {
		fmt.Fprintf(out, "%s\n  no REST binding available for this operation yet\n", name)
		return nil
	}
	fmt.Fprintf(out, "%s\n  %s %s\n\n", name, endpoint.Method, endpoint.Path)
	printSection(out, requestLabel(endpoint), requestFields(endpoint), true)
	fmt.Fprintln(out)
	printSection(out, "Response", endpoint.Response, false)
	return nil
}

// requestLabel names the request section by how the operation carries
// fields: query parameters for GETs, a JSON body otherwise.
func requestLabel(e catalog.Endpoint) string {
	if strings.EqualFold(e.Method, "GET") {
		return "Request (query)"
	}
	return "Request (body)"
}

func requestFields(e catalog.Endpoint) []catalog.Field {
	if strings.EqualFold(e.Method, "GET") {
		return e.Query
	}
	return e.Body
}

// printSection prints one labelled field section, expanding nested complex
// types recursively. showRequired adds the required-or-optional column,
// meaningful only for request fields.
func printSection(w io.Writer, label string, fields []catalog.Field, showRequired bool) {
	fmt.Fprintf(w, "  %s:\n", label)
	if len(fields) == 0 {
		fmt.Fprintln(w, "    (none)")
		return
	}
	printFields(w, fields, "    ", showRequired, map[string]bool{})
}

// printFields renders a block of sibling fields in aligned columns, then
// recurses into any field whose type is a catalogued complex type, indented
// one level deeper. expanding tracks the types open on the current branch
// so a self-referential type does not recurse forever; sibling reuse of a
// type still expands because the entry is cleared after the branch.
func printFields(w io.Writer, fields []catalog.Field, indent string, showRequired bool, expanding map[string]bool) {
	nameW, typeW := 0, 0
	for _, f := range fields {
		nameW = max(nameW, len(f.Name))
		typeW = max(typeW, len(f.Type))
	}
	for _, f := range fields {
		line := indent + fmt.Sprintf("%-*s", nameW, f.Name)
		if typeW > 0 {
			line += "  " + fmt.Sprintf("%-*s", typeW, f.Type)
		}
		if showRequired {
			line += "  " + requiredLabel(f.Required)
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))

		base := strings.TrimSuffix(f.Type, "[]")
		if nested, ok := catalog.LookupType(base); ok && !expanding[base] {
			expanding[base] = true
			printFields(w, nested, indent+"  ", showRequired, expanding)
			delete(expanding, base)
		}
	}
}

func requiredLabel(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

// describeView is the JSON shape of --describe -o json: the catalog
// endpoint plus the resolved operation name and a types map defining every
// complex type its fields reference (transitively), so a machine can
// resolve nested shapes without re-deriving them.
func describeView(name string, e catalog.Endpoint) map[string]any {
	view := map[string]any{"operation": name}
	if e.Unavailable {
		view["unavailable"] = true
		return view
	}
	view["method"] = e.Method
	view["path"] = e.Path
	if len(e.Query) > 0 {
		view["query"] = e.Query
	}
	if len(e.Body) > 0 {
		view["body"] = e.Body
	}
	if len(e.Response) > 0 {
		view["response"] = e.Response
	}
	types := map[string][]catalog.Field{}
	collectReferencedTypes(e.Query, types)
	collectReferencedTypes(e.Body, types)
	collectReferencedTypes(e.Response, types)
	if len(types) > 0 {
		view["types"] = types
	}
	return view
}

// collectReferencedTypes walks fields and records the definition of every
// catalogued complex type they reference, recursing through nested types.
// The acc map both accumulates results and guards against cycles.
func collectReferencedTypes(fields []catalog.Field, acc map[string][]catalog.Field) {
	for _, f := range fields {
		base := strings.TrimSuffix(f.Type, "[]")
		if _, seen := acc[base]; seen {
			continue
		}
		if def, ok := catalog.LookupType(base); ok {
			acc[base] = def
			collectReferencedTypes(def, acc)
		}
	}
}

func runAPI(ctx context.Context, f *cmdutil.Factory, operation string, fields []string, inputFile string) error {
	if len(fields) > 0 && inputFile != "" {
		return &cmdutil.ValidationError{Message: "--field and --input cannot be combined"}
	}
	// Validate the operation name before touching credentials, so a typo
	// is reported as such even when the user is not logged in.
	if endpoint, ok := catalog.Lookup(operation); !ok {
		return &exigoapi.UnknownOperationError{Operation: operation}
	} else if endpoint.Unavailable {
		return &exigoapi.UnsupportedOperationError{Operation: operation}
	}

	requestFields, err := buildRequestFields(fields, inputFile, f.IOStreams.In)
	if err != nil {
		return err
	}

	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	result, callErr := client.Call(ctx, operation, requestFields)
	if result != nil {
		if err := output.WriteJSON(f.IOStreams.Out, result.Fields); err != nil {
			return err
		}
	}
	return callErr
}

// buildRequestFields assembles the operation's request fields: -f/--field
// values as key=value pairs, or --input's JSON object.
func buildRequestFields(fields []string, inputFile string, stdin io.Reader) (map[string]any, error) {
	if inputFile != "" {
		return readInputFields(inputFile, stdin)
	}
	return parseFields(fields)
}

func parseFields(fields []string) (map[string]any, error) {
	pairs := make(map[string]any, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --field %q, expected key=value", field)}
		}
		pairs[key] = typedValue(value)
	}
	return pairs, nil
}

// typedValue converts a --field value to the JSON type it spells: numbers,
// booleans, null, arrays, and objects parse as themselves so customerID=42
// is sent as a JSON number; anything else stays a string.
func typedValue(value string) any {
	var typed any
	if err := json.Unmarshal([]byte(value), &typed); err == nil {
		return typed
	}
	return value
}

func readInputFields(inputFile string, stdin io.Reader) (map[string]any, error) {
	data, err := readInput(inputFile, stdin)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("--input must be a JSON object: %v", err)}
	}
	return fields, nil
}

func readInput(inputFile string, stdin io.Reader) ([]byte, error) {
	if inputFile == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(inputFile)
}
