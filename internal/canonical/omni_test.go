package canonical

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// --- .meta.json OmniRef persistence ---

// TestMetaOmniRoundTrip verifies that an OmniRef set on Meta survives a
// save→load round-trip with its fields intact.
func TestMetaOmniRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	meta := Meta{
		Omni: &contract.OmniRef{Ref: "reviewer", Applied: true, Supported: true},
	}
	writes, err := SaveWithMeta(dir, a, meta)
	if err != nil {
		t.Fatalf("SaveWithMeta: %v", err)
	}
	writeAll(t, writes)

	got, err := LoadMeta(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if got.Omni == nil {
		t.Fatal("OmniRef lost in round-trip (got nil)")
	}
	if got.Omni.Ref != "reviewer" || !got.Omni.Applied || !got.Omni.Supported {
		t.Fatalf("OmniRef round-trip mismatch: %#v", got.Omni)
	}
}

// TestMetaOmniAbsentNoKey verifies that a Meta without an OmniRef does NOT emit
// an "omni" key (so existing meta files are unaffected) and loads as nil.
func TestMetaOmniAbsentNoKey(t *testing.T) {
	dir := t.TempDir()
	a := sampleAgent()
	writes, err := SaveWithMeta(dir, a, Meta{})
	if err != nil {
		t.Fatalf("SaveWithMeta: %v", err)
	}
	writeAll(t, writes)

	raw, err := os.ReadFile(filepath.Join(AgentDir(dir, a.Name), metaFile))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if strings.Contains(string(raw), "omni") {
		t.Fatalf("absent OmniRef must not emit an 'omni' key; got:\n%s", raw)
	}

	got, err := LoadMeta(AgentDir(dir, a.Name))
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if got.Omni != nil {
		t.Fatalf("expected nil OmniRef when absent, got %#v", got.Omni)
	}
}

