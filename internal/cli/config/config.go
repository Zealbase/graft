// Package config owns graft's GLOBAL user configuration, persisted as JSON at an
// XDG path (graft/config.json). It is distinct from the per-project .graft/
// store. The CLI's `config get`/`config set` operate on this file directly,
// bypassing the gateway.
//
// Keys (plan 03):
//
//	sync.gitAuto         bool      auto-commit tracking branches vs builtin-git only
//	scope                string    agents (default) | skills | slash
//	providers.mode       string    all | specific (default all)
//	providers.enabled    []string  active set when mode=specific
//	providers.disabled   []string  excluded set when mode=all
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
	"sort"

	"github.com/adrg/xdg"
)

// SyncConfig holds sync-scoped settings.
type SyncConfig struct {
	GitAuto bool `json:"gitAuto" yaml:"gitAuto"`
}

// Provider-selection modes.
const (
	ProviderModeAll      = "all"      // every supported provider minus Disabled
	ProviderModeSpecific = "specific" // only the Enabled set
	DefaultProviderMode  = ProviderModeAll
)

// SupportedProviders is the canonical list of the eight active provider ids graft targets.
// Kept here (CLI-local) so the CLI never imports internal/transform (gateway-only
// rule); it mirrors transform.Default()'s registration order, sorted.
// NOTE(2026-06-13): antigravity (agy) is intentionally absent — unregistered pending
// research spike. See tasks/_draft/antigravity-deferred.yaml.
// NOTE(2026-06-15): gemini-cli is dewired — kept in code but unregistered from the
// sync engine (user request). Mirrors transform.Default().
func SupportedProviders() []string {
	return []string{
		"claude-code",
		"codex",
		"cursor",
		// dewired: gemini-cli kept in code but unregistered (user request 2026-06-15).
		// "gemini-cli",
		"github-copilot",
		"goose",
		"grok-cli",
		"opencode",
		"roo-code",
	}
}

// IsSupportedProvider reports whether id is one of the eight active supported providers.
func IsSupportedProvider(id string) bool {
	for _, p := range SupportedProviders() {
		if p == id {
			return true
		}
	}
	return false
}

// ProvidersConfig holds provider selection settings.
//
//	mode=all      -> every SupportedProviders() except Disabled
//	mode=specific -> only Enabled (intersected with SupportedProviders())
type ProvidersConfig struct {
	Mode     string   `json:"mode" yaml:"mode"`
	Enabled  []string `json:"enabled" yaml:"enabled"`
	Disabled []string `json:"disabled" yaml:"disabled"`
}

// EffectiveProviders resolves the active provider set per mode. The result is a
// sorted, de-duplicated subset of SupportedProviders(). Unknown ids in
// enabled/disabled are ignored.
func (p ProvidersConfig) EffectiveProviders() []string {
	supported := SupportedProviders()
	switch p.Mode {
	case ProviderModeSpecific:
		set := toSet(p.Enabled)
		return filterSorted(supported, func(id string) bool { return set[id] })
	default: // all (incl. empty/unknown mode)
		excl := toSet(p.Disabled)
		return filterSorted(supported, func(id string) bool { return !excl[id] })
	}
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func filterSorted(all []string, keep func(string) bool) []string {
	out := make([]string, 0, len(all))
	for _, id := range all {
		if keep(id) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
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

// EffectiveProviders resolves the active provider set from the providers config.
func (c *Config) EffectiveProviders() []string {
	return c.Providers.EffectiveProviders()
}

// Defaults / allowed values.
const (
	DefaultScope = "agents"
	DefaultTheme = "dark"
)

// ValidScopes enumerates the allowed scope values.
func ValidScopes() []string { return []string{"agents", "skills", "slash"} }

// ValidProviderModes enumerates the allowed providers.mode values.
func ValidProviderModes() []string { return []string{ProviderModeAll, ProviderModeSpecific} }

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
	if c.Providers.Mode == "" {
		c.Providers.Mode = DefaultProviderMode
	}
	if c.Providers.Enabled == nil {
		c.Providers.Enabled = []string{}
	}
	if c.Providers.Disabled == nil {
		c.Providers.Disabled = []string{}
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
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}

// writeFileAtomic writes data to path durably: MkdirAll the parent, write to a
// temp file in the same directory, then os.Rename it into place. Rename is
// atomic on POSIX (same filesystem), so a crash or concurrent reader never sees
// a truncated/half-written config — readers see either the old file or the new
// one. The temp file is cleaned up on any error before the rename.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".graft-cfg-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup: a no-op once the rename has consumed the temp file.
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp into place: %w", err)
	}
	return nil
}
