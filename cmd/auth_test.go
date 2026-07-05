package cmd_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"exigo-cli/cmd"
	"exigo-cli/internal/cmdutil"
	"exigo-cli/internal/credentials"
)

func TestAuthLoginNonInteractiveThenStatusThenLogout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	f, out, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--login-name", "alice",
		"--company", "ACME",
		"--base-url", server.URL,
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

func TestAuthLoginVerificationRejectsBadCredentialsWithoutStoringThem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Invalid credentials"}`))
	}))
	defer server.Close()

	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--login-name", "alice",
		"--company", "ACME",
		"--base-url", server.URL,
		"--password-stdin",
	})
	f.IOStreams.In = strings.NewReader("wrong\n")
	err := root.Execute()
	if err == nil {
		t.Fatal("expected login to fail against a 401 server")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitAuth {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitAuth)
	}
	if _, err := f.CredentialsStore.Get("default"); err != credentials.ErrNotFound {
		t.Errorf("rejected credentials must not be stored, got %v", err)
	}
}

func TestAuthLoginInconclusiveVerificationStoresCredentialsWithWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // e.g. a tenant where the probe operation itself errors
		w.Write([]byte(`{"message": "An unexpected error has occurred"}`))
	}))
	defer server.Close()

	f, _, errOut := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--login-name", "alice",
		"--company", "ACME",
		"--base-url", server.URL,
		"--password-stdin",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login must succeed on an inconclusive probe: %v", err)
	}
	if !strings.Contains(errOut.String(), "could not verify") {
		t.Errorf("expected a could-not-verify warning, got %q", errOut.String())
	}
	if _, err := f.CredentialsStore.Get("default"); err != nil {
		t.Errorf("credentials must be stored on an inconclusive probe, got %v", err)
	}
}

func TestAuthLoginNoVerifySkipsTheAPICall(t *testing.T) {
	f, _, errOut := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--login-name", "alice",
		"--company", "ACME",
		"--base-url", "https://acme.example.com", // unreachable and unversioned: must not matter
		"--password-stdin",
		"--no-verify",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	if err := root.Execute(); err != nil {
		t.Fatalf("login --no-verify: %v", err)
	}
	if !strings.Contains(errOut.String(), "version path") {
		t.Errorf("expected a warning about the missing /3.0 version path, got %q", errOut.String())
	}
}

func TestAuthLoginRejectsMalformedBaseURL(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{
		"auth", "login",
		"--login-name", "alice",
		"--company", "ACME",
		"--base-url", "sandbox1.exigo.com/3.0", // no scheme
		"--password-stdin",
	})
	f.IOStreams.In = strings.NewReader("s3cret\n")
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for a scheme-less base URL")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
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
