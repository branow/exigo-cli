package cmd_test

import (
	"bytes"
	"testing"

	"github.com/branow/exigo-cli/internal/cmdutil"
	"github.com/branow/exigo-cli/internal/config"
	"github.com/branow/exigo-cli/internal/credentials"
	"github.com/branow/exigo-cli/internal/exigoapi"
	"github.com/branow/exigo-cli/internal/iostreams"
)

// newTestFactory returns a Factory wired to in-memory IO, a fake
// credentials store, and (if serverURL is non-empty) a "default" profile
// already logged in against serverURL, for command tests that need no
// real network, filesystem, or OS keychain access.
func newTestFactory(t *testing.T, serverURL string) (f *cmdutil.Factory, out, errOut *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	streams, _, out, errOut := iostreams.Test()
	cfg := config.New()
	credStore := credentials.NewFakeStore()

	if serverURL != "" {
		cfg.SetProfile("default", config.Profile{BaseURL: serverURL})
		credStore.Set("default", credentials.Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"})
	}

	f = &cmdutil.Factory{
		IOStreams:        streams,
		Config:           cfg,
		CredentialsStore: credStore,
	}
	f.ClientFn = func() (exigoapi.Client, error) {
		profile := f.ActiveProfile()
		baseURL := f.Config.BaseURL("", profile)
		creds, err := f.CredentialsStore.Get(profile)
		if err != nil {
			return nil, cmdutil.ErrNotLoggedIn
		}
		return exigoapi.New(baseURL, creds), nil
	}
	return f, out, errOut
}
