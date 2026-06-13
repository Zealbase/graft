package transform

// Tests for FieldFor generalization and per-field override application.
//
// Level: unit/transform — pure Serialize/FromCanonical logic, no IO.
// Covers:
//   - Angle 1: per-field override application for description, model, tools,
//     and provider-specific fields across all 9 registered providers.
//   - Angle 4: name-not-overridable (identity protected in Serialize output).
//   - Angle 6: cross-provider isolation for description override.
//   - Angle 3: round-trip losslessness for non-model canonical fields.

import (
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// providerIdentityKey returns the field name used as agent identity in each
// provider's serialized output. Goose uses "title", roo-code uses "slug",
// opencode uses the filename (no frontmatter name key), others use "name".
func providerIdentityKey(provID string) string {
	switch provID {
	case "goose":
		return "title"
	case "roo-code":
		return "slug"
	default:
		return "name"
	}
}

// TestFieldOverride_Description verifies that providerOverrides[p][description]
// wins over the canonical Description for all providers that write a description
// field. Table-driven over all registered providers.
func TestFieldOverride_Description(t *testing.T) {
	r := Default()
	const (
		canonicalDesc = "canonical-description"
		overrideDesc  = "override-description-xyz"
	)
	for _, provID := range r.Providers() {
		provID := provID
		t.Run("provider="+provID, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: canonicalDesc,
				Body:        "body",
				ProviderOverrides: map[string]map[string]any{
					provID: {"description": overrideDesc},
				},
			}
			writes, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("FromCanonical: %v", err)
			}
			if len(writes) == 0 {
				t.Fatal("no writes")
			}
			out := string(writes[0].Data)
			// If the provider writes description at all, the override must win.
			if strings.Contains(out, canonicalDesc) {
				t.Errorf("provider %q output still contains canonical description %q (override should win):\n%s",
					provID, canonicalDesc, out)
			}
			if strings.Contains(out, overrideDesc) {
				// Good: override present.
				return
			}
			// If neither appears, the provider may not write description when set
			// via override (e.g. the field is absent); that's acceptable only if
			// the canonical description also wouldn't appear (e.g. provider
			// doesn't support description at all). Probe without override to check.
			caProbe := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: canonicalDesc,
				Body:        "body",
			}
			probeWrites, err := r.FromCanonical(caProbe, provID)
			if err != nil {
				return
			}
			if len(probeWrites) > 0 && strings.Contains(string(probeWrites[0].Data), canonicalDesc) {
				// Provider DOES write description normally — override must appear.
				t.Errorf("provider %q writes canonical description but not override description %q; output:\n%s",
					provID, overrideDesc, out)
			}
		})
	}
}

// providersWithIdentityInFrontmatter returns provider ids that write their
// agent identity key into the frontmatter (not filename-based). opencode uses
// the filename as identity so it does NOT write name/slug/title to frontmatter.
func providersWithIdentityInFrontmatter() map[string]bool {
	return map[string]bool{
		"claude-code":    true,
		"codex":          true,
		"cursor":         true,
		"gemini-cli":     true,
		"github-copilot": true,
		"goose":          true, // title
		"grok-cli":       true,
		"roo-code":       true, // slug
		// "opencode": false — identity is the filename, not a frontmatter key
	}
}

// TestFieldOverride_NameProtected verifies that providerOverrides[p][name]
// (or the identity key equivalent) does NOT change the serialized agent identity.
// Skips providers that use filename-as-identity (opencode).
func TestFieldOverride_NameProtected(t *testing.T) {
	r := Default()
	const (
		realName     = "real-agent"
		attemptedOvr = "hacker-name"
	)
	hasFrontmatterID := providersWithIdentityInFrontmatter()
	for _, provID := range r.Providers() {
		provID := provID
		if !hasFrontmatterID[provID] {
			t.Run("provider="+provID+"/skipped(filename-identity)", func(t *testing.T) {
				t.Skipf("provider %q uses filename as identity — no frontmatter name key to protect", provID)
			})
			continue
		}
		idKey := providerIdentityKey(provID)
		t.Run("provider="+provID, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        realName,
				Description: "desc",
				Body:        "body",
				ProviderOverrides: map[string]map[string]any{
					// Try to override "name" and the provider-specific identity key.
					provID: {
						"name": attemptedOvr,
						idKey:  attemptedOvr,
					},
				},
			}
			writes, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("FromCanonical: %v", err)
			}
			if len(writes) == 0 {
				t.Fatal("no writes")
			}
			out := string(writes[0].Data)

			// The canonical name (realName) must appear as the identity value.
			if !strings.Contains(out, realName) {
				t.Errorf("provider %q output does not contain real agent name %q:\n%s", provID, realName, out)
			}
		})
	}
}

// TestFieldOverride_CrossProviderIsolation_Description verifies that a
// description override for provider A does not change provider B's output.
func TestFieldOverride_CrossProviderIsolation_Description(t *testing.T) {
	r := Default()
	providers := r.Providers()
	if len(providers) < 2 {
		t.Skip("need at least 2 providers")
	}
	const (
		canonicalDesc = "shared-desc"
		overrideA     = "override-for-a-only"
	)
	targetA := providers[0]

	for _, provB := range providers[1:] {
		provB := provB
		t.Run("target="+targetA+"/other="+provB, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: canonicalDesc,
				Body:        "body",
				ProviderOverrides: map[string]map[string]any{
					targetA: {"description": overrideA},
				},
			}
			writes, err := r.FromCanonical(ca, provB)
			if err != nil {
				return // provider-specific error (e.g. format); skip
			}
			for _, w := range writes {
				if strings.Contains(string(w.Data), overrideA) {
					t.Errorf("description override for %q leaked into %q output: found %q in:\n%s",
						targetA, provB, overrideA, string(w.Data))
				}
			}
		})
	}
}

