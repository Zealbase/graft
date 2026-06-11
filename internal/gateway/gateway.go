// Package gateway implements contract.EntryGate — the single object the CLI
// talks to. It is the sole owner of the store, sync engine, status reporter,
// transformer registry, gitx, and the workspace lock; the CLI calls only this
// interface and never reaches into those lower layers directly.
//
// Open wires the real dependencies rooted at a workspace directory:
//
//	store.Open(<root>/.graft/graft.db)
//	transform.Default()                (all ten providers)
//	gitx.New(root)
//	sync.New(store, tr, git, root)
//	status.New(store, tr, root)
//	lock (file flock on <root>/.graft/lock)
//
// Sync serializes per workspace via the lock, auto-validates the changed agents
// (blocking on error findings) before delegating to the engine.
package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	statuspkg "github.com/Shaik-Sirajuddin/graft/internal/core/status"
	syncpkg "github.com/Shaik-Sirajuddin/graft/internal/core/sync"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/lock"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// graftDir / dbName locate the per-workspace store under the root.
const (
	graftDir = ".graft"
	dbName   = "graft.db"
)

// gate is the concrete EntryGate. It owns every lower-layer dependency.
type gate struct {
	root   string
	store  contract.Store
	tr     contract.Transformer
	git    contract.GitX
	engine *syncpkg.Engine
	status *statuspkg.Reporter
}

// compile-time assertion that gate satisfies the frozen contract.
var _ contract.EntryGate = (*gate)(nil)

// dbPath returns the sqlite path for a workspace root.
func dbPath(root string) string {
	return filepath.Join(root, graftDir, dbName)
}

// Open wires a fully-functional EntryGate rooted at root. It creates .graft/ if
// absent (the sqlite driver needs the parent dir to exist) and opens/migrates
// the store. Callers must Close the returned gate.
func Open(root string) (contract.EntryGate, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("gateway: resolve root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(abs, graftDir), 0o755); err != nil {
		return nil, fmt.Errorf("gateway: mkdir .graft: %w", err)
	}
	st, err := store.Open(dbPath(abs))
	if err != nil {
		return nil, fmt.Errorf("gateway: open store: %w", err)
	}
	tr := transform.Default()
	git := gitx.New(abs)
	return &gate{
		root:   abs,
		store:  st,
		tr:     tr,
		git:    git,
		engine: syncpkg.New(st, tr, git, abs),
		status: statuspkg.New(st, tr, abs),
	}, nil
}

// Init creates the .graft store + workspace row for the root and is idempotent.
// It resolves the git context (mode/remote/branch) via gitx.Resolve and reports
// it in the InitResult. Created is true only when the workspace row did not
// already exist.
func (g *gate) Init() (contract.InitResult, error) {
	if err := os.MkdirAll(filepath.Join(g.root, graftDir, "agents"), 0o755); err != nil {
		return contract.InitResult{}, fmt.Errorf("gateway: init mkdir: %w", err)
	}

	gctx := gitx.Resolve(g.root)

	// Detect prior existence before the upsert so Created reflects reality.
	existed := g.workspaceExists(gctx)

	if _, err := g.store.Workspace(g.root, gctx.Remote, gctx.Branch); err != nil {
		return contract.InitResult{}, fmt.Errorf("gateway: init workspace: %w", err)
	}

	return contract.InitResult{
		Root:    g.root,
		GitMode: gctx.Mode,
		Created: !existed,
	}, nil
}

// workspaceExists reports whether this workspace was already initialized. The
// store exposes no existence probe (Workspace is an idempotent upsert), so Init
// uses a sentinel file under .graft to distinguish a first init (Created=true)
// from a repeat init (Created=false). The first call drops the sentinel.
func (g *gate) workspaceExists(gctx gitx.Context) bool {
	marker := filepath.Join(g.root, graftDir, ".initialized")
	if _, err := os.Stat(marker); err == nil {
		return true
	}
	_ = os.WriteFile(marker, []byte(string(gctx.Mode)+"\n"), 0o644)
	return false
}

// List returns the per-agent, per-provider sync state for every tracked agent.
func (g *gate) List() ([]contract.AgentStatus, error) {
	return g.status.List()
}

