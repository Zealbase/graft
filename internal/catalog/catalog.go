// Package catalog provides an offline-embedded baseline of per-provider
// schema, model list, and capability data for the 10 graft providers.
// Data is embedded at compile time via go:embed.
//
// Regen: update internal/catalog/data/ files manually and rerun
// `go test ./internal/catalog/...` to verify manifest hashes still match.
// A future tools/gen-catalog helper will automate live-source refresh.
package catalog

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed data
var dataFS embed.FS

// Providers lists all supported provider ids.
var Providers = []string{
	"claude-code", "codex", "gemini-cli", "cursor", "github-copilot",
	"opencode", "roo-code", "goose", "grok-cli", "antigravity",
}

// Manifest is the top-level catalog manifest.
type Manifest struct {
	Version     string                    `json:"version"`
	GeneratedAt string                    `json:"generatedAt"`
	Providers   map[string]ProviderRecord `json:"providers"`
}

// ProviderRecord holds per-provider metadata in the manifest.
type ProviderRecord struct {
	Hash      string `json:"hash"`
	Source    string `json:"source"`
	FetchedAt string `json:"fetchedAt"`
}

// Models is the shape of a provider's models.json file.
type Models struct {
	Models    []string `json:"models"`
	Source    string   `json:"source"`
	FetchedAt string   `json:"fetchedAt"`
	Note      string   `json:"note,omitempty"`
}

// Capabilities is the shape of a provider's capabilities.json file.
//
// Deprecated marks a provider that was previously active but has been removed
// from the active set (unregistered from the sync/skills engines). Its data is
// kept in the catalog for reference; callers should not treat it as selectable.
// This is distinct from a never-built/planned provider, which simply carries no
// deprecation flag. Defunct is reserved for a provider whose upstream tool no
// longer exists.
type Capabilities struct {
	Tools      []string `json:"tools"`
	PathScope  string   `json:"pathScope"`
	Defunct    bool     `json:"defunct,omitempty"`
	Deprecated bool     `json:"deprecated,omitempty"`
	Note       string   `json:"note,omitempty"`
}

// Catalog is the loaded catalog.
type Catalog struct {
	manifest    Manifest
	schemaCache map[string][]byte
	modelsCache map[string]Models
	capsCache   map[string]Capabilities
}

// Load reads and parses the embedded catalog data. Returns an error if
// manifest.json or any provider file is missing or malformed.
func Load() (*Catalog, error) {
	// read manifest
	mb, err := dataFS.ReadFile("data/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("catalog: read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, fmt.Errorf("catalog: parse manifest: %w", err)
	}
	c := &Catalog{
		manifest:    m,
		schemaCache: make(map[string][]byte),
		modelsCache: make(map[string]Models),
		capsCache:   make(map[string]Capabilities),
	}
	for _, p := range Providers {
		sb, err := dataFS.ReadFile("data/" + p + "/schema.json")
		if err != nil {
			return nil, fmt.Errorf("catalog: read schema for %s: %w", p, err)
		}
		c.schemaCache[p] = sb

		modB, err := dataFS.ReadFile("data/" + p + "/models.json")
		if err != nil {
			return nil, fmt.Errorf("catalog: read models for %s: %w", p, err)
		}
		var mod Models
		if err := json.Unmarshal(modB, &mod); err != nil {
			return nil, fmt.Errorf("catalog: parse models for %s: %w", p, err)
		}
		c.modelsCache[p] = mod

		capB, err := dataFS.ReadFile("data/" + p + "/capabilities.json")
		if err != nil {
			return nil, fmt.Errorf("catalog: read capabilities for %s: %w", p, err)
		}
		var cap Capabilities
		if err := json.Unmarshal(capB, &cap); err != nil {
			return nil, fmt.Errorf("catalog: parse capabilities for %s: %w", p, err)
		}
		c.capsCache[p] = cap
	}
	return c, nil
}

// loadOnce caches the result of Load() so embedded files are only parsed once.
var (
	loadOnceInstance *Catalog
	loadOnceErr      error
	loadOnceMu       sync.Once
)

// LoadOnce returns a shared *Catalog, parsing the embedded data files only on
// the first call.  Subsequent calls return the same pointer without re-parsing.
// This is safe because the embedded data is immutable (compiled into the binary).
// Use Load() if you need a fresh independent instance (e.g. in tests).
//
// Error memoization: if Load() fails on the first call the error is stored and
// returned on every subsequent call without retrying.  This is intentional —
// catalog data is go:embed-compiled into the binary, so a Load failure indicates
// a corrupt or incomplete binary; retrying would always produce the same result.
func LoadOnce() (*Catalog, error) {
	loadOnceMu.Do(func() {
		loadOnceInstance, loadOnceErr = Load()
	})
	return loadOnceInstance, loadOnceErr
}

// Verify recomputes each provider's hash and compares to the manifest.
// Returns a non-nil error listing all mismatches.
func (c *Catalog) Verify() error {
	var mismatches []string
	for _, p := range Providers {
		got, err := computeProviderHash(p)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		want := ""
		if rec, ok := c.manifest.Providers[p]; ok {
			want = rec.Hash
		}
		if got != want {
			mismatches = append(mismatches, fmt.Sprintf("%s: hash mismatch (want %s, got %s)", p, want, got))
		}
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("catalog: verify failed:\n  %s", strings.Join(mismatches, "\n  "))
	}
	return nil
}

// computeProviderHash computes the deterministic sha256 for a provider dir.
// Scheme: sort filenames, concat (filename bytes + file bytes) for each.
func computeProviderHash(provider string) (string, error) {
	files := []string{"capabilities.json", "models.json", "schema.json"}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		data, err := dataFS.ReadFile("data/" + provider + "/" + f)
		if err != nil {
			return "", err
		}
		h.Write([]byte(f))
		h.Write(data)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// ModelsFor returns the baseline model ids for the given provider.
// Returns an error for unknown providers.
// A defensive copy is returned so callers that sort or append cannot corrupt
// the shared LoadOnce singleton's cached slice.
func (c *Catalog) ModelsFor(provider string) ([]string, error) {
	m, ok := c.modelsCache[provider]
	if !ok {
		return nil, fmt.Errorf("catalog: unknown provider %q", provider)
	}
	cp := make([]string, len(m.Models))
	copy(cp, m.Models)
	return cp, nil
}

// Schema returns the schema.json bytes for the given provider.
func (c *Catalog) Schema(provider string) ([]byte, error) {
	s, ok := c.schemaCache[provider]
	if !ok {
		return nil, fmt.Errorf("catalog: unknown provider %q", provider)
	}
	return s, nil
}

// CapabilitiesFor returns the capabilities for the given provider.
func (c *Catalog) CapabilitiesFor(provider string) (Capabilities, error) {
	cap, ok := c.capsCache[provider]
	if !ok {
		return Capabilities{}, fmt.Errorf("catalog: unknown provider %q", provider)
	}
	return cap, nil
}

// Manifest returns the loaded manifest.
func (c *Catalog) Manifest() Manifest { return c.manifest }
