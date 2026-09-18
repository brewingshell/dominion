package tuiclient

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// maxSavedHosts bounds the saved-server list, matching the desktop client.
const maxSavedHosts = 20

// SavedHost is one remembered server in the shared address book.
type SavedHost struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
}

// Config is the on-disk client configuration. It is byte-compatible with the
// desktop client's ~/.config/dominion/client.json: the desktop ignores the
// extra theme key, and the TUI ignores unknown keys.
type Config struct {
	Host   string      `json:"host"`
	Scheme string      `json:"scheme"`
	Saved  []SavedHost `json:"saved,omitempty"`
	Theme  string      `json:"theme,omitempty"`
}

// ConfigPath is the shared client config file.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "dominion", "client.json")
}

// LoadConfig reads the config, defaulting the scheme to http. A missing or
// unreadable file yields defaults rather than an error.
func LoadConfig() Config {
	var c Config
	b, err := os.ReadFile(ConfigPath())
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.Scheme == "" {
		c.Scheme = "http"
	}
	return c
}

// SaveConfig writes the config with owner-only permissions.
func SaveConfig(c Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// SaveHost upserts a named server, keeping the list bounded.
func (c *Config) SaveHost(entry SavedHost) error {
	replaced := false
	for i := range c.Saved {
		if c.Saved[i].Name == entry.Name {
			c.Saved[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		c.Saved = append(c.Saved, entry)
		if len(c.Saved) > maxSavedHosts {
			c.Saved = c.Saved[len(c.Saved)-maxSavedHosts:]
		}
	}
	return SaveConfig(*c)
}

// DeleteHost removes a named server; a missing name is not an error.
func (c *Config) DeleteHost(name string) error {
	out := c.Saved[:0]
	for _, h := range c.Saved {
		if h.Name != name {
			out = append(out, h)
		}
	}
	c.Saved = out
	return SaveConfig(*c)
}
