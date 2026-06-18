package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
)

// readMeta loads an agent's .meta.json sidecar from the canonical store.
func readMeta(t *testing.T, root, name string) canonical.Meta {
	t.Helper()
	m, err := canonical.LoadMeta(canonical.AgentDir(root, name))
	if err != nil {
		t.Fatalf("load meta %q: %v", name, err)
	}
	return m
}

// readBody loads an agent's canonical Body (instructions.md).
func readBody(t *testing.T, root, name string) string {
	t.Helper()
	a, err := canonical.Load(canonical.AgentDir(root, name))
	if err != nil {
		t.Fatalf("load agent %q: %v", name, err)
	}
	return a.Body
}

// writeSandboxMode sets providerOverrides[codex][sandbox_mode] on an agent and
// re-saves it (the CLI has no flag for arbitrary overrides). It preserves the
// agent's existing meta so the on-disk state stays consistent.
func writeSandboxMode(t *testing.T, root, name, mode string) {
	t.Helper()
	dir := canonical.AgentDir(root, name)
	a, err := canonical.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	meta, err := canonical.LoadMeta(dir)
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if a.ProviderOverrides == nil {
		a.ProviderOverrides = map[string]map[string]any{}
	}
	bucket := a.ProviderOverrides["codex"]
	if bucket == nil {
		bucket = map[string]any{}
		a.ProviderOverrides["codex"] = bucket
	}
	bucket["sandbox_mode"] = mode
	writes, err := canonical.SaveWithMeta(root, a, meta)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
