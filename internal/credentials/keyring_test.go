package credentials

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/branow/exigo-cli/internal/iostreams"
	"github.com/branow/gokey"
)

// fakeKeychain stands in for the OS credential store: a map, plus an
// error every operation returns instead of touching it. That is what
// drives the fallback and authorization branches with no real keychain
// present.
type fakeKeychain struct {
	items map[string]string
	err   error

	// foreign marks items another program owns, as /usr/bin/security owns
	// every entry go-keyring wrote: get releases them only with a prompt,
	// set cannot replace them, and only dropLegacy removes them.
	foreign map[string]bool
}

func newFakeKeychain() *fakeKeychain {
	return &fakeKeychain{items: map[string]string{}, foreign: map[string]bool{}}
}

var errOwnerEdit = errors.New("the keychain refused the request (OSStatus -25244)")

func (k *fakeKeychain) get(_, account string, prompt bool) (string, error) {
	if k.err != nil {
		return "", k.err
	}
	if k.foreign[account] && !prompt {
		return "", gokey.ErrBlocked
	}
	value, ok := k.items[account]
	if !ok {
		return "", gokey.ErrNotFound
	}
	return value, nil
}

func (k *fakeKeychain) set(_, account, secret string, _ bool) error {
	if k.err != nil {
		return k.err
	}
	if k.foreign[account] {
		return errOwnerEdit
	}
	k.items[account] = secret
	return nil
}

func (k *fakeKeychain) remove(_, account string, _ bool) error {
	if k.err != nil {
		return k.err
	}
	if _, ok := k.items[account]; !ok {
		return gokey.ErrNotFound
	}
	delete(k.items, account)
	return nil
}

// newTestStore returns a KeyringStore backed by keychain, its plaintext
// fallback path, and the buffer its warnings go to. It runs as if at a
// terminal, so a foreign entry is released after approval.
func newTestStore(t *testing.T, keychain *fakeKeychain) (*KeyringStore, string, *bytes.Buffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.yml")
	streams, _, _, errOut := iostreams.Test()
	store := NewKeyringStore(streams, path)
	store.canPrompt = true
	store.get, store.set, store.remove = keychain.get, keychain.set, keychain.remove
	store.dropLegacy = func(_, account string) error {
		delete(keychain.foreign, account)
		return keychain.remove("", account, false)
	}
	return store, path, errOut
}

