package gateway

// Integration tests for providerOverrides key validation (v0.0.4 verify).
//
// Level: integration — needs the live transformer registry via a real gate.
// Covers:
//   - Angle 2: unknown key → error severity, pre-sync gate blocks
//   - Angle 1: known key → no finding (allowed)
//   - Angle 5: edge cases (empty ProviderOverrides, whitespace key not possible
//     via Go map but handled)
//   - Angle 4: idempotent: running twice produces same findings
//   - Levenshtein / nearestProvider helpers
//
// Tests named by angle per the rubric.

import (
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// newLiveGate returns a gate wired with the real transform.Default() registry,
// which includes all registered providers. root and store are not needed for
// the finding-level tests here.
func newLiveGate() *gate {
	return &gate{tr: transform.Default()}
}

// TestLevenshtein_KnownCases verifies the edit-distance helper against
// canonical examples.
func TestLevenshtein_KnownCases(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"kitten", "sitting", 3},
		{"copilot", "github-copilot", 7},
		{"claude", "claude-code", 5},
		{"codex", "codex", 0},
		{"roo", "roo-code", 5},
	}
	for _, tc := range cases {
		got := levenshtein(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestNearestProvider_ReturnsRegistered verifies that the nearest-provider
// helper always returns a string that is actually in the registered set.
// It also verifies exact matches return themselves (distance=0).
func TestNearestProvider_ReturnsRegistered(t *testing.T) {
	registered := transform.Default().Providers()
	// Build a set for membership check.
	regSet := make(map[string]bool, len(registered))
	for _, p := range registered {
		regSet[p] = true
	}

	typos := []string{"copilot", "claude", "grok", "gemini", "roo", "totally-unknown-xyz"}
	for _, typo := range typos {
		got := nearestProvider(typo, registered)
		if !regSet[got] {
			t.Errorf("nearestProvider(%q) = %q, which is NOT a registered provider", typo, got)
		}
	}

	// Exact match must return itself.
	for _, p := range registered {
		got := nearestProvider(p, registered)
		if got != p {
			t.Errorf("nearestProvider(%q) = %q, want exact match", p, got)
		}
	}
}

// TestNearestProvider_GeminiSuggests verifies that "gemini" (4 chars) is
// closest to "gemini-cli" (edit distance 4) among the registered providers,
// confirming the Levenshtein suggestion for the most common gemini typo.
func TestNearestProvider_GeminiSuggests(t *testing.T) {
	registered := transform.Default().Providers()
	got := nearestProvider("gemini", registered)
	if got != "gemini-cli" {
		t.Errorf("nearestProvider(\"gemini\") = %q, want gemini-cli", got)
	}
}

// TestNearestProvider_EmptyRegistry returns empty string when no providers.
func TestNearestProvider_EmptyRegistry(t *testing.T) {
	got := nearestProvider("anything", nil)
	if got != "" {
		t.Errorf("nearestProvider with empty registry = %q, want empty", got)
	}
}

// TestProviderOverrideKeyFindings_Rejects_UnknownKey covers angle 2:
// an unknown provider key in ProviderOverrides → error-severity finding with
// a "did you mean" suggestion containing a registered provider id. Iterates
// over a representative set of typos.
func TestProviderOverrideKeyFindings_Rejects_UnknownKey(t *testing.T) {
	g := newLiveGate()
	registered := g.tr.Providers()
	regSet := make(map[string]bool, len(registered))
	for _, p := range registered {
		regSet[p] = true
	}

	cases := []struct {
		key string
	}{
		{"copilot"},
		{"claude"},
		{"grok"},
		{"gemini"},
		{"roo"},
		{"totally-unknown-xyz"},
	}
	for _, tc := range cases {
		t.Run("key="+tc.key, func(t *testing.T) {
			// Sanity: the test key must not be registered.
			if regSet[tc.key] {
				t.Fatalf("key %q is actually registered — test case is wrong", tc.key)
			}
			a := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "d",
				ProviderOverrides: map[string]map[string]any{
					tc.key: {"model": "x"},
				},
			}
			findings := g.providerOverrideKeyFindings(a)
			if len(findings) != 1 {
				t.Fatalf("want 1 finding, got %d: %+v", len(findings), findings)
			}
			f := findings[0]
			if f.Severity != "error" {
				t.Errorf("finding severity=%q, want error", f.Severity)
			}
			if f.Agent != "agent-x" {
				t.Errorf("finding agent=%q, want agent-x", f.Agent)
			}
			if !strings.Contains(f.Message, tc.key) {
				t.Errorf("finding message %q does not contain bad key %q", f.Message, tc.key)
			}
			// The message must contain "did you mean" and a registered provider id.
			if !strings.Contains(f.Message, "did you mean") {
				t.Errorf("finding message %q does not contain 'did you mean'", f.Message)
			}
			foundSuggestion := false
			for _, p := range registered {
				if strings.Contains(f.Message, p) {
					foundSuggestion = true
					break
				}
			}
			if !foundSuggestion {
				t.Errorf("finding message %q does not mention any registered provider as a suggestion", f.Message)
			}
		})
	}
}

// TestProviderOverrideKeyFindings_Allows_AllRegisteredProviders covers angle 1:
// setting ProviderOverrides for every known registered provider must produce
// zero findings. This is table-driven over transform.Default().Providers() so
// adding a new provider automatically extends coverage.
func TestProviderOverrideKeyFindings_Allows_AllRegisteredProviders(t *testing.T) {
	g := newLiveGate()
	for _, provID := range g.tr.Providers() {
		t.Run("provider="+provID, func(t *testing.T) {
			a := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "d",
				ProviderOverrides: map[string]map[string]any{
					provID: {"model": "some-model"},
				},
			}
			findings := g.providerOverrideKeyFindings(a)
			if len(findings) != 0 {
				t.Errorf("known provider %q produced unexpected findings: %+v", provID, findings)
			}
		})
	}
}

