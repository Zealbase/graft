// Package skl holds small shared helpers for the per-provider SkillProvider
// implementations. Skills are an additive capability reconciled by symlink; a
// provider's skills.go uses these helpers to keep the per-provider file thin
// while each provider still owns its own skills directory path.
package skl

import (
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// skillMarker is the file that marks a directory as a skill.
const skillMarker = "SKILL.md"

// Detect scans a provider's skills directory and returns one SkillRef per
// immediate subdirectory that contains a SKILL.md file. A missing directory is
// not an error (returns nil, nil) — the provider simply has no skills yet.
func Detect(provider, dir string) ([]contract.SkillRef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var refs []contract.SkillRef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(skillDir, skillMarker)); err != nil {
			continue
		}
		refs = append(refs, contract.SkillRef{
			Name:     e.Name(),
			Provider: provider,
			Path:     skillDir,
		})
	}
	return refs, nil
}

// Unsupported is the SkillProvider implementation shared by every provider that
// does not participate in skills. Embed it (or return it) so SkillsSupported is
// false, SkillDir is "", and DetectSkills is a no-op, while the provider's
// skills.go file still exists for an explicit, complete set.
type Unsupported struct {
	// ProviderName is the provider id reported by Name().
	ProviderName string
}

// Name returns the provider id.
func (u Unsupported) Name() string { return u.ProviderName }

// SkillsSupported is always false for non-supporting providers.
func (Unsupported) SkillsSupported() bool { return false }

// SkillDir is empty for non-supporting providers.
func (Unsupported) SkillDir(string) string { return "" }

// DetectSkills is a no-op for non-supporting providers.
func (Unsupported) DetectSkills(string) ([]contract.SkillRef, error) { return nil, nil }
