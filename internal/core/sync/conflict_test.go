package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// fakeGit wraps a real gitx.GitX but forces the first Merge call to conflict,
// then behaves normally on subsequent calls. It models a phantom conflict (the
// worktree is untouched) to exercise the engine's seam-driven conflict path
// without relying on real provider divergence. The REAL divergence path is
// covered by TestRealTwoProviderConflict below.
type fakeGit struct {
	inner        contract.GitX
	conflictOnce bool
	mergeCalls   int
}

func (f *fakeGit) Init() error                          { return f.inner.Init() }
func (f *fakeGit) HeadHash(ref string) (string, error)  { return f.inner.HeadHash(ref) }
func (f *fakeGit) Branch(name, from string) error       { return f.inner.Branch(name, from) }
func (f *fakeGit) Worktree(n, b string) (string, error) { return f.inner.Worktree(n, b) }
func (f *fakeGit) Diff(ref string) ([]contract.FileChange, error) {
	return f.inner.Diff(ref)
}
func (f *fakeGit) Copy(b string, p []string) error { return f.inner.Copy(b, p) }
func (f *fakeGit) Prune(prefix string) error       { return f.inner.Prune(prefix) }

func (f *fakeGit) Merge(into, from string) (contract.MergeResult, error) {
	f.mergeCalls++
	if f.conflictOnce {
		f.conflictOnce = false
		return contract.MergeResult{
			Clean:     false,
			Conflicts: []contract.Conflict{{Path: ".graft/agents/x/agent.yaml"}},
		}, nil
	}
	return f.inner.Merge(into, from)
}

// TestConflictThenResume exercises the seam-driven (fake) conflict path: the
// engine surfaces a conflict and a --continue converges to done.
func TestConflictThenResume(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "x", "desc", "body")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()
	real := gitx.New(dir)
	fg := &fakeGit{inner: real, conflictOnce: true}

	eng := New(st, tr, fg, dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if res.Status != contract.RunConflict {
		t.Fatalf("status = %s, want conflict", res.Status)
	}
	if len(res.Conflicts) == 0 || res.Conflicts[0].Agent != "x" {
		t.Fatalf("conflict agent = %v, want x", res.Conflicts)
	}

	// A BARE re-run (no --continue) auto-continues the open conflict run. With the
	// fake's forced conflict now exhausted, the merge converges to done.
	res2, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("bare re-run: %v", err)
	}
	if res2.Status != contract.RunDone {
		t.Fatalf("bare re-run status = %s, want done (conflicts=%v)", res2.Status, res2.Conflicts)
	}
	if res2.RunID != res.RunID {
		t.Fatalf("bare re-run started a new run %s (orig %s)", res2.RunID, res.RunID)
	}
}

// TestConflictResumeExplicitContinueAlias confirms --continue is still accepted
// and behaves identically to a bare re-run.
func TestConflictResumeExplicitContinueAlias(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "x", "desc", "body")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fg := &fakeGit{inner: gitx.New(dir), conflictOnce: true}
	eng := New(st, transform.Default(), fg, dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil || res.Status != contract.RunConflict {
		t.Fatalf("first run: res=%+v err=%v", res, err)
	}
	res2, err := eng.Run(contract.SyncOpts{Continue: true})
	if err != nil {
		t.Fatalf("explicit --continue: %v", err)
	}
	if res2.Status != contract.RunDone || res2.RunID != res.RunID {
		t.Fatalf("explicit --continue status=%s run=%s (orig %s)", res2.Status, res2.RunID, res.RunID)
	}
}

// writeOpencodeAgent drops an opencode agent file with a chosen model so two
// providers can diverge on the same canonical field.
func writeOpencodeAgent(t *testing.T, dir, name, desc, model, body string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + desc + "\nmodel: " + model + "\n---\n" + body + "\n"
	writeFile(t, dir, filepath.Join(".opencode", "agents", name+".md"), content)
}

func writeClaudeAgentModel(t *testing.T, dir, name, desc, model, body string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + desc + "\nmodel: " + model + "\n---\n" + body + "\n"
	writeFile(t, dir, filepath.Join(".claude", "agents", name+".md"), content)
}

