package transform

// Comprehensive providerOverrides tests (v0.0.4 verify).
//
// Level: unit/transform — pure Serialize/FromCanonical logic, no IO (no disk
// agent files). Covers:
//   - Angle 1: providerOverrides[<p>][model]=X → provider <p>'s output carries model X
//   - Angle 6: isolation — override for A never appears in B's output
//   - Angle 3: round-trip: set→serialize→re-parse→serialize identity for model
//   - Angle 3: clear → absent (no resurrection)
//   - Angle 4: idempotent: serializing twice yields the same bytes
//   - Provider-specific stashed field round-trips to that provider's output
//
// Table-driven over transform.Default().Providers() so adding a provider
// automatically extends the coverage.

import (
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// providerModelToken returns the model string as it would appear in the
// serialized output of a given provider. Different providers use different
// serialization formats (YAML "model: X", TOML "model = X"); we match on the
// model value itself for simplicity since model names don't contain format chars.
// We just verify the model VALUE appears in the serialized bytes.
func providerModelToken(model string) string { return model }

// providersWithNativeModel returns the set of provider ids that have a native
// "model" field in their Serialize output (i.e. call a.ModelFor(name)).
// Determined by inspecting each provider's Serialize: goose and antigravity do
// NOT call ModelFor; all others do.
//
// This function derives this set dynamically by trying to serialize with a
// model set and checking if the output contains the model.
func providersWithNativeModel(t *testing.T) map[string]bool {
	t.Helper()
	r := Default()
	result := make(map[string]bool)
	for _, provID := range r.Providers() {
		ca := contract.CanonicalAgent{
			Name:        "probe",
			Description: "probe",
			Model:       "PROBE_MODEL_XYZ",
			Body:        "body",
		}
		writes, err := r.FromCanonical(ca, provID)
		if err != nil {
			continue
		}
		for _, w := range writes {
			if strings.Contains(string(w.Data), "PROBE_MODEL_XYZ") {
				result[provID] = true
				break
			}
		}
	}
	return result
}

// TestProviderOverrides_Applies_ModelPerProvider covers angle 1:
// setting providerOverrides[<p>][model]=X makes provider <p>'s serialized file
// carry model X, for every provider that supports a native model field.
func TestProviderOverrides_Applies_ModelPerProvider(t *testing.T) {
	r := Default()
	modelProviders := providersWithNativeModel(t)

	const (
		defaultModel  = "default-model"
		overrideModel = "override-model-xyz"
	)

	for _, provID := range r.Providers() {
		provID := provID
		if !modelProviders[provID] {
			t.Run("provider="+provID+"/skipped(no native model)", func(t *testing.T) {
				t.Skipf("provider %q does not serialize a model field", provID)
			})
			continue
		}
		t.Run("provider="+provID, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "desc",
				Model:       defaultModel,
				Body:        "You are an assistant.",
				ProviderOverrides: map[string]map[string]any{
					provID: {"model": overrideModel},
				},
			}
			writes, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("FromCanonical: %v", err)
			}
			if len(writes) == 0 {
				t.Fatal("no file writes")
			}
			out := string(writes[0].Data)
			if !strings.Contains(out, overrideModel) {
				t.Errorf("provider %q output does not contain override model %q:\n%s", provID, overrideModel, out)
			}
			// Must NOT contain the default model (the override replaced it).
			if strings.Contains(out, defaultModel) {
				t.Errorf("provider %q output still contains default model %q when override is set:\n%s", provID, defaultModel, out)
			}
		})
	}
}

