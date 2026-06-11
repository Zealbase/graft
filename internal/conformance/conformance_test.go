// Package conformance holds the single, table-driven provider conformance
// harness for graft (build phase c, the b<->c gate).
//
// It is intentionally ONE suite parameterized over transform.Default() — every
// registered provider is driven by the SAME assertions against its own golden
// fixtures under internal/providers/<dir>/testdata/. A new provider is picked
// up automatically: register it in transform.Default(), drop a testdata folder,
// and the harness exercises it with no edit here.
//
// Per provider, three checks run (matching plan 05 "Test architecture"):
//
//	parse     Parse(in) -> ToCanonical equals want.canonical.yaml.
//	roundtrip Serialize(ToCanonical(Parse(in))) re-parsed yields the same
//	          canonical (lossless), and matches want.<ext> when present.
//	schema    Schema() is a well-formed JSON Schema (compiles under
//	          santhosh-tekuri/jsonschema/v6).
//
// A registered provider with no testdata fixture is a hard FAILURE (no silent
// skips), so a provider can never quietly ship unverified.
package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// providersRel is the path (relative to this test package's working dir) to the
// directory holding one sub-directory per provider implementation.
const providersRel = "../providers"

// testdataDir maps a registry provider id to its testdata directory. The id and
// the package directory differ only by hyphenation (claude-code -> claudecode),
// so stripping hyphens is a total, collision-free mapping for all current
// providers. Auto-discovery therefore needs no per-provider table here.
func testdataDir(providerID string) string {
	dir := strings.ReplaceAll(providerID, "-", "")
	return filepath.Join(providersRel, dir, "testdata")
}

// inFixture returns the single in.* fixture path for a provider, or "" if none.
func inFixture(td string) string {
	matches, err := filepath.Glob(filepath.Join(td, "in.*"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// wantOutputFixture returns the want.<ext> golden for the serialized provider
// file (everything matching want.* that is NOT want.canonical.yaml), or "".
func wantOutputFixture(td string) string {
	matches, _ := filepath.Glob(filepath.Join(td, "want.*"))
	for _, m := range matches {
		if filepath.Base(m) == "want.canonical.yaml" {
			continue
		}
		return m
	}
	return ""
}

// canonicalYAML normalizes a canonical agent to the comparison form used by the
// golden want.canonical.yaml files: yaml.Marshal of the contract struct. Body is
// excluded from the comparison (the golden files carry an empty body and the
// instructions content is asserted via the lossless round-trip instead).
func canonicalYAML(t *testing.T, a contract.CanonicalAgent) string {
	t.Helper()
	a.Body = ""
	b, err := yaml.Marshal(a)
	if err != nil {
		t.Fatalf("marshal canonical: %v", err)
	}
	return string(b)
}

// loadWantCanonical reads want.canonical.yaml into the comparison form.
func loadWantCanonical(t *testing.T, td string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(td, "want.canonical.yaml"))
	if err != nil {
		t.Fatalf("read want.canonical.yaml: %v", err)
	}
	var want contract.CanonicalAgent
	if err := yaml.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse want.canonical.yaml: %v", err)
	}
	return canonicalYAML(t, want)
}

// TestConformance is the whole suite: one subtree per registered provider.
func TestConformance(t *testing.T) {
	reg := transform.Default()
	ids := reg.Providers()
	if len(ids) == 0 {
		t.Fatal("transform.Default() registered no providers")
	}

	for _, id := range ids {
		id := id
		t.Run(id, func(t *testing.T) {
			prov, ok := reg.Provider(id)
			if !ok {
				t.Fatalf("registry inconsistent: Providers() listed %q but Provider() missing", id)
			}

			td := testdataDir(id)
			in := inFixture(td)
			// Hard failure on a registered provider with no fixture — never skip.
			if in == "" {
				t.Fatalf("provider %q has NO testdata fixture (looked in %s/in.*) — register a golden or the conformance gate cannot verify it", id, td)
			}

			// --- check 1: parse -> ToCanonical == want.canonical.yaml ---
			pa, err := prov.Parse(in)
			if err != nil {
				t.Fatalf("[parse] Parse(%s): %v", in, err)
			}
			ca, err := prov.ToCanonical(pa)
			if err != nil {
				t.Fatalf("[parse] ToCanonical: %v", err)
			}
			t.Run("parse", func(t *testing.T) {
				got := canonicalYAML(t, ca)
				want := loadWantCanonical(t, td)
				if got != want {
					t.Errorf("canonical mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
				}
			})

			// --- check 2: lossless round-trip + want.<ext> ---
			t.Run("roundtrip", func(t *testing.T) {
				writes, err := prov.Serialize(ca)
				if err != nil {
					t.Fatalf("[roundtrip] Serialize: %v", err)
				}
				if len(writes) == 0 {
					t.Fatalf("[roundtrip] Serialize produced no FileWrite")
				}

				// 2a: serialized bytes match the golden want.<ext> when present.
				if wf := wantOutputFixture(td); wf != "" {
					want, err := os.ReadFile(wf)
					if err != nil {
						t.Fatalf("[roundtrip] read %s: %v", wf, err)
					}
					if string(writes[0].Data) != string(want) {
						t.Errorf("[roundtrip] serialized output != %s\n--- got ---\n%s\n--- want ---\n%s",
							filepath.Base(wf), writes[0].Data, want)
					}
				}

				// 2b: re-parse the serialized file -> canonical is identical
				// (lossless: Serialize . ToCanonical . Parse is a fixed point on
				// canonical). Write to a temp file with the original basename so
				// extension-driven providers parse correctly.
				dir := t.TempDir()
				tmp := filepath.Join(dir, filepath.Base(writes[0].Path))
				if err := os.WriteFile(tmp, writes[0].Data, 0o644); err != nil {
					t.Fatalf("[roundtrip] write temp: %v", err)
				}
				pa2, err := prov.Parse(tmp)
				if err != nil {
					t.Fatalf("[roundtrip] re-Parse serialized file: %v", err)
				}
				ca2, err := prov.ToCanonical(pa2)
				if err != nil {
					t.Fatalf("[roundtrip] re-ToCanonical: %v", err)
				}
				got := canonicalYAML(t, ca2)
				first := canonicalYAML(t, ca)
				if got != first {
					t.Errorf("[roundtrip] canonical not stable across serialize/parse\n--- first ---\n%s\n--- after round-trip ---\n%s", first, got)
				}
				// Body must survive the round-trip too (excluded from the YAML
				// comparison above, so assert it explicitly).
				if ca2.Body != ca.Body {
					t.Errorf("[roundtrip] body not preserved\n--- first ---\n%q\n--- after round-trip ---\n%q", ca.Body, ca2.Body)
				}
			})

			// --- check 3: Schema() is a well-formed JSON Schema ---
			t.Run("schema", func(t *testing.T) {
				raw := prov.Schema()
				if len(raw) == 0 {
					t.Fatalf("[schema] Schema() returned no bytes")
				}
				doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
				if err != nil {
					t.Fatalf("[schema] Schema() is not valid JSON: %v", err)
				}
				url := "tfs://conformance/" + id + "/schema.json"
				c := jsonschema.NewCompiler()
				if err := c.AddResource(url, doc); err != nil {
					t.Fatalf("[schema] AddResource: %v", err)
				}
				if _, err := c.Compile(url); err != nil {
					t.Errorf("[schema] Schema() does not compile as a JSON Schema: %v", err)
				}
			})
		})
	}
}
