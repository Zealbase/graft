package status

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	syncpkg "github.com/Shaik-Sirajuddin/graft/internal/core/sync"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	writeFile(t, dir, "README.md", "x\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "init")
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeClaude(t *testing.T, dir, name, body string) {
	writeFile(t, dir, filepath.Join(".claude", "agents", name+".md"),
		"---\nname: "+name+"\ndescription: d\nmodel: sonnet\n---\n"+body+"\n")
}

func TestStatusInSyncAndDrift(t *testing.T) {
	requireGit(t)
	dir := newRepo(t)
	writeClaude(t, dir, "helper", "Original body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()

	// Sync to establish canonical + provider files. Point ScopeHome providers
	// (antigravity) at a hermetic temp HOME so the sync never writes to the real
	// ~/.gemini/antigravity-cli. The reporter must use the SAME home base.
	home := t.TempDir()
	eng := syncpkg.New(st, tr, gitx.New(dir), dir).SetHomeBase(home)
	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("seed sync: %v %v", err, res.Status)
	}

	rep := New(st, tr, dir).SetHomeBase(home)

	// Right after sync the claude file matches canonical → in sync.
	list, err := rep.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "helper" {
		t.Fatalf("list = %+v", list)
	}
	if !list[0].InSync {
		t.Fatalf("expected helper in sync, got %+v", list[0])
	}
	if inSync, ok := list[0].Providers["claude-code"]; !ok || !inSync {
		t.Fatalf("claude-code provider not in sync: %+v", list[0].Providers)
	}

	// Now hand-edit the claude file so it diverges from canonical → drift.
	writeClaude(t, dir, "helper", "Hand edited body that diverges.")

	name := "helper"
	report, err := rep.Status(&name)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(report.Agents) != 1 {
		t.Fatalf("agents = %+v", report.Agents)
	}
	if report.Agents[0].InSync {
		t.Fatalf("expected drift after edit, got in-sync: %+v", report.Agents[0])
	}
	if report.OutOfSyncProviders["claude-code"] != 1 {
		t.Fatalf("expected claude-code out of sync count 1, got %v", report.OutOfSyncProviders)
	}
}

// TestStatusIncludesScopeHomeProvider verifies the reporter mirrors the engine's
// per-provider base resolution: a ScopeHome provider writes its file under $HOME,
// and status must Detect it there.
//
// TODO(2026-06-13): antigravity (agy) was the only ScopeHome provider and is now
// unregistered pending research spike (see tasks/_draft/antigravity-deferred.yaml).
// Re-enable this test (using the re-registered ScopeHome provider) after the spike.
func TestStatusIncludesScopeHomeProvider(t *testing.T) {
	t.Skip("antigravity (the only ScopeHome provider) unregistered pending research spike — re-enable after re-registration")
	requireGit(t)
	dir := newRepo(t)
	writeClaude(t, dir, "helper", "Body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()

	home := t.TempDir()
	eng := syncpkg.New(st, tr, gitx.New(dir), dir).SetHomeBase(home)
	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("seed sync: %v %v", err, res.Status)
	}

	// Sanity: the engine wrote antigravity under $HOME (not the workspace root).
	homeFile := filepath.Join(home, ".gemini", "antigravity-cli", "agents", "helper", "agent.json")
	if _, err := os.Stat(homeFile); err != nil {
		t.Fatalf("precondition: antigravity file not under HOME: %v", err)
	}

	// Reporter pointed at the SAME home base must include antigravity in-sync.
	rep := New(st, tr, dir).SetHomeBase(home)
	list, err := rep.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "helper" {
		t.Fatalf("list = %+v", list)
	}
	inSync, ok := list[0].Providers["antigravity"]
	if !ok {
		t.Fatalf("antigravity missing from status providers: %+v", list[0].Providers)
	}
	if !inSync {
		t.Fatalf("antigravity reported out of sync right after sync: %+v", list[0].Providers)
	}

	// A reporter pointed at a DIFFERENT (empty) home must NOT see antigravity —
	// confirming the base resolution is actually honored.
	repOther := New(st, tr, dir).SetHomeBase(t.TempDir())
	listOther, err := repOther.List()
	if err != nil {
		t.Fatalf("list other: %v", err)
	}
	if _, present := providersFor(listOther, "helper")["antigravity"]; present {
		t.Fatalf("antigravity present despite empty home base: %+v", listOther)
	}
}

// providersFor is a tiny test helper: returns the provider map for a named agent.
func providersFor(list []contract.AgentStatus, name string) map[string]bool {
	for _, a := range list {
		if a.Name == name {
			return a.Providers
		}
	}
	return map[string]bool{}
}
