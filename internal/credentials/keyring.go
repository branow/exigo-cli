package credentials

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/branow/exigo-cli/internal/iostreams"
	"github.com/branow/gokey"
)

const keyringService = "exigo-cli"

// KeyringStore persists credentials in the OS credential store, falling
// back to a plaintext file and warning on stderr when the machine has
// none.
type KeyringStore struct {
	streams  *iostreams.IOStreams
	fallback *PlaintextStore

	// canPrompt is whether a call may raise the OS authorization dialog.
	// It is decided once, where the terminal is known.
	canPrompt bool

	// The credential store calls are fields so tests drive every branch
	// without touching the machine's real keychain. prompt is whether that
	// one call may raise the dialog.
	get    func(service, account string, prompt bool) (string, error)
	set    func(service, account, secret string, prompt bool) error
	remove func(service, account string, prompt bool) error

	// dropLegacy deletes an entry another program owns; see Set.
	dropLegacy func(service, account string) error
}

// NewKeyringStore returns a KeyringStore that warns via streams and falls
// back to fallbackPath when the machine has no OS credential store. The
// keychain may raise its authorization dialog only when a human is
// watching the terminal: unattended, a dialog nobody can see would hang
// the command instead of failing it.
func NewKeyringStore(streams *iostreams.IOStreams, fallbackPath string) *KeyringStore {
	return &KeyringStore{
		streams:   streams,
		fallback:  NewPlaintextStore(fallbackPath),
		canPrompt: streams.CanPrompt(),
		get: func(service, account string, prompt bool) (string, error) {
			return gokey.Get(service, account, gokey.WithPrompt(prompt))
		},
		set: func(service, account, secret string, prompt bool) error {
			return gokey.Set(service, account, secret, gokey.WithPrompt(prompt))
		},
		remove: func(service, account string, prompt bool) error {
			return gokey.Delete(service, account, gokey.WithPrompt(prompt))
		},

		dropLegacy: deleteWithSecurityTool,
	}
}

// Get returns the stored credentials for profile. A profile absent from
// the credential store is also looked up in the plaintext fallback — its
// entry may have been written there while no store was available.
func (s *KeyringStore) Get(profile string) (Credentials, error) {
	// Read silently first: an entry this binary owns needs no dialog, and
	// only a refusal tells that it does not.
	value, err := s.get(keyringService, profile, false)
	approved := false
	if errors.Is(err, gokey.ErrBlocked) && s.canPrompt {
		value, err = s.get(keyringService, profile, true)
		approved = err == nil
	}
	switch {
	case errors.Is(err, gokey.ErrNotFound):
		return s.fallback.Get(profile)
	case errors.Is(err, gokey.ErrBlocked):
		return Credentials{}, blockedError("read", profile, err)
	case errors.Is(err, gokey.ErrUnavailable):
		s.warnFallback(err)
		return s.fallback.Get(profile)
	case err != nil:
		return Credentials{}, err
	}
	plain, legacy, err := decodeLegacy(value)
	if err != nil {
		return Credentials{}, fmt.Errorf("decoding keychain entry for profile %q: %w", profile, err)
	}
	creds, err := decodeCredentials(plain)
	if err != nil {
		return Credentials{}, err
	}
	// An entry that needed approval belongs to another program - a
	// go-keyring version of this CLI, another build of it - and would ask
	// again on every run until this binary stores it again.
	if legacy || approved {
		s.migrate(profile, creds)
	}
	return creds, nil
}

// Set stores creds for profile, overwriting any existing entry.
func (s *KeyringStore) Set(profile string, creds Credentials) error {
	value, err := encodeCredentials(creds)
	if err != nil {
		return err
	}
	err = s.set(keyringService, profile, value, s.canPrompt)
	if isOwnerEdit(err) {
		// The keychain will not let this binary replace an item another
		// program owns: every entry the go-keyring versions of this CLI
		// wrote through /usr/bin/security. Its owner deletes it without
		// asking, and the new item is then bound to this binary.
		if s.dropLegacy(keyringService, profile) == nil {
			err = s.set(keyringService, profile, value, s.canPrompt)
		}
	}
	switch {
	case errors.Is(err, gokey.ErrBlocked):
		return blockedError("replace", profile, err)
	case errors.Is(err, gokey.ErrUnavailable):
		s.warnFallback(err)
		return s.fallback.Set(profile, creds)
	case err != nil:
		return err
	}
	return nil
}

// Delete removes the stored credentials for profile, from the plaintext
// fallback when the credential store has no entry (see Get).
func (s *KeyringStore) Delete(profile string) error {
	switch err := s.remove(keyringService, profile, s.canPrompt); {
	case errors.Is(err, gokey.ErrNotFound):
		return s.fallback.Delete(profile)
	case errors.Is(err, gokey.ErrBlocked):
		return blockedError("delete", profile, err)
	case errors.Is(err, gokey.ErrUnavailable):
		s.warnFallback(err)
		return s.fallback.Delete(profile)
	case err != nil:
		return err
	}
	return nil
}

// migrate stores an entry another program owns again, as plain JSON bound
// to this binary (see Set), so the next read raises no dialog. Failing costs only that
// dialog on later reads, so it warns instead of failing the read.
func (s *KeyringStore) migrate(profile string, creds Credentials) {
	if err := s.Set(profile, creds); err != nil {
		fmt.Fprintf(s.streams.ErrOut, "warning: could not re-save the keychain entry for profile %q, so macOS will keep asking for permission: %v\n", profile, err)
	}
}

// isOwnerEdit reports the keychain refusing to replace an item another
// program owns (errSecInvalidOwnerEdit). gokey has no sentinel for it, so
// the status it keeps in the message is matched. Only this refusal may
// lead to deleting the old item: after any other failure the retry would
// most likely fail too, and the entry would be gone.
func isOwnerEdit(err error) bool {
	return err != nil && strings.Contains(err.Error(), "OSStatus -25244")
}

// deleteWithSecurityTool deletes an item through /usr/bin/security, the
// owner of every entry go-keyring wrote. It only deletes: the item's
// address is passed, and no secret is read or written through it.
func deleteWithSecurityTool(service, account string) error {
	out, err := exec.Command("/usr/bin/security", "delete-generic-password", "-s", service, "-a", account).CombinedOutput()
	if err != nil {
		return fmt.Errorf("security delete-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
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

// The prefixes go-keyring put on every value it stored on macOS, which is
// every entry written before this CLI moved to gokey. gokey returns the
// stored bytes as they are, so they are decoded here.
const (
	legacyBase64Prefix = "go-keyring-base64:"
	legacyHexPrefix    = "go-keyring-encoded:"
)

// decodeLegacy reports whether value is in go-keyring's macOS layout and,
// if so, returns the value it encodes.
func decodeLegacy(value string) (string, bool, error) {
	trimmed := strings.TrimSpace(value)
	if rest, ok := strings.CutPrefix(trimmed, legacyBase64Prefix); ok {
		decoded, err := base64.StdEncoding.DecodeString(rest)
		return string(decoded), true, err
	}
	if rest, ok := strings.CutPrefix(trimmed, legacyHexPrefix); ok {
		decoded, err := hex.DecodeString(rest)
		return string(decoded), true, err
	}
	return value, false, nil
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
