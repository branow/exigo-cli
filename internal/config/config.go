// Package config manages exigo-cli's non-secret preferences: named
// profiles (base URL, company, output format) and which one is active,
// resolved with flag > env var > config file > built-in default
// precedence. Credentials never live here; see internal/credentials.
package config

import "os"

// Profile holds the non-secret settings for one named Exigo tenant.
type Profile struct {
	BaseURL string `yaml:"base_url"`
	Company string `yaml:"company"`
	Output  string `yaml:"output"`
}

// Config is the on-disk preferences file: the active profile name and the
// set of named profiles.
type Config struct {
	CurrentProfileName string             `yaml:"current_profile"`
	Profiles           map[string]Profile `yaml:"profiles"`
}

const (
	defaultProfileName = "default"
	defaultOutput      = "table"
)

// New returns an empty Config, as used when no config file exists yet.
func New() *Config {
	return &Config{Profiles: map[string]Profile{}}
}

// CurrentProfile resolves the active profile name: flagOverride (if
// non-empty), then EXIGO_PROFILE, then the config file's stored value,
// then "default".
func (c *Config) CurrentProfile(flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("EXIGO_PROFILE"); v != "" {
		return v
	}
	if c.CurrentProfileName != "" {
		return c.CurrentProfileName
	}
	return defaultProfileName
}

// SetProfile stores profile as the named profile's settings and makes it
// the active profile.
func (c *Config) SetProfile(name string, profile Profile) {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = profile
	c.CurrentProfileName = name
}

// SwitchProfile makes name the active profile without changing its stored
// settings. It fails if the profile has never been configured.
func (c *Config) SwitchProfile(name string) error {
	if _, ok := c.Profiles[name]; !ok {
		return &ProfileNotFoundError{Name: name}
	}
	c.CurrentProfileName = name
	return nil
}

// ProfileNotFoundError reports that a named profile has no stored
// settings.
type ProfileNotFoundError struct{ Name string }

func (e *ProfileNotFoundError) Error() string {
	return "profile not found: " + e.Name
}

// BaseURL resolves the base URL for profile: flagOverride, then
// EXIGO_BASE_URL, then the config file's stored value.
func (c *Config) BaseURL(flagOverride, profile string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("EXIGO_BASE_URL"); v != "" {
		return v
	}
	return c.Profiles[profile].BaseURL
}

// Company resolves the tenant company code for profile: flagOverride, then
// EXIGO_COMPANY, then the config file's stored value.
func (c *Config) Company(flagOverride, profile string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("EXIGO_COMPANY"); v != "" {
		return v
	}
	return c.Profiles[profile].Company
}

// OutputFormat resolves the output format for profile: flagOverride, then
// EXIGO_OUTPUT, then the config file's stored value, then "table".
func (c *Config) OutputFormat(flagOverride, profile string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if v := os.Getenv("EXIGO_OUTPUT"); v != "" {
		return v
	}
	if p, ok := c.Profiles[profile]; ok && p.Output != "" {
		return p.Output
	}
	return defaultOutput
}