// Status returns a StatusReport for one agent (name != nil) or all agents.
func (g *gate) Status(name *string) (contract.StatusReport, error) {
	return g.status.Status(name)
}

// Sync serializes on the workspace lock, runs the implicit validate-before-sync
// gate over the targeted agents (blocking on error-severity findings), then
// delegates to the engine.
func (g *gate) Sync(opts contract.SyncOpts) (contract.RunResult, error) {
	h, err := lock.Lock(context.Background(), g.root)
	if err != nil {
		return contract.RunResult{}, fmt.Errorf("gateway: acquire workspace lock: %w", err)
	}
	defer h.Unlock()

	// Auto-validate the agents this sync will touch. A resume (--continue) skips
	// the gate because the canonical tree is already the merged-in-progress state.
	if !opts.Continue {
		targets := opts.Names
		if len(targets) == 0 {
			// Empty = all changed; validate every tracked canonical agent.
			all, nerr := g.agentNames()
			if nerr != nil {
				return contract.RunResult{}, nerr
			}
			targets = all
		}
		findings, verr := g.validateAgents(targets)
		if verr != nil {
			return contract.RunResult{}, fmt.Errorf("gateway: pre-sync validate: %w", verr)
		}
		if blocking := errorFindings(findings); len(blocking) > 0 {
			return contract.RunResult{}, &ValidationError{Findings: blocking}
		}
	}

	return g.engine.Run(opts)
}

// Validate runs schema + semantic validation over the canonical agents under
// .graft/agents. scope is a provider id to constrain reporting to agents that
// the provider has on disk, "" / "all" for every tracked agent.
func (g *gate) Validate(scope string) ([]contract.Finding, error) {
	names, err := g.agentNamesForScope(scope)
	if err != nil {
		return nil, err
	}
	return g.validateAgents(names)
}

// Close releases the gateway's resources.
func (g *gate) Close() error {
	if g.store != nil {
		return g.store.Close()
	}
	return nil
}

// --- helpers -------------------------------------------------------------

// agentsDir is the canonical store directory under the root.
func (g *gate) agentsDirPath() string {
	return filepath.Join(g.root, graftDir, "agents")
}

// agentNames lists every agent directory under .graft/agents.
func (g *gate) agentNames() ([]string, error) {
	entries, err := os.ReadDir(g.agentsDirPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("gateway: read agents dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// agentNamesForScope resolves the agent set for a validate scope. A provider
// scope keeps only agents that the named provider has on disk; "" / "all"
// returns every tracked agent.
func (g *gate) agentNamesForScope(scope string) ([]string, error) {
	all, err := g.agentNames()
	if err != nil {
		return nil, err
	}
	if scope == "" || scope == "all" {
		return all, nil
	}
	prov, ok := g.tr.Provider(scope)
	if !ok {
		return nil, fmt.Errorf("gateway: unknown provider %q", scope)
	}
	refs, derr := prov.Detect(g.root)
	if derr != nil {
		return nil, fmt.Errorf("gateway: detect %s: %w", scope, derr)
	}
	have := map[string]bool{}
	for _, r := range refs {
		have[r.Name] = true
	}
	var out []string
	for _, n := range all {
		if have[n] {
			out = append(out, n)
		}
	}
	return out, nil
}

// validateAgents loads each named canonical agent and runs canonical.Validate,
// aggregating findings. A missing canonical agent is itself an error finding.
func (g *gate) validateAgents(names []string) ([]contract.Finding, error) {
	var findings []contract.Finding
	for _, name := range names {
		can, err := canonical.Load(canonical.AgentDir(g.root, name))
		if err != nil {
			findings = append(findings, contract.Finding{
				Agent:    name,
				Severity: "error",
				Message:  fmt.Sprintf("load canonical agent: %v", err),
			})
			continue
		}
		fs, verr := canonical.Validate(can)
		if verr != nil {
			return nil, fmt.Errorf("gateway: validate %s: %w", name, verr)
		}
		findings = append(findings, fs...)
	}
	return findings, nil
}

// errorFindings keeps only error-severity findings (warnings never block).
func errorFindings(in []contract.Finding) []contract.Finding {
	var out []contract.Finding
	for _, f := range in {
		if f.Severity == "error" {
			out = append(out, f)
		}
	}
	return out
}
