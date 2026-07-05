// Package credentials stores and retrieves Exigo login secrets, backed by
// the OS keychain with a plaintext-file fallback. It is kept separate from
// internal/config so secrets never land in the preferences file.
package credentials

import "errors"

// Credentials are the secret values needed to authenticate against Exigo:
// a login name, password, and tenant company code.
type Credentials struct {
	LoginName string
	Password  string
	Company   string
}

// Store persists and retrieves Credentials per named profile.
type Store interface {
	Get(profile string) (Credentials, error)
	Set(profile string, creds Credentials) error
	Delete(profile string) error
}

// ErrNotFound indicates no credentials are stored for the requested
// profile.
var ErrNotFound = errors.New("no credentials stored for profile")
