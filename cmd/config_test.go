package cmd_test

import (
	"strings"
	"testing"

	"github.com/branow/exigo-cli/cmd"
	"github.com/branow/exigo-cli/internal/cmdutil"
)

func TestConfigSetGetRoundTrip(t *testing.T) {
	f, out, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "base-url", "https://api.exigo.com"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}

	root = cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "get", "base-url"})
	if err := root.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := out.String(); got != "https://api.exigo.com\n" {
		t.Errorf("got %q, want %q", got, "https://api.exigo.com\n")
	}
}

func TestConfigSetDoesNotSwitchActiveProfile(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "--profile", "sandbox", "output", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := f.Config.CurrentProfile(""); got != "default" {
		t.Errorf("config set switched the active profile to %q", got)
	}
	if got := f.Config.OutputFormat("", "sandbox"); got != "json" {
		t.Errorf("got output %q for profile sandbox, want json", got)
	}
}

func TestConfigSetRejectsMalformedBaseURL(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "base-url", "sandbox1.exigo.com/3.0"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for a scheme-less base URL")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestConfigSetWarnsOnUnversionedBaseURL(t *testing.T) {
	f, _, errOut := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "base-url", "https://sandbox1.exigo.com/"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(errOut.String(), "version path") {
		t.Errorf("expected a warning about the missing /3.0 version path, got %q", errOut.String())
	}
}

func TestConfigSetRejectsInvalidOutputFormat(t *testing.T) {
	f, _, _ := newTestFactory(t, "")

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "set", "output", "yaml"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected a validation error for an unsupported output format")
	}
	if got := cmdutil.ExitCode(err); got != cmdutil.ExitValidation {
		t.Errorf("got exit code %d, want %d", got, cmdutil.ExitValidation)
	}
}

func TestConfigGetUnknownKey(t *testing.T) {
	f, _, _ := newTestFactory(t, "")
	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "get", "nonsense"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an unknown config key")
	}
}

func TestConfigList(t *testing.T) {
	f, out, _ := newTestFactory(t, "")
	f.Config.SetProfile("sandbox", f.Config.Profiles["sandbox"])

	root := cmd.NewRootCmd(f)
	root.SetArgs([]string{"config", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "sandbox") {
		t.Errorf("expected output to mention profile sandbox, got %q", out.String())
	}
}