func TestKeyringRoundTripsThroughTheKeychain(t *testing.T) {
	store, path, _ := newTestStore(t, newFakeKeychain())

	want := Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"}
	if err := store.Set("default", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// The keychain took the entry, so nothing may have been written to the
	// plaintext file.
	if _, err := NewPlaintextStore(path).Get("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("plaintext fallback error = %v, want ErrNotFound", err)
	}
	if err := store.Delete("default"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
}

// A profile absent from the keychain must still resolve via the plaintext
// fallback — its entry may have been written there while the keychain was
// unavailable.
func TestKeyringGetFallsBackToPlaintextWhenKeychainHasNoEntry(t *testing.T) {
	store, path, _ := newTestStore(t, newFakeKeychain())

	want := Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"}
	if err := NewPlaintextStore(path).Set("default", want); err != nil {
		t.Fatalf("seed plaintext store: %v", err)
	}

	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestKeyringGetReportsNotFoundWhenNeitherStoreHasEntry(t *testing.T) {
	store, _, _ := newTestStore(t, newFakeKeychain())

	if _, err := store.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got error %v, want ErrNotFound", err)
	}
}

func TestKeyringSetFallsBackToPlaintextWhenKeychainIsUnavailable(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.err = fmt.Errorf("%w: no D-Bus session bus", gokey.ErrUnavailable)
	store, path, errOut := newTestStore(t, keychain)

	want := Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"}
	if err := store.Set("default", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := NewPlaintextStore(path).Get("default")
	if err != nil {
		t.Fatalf("plaintext Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !strings.Contains(errOut.String(), "falling back to plaintext") {
		t.Errorf("stderr = %q, want a fallback warning", errOut.String())
	}
}

// A keychain that is present but fails for an unexplained reason is not
// an absent one: writing the secret to a plaintext file instead would
// quietly downgrade where it lives, so the failure surfaces.
func TestKeyringReportsUnexpectedKeychainFailures(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.err = errors.New("the keychain is sulking")
	store, path, errOut := newTestStore(t, keychain)

	creds := Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"}
	operations := map[string]func() error{
		"Get":    func() error { _, err := store.Get("default"); return err },
		"Set":    func() error { return store.Set("default", creds) },
		"Delete": func() error { return store.Delete("default") },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, keychain.err) {
				t.Fatalf("error = %v, want the keychain failure", err)
			}
		})
	}
	if _, err := NewPlaintextStore(path).Get("default"); !errors.Is(err, ErrNotFound) {
		t.Errorf("plaintext fallback error = %v, want ErrNotFound", err)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want no fallback warning", errOut.String())
	}
}

// An entry the keychain holds but will not release is neither a missing
// entry nor an unavailable keychain: silently reading the plaintext file
// instead would hide it, so every operation reports it with its remedy.
func TestKeyringReportsBlockedEntryWithItsRemedy(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.err = fmt.Errorf("%w (OSStatus -25308)", gokey.ErrBlocked)
	store, path, _ := newTestStore(t, keychain)

	seeded := Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"}
	if err := NewPlaintextStore(path).Set("default", seeded); err != nil {
		t.Fatalf("seed plaintext store: %v", err)
	}

	operations := map[string]func() error{
		"Get":    func() error { _, err := store.Get("default"); return err },
		"Set":    func() error { return store.Set("default", seeded) },
		"Delete": func() error { return store.Delete("default") },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if err == nil {
				t.Fatal("a blocked keychain produced no error")
			}
			for _, want := range []string{"default", "security delete-generic-password", keyringService} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

var legacyCreds = Credentials{LoginName: "alice", Password: "s3cret", Company: "ACME"}

// legacyEntry returns legacyCreds as go-keyring stored them on macOS.
func legacyEntry(t *testing.T) string {
	t.Helper()
	plain, err := encodeCredentials(legacyCreds)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(plain))
}

// Entries written before the move to gokey carry go-keyring's macOS
// encoding. They must still read, and are stored again as plain JSON
// owned by this binary.
func TestKeyringReadsAndRewritesGoKeyringEntries(t *testing.T) {
	plain, err := encodeCredentials(legacyCreds)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	legacy := map[string]string{
		"base64": "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(plain)),
		"hex":    "go-keyring-encoded:" + hex.EncodeToString([]byte(plain)),
		"padded": " go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(plain)) + "\n",
	}
	for name, stored := range legacy {
		t.Run(name, func(t *testing.T) {
			keychain := newFakeKeychain()
			keychain.items["default"] = stored
			keychain.foreign["default"] = true
			store, _, errOut := newTestStore(t, keychain)

			got, err := store.Get("default")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got != legacyCreds {
				t.Errorf("got %+v, want %+v", got, legacyCreds)
			}
			if keychain.items["default"] != plain || keychain.foreign["default"] {
				t.Errorf("stored %q (foreign %v) after Get, want it rewritten as %q and owned by this binary", keychain.items["default"], keychain.foreign["default"], plain)
			}
			if errOut.Len() != 0 {
				t.Errorf("stderr = %q, want no warning", errOut.String())
			}
		})
	}
}

func TestKeyringReportsUndecodableGoKeyringEntry(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.items["default"] = "go-keyring-base64:not base64!"
	store, _, _ := newTestStore(t, keychain)

	_, err := store.Get("default")
	if err == nil || !strings.Contains(err.Error(), `profile "default"`) {
		t.Fatalf("error = %v, want a decode error naming the profile", err)
	}
}

// The legacy entry could not be removed: the read still succeeds, and the
// warning says it will keep prompting rather than failing silently.
func TestKeyringWarnsWhenLegacyEntryCannotBeReplaced(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.items["default"] = legacyEntry(t)
	keychain.foreign["default"] = true
	store, _, errOut := newTestStore(t, keychain)
	store.dropLegacy = func(_, _ string) error { return errors.New("exit status 44") }

	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != legacyCreds {
		t.Errorf("got %+v, want %+v", got, legacyCreds)
	}
	if !strings.Contains(errOut.String(), "will keep asking for permission") {
		t.Errorf("stderr = %q, want a warning that macOS will keep asking", errOut.String())
	}
}

// Logging in again over an entry the go-keyring versions wrote must
// replace it, not fail on the keychain's owner check.
func TestKeyringSetReplacesEntryAnotherProgramOwns(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.items["default"] = legacyEntry(t)
	keychain.foreign["default"] = true
	store, _, _ := newTestStore(t, keychain)

	want := Credentials{LoginName: "bob", Password: "n3w", Company: "ACME"}
	if err := store.Set("default", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Only the owner refusal may delete the old entry: after any other
// failure the retry would fail too and the stored login would be lost.
func TestKeyringSetKeepsEntryOnOtherFailures(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.items["default"] = legacyEntry(t)
	keychain.err = errors.New("the keychain is sulking")
	store, _, _ := newTestStore(t, keychain)
	dropped := false
	store.dropLegacy = func(_, _ string) error { dropped = true; return nil }

	if err := store.Set("default", legacyCreds); !errors.Is(err, keychain.err) {
		t.Fatalf("Set error = %v, want the keychain failure", err)
	}
	if dropped {
		t.Error("Set deleted the stored entry after a failure other than the owner refusal")
	}
}

// An entry another build of this CLI stored is released after approval,
// and must be stored again by this binary: otherwise every later run asks
// again.
func TestKeyringRebindsEntryThatNeededApproval(t *testing.T) {
	plain, err := encodeCredentials(legacyCreds)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	keychain := newFakeKeychain()
	keychain.items["default"] = plain
	keychain.foreign["default"] = true
	store, _, errOut := newTestStore(t, keychain)

	got, err := store.Get("default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != legacyCreds {
		t.Errorf("got %+v, want %+v", got, legacyCreds)
	}
	if keychain.foreign["default"] {
		t.Error("entry still owned by another program after an approved read")
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want no warning", errOut.String())
	}
}

// With no terminal there is no one to approve, so the read stops with the
// remedy and leaves the entry alone.
func TestKeyringReportsBlockedEntryWithoutATerminal(t *testing.T) {
	keychain := newFakeKeychain()
	keychain.items["default"] = legacyEntry(t)
	keychain.foreign["default"] = true
	store, _, _ := newTestStore(t, keychain)
	store.canPrompt = false

	if _, err := store.Get("default"); !errors.Is(err, gokey.ErrBlocked) {
		t.Fatalf("Get error = %v, want ErrBlocked", err)
	}
	if !keychain.foreign["default"] || keychain.items["default"] != legacyEntry(t) {
		t.Error("entry replaced without approval")
	}
}
