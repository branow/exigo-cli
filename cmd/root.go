// Package cmd wires exigo-cli's Cobra command tree to a cmdutil.Factory;
// commands hold no business logic beyond flag parsing and calling into
// internal packages.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"exigo-cli/internal/cmdutil"
	"exigo-cli/internal/config"
	"exigo-cli/internal/credentials"
	"exigo-cli/internal/exigoapi"
	"exigo-cli/internal/iostreams"
)

// Execute builds the real Factory and runs the CLI, returning any error
// for main.go to translate into a process exit code.
func Execute() error {
	f, err := newRealFactory()
	if err != nil {
		return err
	}
	return NewRootCmd(f).Execute()
}

// NewRootCmd builds the root "exigo" command and its full subcommand tree
// around f.
func NewRootCmd(f *cmdutil.Factory) *cobra.Command {
	var noColor bool

	root := &cobra.Command{
		Use:           "exigo",
		Short:         "Command-line interface for the Exigo API",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if f.Output != "" && !validOutputFormat(f.Output) {
				return &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --output %q (expected table or json)", f.Output)}
			}
			f.IOStreams.SetNoColor(noColor)
			f.IOStreams.SetNoInput(f.NoInput)
			return nil
		},
	}

	root.PersistentFlags().StringVarP(&f.Output, "output", "o", "", "output format: table|json")
	root.PersistentFlags().StringVar(&f.Profile, "profile", "", "profile to use")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	root.PersistentFlags().BoolVar(&f.NoInput, "no-input", false, "disable interactive prompts")
	root.PersistentFlags().BoolVarP(&f.Quiet, "quiet", "q", false, "suppress non-essential output")
	// No -f shorthand here: "api" already uses -f for --field, and pflag
	// panics on a shorthand collision when persistent flags are merged in.
	root.PersistentFlags().BoolVar(&f.Force, "force", false, "skip confirmation prompts")

	root.AddCommand(
		newAuthCmd(f),
		newConfigCmd(f),
		newAPICmd(f),
		newCompletionCmd(),
		newVersionCmd(f),
	)
	return root
}

func newRealFactory() (*cmdutil.Factory, error) {
	streams := iostreams.System()
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	credPath, err := credentials.DefaultPlaintextPath()
	if err != nil {
		return nil, err
	}

	f := &cmdutil.Factory{
		IOStreams:        streams,
		Config:           cfg,
		CredentialsStore: credentials.NewKeyringStore(streams, credPath),
	}
	f.ClientFn = func() (exigoapi.Client, error) {
		return newClient(f)
	}
	return f, nil
}

func newClient(f *cmdutil.Factory) (exigoapi.Client, error) {
	profile := f.ActiveProfile()
	creds, err := f.CredentialsStore.Get(profile)
	if err != nil {
		return nil, cmdutil.ErrNotLoggedIn
	}
	endpoint := f.Config.BaseURL("", profile)
	if endpoint == "" {
		endpoint = exigoapi.DefaultEndpoint(creds.Company)
	}
	return exigoapi.New(endpoint, creds), nil
}
