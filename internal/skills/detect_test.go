package skills

import (
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

func find(ds []DetectedSkill, name string) *DetectedSkill {
	for i := range ds {
		if ds[i].Name == name {
			return &ds[i]
		}
	}
	return nil
}

func TestDetect_CanonicalOnly(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "alpha")

	m := New(root)
	ds, err := m.Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	a := find(ds, "alpha")
	if a == nil {
		t.Fatalf("alpha not detected")
	}
	if a.Origin != OriginCanonical {
		t.Fatalf("alpha origin = %q, want canonical", a.Origin)
	}
	if a.InstallCandidate() {
		t.Errorf("canonical alpha should not be an install candidate")
	}
	// Not yet linked anywhere -> every supporting provider is missing.
	if len(a.Providers) != 3 {
		t.Fatalf("alpha providers = %v, want 3 supporting", a.Providers)
	}
	for prov, st := range a.Providers {
		if st != contract.SkillMissing {
			t.Errorf("%s/alpha = %q, want missing", prov, st)
		}
	}
}

func TestDetect_FoundInProviderIsInstallCandidate(t *testing.T) {
	root := t.TempDir()
	// Skill present only in a provider dir, not canonical.
	writeSkillDir(t, filepath.Join(root, ".claude", "skills", "scout"), "# scout\n")

	m := New(root)
	ds, err := m.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	s := find(ds, "scout")
	if s == nil {
		t.Fatalf("scout not detected")
	}
	if s.Origin != OriginProviderOnly || !s.InstallCandidate() {
		t.Fatalf("scout origin = %q (candidate=%v), want provider-only install candidate", s.Origin, s.InstallCandidate())
	}
	if len(s.Sources) == 0 || s.Sources[0].Provider != "claude-code" {
		t.Fatalf("scout sources = %v, want a claude-code ref", s.Sources)
	}
}

func TestDetect_PartialLinked(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "alpha")
	m := New(root)

	// Link alpha into opencode only.
	if _, err := m.Apply(root, contract.SkillOpts{Provider: "opencode"}); err != nil {
		t.Fatal(err)
	}

	ds, err := m.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	a := find(ds, "alpha")
	if a == nil {
		t.Fatalf("alpha not detected")
	}
	if a.Providers["opencode"] != contract.SkillLinked {
		t.Errorf("opencode/alpha = %q, want linked", a.Providers["opencode"])
	}
	if a.Providers["claude-code"] != contract.SkillMissing {
		t.Errorf("claude-code/alpha = %q, want missing", a.Providers["claude-code"])
	}
	if a.Providers["gemini-cli"] != contract.SkillMissing {
		t.Errorf("gemini-cli/alpha = %q, want missing", a.Providers["gemini-cli"])
	}
}

func TestDetect_MixedCanonicalAndProviderOnly(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "canon")
	writeSkillDir(t, filepath.Join(root, ".gemini", "skills", "geo"), "# geo\n")

	m := New(root)
	ds, err := m.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if c := find(ds, "canon"); c == nil || c.Origin != OriginCanonical {
		t.Errorf("canon classify wrong: %+v", c)
	}
	if g := find(ds, "geo"); g == nil || g.Origin != OriginProviderOnly {
		t.Errorf("geo classify wrong: %+v", g)
	}
	// Sorted by name: canon before geo.
	if len(ds) < 2 || ds[0].Name != "canon" {
		t.Errorf("detect not sorted by name: %v", names(ds))
	}
}

func names(ds []DetectedSkill) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return out
}
