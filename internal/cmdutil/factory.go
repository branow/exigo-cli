// Package cmdutil bundles the dependencies every command needs — IO
// streams, config, a client constructor, and a credentials store — into
// one Factory so commands take it as a constructor argument instead of
// reaching for globals, keeping them unit-testable.
package cmdutil

import (
	"exigo-cli/internal/config"
	"exigo-cli/internal/credentials"
	"exigo-cli/internal/exigoapi"
	"exigo-cli/internal/iostreams"
)

// Factory bundles the dependencies commands need, plus the resolved
// values of exigo-cli's global persistent flags.
type Factory struct {
	IOStreams        *iostreams.IOStreams
	Config           *config.Config
	CredentialsStore credentials.Store
	ClientFn         func() (exigoapi.Client, error)

	Profile string
	Output  string
	NoInput bool
	Quiet   bool
	Force   bool
}

// ActiveProfile resolves which profile this invocation targets, applying
// the --profile flag over config/env/default precedence.
func (f *Factory) ActiveProfile() string {
	return f.Config.CurrentProfile(f.Profile)
}

// OutputFormat resolves which output format this invocation should use,
// applying the -o/--output flag over config/env/default precedence.
func (f *Factory) OutputFormat() string {
	return f.Config.OutputFormat(f.Output, f.ActiveProfile())
}
