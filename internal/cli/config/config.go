// Package config owns graft's GLOBAL user configuration, persisted as JSON at an
// XDG path (graft/config.json). It is distinct from the per-project .graft/
// store. The CLI's `config get`/`config set` operate on this file directly,
// bypassing the gateway.
//
// Keys (plan 03):
//
//	sync.gitAuto         bool      auto-commit tracking branches vs builtin-git only
//	scope                string    agents (default) | skills | slash
//	providers.enabled    []string  subset of the ten provider ids
//	theme                string    dark | dark-dim | light | colorblind
//	skills.enabled       bool      master switch for the init/sync skill hook (default true)
//	skills.autoInstall   bool      install missing referenced skills without prompting
//	skills.providers     []string  restrict which supporting providers get links
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// SyncConfig holds sync-scoped settings.
type SyncConfig struct {
	GitAuto bool `json:"gitAuto" yaml:"gitAuto"`
}

// ProvidersConfig holds provider selection settings.
type ProvidersConfig struct {
	Enabled []string `json:"enabled" yaml:"enabled"`
}

// SkillsConfig holds skills-scoped settings. Enabled is a pointer so an unset
// value can default to true while an explicit false is preserved.
type SkillsConfig struct {
	Enabled     *bool    `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	AutoInstall bool     `json:"autoInstall" yaml:"autoInstall"`
	Providers   []string `json:"providers" yaml:"providers"`
}

// EnabledOrDefault reports whether the skill hook is enabled (default true).
func (s SkillsConfig) EnabledOrDefault() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

// Config is graft's global user config.
type Config struct {
	Sync      SyncConfig      `json:"sync" yaml:"sync"`
	Scope     string          `json:"scope" yaml:"scope"`
	Providers ProvidersConfig `json:"providers" yaml:"providers"`
	Theme     string          `json:"theme" yaml:"theme"`
	Skills    SkillsConfig    `json:"skills" yaml:"skills"`
}

// Defaults / allowed values.
const (
	DefaultScope = "agents"
	DefaultTheme = "dark"
)

// ValidScopes enumerates the allowed scope values.
func ValidScopes() []string { return []string{"agents", "skills", "slash"} }

// ApplyDefaults normalizes a config, filling unset fields with defaults. It is
// run on both read and write so the persisted form is always complete.
func ApplyDefaults(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Scope == "" {
		c.Scope = DefaultScope
	}
	if c.Theme == "" {
		c.Theme = DefaultTheme
	}
	if c.Providers.Enabled == nil {
		c.Providers.Enabled = []string{}
	}
	if c.Skills.Enabled == nil {
		def := true
		c.Skills.Enabled = &def
	}
	if c.Skills.Providers == nil {
		c.Skills.Providers = []string{}
	}
	return c
}

// Resolver loads and persists the global config.
type Resolver interface {
	Get() (*Config, error)
	Save(*Config) error
	Path() (string, error)
}

// DefaultResolver persists JSON at the XDG config path. ConfigPath overrides the
// location (test seam); when empty an XDG-compliant default is used.
type DefaultResolver struct {
	ConfigPath string
}

// Path resolves the persisted config file location.
func (r *DefaultResolver) Path() (string, error) {
	if r != nil && r.ConfigPath != "" {
		return r.ConfigPath, nil
	}
	return xdg.ConfigFile("graft/config.json")
}

// Get reads persisted config. A missing file resolves to defaults (no error).
func (r *DefaultResolver) Get() (*Config, error) {
	path, err := r.Path()
	if err != nil {
		return nil, fmt.Errorf("config: resolve path: %w", err)
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ApplyDefaults(&Config{}), nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return ApplyDefaults(&cfg), nil
}

// Save writes config (after applying defaults) to the persisted path.
func (r *DefaultResolver) Save(cfg *Config) error {
	path, err := r.Path()
	if err != nil {
		return fmt.Errorf("config: resolve path: %w", err)
	}
	normalized := ApplyDefaults(cfg)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
