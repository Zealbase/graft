package gateway

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
)

// errNotImplemented marks a contract method frozen at s-0 but not yet wired.
var errNotImplemented = errors.New("graft: not implemented yet")

// agentsDir is the canonical store's agents parent directory.
func (g *gate) agentsDir() string { return filepath.Join(g.root, graftDir, "agents") }

// writeFiles persists canonical FileWrites to disk (mkdir -p + write).
func writeFiles(writes []contract.FileWrite) error {
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// CreateAgent scaffolds a default canonical agent in .graft/agents/<name>
// (plan-sync task 2). Empty meta ⇒ the next sync treats it as canonical-drifted
// and fans it out to every enabled provider.
func (g *gate) CreateAgent(name, prompt string) (contract.CanonicalAgent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return contract.CanonicalAgent{}, fmt.Errorf("gateway: agent name is required")
	}
	dir := canonical.AgentDir(g.agentsDir(), name)
	if _, err := os.Stat(dir); err == nil {
		return contract.CanonicalAgent{}, fmt.Errorf("gateway: agent %q already exists", name)
	}
	a := canonical.BuildDefault(name, prompt)
	writes, err := canonical.SaveWithMeta(g.agentsDir(), a, canonical.Meta{})
	if err != nil {
		return contract.CanonicalAgent{}, fmt.Errorf("gateway: scaffold %q: %w", name, err)
	}
	if err := writeFiles(writes); err != nil {
		return contract.CanonicalAgent{}, fmt.Errorf("gateway: write agent %q: %w", name, err)
	}
	return a, nil
}

// SetAgentModel sets (model != "") or clears (model == "") a per-provider model
// override on an agent and returns model-validation findings (v0.0.3 task 3).
// Clearing truly removes the key (canonical.pruneOverrides drops empty buckets),
// so the value cannot resurrect on the next sync.
func (g *gate) SetAgentModel(name, provider, model string) ([]contract.Finding, error) {
	name = strings.TrimSpace(name)
	provider = strings.TrimSpace(provider)
	if name == "" || provider == "" {
		return nil, fmt.Errorf("gateway: agent name and provider are required")
	}
	if _, ok := g.tr.Provider(provider); !ok {
		return nil, fmt.Errorf("gateway: unknown provider %q", provider)
	}
	dir := canonical.AgentDir(g.agentsDir(), name)
	a, err := canonical.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("gateway: load agent %q: %w", name, err)
	}
	meta, err := canonical.LoadMeta(dir)
	if err != nil {
		return nil, fmt.Errorf("gateway: load meta %q: %w", name, err)
	}
	if a.ProviderOverrides == nil {
		a.ProviderOverrides = map[string]map[string]any{}
	}
	model = strings.TrimSpace(model)
	if model == "" {
		if bucket, ok := a.ProviderOverrides[provider]; ok {
			delete(bucket, "model")
			if len(bucket) == 0 {
				delete(a.ProviderOverrides, provider)
			}
		}
	} else {
		bucket := a.ProviderOverrides[provider]
		if bucket == nil {
			bucket = map[string]any{}
			a.ProviderOverrides[provider] = bucket
		}
		bucket["model"] = model
	}
	writes, err := canonical.SaveWithMeta(g.agentsDir(), a, meta)
	if err != nil {
		return nil, fmt.Errorf("gateway: save agent %q: %w", name, err)
	}
	if err := writeFiles(writes); err != nil {
		return nil, fmt.Errorf("gateway: write agent %q: %w", name, err)
	}
	return g.modelFindings(a), nil
}

// Destroy removes this workspace's graft-managed state — the global-db
// workspace rows (cascade), the lock file, and the in-repo .graft/ (or only its
// non-store parts when KeepStore) — leaving every provider agent file in place
// (v0.0.3 task 1).
func (g *gate) Destroy(opts contract.DestroyOpts) (contract.DestroyResult, error) {
	var res contract.DestroyResult
	gctx := gitx.Resolve(g.root)

	ws, err := g.store.FindWorkspace(g.root, gctx.Remote, gctx.Branch)
	if err != nil {
		return res, fmt.Errorf("gateway: destroy find workspace: %w", err)
	}
	if ws != nil {
		if err := g.store.DeleteWorkspace(ws.ID); err != nil {
			return res, fmt.Errorf("gateway: destroy delete workspace: %w", err)
		}
		res.RemovedRows = 1 // workspace row + its cascade rows
	}

	if lp, err := globalLockPath(g.root, gctx.Remote, gctx.Branch); err == nil {
		if rmErr := os.Remove(lp); rmErr == nil {
			res.RemovedLock = true
		}
	}

	graftPath := filepath.Join(g.root, graftDir)
	if opts.KeepStore {
		// Retain the canonical store (.graft/agents); drop the rest.
		entries, _ := os.ReadDir(graftPath)
		for _, e := range entries {
			if e.Name() == "agents" {
				continue
			}
			_ = os.RemoveAll(filepath.Join(graftPath, e.Name()))
		}
	} else {
		if err := os.RemoveAll(graftPath); err != nil {
			return res, fmt.Errorf("gateway: destroy remove .graft: %w", err)
		}
		res.RemovedDir = true
	}
	return res, nil
}

// Update — plan-sync task 6 (cli). Implemented in update_v003.go.
