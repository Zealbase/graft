// Package cli builds graft's cobra command tree. Every command talks ONLY to a
// contract.EntryGate (the gateway); it never imports store/core/gitx/transform/
// providers directly. config get/set are the sole exception — they operate on
// the global XDG config via the Resolver, bypassing the gateway.
package cli

import "github.com/Shaik-Sirajuddin/graft/internal/cli/config"

// Cli is the executable CLI surface.
type Cli interface {
	// Install executes the root command.
	Install() error
}

// ResolveConfig loads the persisted global config with defaults applied.
func ResolveConfig(r config.Resolver) (*config.Config, error) {
	cfg, err := r.Get()
	if err != nil {
		return nil, err
	}
	return config.ApplyDefaults(cfg), nil
}

// SaveConfig persists the global config after applying defaults.
func SaveConfig(r config.Resolver, cfg *config.Config) error {
	return r.Save(config.ApplyDefaults(cfg))
}