// TestRealTwoProviderConflict drives the REAL provider-granular merge through the
// actual git binary (no fake): two providers define the same agent with a
// divergent `model`, which must surface a real git conflict (markers in the
// canonical file). Editing the canonical to resolve + --continue converges to
// done and propagates the resolved model to all providers.
func TestRealTwoProviderConflict(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	// Same agent "dev", divergent model across two providers, same body so only
	// the model line conflicts.
	writeClaudeAgentModel(t, dir, "dev", "a developer", "opus", "Shared body.")
	writeOpencodeAgent(t, dir, "dev", "a developer", "sonnet", "Shared body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	eng := New(st, transform.Default(), gitx.New(dir), dir).SetHomeBase(t.TempDir())

	// First sync: the two providers diverge on model -> conflict.
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunConflict {
		t.Fatalf("status=%s, want conflict (changed=%v)", res.Status, res.Changed)
	}
	if len(res.Conflicts) == 0 || res.Conflicts[0].Agent != "dev" {
		t.Fatalf("conflicts=%v, want one for dev", res.Conflicts)
	}

	// The surfaced canonical file in the WORKING TREE must carry git markers.
	canPath := filepath.Join(dir, ".graft", "agents", "dev", "agent.yaml")
	raw, err := os.ReadFile(canPath)
	if err != nil {
		t.Fatalf("read surfaced canonical: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "<<<<<<<") || !strings.Contains(body, ">>>>>>>") {
		t.Fatalf("expected conflict markers in canonical, got:\n%s", body)
	}
	if !strings.Contains(body, "opus") || !strings.Contains(body, "sonnet") {
		t.Fatalf("expected both candidate models in conflict, got:\n%s", body)
	}

	// A BARE re-run while markers are STILL PRESENT must re-surface the SAME
	// conflict (no error, no fresh run, still resumable).
	resurf, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("bare re-run with markers: unexpected error %v", err)
	}
	if resurf.Status != contract.RunConflict {
		t.Fatalf("bare re-run with markers status=%s, want conflict", resurf.Status)
	}
	if resurf.RunID != res.RunID {
		t.Fatalf("bare re-run with markers started a new run %s (orig %s)", resurf.RunID, res.RunID)
	}
	if len(resurf.Conflicts) == 0 || resurf.Conflicts[0].Agent != "dev" {
		t.Fatalf("bare re-run conflicts=%v, want one for dev", resurf.Conflicts)
	}

	// User resolves: pick opus, remove markers.
	resolved := resolveModel(body, "opus")
	if strings.Contains(resolved, "<<<<<<<") {
		t.Fatalf("resolver left markers:\n%s", resolved)
	}
	if err := os.WriteFile(canPath, []byte(resolved), 0o644); err != nil {
		t.Fatal(err)
	}

	// A BARE re-run (no --continue) now auto-continues: completes the merge and
	// converges to done.
	res2, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res2.Status != contract.RunDone {
		t.Fatalf("resume status=%s, want done (conflicts=%v)", res2.Status, res2.Conflicts)
	}
	if res2.RunID != res.RunID {
		t.Fatalf("resume started a new run")
	}

	// Resolved model propagated to canonical + both providers.
	can, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if can.Model != "opus" {
		t.Fatalf("resolved model = %q, want opus", can.Model)
	}
	claude, _ := os.ReadFile(filepath.Join(dir, ".claude", "agents", "dev.md"))
	if !strings.Contains(string(claude), "opus") {
		t.Fatalf("claude file did not get resolved model:\n%s", claude)
	}

	// No conflict run remains resumable.
	gctx := gitx.Resolve(dir)
	ws, _ := st.Workspace(dir, gctx.Remote, gctx.Branch, gctx.Mode)
	if again, _ := st.OpenConflictRun(ws.ID); again != nil {
		t.Fatalf("conflict run still resumable: %+v", again)
	}

	// Temp graft branches pruned.
	if out, _ := combinedGit(dir, "branch", "--list", "graft/*"); out != "" {
		t.Fatalf("temp branches survived: %q", out)
	}
}

// resolveModel removes git conflict markers from a YAML canonical, keeping the
// `model:` line that names keep and all non-conflicting lines.
func resolveModel(body, keep string) string {
	var out []string
	inConflict := false
	keepSide := false
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			inConflict = true
			keepSide = true // first (ours) side
			continue
		case strings.HasPrefix(line, "======="):
			keepSide = false // second (theirs) side
			continue
		case strings.HasPrefix(line, ">>>>>>>"):
			inConflict = false
			continue
		}
		if inConflict {
			// Keep whichever side carries the chosen model.
			if strings.Contains(line, keep) {
				out = append(out, line)
			}
			_ = keepSide
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// TestTwoProviderAutoMerge: two providers for the same agent that agree on the
// shared canonical fields and differ only where one expresses a field the other
// does NOT (capability variance). claude maps `tools` to the canonical Tools
// field; opencode does not (it keeps tools in overrides). They share model,
// description and body. The per-provider canonical branches therefore touch
// DIFFERENT lines -> git auto-merges, no conflict, no user interaction. This is
// also the capability-variance guard: an unsupported-field difference is never a
// conflict.
func TestTwoProviderAutoMerge(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	// claude declares tools (a canonical field only claude expresses) + shared
	// model/description/body.
	writeFile(t, dir, filepath.Join(".claude", "agents", "dev.md"),
		"---\nname: dev\ndescription: a dev\nmodel: opus\ntools: Read, Edit\n---\nShared body.\n")
	// opencode declares the SAME model/description/body, no canonical tools.
	writeOpencodeAgent(t, dir, "dev", "a dev", "opus", "Shared body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	eng := New(st, transform.Default(), gitx.New(dir), dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (capability variance must auto-merge); conflicts=%v", res.Status, res.Conflicts)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts on non-overlapping change: %v", res.Conflicts)
	}
	can, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if can.Model != "opus" {
		t.Fatalf("model=%q, want opus", can.Model)
	}
	// claude's tools survive the auto-merge (the field opencode doesn't express).
	if len(can.Tools) == 0 {
		t.Fatalf("tools lost in auto-merge: %+v", can)
	}
}

// TestSingleProviderNoConflict: one changed provider for an agent -> one branch
// -> clean merge -> no conflict (regression guard for the single-provider path).
func TestSingleProviderNoConflict(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgentModel(t, dir, "solo", "only claude", "opus", "Body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	eng := New(st, transform.Default(), gitx.New(dir), dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone || len(res.Conflicts) != 0 {
		t.Fatalf("single-provider sync must be clean, got %+v", res)
	}
}