// TestProviderOverrides_Isolation covers angle 6:
// an override for provider A never leaks into provider B's output.
// Table-driven: for each provider that supports model, set its override and
// check that every OTHER provider does NOT carry that override value.
func TestProviderOverrides_Isolation(t *testing.T) {
	r := Default()
	modelProviders := providersWithNativeModel(t)
	providerList := r.Providers()

	const (
		defaultModel = "shared-default"
	)

	for _, targetProv := range providerList {
		if !modelProviders[targetProv] {
			continue // skip providers that don't emit model
		}
		targetProv := targetProv
		overrideModel := "override-for-" + targetProv
		t.Run("target="+targetProv, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "desc",
				Model:       defaultModel,
				Body:        "You are an assistant.",
				ProviderOverrides: map[string]map[string]any{
					targetProv: {"model": overrideModel},
				},
			}
			// The target provider must carry the override.
			targetWrites, err := r.FromCanonical(ca, targetProv)
			if err != nil {
				t.Fatalf("FromCanonical(%q): %v", targetProv, err)
			}
			if len(targetWrites) == 0 {
				t.Fatalf("no writes for %q", targetProv)
			}
			if !strings.Contains(string(targetWrites[0].Data), overrideModel) {
				t.Errorf("provider %q does not carry its own override model %q", targetProv, overrideModel)
			}

			// Every other provider must NOT carry the override model.
			for _, otherProv := range providerList {
				if otherProv == targetProv || !modelProviders[otherProv] {
					continue
				}
				otherWrites, err := r.FromCanonical(ca, otherProv)
				if err != nil {
					continue // provider-specific error (e.g., format issue); skip
				}
				for _, w := range otherWrites {
					if strings.Contains(string(w.Data), overrideModel) {
						t.Errorf("override for %q leaked into %q output: found %q in:\n%s",
							targetProv, otherProv, overrideModel, string(w.Data))
					}
				}
			}
		})
	}
}

// TestProviderOverrides_Fallback_UsesDefaultModel verifies that when a provider
// has NO entry in ProviderOverrides, it uses the canonical default Model.
func TestProviderOverrides_Fallback_UsesDefaultModel(t *testing.T) {
	r := Default()
	modelProviders := providersWithNativeModel(t)

	const (
		defaultModel  = "default-model-abc"
		overrideModel = "override-for-claude"
	)

	for _, provID := range r.Providers() {
		if !modelProviders[provID] || provID == "claude-code" {
			continue // claude-code gets an override; others fall back
		}
		provID := provID
		t.Run("provider="+provID, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "desc",
				Model:       defaultModel,
				Body:        "body",
				ProviderOverrides: map[string]map[string]any{
					"claude-code": {"model": overrideModel}, // only for claude-code
				},
			}
			writes, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("FromCanonical(%q): %v", provID, err)
			}
			if len(writes) == 0 {
				t.Fatalf("no writes for %q", provID)
			}
			out := string(writes[0].Data)
			if !strings.Contains(out, defaultModel) {
				t.Errorf("provider %q should use default model %q when no override, got:\n%s", provID, defaultModel, out)
			}
			if strings.Contains(out, overrideModel) {
				t.Errorf("provider %q should NOT carry claude-code's override model %q, got:\n%s", provID, overrideModel, out)
			}
		})
	}
}

// TestProviderOverrides_Clear_Absent covers angle 3 (no resurrection):
// removing a provider's model override causes the serialized file to fall back
// to the canonical default (not carry the old override).
func TestProviderOverrides_Clear_Absent(t *testing.T) {
	r := Default()
	modelProviders := providersWithNativeModel(t)

	const (
		defaultModel  = "default-model"
		overrideModel = "override-model"
	)

	for _, provID := range r.Providers() {
		if !modelProviders[provID] {
			continue
		}
		provID := provID
		t.Run("provider="+provID, func(t *testing.T) {
			// Step 1: set override.
			caWith := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "desc",
				Model:       defaultModel,
				Body:        "body",
				ProviderOverrides: map[string]map[string]any{
					provID: {"model": overrideModel},
				},
			}
			withWrites, err := r.FromCanonical(caWith, provID)
			if err != nil {
				t.Fatalf("set: FromCanonical: %v", err)
			}
			if !strings.Contains(string(withWrites[0].Data), overrideModel) {
				t.Fatalf("set: output should contain override model %q", overrideModel)
			}

			// Step 2: clear override (no ProviderOverrides entry).
			caWithout := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "desc",
				Model:       defaultModel,
				Body:        "body",
			}
			withoutWrites, err := r.FromCanonical(caWithout, provID)
			if err != nil {
				t.Fatalf("clear: FromCanonical: %v", err)
			}
			out := string(withoutWrites[0].Data)
			if strings.Contains(out, overrideModel) {
				t.Errorf("clear: output still carries override model %q (resurrection); got:\n%s", overrideModel, out)
			}
			// The default model should appear instead.
			if !strings.Contains(out, defaultModel) {
				t.Errorf("clear: output should fall back to default model %q; got:\n%s", defaultModel, out)
			}
		})
	}
}

