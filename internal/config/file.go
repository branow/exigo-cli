package config

import (
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Dir returns the base directory exigo-cli stores its files under:
// XDG_CONFIG_HOME when set, the platform's native config directory on
// Windows (%AppData%\exigo), and ~/.config/exigo elsewhere — macOS
// included, following the ~/.config convention of CLI tools like gh
// rather than ~/Library/Application Support.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "exigo"), nil
	}
	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "exigo"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "exigo"), nil
}

// Path returns the config file's location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yml"), nil
}

// Load reads the config file, returning a fresh empty Config if it does
// not exist yet.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	c := New()
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	return c, nil
}

// Save writes the config to disk, creating its parent directory if
// needed.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
