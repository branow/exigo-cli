package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"exigo-cli/internal/cmdutil"
	"exigo-cli/internal/exigoapi"
	"exigo-cli/internal/exigoapi/catalog"
	"exigo-cli/internal/output"
)

// newAPICmd builds "exigo api", a generic invoker for any of the Exigo
// REST API's catalogued operations (see internal/exigoapi/catalog/),
// without the CLI needing a schema-specific subcommand per operation.
func newAPICmd(f *cmdutil.Factory) *cobra.Command {
	var fields []string
	var inputFile string
	var listOperations bool

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
-f/--field.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  exigo api GetCustomers -f customerID=12345
  exigo api CreateCustomer -f firstName=Jane -f lastName=Doe -f email=jane@example.com
  exigo api CreateOrder --input order.json
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
			return runAPI(cmd.Context(), f, args[0], fields, inputFile)
		},
	}

	cmd.Flags().StringArrayVarP(&fields, "field", "f", nil, "request field as key=value (repeatable)")
	cmd.Flags().StringVar(&inputFile, "input", "", `read request fields as a JSON object from a file ("-" for stdin)`)
	cmd.Flags().BoolVar(&listOperations, "list", false, "list all catalogued operation names")
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
