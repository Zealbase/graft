package skills

import (
	"path/filepath"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SkillOrigin classifies how a detected skill relates to the canonical store.
type SkillOrigin string

const (
	// OriginCanonical: the skill exists in .agent/skills (the source of truth).
	OriginCanonical SkillOrigin = "canonical"
	// OriginProviderOnly: found in a supporting provider's skills dir but NOT yet
	// in the canonical store — an install candidate (copy-in offer).
	OriginProviderOnly SkillOrigin = "provider-only"
)

// DetectedSkill is one skill seen during detection, merged across the canonical
// store and every supporting provider's skills dir.
type DetectedSkill struct {
	Name string
	// Origin is canonical when present in .agent/skills, else provider-only.
	Origin SkillOrigin
	// CanonicalDir is the .agent/skills/<name> path (set when Origin==canonical).
	CanonicalDir string
	// Providers lists the supporting providers that already have an entry (link
	// or real dir) named for this skill, with that entry's live link state.
	Providers map[string]contract.SkillLinkState
	// Sources lists where a provider-only skill was found (install candidates).
	Sources []contract.SkillRef
}

// InstallCandidate reports whether this skill is found in a provider dir but not
// yet canonical — i.e. it can be copied into .agent/skills.
func (d DetectedSkill) InstallCandidate() bool { return d.Origin == OriginProviderOnly }

// Detect merges the canonical skills store with each supporting provider's
// detected skills and classifies every skill name. For canonical skills it also
// records each supporting provider's LIVE link state at the expected link path
// (linked / missing / wrong-link / conflict). Provider-only skills are surfaced
// as install candidates with their source refs. Results are sorted by name.
func Detect(reg *Registry, store *Store, root string) ([]DetectedSkill, error) {
	merged := map[string]*DetectedSkill{}

	get := func(name string) *DetectedSkill {
		d, ok := merged[name]
		if !ok {
			d = &DetectedSkill{Name: name, Providers: map[string]contract.SkillLinkState{}}
			merged[name] = d
		}
		return d
	}

	// 1. Canonical store skills (source of truth).
	canon, err := store.List()
	if err != nil {
		return nil, err
	}
	for _, sk := range canon {
		d := get(sk.Name)
		d.Origin = OriginCanonical
		d.CanonicalDir = sk.Dir
	}

	// 2. Each supporting provider's on-disk skills (to find provider-only ones).
	// Note: a canonical skill SYMLINKED into a provider dir is NOT returned by
	// DetectSkills (it skips symlinks via DirEntry.IsDir), so canonical link state
	// is computed directly in step 3 — DetectSkills here only surfaces real
	// (non-symlink) skill dirs that may be install candidates.
	for _, p := range reg.Supporting() {
		refs, derr := p.DetectSkills(root)
		if derr != nil {
			return nil, derr
		}
		for _, ref := range refs {
			d := get(ref.Name)
			if d.Origin == "" {
				// Found only in a provider so far -> install candidate.
				d.Origin = OriginProviderOnly
			}
			if d.Origin == OriginProviderOnly {
				d.Sources = append(d.Sources, ref)
			}
		}
	}

	// 3. For every CANONICAL skill, compute the LIVE link state at each supporting
	// provider's expected link path (the authoritative per-provider state).
	for _, p := range reg.Supporting() {
		for _, d := range merged {
			if d.Origin != OriginCanonical {
				continue
			}
			linkPath := filepath.Join(p.SkillDir(root), d.Name)
			st, err := LiveState(d.CanonicalDir, linkPath)
			if err != nil {
				st = contract.SkillMissing
			}
			d.Providers[p.Name()] = st
		}
	}

	out := make([]DetectedSkill, 0, len(merged))
	for _, d := range merged {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
