package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"exigo-cli/internal/cmdutil"
	"exigo-cli/internal/config"
	"exigo-cli/internal/credentials"
	"exigo-cli/internal/exigoapi"
)

// newAuthLoginCmd builds "exigo auth login", supporting both interactive
// prompts and fully non-interactive, scriptable operation via flags so it
// is not a dead end for CI use.
func newAuthLoginCmd(f *cmdutil.Factory) *cobra.Command {
	var loginName, companyCode, baseURL string
	var passwordStdin bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to an Exigo tenant and store credentials",
		Example: `  # Interactive login, prompting for each value
  exigo auth login

  # Non-interactive login for CI, reading the password from stdin
  echo "$EXIGO_PASSWORD" | exigo auth login --login-name svc-account --company ACME --password-stdin

  # Log in under a named profile against a sandbox REST endpoint
  exigo auth login --profile sandbox --login-name dev --company SANDBOX --base-url https://sandboxapi6.exigo.com/3.0 --password-stdin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(f, loginName, companyCode, baseURL, passwordStdin)
		},
	}

	cmd.Flags().StringVar(&loginName, "login-name", "", "Exigo login name")
	cmd.Flags().StringVar(&companyCode, "company", "", "Exigo company (tenant) code")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Exigo REST base URL (default https://<company>-api.exigo.com/3.0)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin")
	return cmd
}

func runAuthLogin(f *cmdutil.Factory, loginName, companyCode, baseURL string, passwordStdin bool) error {
	profile := f.ActiveProfile()
	reader := bufio.NewReader(f.IOStreams.In)

	loginName, err := resolvePromptValue(f, reader, loginName, "Login name")
	if err != nil {
		return err
	}
	companyCode, err = resolvePromptValue(f, reader, companyCode, "Company")
	if err != nil {
		return err
	}
	baseURL, err = resolveBaseURL(f, reader, baseURL, companyCode)
	if err != nil {
		return err
	}
	password, err := resolvePassword(f, reader, passwordStdin)
	if err != nil {
		return err
	}

	creds := credentials.Credentials{LoginName: loginName, Password: password, Company: companyCode}
	if err := f.CredentialsStore.Set(profile, creds); err != nil {
		return err
	}

	f.Config.SetProfile(profile, config.Profile{
		BaseURL: baseURL,
		Company: companyCode,
		Output:  f.Config.OutputFormat("", profile),
	})
	if err := f.Config.SwitchProfile(profile); err != nil {
		return err
	}
	if err := f.Config.Save(); err != nil {
		return err
	}

	if !f.Quiet {
		fmt.Fprintf(f.IOStreams.Out, "Logged in as %s (company %s) using profile %q\n", loginName, companyCode, profile)
	}
	return nil
}

// resolvePromptValue returns flagValue if set, otherwise prompts for it
// interactively, otherwise fails with a validation error naming the flag
// to use non-interactively.
func resolvePromptValue(f *cmdutil.Factory, reader *bufio.Reader, flagValue, label string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if !f.IOStreams.CanPrompt() {
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("%s is required (pass the corresponding flag in non-interactive mode)", label)}
	}
	fmt.Fprintf(f.IOStreams.Out, "%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return "", &cmdutil.ValidationError{Message: fmt.Sprintf("%s is required", label)}
	}
	return value, nil
}

// resolveBaseURL returns flagValue if set, otherwise prompts for the REST
// base URL (offering the company's default host), otherwise falls back to
// the company default directly — unlike the other login values, a missing
// endpoint is never a validation error since the per-company production
// host is a documented, sensible default.
func resolveBaseURL(f *cmdutil.Factory, reader *bufio.Reader, flagValue, companyCode string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	defaultURL := exigoapi.DefaultEndpoint(companyCode)
	if !f.IOStreams.CanPrompt() {
		return defaultURL, nil
	}
	fmt.Fprintf(f.IOStreams.Out, "REST base URL [%s]: ", defaultURL)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if value := strings.TrimSpace(line); value != "" {
		return value, nil
	}
	return defaultURL, nil
}

// resolvePassword reads the password from stdin when passwordStdin is set,
// otherwise prompts with hidden input on a real terminal, otherwise fails
// with a validation error. Only the trailing line ending is stripped —
// whitespace can be part of the password itself.
func resolvePassword(f *cmdutil.Factory, reader *bufio.Reader, passwordStdin bool) (string, error) {
	if passwordStdin {
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return nonEmptyPassword(string(data))
	}
	if stdin := f.IOStreams.StdinFd(); stdin != nil && f.IOStreams.CanPrompt() {
		fmt.Fprint(f.IOStreams.Out, "Password: ")
		bytePassword, err := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(f.IOStreams.Out)
		if err != nil {
			return "", err
		}
		return nonEmptyPassword(string(bytePassword))
	}
	return "", &cmdutil.ValidationError{Message: "password is required (use --password-stdin in non-interactive mode)"}
}

func nonEmptyPassword(raw string) (string, error) {
	password := strings.TrimRight(raw, "\r\n")
	if password == "" {
		return "", &cmdutil.ValidationError{Message: "password is required"}
	}
	return password, nil
}
