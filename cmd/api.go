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
	"exigo-cli/internal/output"
)

// newAPICmd builds "exigo api", a generic invoker for any of the Exigo
// SOAP API's 140+ named operations, without the CLI needing a
// schema-specific subcommand per operation.
func newAPICmd(f *cmdutil.Factory) *cobra.Command {
	var fields []string
	var inputFile string

	cmd := &cobra.Command{
		Use:   "api <Operation>",
		Short: "Invoke a named Exigo SOAP API operation",
		Long: `Invoke a named Exigo SOAP API operation against the active profile's
endpoint (e.g. GetCustomers, CreateCustomer, UpdateCustomerExtended).

-f/--field values become child elements of the operation's request body.
--input reads a JSON object of field name/value pairs from a file (or "-"
for stdin) instead, and cannot be combined with -f/--field.`,
		Args: cobra.ExactArgs(1),
		Example: `  exigo api GetCustomers -f CustomerID=12345
  exigo api CreateCustomer -f FirstName=Jane -f LastName=Doe -f Email=jane@example.com
  exigo api UpdateCustomerExtended --input customer.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAPI(f, cmd.Context(), args[0], fields, inputFile)
		},
	}

	cmd.Flags().StringArrayVarP(&fields, "field", "f", nil, "request field as key=value (repeatable)")
	cmd.Flags().StringVar(&inputFile, "input", "", `read request fields as a JSON object from a file ("-" for stdin)`)
	return cmd
}

func runAPI(f *cmdutil.Factory, ctx context.Context, operation string, fields []string, inputFile string) error {
	if len(fields) > 0 && inputFile != "" {
		return &cmdutil.ValidationError{Message: "--field and --input cannot be combined"}
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
// values as key=value pairs, or --input's JSON object of field values.
func buildRequestFields(fields []string, inputFile string, stdin io.Reader) (map[string]string, error) {
	if inputFile != "" {
		return readInputFields(inputFile, stdin)
	}
	return parseFields(fields)
}

func parseFields(fields []string) (map[string]string, error) {
	pairs := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --field %q, expected key=value", field)}
		}
		pairs[key] = value
	}
	return pairs, nil
}

func readInputFields(inputFile string, stdin io.Reader) (map[string]string, error) {
	data, err := readInput(inputFile, stdin)
	if err != nil {
		return nil, err
	}
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("--input must be a JSON object of string fields: %v", err)}
	}
	return fields, nil
}

func readInput(inputFile string, stdin io.Reader) ([]byte, error) {
	if inputFile == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(inputFile)
}