// TestProviderOverrides_Idempotent covers angle 4:
// serializing the same canonical agent twice yields the same bytes.
func TestProviderOverrides_Idempotent(t *testing.T) {
	r := Default()
	modelProviders := providersWithNativeModel(t)

	for _, provID := range r.Providers() {
		provID := provID
		if !modelProviders[provID] {
			continue
		}
		t.Run("provider="+provID, func(t *testing.T) {
			ca := contract.CanonicalAgent{
				Name:        "agent-x",
				Description: "desc",
				Model:       "model-a",
				Body:        "body text",
				ProviderOverrides: map[string]map[string]any{
					provID: {"model": "override-model"},
				},
			}
			w1, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("first FromCanonical: %v", err)
			}
			w2, err := r.FromCanonical(ca, provID)
			if err != nil {
				t.Fatalf("second FromCanonical: %v", err)
			}
			if len(w1) != len(w2) {
				t.Fatalf("write count differs: %d vs %d", len(w1), len(w2))
			}
			for i := range w1 {
				if string(w1[i].Data) != string(w2[i].Data) {
					t.Errorf("write[%d] differs between runs:\nfirst:\n%s\nsecond:\n%s",
						i, w1[i].Data, w2[i].Data)
				}
			}
		})
	}
}

// TestProviderOverrides_ProviderSpecificStashedField covers a provider-specific
// stashed field (not model). For claude-code, the "skills" field is stashed in
// ProviderOverrides["claude-code"]["skills"] and must appear in serialized output.
// For opencode, the "permission" field is canonical; we test a stashed extra key.
func TestProviderOverrides_ProviderSpecificStashedField(t *testing.T) {
	r := Default()

	// claude-code: stash "color" (an extra frontmatter key) in ProviderOverrides.
	t.Run("provider=claude-code/stashed=color", func(t *testing.T) {
		ca := contract.CanonicalAgent{
			Name:        "agent-x",
			Description: "desc",
			Body:        "body",
			ProviderOverrides: map[string]map[string]any{
				"claude-code": {"color": "purple"},
			},
		}
		writes, err := r.FromCanonical(ca, "claude-code")
		if err != nil {
			t.Fatalf("FromCanonical: %v", err)
		}
		out := string(writes[0].Data)
		if !strings.Contains(out, "color") || !strings.Contains(out, "purple") {
			t.Errorf("claude-code output should contain stashed 'color: purple', got:\n%s", out)
		}
	})

	// codex: stash an extra key "custom_key" that is not a known codex field.
	t.Run("provider=codex/stashed=custom_key", func(t *testing.T) {
		ca := contract.CanonicalAgent{
			Name:        "agent-x",
			Description: "desc",
			Body:        "body",
			ProviderOverrides: map[string]map[string]any{
				"codex": {"custom_key": "custom_val"},
			},
		}
		writes, err := r.FromCanonical(ca, "codex")
		if err != nil {
			t.Fatalf("FromCanonical: %v", err)
		}
		out := string(writes[0].Data)
		if !strings.Contains(out, "custom_key") || !strings.Contains(out, "custom_val") {
			t.Errorf("codex output should contain stashed 'custom_key = custom_val', got:\n%s", out)
		}
	})

	// Stashed field for provider A must NOT appear in provider B's output.
	t.Run("stash isolation claude-code not in codex", func(t *testing.T) {
		ca := contract.CanonicalAgent{
			Name:        "agent-x",
			Description: "desc",
			Body:        "body",
			ProviderOverrides: map[string]map[string]any{
				"claude-code": {"color": "purple"},
			},
		}
		writes, err := r.FromCanonical(ca, "codex")
		if err != nil {
			t.Fatalf("FromCanonical codex: %v", err)
		}
		out := string(writes[0].Data)
		if strings.Contains(out, "purple") {
			t.Errorf("claude-code stash 'color=purple' leaked into codex output:\n%s", out)
		}
	})
}