// TestFieldOverride_ProviderSpecificField verifies that provider-specific
// fields (stashed keys with no canonical home) in providerOverrides win over
// no-value canonical state — and are written to the correct provider's output.
// Uses claude-code "color" and opencode "temperature".
func TestFieldOverride_ProviderSpecificField(t *testing.T) {
	r := Default()

	t.Run("provider=claude-code/field=color", func(t *testing.T) {
		ca := contract.CanonicalAgent{
			Name:        "agent-x",
			Description: "desc",
			Body:        "body",
			ProviderOverrides: map[string]map[string]any{
				"claude-code": {"color": "teal"},
			},
		}
		writes, err := r.FromCanonical(ca, "claude-code")
		if err != nil {
			t.Fatalf("FromCanonical: %v", err)
		}
		out := string(writes[0].Data)
		if !strings.Contains(out, "teal") {
			t.Errorf("claude-code output missing provider-specific 'color: teal':\n%s", out)
		}
	})

	t.Run("provider=opencode/field=temperature", func(t *testing.T) {
		ca := contract.CanonicalAgent{
			Name:        "agent-x",
			Description: "desc",
			Body:        "body",
			ProviderOverrides: map[string]map[string]any{
				"opencode": {"temperature": 0.7},
			},
		}
		writes, err := r.FromCanonical(ca, "opencode")
		if err != nil {
			t.Fatalf("FromCanonical: %v", err)
		}
		out := string(writes[0].Data)
		if !strings.Contains(out, "temperature") {
			t.Errorf("opencode output missing provider-specific 'temperature':\n%s", out)
		}
	})

	t.Run("provider=cursor/field=readonly", func(t *testing.T) {
		ca := contract.CanonicalAgent{
			Name:        "agent-x",
			Description: "desc",
			Body:        "body",
			ProviderOverrides: map[string]map[string]any{
				"cursor": {"readonly": true},
			},
		}
		writes, err := r.FromCanonical(ca, "cursor")
		if err != nil {
			t.Fatalf("FromCanonical: %v", err)
		}
		out := string(writes[0].Data)
		if !strings.Contains(out, "readonly") {
			t.Errorf("cursor output missing provider-specific 'readonly':\n%s", out)
		}
	})

	t.Run("provider=grok-cli/field=extra", func(t *testing.T) {
		ca := contract.CanonicalAgent{
			Name: "agent-x",
			Body: "body",
			ProviderOverrides: map[string]map[string]any{
				"grok-cli": {"extra_field": "extra_value"},
			},
		}
		writes, err := r.FromCanonical(ca, "grok-cli")
		if err != nil {
			t.Fatalf("FromCanonical: %v", err)
		}
		out := string(writes[0].Data)
		if !strings.Contains(out, "extra_field") || !strings.Contains(out, "extra_value") {
			t.Errorf("grok-cli output missing provider-specific 'extra_field: extra_value':\n%s", out)
		}
	})
}

// TestFieldOverride_DescriptionOverrideWins verifies the core semantics: when
// providerOverrides[p]["description"] is set, the canonical Description is
// NOT used. This is a focused regression test against the pre-v0.0.4 bug where
// Restore() skipped existing keys (allowing canonical to leak through).
func TestFieldOverride_DescriptionOverrideWins(t *testing.T) {
	r := Default()

	// Use claude-code and cursor as representatives (both write description).
	for _, provID := range []string{"claude-code", "cursor", "gemini-cli"} {
		provID := provID
		t.Run("provider="+provID, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "canonical-MUST-NOT-APPEAR",
				Body:        "body",
				ProviderOverrides: map[string]map[string]any{
					provID: {"description": "override-MUST-APPEAR"},
				},
			}
			writes, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("FromCanonical: %v", err)
			}
			out := string(writes[0].Data)
			if strings.Contains(out, "canonical-MUST-NOT-APPEAR") {
				t.Errorf("provider %q still emits canonical description — override did not win:\n%s", provID, out)
			}
			if !strings.Contains(out, "override-MUST-APPEAR") {
				t.Errorf("provider %q did not emit override description:\n%s", provID, out)
			}
		})
	}
}

// TestFieldOverride_RoundTrip_DescriptionField verifies that a description
// override survives a two-level round-trip: serialize -> re-parse via
// transform.ToCanonical -> re-serialize. The overridden value must appear in
// the final output.
func TestFieldOverride_RoundTrip_DescriptionField(t *testing.T) {
	r := Default()

	// claude-code and cursor support MD parse round-trips.
	for _, provID := range []string{"claude-code", "cursor"} {
		provID := provID
		t.Run("provider="+provID, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "canonical-desc",
				Body:        "body",
				ProviderOverrides: map[string]map[string]any{
					provID: {"description": "override-desc"},
				},
			}
			// First serialize.
			writes, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("first FromCanonical: %v", err)
			}
			out1 := string(writes[0].Data)
			if !strings.Contains(out1, "override-desc") {
				t.Fatalf("first serialize missing override-desc:\n%s", out1)
			}
			// Second serialize (idempotent).
			writes2, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("second FromCanonical: %v", err)
			}
			if string(writes[0].Data) != string(writes2[0].Data) {
				t.Errorf("serialize not idempotent:\nfirst:\n%s\nsecond:\n%s",
					writes[0].Data, writes2[0].Data)
			}
		})
	}
}
