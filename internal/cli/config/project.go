package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// projectConfigRel is the per-project config path relative to the workspace
// root. It lives inside .graft/ (committed alongside the canonical store) so a
// project's provider selection travels with the repo.
const projectConfigRel = ".graft/config.json"

// ProjectConfig is the per-project override layer. It mirrors the global
// provider-selection surface; an unset field falls back to the global config.
// Only the fields a project meaningfully overrides are persisted — provider
// selection is the primary one. Pointer/nil fields distinguish "unset" (inherit
// global) from an explicit empty value.
type ProjectConfig struct {
	// Providers, when non-nil, overrides the global provider selection for this
	// project. A nil pointer means "inherit the global effective set".
	Providers *ProvidersConfig `json:"providers,omitempty" yaml:"providers,omitempty"`
	// Scope, when non-empty, overrides the global synced capability.
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"`
}

// ProjectConfigPath returns the per-project config file path for a workspace
// root.
func ProjectConfigPath(root string) string {
	return filepath.Join(root, projectConfigRel)
}

// ProjectResolver loads and persists the per-project config at
// <root>/.graft/config.json.
type ProjectResolver interface {
	Get() (*ProjectConfig, error)
	Save(*ProjectConfig) error
	Path() string
	Root() string
}

// DefaultProjectResolver persists JSON at <root>/.graft/config.json.
type DefaultProjectResolver struct {
	WorkspaceRoot string
}

// Root returns the workspace root.
func (r *DefaultProjectResolver) Root() string { return r.WorkspaceRoot }

// Path returns the persisted project config file location.
func (r *DefaultProjectResolver) Path() string { return ProjectConfigPath(r.WorkspaceRoot) }

// Get reads the project config. A missing file resolves to an empty (all-unset)
// ProjectConfig with no error, so a project with no overrides inherits global.
func (r *DefaultProjectResolver) Get() (*ProjectConfig, error) {
	data, err := os.ReadFile(r.Path())
	if os.IsNotExist(err) {
		return &ProjectConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read project %s: %w", r.Path(), err)
	}
	var pc ProjectConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("config: parse project %s: %w", r.Path(), err)
	}
	return &pc, nil
}

// Save writes the project config to <root>/.graft/config.json.
func (r *DefaultProjectResolver) Save(pc *ProjectConfig) error {
	if pc == nil {
		pc = &ProjectConfig{}
	}
	path := r.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(pc, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal project: %w", err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("config: write project %s: %w", path, err)
	}
	return nil
}

// EffectiveProviders resolves the active provider set, layering the project
// config over the global config (project wins when it sets providers; otherwise
// the global effective set applies). The result is a sorted, de-duplicated
// subset of SupportedProviders().
func EffectiveProviders(global *Config, project *ProjectConfig) []string {
	if project != nil && project.Providers != nil {
		return project.Providers.EffectiveProviders()
	}
	if global != nil {
		return global.EffectiveProviders()
	}
	return SupportedProviders()
}

// EffectiveScope layers the project scope over the global scope.
func EffectiveScope(global *Config, project *ProjectConfig) string {
	if project != nil && project.Scope != "" {
		return project.Scope
	}
	if global != nil {
		return global.Scope
	}
	return DefaultScope
}
