package cmd

import (
	"github.com/spf13/cobra"

	"exigo-cli/internal/cmdutil"
	"exigo-cli/internal/output"
)

// newAuthStatusCmd builds "exigo auth status", reporting whether the
// active profile has stored credentials.
func newAuthStatusCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status for a profile",
		Example: `  exigo auth status
  exigo auth status --profile sandbox
  exigo auth status -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus(f)
		},
	}
}

func runAuthStatus(f *cmdutil.Factory) error {
	profile := f.ActiveProfile()
	creds, err := f.CredentialsStore.Get(profile)
	if err != nil {
		return cmdutil.ErrNotLoggedIn
	}
	baseURL := f.Config.BaseURL("", profile)

	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, struct {
			Profile   string `json:"profile"`
			LoginName string `json:"login_name"`
			Company   string `json:"company"`
			BaseURL   string `json:"base_url"`
		}{profile, creds.LoginName, creds.Company, baseURL})
	}
	return output.WriteTable(f.IOStreams.Out,
		[]string{"PROFILE", "LOGIN NAME", "COMPANY", "BASE URL"},
		[][]string{{profile, creds.LoginName, creds.Company, baseURL}},
	)
}