// TestMetaOmniBackCompatLoad verifies that a legacy .meta.json with no omni
// field (written before omni existed) loads cleanly with a nil OmniRef and its
// other fields intact.
func TestMetaOmniBackCompatLoad(t *testing.T) {
	dir := t.TempDir()
	agentD := AgentDir(dir, "legacy")
	if err := os.MkdirAll(agentD, 0o755); err != nil {
		t.Fatal(err)
	}
	// Hand-write an old-format meta with NO omni key.
	old := `{
  "canonicalHash": "abc123",
  "providers": {
    "claude": {"sourceHash": "deadbeef", "lastCommitHash": "cafe"}
  }
}
`
	if err := os.WriteFile(filepath.Join(agentD, metaFile), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadMeta(agentD)
	if err != nil {
		t.Fatalf("LoadMeta legacy: %v", err)
	}
	if got.Omni != nil {
		t.Fatalf("legacy meta must load with nil OmniRef, got %#v", got.Omni)
	}
	if got.CanonicalHash != "abc123" {
		t.Fatalf("legacy canonicalHash lost: %q", got.CanonicalHash)
	}
	pm, ok := got.Providers["claude"]
	if !ok || pm.SourceHash != "deadbeef" {
		t.Fatalf("legacy provider meta lost: %#v", got.Providers)
	}
}

// --- PrependOmniBlock / ReplaceOmniBlock ---

func TestPrependOmniBlockOnce(t *testing.T) {
	body := "Original instructions.\nSecond line.\n"
	out := PrependOmniBlock(body, "reviewer", "Be a careful reviewer.")

	if !strings.HasPrefix(out, omniOpenPrefix+"reviewer"+omniOpenSuffix+"\n") {
		t.Fatalf("output must start with the open marker; got:\n%s", out)
	}
	if !strings.Contains(out, "Be a careful reviewer.") {
		t.Fatal("sys-instructions missing from block")
	}
	if !strings.Contains(out, omniClose) {
		t.Fatal("close marker missing")
	}
	if !strings.HasSuffix(out, body) {
		t.Fatalf("original body must be preserved verbatim at the end; got:\n%s", out)
	}
	if !HasOmniBlock(out) {
		t.Fatal("HasOmniBlock should be true after prepend")
	}
}

// TestPrependOmniBlockIdempotent verifies that prepending twice with the same
// args yields a result identical to prepending once (no duplication, no nesting).
func TestPrependOmniBlockIdempotent(t *testing.T) {
	body := "Original instructions.\n"
	once := PrependOmniBlock(body, "reviewer", "Be careful.")
	twice := PrependOmniBlock(once, "reviewer", "Be careful.")
	if once != twice {
		t.Fatalf("prepend must be idempotent:\nonce=%q\ntwice=%q", once, twice)
	}
	// Exactly one open marker, one close marker.
	if got := strings.Count(twice, omniOpenPrefix); got != 1 {
		t.Fatalf("expected exactly 1 open marker, got %d", got)
	}
	if got := strings.Count(twice, omniClose); got != 1 {
		t.Fatalf("expected exactly 1 close marker, got %d", got)
	}
}

// TestReplaceOmniBlockInPlace verifies that re-prepending with different ref /
// sys-instructions replaces the leading block in place rather than duplicating.
func TestReplaceOmniBlockInPlace(t *testing.T) {
	body := "Original instructions.\n"
	first := PrependOmniBlock(body, "old-ref", "Old instructions.")
	second := ReplaceOmniBlock(first, "new-ref", "New instructions.")

	if strings.Contains(second, "old-ref") || strings.Contains(second, "Old instructions.") {
		t.Fatalf("old block must be replaced, not retained:\n%s", second)
	}
	if !strings.Contains(second, "new-ref") || !strings.Contains(second, "New instructions.") {
		t.Fatalf("new block missing:\n%s", second)
	}
	if got := strings.Count(second, omniOpenPrefix); got != 1 {
		t.Fatalf("expected exactly 1 open marker after replace, got %d", got)
	}
	if !strings.HasSuffix(second, body) {
		t.Fatalf("original body must survive replace:\n%s", second)
	}
}

// TestPrependOmniBlockEmptyRefNoop verifies that an empty ref is a no-op.
func TestPrependOmniBlockEmptyRefNoop(t *testing.T) {
	body := "Original instructions.\n"
	if out := PrependOmniBlock(body, "", "ignored"); out != body {
		t.Fatalf("empty ref must be a no-op; got:\n%s", out)
	}
	if HasOmniBlock(body) {
		t.Fatal("body with no block should report HasOmniBlock=false")
	}
}

// TestPrependOmniBlockPreservesUserSentinel verifies that a user-authored body
// containing a literal graft sentinel that is NOT the leading managed block is
// preserved verbatim — never hijacked, stripped, or duplicated.
func TestPrependOmniBlockPreservesUserSentinel(t *testing.T) {
	// User wrote text that mentions the sentinel mid-body. It is NOT at offset 0.
	userBody := "Here is how graft works:\n" +
		omniOpenPrefix + "example -->\nsome text\n" + omniClose + "\nMore docs.\n"

	out := PrependOmniBlock(userBody, "reviewer", "Be careful.")

	// The user's literal sentinel text must survive verbatim at the tail.
	if !strings.HasSuffix(out, userBody) {
		t.Fatalf("user body with literal sentinel must be preserved verbatim:\n%s", out)
	}
	// There must now be the managed open marker plus the user's, total 2.
	if got := strings.Count(out, omniOpenPrefix); got != 2 {
		t.Fatalf("expected managed + user open markers (2), got %d:\n%s", got, out)
	}

	// Re-prepend: still idempotent and still preserves the user's sentinel.
	out2 := PrependOmniBlock(out, "reviewer", "Be careful.")
	if out != out2 {
		t.Fatalf("re-prepend over user-sentinel body must be idempotent:\n%q\n%q", out, out2)
	}
}

// TestStripLeadingHalfOpenSentinelNotHijacked verifies that a body starting with
// a half-open / malformed sentinel (open marker but no close) is NOT treated as
// a managed block: prepend leaves it intact below the new block.
func TestStripLeadingHalfOpenSentinelNotHijacked(t *testing.T) {
	// Leading open marker but no matching close line anywhere.
	malformed := omniOpenPrefix + "halfopen -->\nsome user text with no close marker\n"
	if HasOmniBlock(malformed) {
		t.Fatal("a half-open leading sentinel must NOT be recognized as a managed block")
	}
	out := PrependOmniBlock(malformed, "reviewer", "Be careful.")
	if !strings.HasSuffix(out, malformed) {
		t.Fatalf("malformed leading sentinel must be preserved verbatim below the new block:\n%s", out)
	}
}

// TestPrependOmniBlockBodyFidelity verifies CRLF / multiline / trailing-newline
// fidelity for the user body outside the managed block.
func TestPrependOmniBlockBodyFidelity(t *testing.T) {
	cases := []string{
		"line1\r\nline2\r\nline3\r\n",     // CRLF
		"a\n\n\nb\n",                       // internal blank lines
		"no trailing newline",              // no trailing newline
		"emoji 🚀 and ünïcödé\nsecond\n",   // non-ASCII
		"",                                 // empty body still gets a block since ref != ""
	}
	for _, body := range cases {
		out := PrependOmniBlock(body, "ref", "sys")
		if body == "" {
			// Empty body: result is just the block, no trailing body.
			if !strings.HasPrefix(out, omniOpenPrefix) || !strings.HasSuffix(out, omniClose) {
				t.Fatalf("empty body: unexpected block shape:\n%q", out)
			}
			continue
		}
		if !strings.HasSuffix(out, body) {
			t.Fatalf("body not preserved byte-exact for %q; got:\n%q", body, out)
		}
	}
}

// --- DefaultOmniResolver ---

func TestDefaultOmniResolverUnsupported(t *testing.T) {
	var r contract.OmniResolver = DefaultOmniResolver{}
	if r.Supported("anything") {
		t.Fatal("DefaultOmniResolver.Supported must always be false")
	}
	out, err := r.Resolve("anything")
	if err == nil {
		t.Fatal("DefaultOmniResolver.Resolve must return an error")
	}
	if out != "" {
		t.Fatalf("DefaultOmniResolver.Resolve must return empty string, got %q", out)
	}
}

// TestContainsOmniMarker covers the sentinel-collision guard used by the gateway
// to refuse self-corrupting sys-instructions.
func TestContainsOmniMarker(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"plain text", "Just normal instructions.\nMore lines.", false},
		{"close marker own line", "Real.\n<!-- /graft:omni -->\nSmuggled.", true},
		{"close marker with CRLF", "Real.\r\n<!-- /graft:omni -->\r\nSmuggled.", true},
		{"open prefix at line start", "<!-- graft:omni foo -->\nbody", true},
		{"marker embedded mid-line not flagged", "see <!-- /graft:omni --> inline", false},
		{"close marker as the whole string", omniClose, true},
		{"empty string", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsOmniMarker(tc.in); got != tc.want {
				t.Fatalf("ContainsOmniMarker(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
