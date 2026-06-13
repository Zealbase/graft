package gateway

import (
	"errors"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// errNotImplemented marks a contract method frozen at s-0 but not yet wired by
// its owning agent. Replaced phase-by-phase; never ships in a release build.
var errNotImplemented = errors.New("graft: not implemented yet (s-0 contract stub)")

// CreateAgent — plan-sync task 2 (cli/canonical). Stub.
func (g *gate) CreateAgent(name, prompt string) (contract.CanonicalAgent, error) {
	return contract.CanonicalAgent{}, errNotImplemented
}

// SetAgentModel — v0.0.3 task 3 (cli). Stub.
func (g *gate) SetAgentModel(name, provider, model string) ([]contract.Finding, error) {
	return nil, errNotImplemented
}

// Update — plan-sync task 6 (cli). Stub.
func (g *gate) Update(opts contract.UpdateOpts) (contract.UpdateResult, error) {
	return contract.UpdateResult{}, errNotImplemented
}

// Destroy — v0.0.3 task 1 (cli/db). Stub.
func (g *gate) Destroy(opts contract.DestroyOpts) (contract.DestroyResult, error) {
	return contract.DestroyResult{}, errNotImplemented
}
