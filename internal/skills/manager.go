package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// Manager orchestrates skill detection, install (copy-in), symlink apply, and
// live status over a workspace root, using the Default() registry. It owns no
// database: every link state is computed live from the filesystem.
type Manager struct {
	reg   *Registry
	store *Store
	root  string
}

// New returns a Manager for the workspace root with all providers registered
// (only the supporting ones are ever acted upon).
func New(root string) *Manager {
	return &Manager{reg: Default(), store: NewStore(root), root: root}
}

// NewWithRegistry is a test seam letting callers inject a custom registry.
func NewWithRegistry(root string, reg *Registry) *Manager {
	return &Manager{reg: reg, store: NewStore(root), root: root}
}

// Registry exposes the underlying registry (read-only use by callers/tests).
func (m *Manager) Registry() *Registry { return m.reg }

// Store exposes the canonical store.
func (m *Manager) Store() *Store { return m.store }

// Detect merges the canonical store with supporting providers and classifies
// every skill (canonical, provider-only install candidate, per-provider state).
func (m *Manager) Detect(root string) ([]DetectedSkill, error) {
	return Detect(m.reg, m.store, m.rootOr(root))
}

// Install installs a skill into the canonical store and applies it. nameOrPath
// is either:
//   - a filesystem path to a skill dir (containing SKILL.md) to copy in, or
//   - a bare skill name already present in the canonical store, or
//   - a bare skill name found in a supporting provider's dir (copied in from there).
//
// After the copy-in it runs Apply so the new canonical skill is symlinked into
// every supporting provider (respecting opts). It returns the canonical Skill.
func (m *Manager) Install(nameOrPath string, opts contract.SkillOpts) (contract.Skill, error) {
	srcDir, name, err := m.resolveSource(nameOrPath)
	if err != nil {
		return contract.Skill{}, err
	}

	var installed contract.Skill
	if srcDir == "" {
		// Already canonical (name in store) — nothing to copy in.
		installed = contract.Skill{Name: name, Dir: m.store.SkillDir(name)}
	} else {
		installed, err = m.store.Install(srcDir, name)
		if err != nil {
			return contract.Skill{}, err
		}
	}

	if _, err := m.Apply(m.root, opts); err != nil {
		return installed, err
	}
	return installed, nil
}

// resolveSource turns nameOrPath into (srcDir, name). srcDir is "" when the name
// is already canonical (no copy-in needed).
func (m *Manager) resolveSource(nameOrPath string) (srcDir, name string, err error) {
	// A filesystem path to a skill dir?
	if looksLikePath(nameOrPath) {
		clean := filepath.Clean(nameOrPath)
		if isSkillDir(clean) {
			return clean, filepath.Base(clean), nil
		}
		return "", "", fmt.Errorf("skills: %q is not a skill dir", nameOrPath)
	}

	name = nameOrPath
	// Already canonical?
	if m.store.Has(name) {
		return "", name, nil
	}
	// Found in a supporting provider's dir? Copy from there.
	for _, p := range m.reg.Supporting() {
		refs, derr := p.DetectSkills(m.root)
		if derr != nil {
			return "", "", derr
		}
		for _, ref := range refs {
			if ref.Name == name {
				return ref.Path, name, nil
			}
		}
	}
	return "", "", fmt.Errorf("skills: %q not found in canonical store or any provider", name)
}

// Apply symlinks every canonical skill into every supporting provider's skills
// dir and returns the resulting live link state per (provider, skill). It honors
// opts.Provider (limit to one provider id) and opts.Override (replace a real
// non-symlink entry with a symlink). Non-supporting providers are never touched.
func (m *Manager) Apply(root string, opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	r := m.rootOr(root)
	canon, err := m.store.List()
	if err != nil {
		return nil, err
	}

	var out []contract.SkillStatus
	var errs []error
	for _, p := range m.reg.Supporting() {
		if opts.Provider != "" && p.Name() != opts.Provider {
			continue
		}
		provDir := p.SkillDir(r)
		for _, sk := range canon {
			linkPath := filepath.Join(provDir, sk.Name)
			state, lerr := Link(sk.Dir, linkPath, opts.Override)
			if lerr != nil {
				// Accumulate and keep going so one provider/skill failure does not
				// silently abort linking everything else; report partial + joined err.
				errs = append(errs, fmt.Errorf("%s/%s: %w", p.Name(), sk.Name, lerr))
				continue
			}
			out = append(out, contract.SkillStatus{
				Skill:    sk.Name,
				Provider: p.Name(),
				State:    state,
				LinkPath: linkPath,
			})
		}
	}
	// Sort regardless of partial failure so callers always get a deterministic
	// ordering alongside any joined error.
	sortStatuses(out)
	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}
	return out, nil
}

// Status reports the LIVE link state (lstat/readlink, no mutation) of every
// canonical skill across every supporting provider. It honors opts.Provider.
// Canonical skills with no entry at a provider report SkillMissing.
func (m *Manager) Status(root string, opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	r := m.rootOr(root)
	canon, err := m.store.List()
	if err != nil {
		return nil, err
	}

	var out []contract.SkillStatus
	for _, p := range m.reg.Supporting() {
		if opts.Provider != "" && p.Name() != opts.Provider {
			continue
		}
		provDir := p.SkillDir(r)
		for _, sk := range canon {
			linkPath := filepath.Join(provDir, sk.Name)
			state, lerr := LiveState(sk.Dir, linkPath)
			if lerr != nil {
				return nil, lerr
			}
			out = append(out, contract.SkillStatus{
				Skill:    sk.Name,
				Provider: p.Name(),
				State:    state,
				LinkPath: linkPath,
			})
		}
	}
	sortStatuses(out)
	return out, nil
}

// List returns the canonical skills in the store.
func (m *Manager) List() ([]contract.Skill, error) { return m.store.List() }

func (m *Manager) rootOr(root string) string {
	if root != "" {
		return root
	}
	return m.root
}

func sortStatuses(s []contract.SkillStatus) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Provider != s[j].Provider {
			return s[i].Provider < s[j].Provider
		}
		return s[i].Skill < s[j].Skill
	})
}

// looksLikePath reports whether s is a filesystem path rather than a bare skill
// name (contains a separator or starts with . / ~).
func looksLikePath(s string) bool {
	if s == "" {
		return false
	}
	if os.IsPathSeparator(s[0]) || s[0] == '.' || s[0] == '~' {
		return true
	}
	for i := 0; i < len(s); i++ {
		if os.IsPathSeparator(s[i]) {
			return true
		}
	}
	return false
}
