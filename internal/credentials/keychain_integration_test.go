//go:build darwin && keychain

// This file is excluded from the normal suite twice over: it needs the
// `keychain` build tag and EXIGO_KEYCHAIN_TEST=1. `go test ./...` must
// never touch the user's login keychain.
//
//	EXIGO_KEYCHAIN_TEST=1 go test -tags keychain ./internal/credentials/ -run Keychain -v
package credentials

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestKeychainRoundTrip exercises the real Security framework: add, read
// back, replace, delete. It files its items under a service of its own so
// it can never read, change or delete an entry a real profile uses.
func TestKeychainRoundTrip(t *testing.T) {
	if os.Getenv("EXIGO_KEYCHAIN_TEST") != "1" {
		t.Skip("set EXIGO_KEYCHAIN_TEST=1 to run against the real keychain")
	}
	service := fmt.Sprintf("exigo-cli-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	account := "integration"
	t.Cleanup(func() {
		if err := itemDelete(service, account, allowUI); err != nil && !errors.Is(err, errMissing) {
			t.Errorf("cleanup: %v", err)
		}
	})

	if _, err := itemGet(service, account, allowUI); !errors.Is(err, errMissing) {
		t.Fatalf("error before the item exists = %v, want errMissing", err)
	}
	if err := itemSet(service, account, "first", allowUI); err != nil {
		t.Fatalf("itemSet: %v", err)
	}
	secret, err := itemGet(service, account, allowUI)
	if err != nil || secret != "first" {
		t.Fatalf("secret = %q, err = %v", secret, err)
	}

	// A second Set must replace the value rather than fail on a duplicate.
	if err := itemSet(service, account, "second", allowUI); err != nil {
		t.Fatalf("itemSet over an existing item: %v", err)
	}
	secret, err = itemGet(service, account, allowUI)
	if err != nil || secret != "second" {
		t.Fatalf("secret after replacement = %q, err = %v", secret, err)
	}

	if err := itemDelete(service, account, allowUI); err != nil {
		t.Fatalf("itemDelete: %v", err)
	}
	if _, err := itemGet(service, account, allowUI); !errors.Is(err, errMissing) {
		t.Fatalf("error after delete = %v, want errMissing", err)
	}
}