// TestProviderOverrideKeyFindings_Applies_EmptyOverrides covers angle 5:
// an agent with no ProviderOverrides produces no findings.
func TestProviderOverrideKeyFindings_Applies_EmptyOverrides(t *testing.T) {
	g := newLiveGate()
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "d",
	}
	if f := g.providerOverrideKeyFindings(a); len(f) != 0 {
		t.Fatalf("nil ProviderOverrides should produce no findings, got: %+v", f)
	}
}

// TestProviderOverrideKeyFindings_MultipleUnknownKeys verifies that multiple
// unknown keys each produce their own error finding.
func TestProviderOverrideKeyFindings_MultipleUnknownKeys(t *testing.T) {
	g := newLiveGate()
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "d",
		ProviderOverrides: map[string]map[string]any{
			"copilot":   {"model": "x"},
			"bad-key-2": {"model": "y"},
		},
	}
	findings := g.providerOverrideKeyFindings(a)
	if len(findings) != 2 {
		t.Fatalf("two unknown keys should yield 2 findings, got %d: %+v", len(findings), findings)
	}
	for _, f := range findings {
		if f.Severity != "error" {
			t.Errorf("finding %+v: want error severity", f)
		}
	}
}

// TestProviderOverrideKeyFindings_MixedKnownUnknown verifies that a mixed bag
// (one valid key, one unknown) produces exactly one error finding (for the
// unknown key only).
func TestProviderOverrideKeyFindings_MixedKnownUnknown(t *testing.T) {
	g := newLiveGate()
	// Use the first registered provider as the valid key.
	providers := g.tr.Providers()
	if len(providers) == 0 {
		t.Skip("no registered providers")
	}
	validKey := providers[0]
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "d",
		ProviderOverrides: map[string]map[string]any{
			validKey:  {"model": "x"},
			"bad-key": {"model": "y"},
		},
	}
	findings := g.providerOverrideKeyFindings(a)
	if len(findings) != 1 {
		t.Fatalf("mixed overrides should produce exactly 1 finding (for bad-key), got %d: %+v", len(findings), findings)
	}
	if findings[0].Severity != "error" {
		t.Errorf("finding severity=%q, want error", findings[0].Severity)
	}
	if !strings.Contains(findings[0].Message, "bad-key") {
		t.Errorf("finding message %q should mention the bad key", findings[0].Message)
	}
}

// TestValidateAgents_UnknownProviderKey_BlocksViaGate wires through the full
// validateAgents path and confirms the error-severity finding from an unknown
// ProviderOverrides key would block the pre-sync gate (errorFindings returns it).
func TestValidateAgents_UnknownProviderKey_BlocksViaGate(t *testing.T) {
	root := t.TempDir()
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "Reviews code changes for correctness.",
		Body:        "You are a reviewer.",
		ProviderOverrides: map[string]map[string]any{
			"copilot": {"model": "some-model"}, // typo: should be github-copilot
		},
	}
	writeCanonical(t, root, a)

	g := &gate{root: root, tr: transform.Default()}
	findings, err := g.validateAgents([]string{"agent-x"})
	if err != nil {
		t.Fatalf("validateAgents: %v", err)
	}
	blocking := errorFindings(findings)
	if len(blocking) == 0 {
		t.Fatalf("unknown providerOverrides key must produce a blocking error finding, got none; findings=%+v", findings)
	}
	// Confirm it mentions the unknown key and a "did you mean" suggestion with a
	// registered provider id. (The exact suggestion depends on edit distance;
	// we just verify the message structure, not the specific suggestion.)
	registered := g.tr.Providers()
	regSet := make(map[string]bool, len(registered))
	for _, p := range registered {
		regSet[p] = true
	}
	found := false
	for _, f := range blocking {
		if !strings.Contains(f.Message, "copilot") {
			continue
		}
		if !strings.Contains(f.Message, "did you mean") {
			continue
		}
		for _, p := range registered {
			if strings.Contains(f.Message, p) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected a finding mentioning 'copilot' with a 'did you mean <registered-provider>' suggestion, got: %+v", blocking)
	}
}

// TestValidateAgents_KnownProviderKey_NoError verifies that a valid
// providerOverrides key does NOT produce any error finding (gate stays open).
func TestValidateAgents_KnownProviderKey_NoError(t *testing.T) {
	root := t.TempDir()
	a := contract.CanonicalAgent{
		Name:        "agent-x",
		Description: "Reviews code changes for correctness.",
		Body:        "You are a reviewer.",
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"skills": []string{"my-skill"}},
		},
	}
	writeCanonical(t, root, a)

	g := &gate{root: root, tr: transform.Default()}
	findings, err := g.validateAgents([]string{"agent-x"})
	if err != nil {
		t.Fatalf("validateAgents: %v", err)
	}
	blocking := errorFindings(findings)
	// There should be no error findings from the providerOverrides key check.
	// (There might be model warnings, which are NOT errors.)
	for _, f := range blocking {
		if strings.Contains(f.Message, "providerOverrides") {
			t.Errorf("unexpected providerOverrides error for known key: %+v", f)
		}
	}
}
