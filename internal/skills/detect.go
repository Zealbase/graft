package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SkillOrigin classifies how a detected skill relates to the canonical store.
type SkillOrigin string

const (
	// OriginCanonical: the skill exists in .agents/skills (the source of truth).
	OriginCanonical SkillOrigin = "canonical"
	// OriginProviderOnly: found in a supporting provider's skills dir but NOT yet
	// in the canonical store — an install candidate (copy-in offer).
	OriginProviderOnly SkillOrigin = "provider-only"
)

// DetectedSkill is one skill seen during detection, merged across the canonical
// store and every supporting provider's skills dir.
type DetectedSkill struct {
	Name string
	// Origin is canonical when present in .agents/skills, else provider-only.
	Origin SkillOrigin
	// CanonicalDir is the .agents/skills/<name> path (set when Origin==canonical).
	CanonicalDir string
	// Providers lists the supporting providers that already have an entry (link
	// or real dir) named for this skill, with that entry's live link state.
	Providers map[string]contract.SkillLinkState
	// Sources lists where a provider-only skill was found (install candidates).
	Sources []contract.SkillRef
}

// InstallCandidate reports whether this skill is found in a provider dir but not
// yet canonical — i.e. it can be copied into .agents/skills.
func (d DetectedSkill) InstallCandidate() bool { return d.Origin == OriginProviderOnly }

// Detect merges the canonical skills store with each supporting provider's
// detected skills and classifies every skill name. For canonical skills it also
// records each supporting provider's LIVE link state at the expected link path
// (linked / missing / wrong-link / conflict). Provider-only skills -- found in a
// project skills dir OR a home/user-scope skills dir (home != "") -- are surfaced
// as install candidates with their source refs. Results are sorted by name.
func Detect(reg *Registry, store *Store, root, home string) ([]DetectedSkill, error) {
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
	// Note: DetectSkills returns BOTH real skill dirs AND symlinks-to-skill-dirs
	// (it accepts either), so a canonical skill already SYMLINKED into a provider
	// dir IS returned here. It is NOT downgraded to an install candidate only
	// because of the Origin guard below: a name already classified OriginCanonical
	// (from step 1) is never reset to OriginProviderOnly and gets no Sources entry.
	// Canonical link state is the authoritative per-provider state computed in
	// step 3; this step exists solely to surface NON-canonical names as candidates.
	for _, p := range reg.Supporting() {
		refs, derr := p.DetectSkills(root)
		if derr != nil {
			return nil, derr
		}
		// Home/user-scope skills (e.g. ~/.claude/skills) are install candidates
		// too: personal skills should be visible so they can be copied into the
		// canonical store. They are read-only sources and never receive symlinks.
		if home != "" {
			homeRefs, herr := scanHome(p, home)
			if herr != nil {
				return nil, herr
			}
			refs = append(refs, homeRefs...)
		}
		for _, ref := range refs {
			d := get(ref.Name)
			if d.Origin == "" {
				// Found only in a provider/home dir so far -> install candidate.
				d.Origin = OriginProviderOnly
			}
			if d.Origin == OriginProviderOnly {
				// Sources accumulate in Supporting() order, which is alphabetical by
				// provider id. When the same provider-only name appears in multiple
				// providers, that ordering is the tie-break: Sources[0] is the
				// lexically smallest provider, matching the install source chosen by
				// Manager.resolveSource.
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
				// A real I/O error (e.g. EACCES) must not masquerade as "missing"
				// — that would silently mislabel a permission problem as drift.
				return nil, fmt.Errorf("skills: live state %s/%s: %w", p.Name(), d.Name, err)
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

// scanHome scans a supporting provider's home/user-scope skill dirs (e.g.
// ~/.claude/skills) and returns a SkillRef per <dir>/<skill>/SKILL.md found.
// Missing dirs are skipped silently. These are read-only install sources; they
// are never symlinked into. The provider id is recorded on each ref so callers
// know the source. A real I/O error (not just absence) is returned.
func scanHome(p contract.SkillProvider, home string) ([]contract.SkillRef, error) {
	var refs []contract.SkillRef
	for _, dir := range p.HomeSkillDirs(home) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("skills: scan home %q: %w", dir, err)
		}
		for _, e := range entries {
			skillDir := filepath.Join(dir, e.Name())
			if !isSkillDir(skillDir) {
				continue
			}
			refs = append(refs, contract.SkillRef{
				Name:     e.Name(),
				Provider: p.Name(),
				Path:     skillDir,
			})
		}
	}
	return refs, nil
}
