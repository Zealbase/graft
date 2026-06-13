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
//	lock (file flock on ~/.local/share/graft/locks/<ws-hash>.lock)
//
// Sync serializes per workspace via the lock, auto-validates the changed agents
// (blocking on error findings) before delegating to the engine.
package gateway

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	statuspkg "github.com/Shaik-Sirajuddin/graft/internal/core/status"
	syncpkg "github.com/Shaik-Sirajuddin/graft/internal/core/sync"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/lock"
	"github.com/Shaik-Sirajuddin/graft/internal/skills"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// graftDir is the in-repo portable store directory under the workspace root.
const graftDir = ".graft"

// legacyDBName is the old per-repo db filename (now migrated to the global db).
const legacyDBName = "graft.db"

// gate is the concrete EntryGate. It owns every lower-layer dependency.
type gate struct {
	root   string
	store  contract.Store
	tr     contract.Transformer
	git    contract.GitX
	engine *syncpkg.Engine
	status *statuspkg.Reporter

	// skills is the lazily-built skill manager (symlink-based, no db).
	skills *skills.Manager
	// skillHook gates the implicit init/sync skill-apply hook (set by the CLI).
	skillHook SkillHookConfig
	// enabledProviders is the effective provider set (from CLI config) the model
	// validation check is restricted to. Empty/nil = all providers the
	// transformer knows.
	enabledProviders []string
}

// compile-time assertion that gate satisfies the frozen contract.
var _ contract.EntryGate = (*gate)(nil)

