package cmd_test

import (
	"strings"
	"testing"

	"exigo-cli/cmd"
	"exigo-cli/internal/cmdutil"
)

func TestAuthLoginNonInteractiveThenStatusThenLogout(t *testing.T) {
	f, out, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--login-name", "alice",
		"--company", "ACME",
		"--base-url", "https://api.exigo.com",
		"--password-stdin",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out.String(), "alice") {
		t.Errorf("expected login confirmation to mention alice, got %q", out.String())
	}
	out.Reset()

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "alice") || !strings.Contains(out.String(), "ACME") {
		t.Errorf("expected status to show login name and company, got %q", out.String())
	}
	out.Reset()

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "logout"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logout: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "status"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error after logout")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
}

func TestAuthLoginRequiresLoginNameNonInteractively(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--company", "ACME",
		"--base-url", "https://api.exigo.com",
		"--password-stdin",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error when --login-name is missing non-interactively")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestAuthSwitchRequiresExistingProfile(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"auth", "switch", "--profile", "sandbox"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected an error switching to an unconfigured profile")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}
