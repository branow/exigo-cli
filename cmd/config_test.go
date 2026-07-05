package cmd_test

import (
	"strings"
	"testing"

	"exigo-cli/cmd"
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
