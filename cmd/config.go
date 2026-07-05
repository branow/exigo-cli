package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"exigo-cli/internal/cmdutil"
	"exigo-cli/internal/output"
)

const (
	keyOutput  = "output"
	keyBaseURL = "base-url"
	keyCompany = "company"
)

// newConfigCmd groups the config get/set/list subcommands for reading and
// writing exigo-cli's non-secret preferences.
func newConfigCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage exigo-cli configuration",
	}
	cmd.AddCommand(newConfigGetCmd(f), newConfigSetCmd(f), newConfigListCmd(f))
	return cmd
}

func newConfigGetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a configuration value for the active profile",
		Args:  cobra.ExactArgs(1),
		Example: `  exigo config get base-url
  exigo config get output
  exigo config get --profile sandbox company`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(f, args[0])
		},
	}
}

func newConfigSetCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value for the active profile",
		Args:  cobra.ExactArgs(2),
		Example: `  exigo config set output json
  exigo config set base-url https://acme-api.exigo.com/3.0
  exigo config set --profile sandbox company SANDBOX`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(f, args[0], args[1])
		},
	}
}

func newConfigListCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		Example: `  exigo config list
  exigo config list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigList(f)
		},
	}
}

func runConfigGet(f *cmdutil.Factory, key string) error {
	value, err := configValue(f, f.ActiveProfile(), key)
	if err != nil {
		return err
	}
	fmt.Fprintln(f.IOStreams.Out, value)
	return nil
}

func configValue(f *cmdutil.Factory, profile, key string) (string, error) {
	switch key {
	case keyOutput:
		return f.Config.OutputFormat("", profile), nil
	case keyBaseURL:
		return f.Config.BaseURL("", profile), nil
	case keyCompany:
		return f.Config.Company("", profile), nil
	default:
		return "", unknownConfigKeyError(key)
	}
}

// validOutputFormat reports whether value names a supported -o/--output
// format.
func validOutputFormat(value string) bool {
	return value == "table" || value == "json"
}

func runConfigSet(f *cmdutil.Factory, key, value string) error {
	profile := f.ActiveProfile()
	p := f.Config.Profiles[profile]
	switch key {
	case keyOutput:
		if !validOutputFormat(value) {
			return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid output format %q (expected table or json)", value)}
		}
		p.Output = value
	case keyBaseURL:
		p.BaseURL = value
	case keyCompany:
		p.Company = value
	default:
		return unknownConfigKeyError(key)
	}
	f.Config.SetProfile(profile, p)
	return f.Config.Save()
}

func runConfigList(f *cmdutil.Factory) error {
	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, f.Config.Profiles)
	}

	current := f.ActiveProfile()
	names := make([]string, 0, len(f.Config.Profiles))
	for name := range f.Config.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		p := f.Config.Profiles[name]
		marker := ""
		if name == current {
			marker = "*"
		}
		rows = append(rows, []string{marker, name, p.BaseURL, p.Company, p.Output})
	}
	return output.WriteTable(f.IOStreams.Out, []string{"", "PROFILE", "BASE URL", "COMPANY", "OUTPUT"}, rows)
}

func unknownConfigKeyError(key string) error {
	return &cmdutil.ValidationError{Message: fmt.Sprintf("unknown config key %q (expected one of: output, base-url, company)", key)}
}
