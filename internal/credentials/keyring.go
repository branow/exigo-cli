package credentials

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/branow/exigo-cli/internal/iostreams"
)

const keyringService = "exigo-cli"

// KeyringStore persists credentials in the OS keychain, falling back to a
// plaintext file and warning on stderr when no keychain backend is
// available.
type KeyringStore struct {
	streams  *iostreams.IOStreams
	fallback *PlaintextStore

	// The keychain operations are fields so tests drive every branch
	// without touching the machine's real keychain.
	ui     ui
	get    func(service, account string, allow ui) (string, error)
	set    func(service, account, secret string, allow ui) error
	remove func(service, account string, allow ui) error
}

// NewKeyringStore returns a KeyringStore that warns via streams and falls
// back to fallbackPath when the OS keychain is unavailable. The keychain
// may raise its authorization dialog only when a human is watching the
// terminal: unattended, a dialog nobody can see would hang the command
// instead of failing it.
func NewKeyringStore(streams *iostreams.IOStreams, fallbackPath string) *KeyringStore {
	allow := noUI
	if streams.CanPrompt() {
		allow = allowUI
	}
	return &KeyringStore{
		streams:  streams,
		fallback: NewPlaintextStore(fallbackPath),
		ui:       allow,
		get:      itemGet,
		set:      itemSet,
		remove:   itemDelete,
	}
}

// Get returns the stored credentials for profile. A profile absent from
// the keychain is also looked up in the plaintext fallback — its entry may
// have been written there while the keychain was unavailable.
func (s *KeyringStore) Get(profile string) (Credentials, error) {
	value, err := s.get(keyringService, profile, s.ui)
	switch {
	case errors.Is(err, errMissing):
		return s.fallback.Get(profile)
	case errors.Is(err, errBlocked):
		return Credentials{}, blockedError("read", profile, err)
	case err != nil:
		s.warnFallback(err)
		return s.fallback.Get(profile)
	}
	return decodeCredentials(value)
}

// Set stores creds for profile, overwriting any existing entry.
func (s *KeyringStore) Set(profile string, creds Credentials) error {
	value, err := encodeCredentials(creds)
	if err != nil {
		return err
	}
	switch err := s.set(keyringService, profile, value, s.ui); {
	case errors.Is(err, errBlocked):
		return blockedError("replace", profile, err)
	case err != nil:
		s.warnFallback(err)
		return s.fallback.Set(profile, creds)
	}
	return nil
}

// Delete removes the stored credentials for profile, from the plaintext
// fallback when the keychain has no entry (see Get).
func (s *KeyringStore) Delete(profile string) error {
	switch err := s.remove(keyringService, profile, s.ui); {
	case errors.Is(err, errMissing):
		return s.fallback.Delete(profile)
	case errors.Is(err, errBlocked):
		return blockedError("delete", profile, err)
	case err != nil:
		s.warnFallback(err)
		return s.fallback.Delete(profile)
	}
	return nil
}

func (s *KeyringStore) warnFallback(cause error) {
	fmt.Fprintf(s.streams.ErrOut, "warning: OS keychain unavailable (%v); falling back to plaintext credential storage\n", cause)
}

// blockedError reports the one keychain failure the user can act on: the
// entry exists but its access control does not name this binary, which is
// what every entry written by the go-keyring versions of this CLI looks
// like. Re-running the command on a terminal lets the user approve the
// dialog once, after which the entry is rewritten under this binary's own
// identity; deleting it by hand is the way out when no dialog can be shown.
func blockedError(action, profile string, cause error) error {
	return fmt.Errorf("cannot %s the keychain entry for profile %q: %w; approve the keychain prompt on an interactive terminal, or delete the entry with: security delete-generic-password -s %s -a %s",
		action, profile, cause, keyringService, profile)
}

func encodeCredentials(c Credentials) (string, error) {
	data, err := json.Marshal(c)
	return string(data), err
}

func decodeCredentials(value string) (Credentials, error) {
	var c Credentials
	err := json.Unmarshal([]byte(value), &c)
	return c, err
}