// Open wires a fully-functional EntryGate rooted at root. The sqlite store now
// lives at a GLOBAL XDG path (~/.local/share/graft/graft.db) shared by every
// workspace — identity is (root,remote,branch), so one db serves all. The
// in-repo .graft/ holds only the portable store (agents/ + .meta.json).
//
// On first run for a repo that still has an old per-repo db
// (<root>/.graft/graft.db), Open auto-migrates it into the global db and removes
// the old in-repo runtime bits (db, lock, .initialized). It also writes a
// .graft/.gitignore that keeps agents/ + .meta.json committed while ignoring any
// stray local artifacts. Callers must Close the returned gate.
func Open(root string) (contract.EntryGate, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("gateway: resolve root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(abs, graftDir), 0o755); err != nil {
		return nil, fmt.Errorf("gateway: mkdir .graft: %w", err)
	}

	gdb, err := globalDBPath()
	if err != nil {
		return nil, fmt.Errorf("gateway: resolve global db path: %w", err)
	}

	// One-time migration of an old in-repo db into the global db, then clean up.
	// Runs BEFORE we open the global store so store.Migrate's own destination
	// connection does not race ours. Failure must not brick the workspace.
	if merr := migrateLegacyRepo(abs, gdb); merr != nil {
		log.Printf("[WARN] gateway: legacy db migration: %v", merr)
	}

	st, err := store.Open(gdb)
	if err != nil {
		return nil, fmt.Errorf("gateway: open store: %w", err)
	}

	// Ensure the in-repo .graft/ only commits the portable parts.
	if werr := writeGraftGitignore(abs); werr != nil {
		log.Printf("[WARN] gateway: write .graft/.gitignore: %v", werr)
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

	// Derive Created from the global store: "initialized" == a workspace row
	// already exists for this identity. FindWorkspace is a read-only probe with
	// no side effects, so it must run BEFORE the Workspace upsert below.
	existing, ferr := g.store.FindWorkspace(g.root, gctx.Remote, gctx.Branch)
	if ferr != nil {
		return contract.InitResult{}, fmt.Errorf("gateway: init find workspace: %w", ferr)
	}
	existed := existing != nil

	if _, err := g.store.Workspace(g.root, gctx.Remote, gctx.Branch, gctx.Mode); err != nil {
		return contract.InitResult{}, fmt.Errorf("gateway: init workspace: %w", err)
	}

	// Skills hook (plan-skills 03): seed .agents/skills and link any skills
	// already present in supporting provider dirs. Gated on skills.enabled;
	// never fails init.
	g.applySkillsHook()

	return contract.InitResult{
		Root:    g.root,
		GitMode: gctx.Mode,
		Created: !existed,
	}, nil
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
	lockPath, err := g.workspaceLockPath()
	if err != nil {
		return contract.RunResult{}, err
	}
	h, err := lock.Lock(context.Background(), lockPath)
	if err != nil {
		return contract.RunResult{}, fmt.Errorf("gateway: acquire workspace lock: %w", err)
	}
	defer h.Unlock()

	// Skip the pre-sync validate gate when resuming/auto-continuing a conflict
	// run. Resume is now implicit: a bare `graft sync` auto-continues an open
	// conflict run, so opts.Continue is only a redundant alias — the real signal
	// is an open conflict run for this workspace. While markers are still present
	// the canonical agent.yaml is marker-laden and would fail to parse; the
	// engine owns conflict handling and will re-surface the conflict, so we must
	// not pre-validate. The canonical tree is the merged-in-progress state.
	skipGate := opts.Continue
	if !skipGate {
		if g.conflictRunOpen() {
			skipGate = true
		}
	}

	// Auto-validate the agents this sync will touch.
	if !skipGate {
		var targets []string
		if len(opts.Names) == 0 {
			// Empty = all changed; agentNames lists only existing canonical dirs.
			all, nerr := g.agentNames()
			if nerr != nil {
				return contract.RunResult{}, nerr
			}
			targets = all
		} else {
			// Named targets: only gate agents whose canonical ALREADY exists. On
			// an agent's first sync the engine generates the canonical during the
			// run, so there is nothing to validate up front — skip the gate for
			// those and let the engine canonicalize first. Already-tracked agents
			// are still fully validated.
			targets = g.existingCanonical(opts.Names)
		}
		findings, verr := g.validateAgents(targets)
		if verr != nil {
			return contract.RunResult{}, fmt.Errorf("gateway: pre-sync validate: %w", verr)
		}
		if blocking := errorFindings(findings); len(blocking) > 0 {
			return contract.RunResult{}, &ValidationError{Findings: blocking}
		}
	}

	// Ingest wiring (plan-sync task 5): forward the caller's ingestion intent to
	// the engine seam. The contract documents opts.Ingest as "default true at the
	// CLI" — the CLI's --ingest flag defaults true, so a normal sync ingests
	// provider-only agents; an explicit --ingest=false suppresses it. Every
	// gateway.Sync caller therefore passes Ingest=true on the happy path.
	g.engine.SetIngest(opts.Ingest)

	res, err := g.engine.Run(opts)
	if err != nil {
		return res, err
	}

	// Skills hook (plan-skills 03 + v0.0.4 verify): after a successful agent sync,
	// run the skill Apply pass so a fresh checkout gets canonical skills symlinked
	// into the supporting providers. Skip while a conflict is unresolved (the
	// canonical tree is mid-merge). Gated on skills.enabled; never fails the agent
	// sync. The per-skill outcome is folded into the RunResult so skill link state
	// is part of the in-sync determination and the summary output.
	if res.Status == contract.RunDone {
		sk := g.applySkillsHookOutcome()
		res.SkillsLinked = sk.Linked
		res.SkillsConflicted = sk.Conflicted
		res.SkillsPruned = sk.Pruned
		// Only Linked/Conflicted are exposed on RunResult (omitempty). The "K
		// skills" count for the in-sync summary is derived by the CLI from its own
		// SkillList call (gated on skills.enabled); CanonicalSkills is computed here
		// only to short-circuit the no-skills case in applySkillsHookOutcome.
	}

	return res, nil
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

// conflictRunOpen reports whether an unresolved conflict run exists for this
// workspace. It uses the READ-ONLY FindWorkspace probe (no upsert side effect —
// closes the earlier CL2 review gap): if the workspace has never been
// initialized there is no conflict run to resume. Any error or absent workspace
// is treated as "no open conflict run"; the engine remains the authority.
func (g *gate) conflictRunOpen() bool {
	gctx := gitx.Resolve(g.root)
	ws, err := g.store.FindWorkspace(g.root, gctx.Remote, gctx.Branch)
	if err != nil || ws == nil {
		return false
	}
	cr, err := g.store.OpenConflictRun(ws.ID)
	return err == nil && cr != nil
}

// workspaceLockPath returns the global per-workspace lock file path for the
// current workspace identity (root+remote+branch).
func (g *gate) workspaceLockPath() (string, error) {
	gctx := gitx.Resolve(g.root)
	p, err := globalLockPath(g.root, gctx.Remote, gctx.Branch)
	if err != nil {
		return "", fmt.Errorf("gateway: resolve lock path: %w", err)
	}
	return p, nil
}

// existingCanonical filters names to those that already have a canonical agent
// on disk under .graft/agents/<name>/agent.yaml. Names without a canonical yet
// (first sync) are dropped so the pre-sync validate gate skips them.
func (g *gate) existingCanonical(names []string) []string {
	var out []string
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(canonical.AgentDir(g.root, name), "agent.yaml")); err == nil {
			out = append(out, name)
		}
	}
	return out
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

		// providerOverrides key check (errors — blocks sync). Flags any key in
		// the ProviderOverrides map that is not a registered provider id.
		findings = append(findings, g.providerOverrideKeyFindings(can)...)

		// Real-time model check (warnings only — never block sync). Flags a model
		// that the provider's model list does not know; silently skips when the
		// list is unavailable (offline/no cache) or the provider has no list.
		findings = append(findings, g.modelFindings(can)...)
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
