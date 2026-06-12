package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// --- a minimal fake project provider (the change source) ---------------------

// fakeProjectProvider reads/writes a trivial agent file under <root>/.fake/<name>.txt
// so a sync has something to canonicalize. It is ScopeProject (does not implement
// ScopedProvider).
type fakeProjectProvider struct{}

func (fakeProjectProvider) Name() string { return "fake-project" }
func (fakeProjectProvider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".fake")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".txt" {
			continue
		}
		base := name[:len(name)-len(".txt")]
		refs = append(refs, contract.AgentRef{Name: base, Provider: "fake-project", Path: filepath.Join(dir, name)})
	}
	return refs, nil
}
func (fakeProjectProvider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, err
	}
	base := filepath.Base(path)
	name := base[:len(base)-len(".txt")]
	return contract.ProviderAgent{
		Provider: "fake-project",
		Ref:      contract.AgentRef{Name: name, Provider: "fake-project", Path: path},
		Body:     string(raw),
		Raw:      raw,
	}, nil
}
func (fakeProjectProvider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	return contract.CanonicalAgent{Name: p.Ref.Name, Description: "from fake", Body: p.Body}, nil
}
func (fakeProjectProvider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	return []contract.FileWrite{{
		Path: filepath.Join(".fake", a.Name+".txt"),
		Data: []byte(a.Body),
	}}, nil
}
func (fakeProjectProvider) Schema() []byte { return nil }

// --- a fake ScopeHome provider (paths resolved against $HOME) ----------------

// fakeHomeProvider stores agent files under <base>/.fakehome/<name>.json and
// declares PathScope == ScopeHome, so the engine must resolve <base> to $HOME for
// BOTH Detect and Serialize. Detect/Serialize paths are RELATIVE to the scope base.
type fakeHomeProvider struct{}

func (fakeHomeProvider) Name() string                  { return "fake-home" }
func (fakeHomeProvider) PathScope() contract.PathScope { return contract.ScopeHome }
func (fakeHomeProvider) Schema() []byte                { return nil }
func (fakeHomeProvider) Detect(base string) ([]contract.AgentRef, error) {
	dir := filepath.Join(base, ".fakehome")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := e.Name()[:len(e.Name())-len(".json")]
		refs = append(refs, contract.AgentRef{Name: name, Provider: "fake-home", Path: filepath.Join(dir, e.Name())})
	}
	return refs, nil
}
func (fakeHomeProvider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, err
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	name, _ := m["name"].(string)
	return contract.ProviderAgent{
		Provider: "fake-home",
		Ref:      contract.AgentRef{Name: name, Provider: "fake-home", Path: path},
		Fields:   m,
		Raw:      raw,
	}, nil
}
func (fakeHomeProvider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	desc, _ := p.Fields["description"].(string)
	return contract.CanonicalAgent{Name: p.Ref.Name, Description: desc}, nil
}
func (fakeHomeProvider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	b, _ := json.Marshal(map[string]string{"name": a.Name, "description": a.Description})
	// Path RELATIVE to the scope base ($HOME) — the engine prepends $HOME.
	return []contract.FileWrite{{
		Path: filepath.Join(".fakehome", a.Name+".json"),
		Data: b,
	}}, nil
}

// fakeRegistry builds a transform registry containing the fake project + home
// providers only (no real providers, so the test is fully hermetic).
func fakeRegistry() *transform.Registry {
	r := transform.New()
	r.Register(fakeProjectProvider{})
	r.Register(fakeHomeProvider{})
	return r
}

// TestScopeHome_WriteUnderHomeNotRoot: a ScopeHome provider's file is WRITTEN
// under $HOME (the temp home), never the workspace root.
func TestScopeHome_WriteUnderHomeNotRoot(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	home := t.TempDir()

	// Seed a project-scoped source so the agent "scout" is detected + canonicalized.
	writeFile(t, dir, filepath.Join(".fake", "scout.txt"), "scout body\n")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	eng := New(st, fakeRegistry(), gitx.New(dir), dir).SetHomeBase(home)
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// The home-scoped file MUST be under $HOME, not the workspace root.
	homeFile := filepath.Join(home, ".fakehome", "scout.json")
	if _, err := os.Stat(homeFile); err != nil {
		t.Fatalf("home-scoped file not written under HOME: %v", err)
	}
	rootFile := filepath.Join(dir, ".fakehome", "scout.json")
	if _, err := os.Stat(rootFile); !os.IsNotExist(err) {
		t.Fatalf("home-scoped file leaked into workspace root: %v", err)
	}
	// The project-scoped provider still writes under the workspace root.
	if _, err := os.Stat(filepath.Join(dir, ".fake", "scout.txt")); err != nil {
		t.Fatalf("project-scoped file missing under root: %v", err)
	}
}

// TestScopeHome_ReadFromHomeNotRoot: a ScopeHome provider's file is DETECTED from
// $HOME (the temp home), not the workspace root.
func TestScopeHome_ReadFromHomeNotRoot(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	home := t.TempDir()

	// Place a home-scoped agent ONLY under the temp HOME (the provider's scope).
	homeAgent := filepath.Join(home, ".fakehome", "homie.json")
	if err := os.MkdirAll(filepath.Dir(homeAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homeAgent, []byte(`{"name":"homie","description":"lives in home"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Decoy at the workspace root: must be IGNORED for a ScopeHome provider.
	decoy := filepath.Join(dir, ".fakehome", "homie.json")
	if err := os.MkdirAll(filepath.Dir(decoy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decoy, []byte(`{"name":"homie","description":"DECOY in root"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	eng := New(st, fakeRegistry(), gitx.New(dir), dir).SetHomeBase(home)
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done", res.Status)
	}
	if len(res.Changed) != 1 || res.Changed[0] != "homie" {
		t.Fatalf("changed=%v, want [homie] (detected from HOME)", res.Changed)
	}

	// The canonical was derived from the HOME copy, not the decoy.
	can, err := canonical.Load(canonical.AgentDir(dir, "homie"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if can.Description != "lives in home" {
		t.Fatalf("canonical description=%q, want 'lives in home' (HOME source, not root decoy)", can.Description)
	}
}

// TestLastCommitHash_PopulatedAfterSync: after a clean sync in a committed repo,
// each provider's .meta.json lastCommitHash equals the base-branch HEAD.
func TestLastCommitHash_PopulatedAfterSync(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil || res.Status != contract.RunDone {
		t.Fatalf("sync: res=%+v err=%v", res, err)
	}

	// Expected commit = current base-branch HEAD.
	want := gitHead(t, dir)
	if want == "" {
		t.Fatal("could not resolve base HEAD")
	}

	meta, err := canonical.LoadMeta(canonical.AgentDir(dir, "reviewer"))
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if len(meta.Providers) == 0 {
		t.Fatal("meta has no provider entries")
	}
	for prov, pm := range meta.Providers {
		if pm.LastCommitHash == "" {
			t.Errorf("provider %q lastCommitHash empty after sync", prov)
		}
		if pm.LastCommitHash != want {
			t.Errorf("provider %q lastCommitHash=%q, want base HEAD %q", prov, pm.LastCommitHash, want)
		}
	}
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	out, err := combinedGit(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return trimSpace(out)
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
