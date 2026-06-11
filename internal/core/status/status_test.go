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

	// Sync to establish canonical + provider files.
	eng := syncpkg.New(st, tr, gitx.New(dir), dir)
	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("seed sync: %v %v", err, res.Status)
	}

	rep := New(st, tr, dir)

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
