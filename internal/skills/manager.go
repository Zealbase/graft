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
	// home is the resolved user home dir for home-scope skill detection (e.g.
	// ~/.claude/skills). Resolved once at construction; "" disables home-scope
	// detection (e.g. when os.UserHomeDir fails or in HOME-isolated tests).
	home string
}

// New returns a Manager for the workspace root with all providers registered
// (only the supporting ones are ever acted upon).
func New(root string) *Manager {
	home, _ := os.UserHomeDir()
	return &Manager{reg: Default(), store: NewStore(root), root: root, home: home}
}

// NewWithRegistry is a test seam letting callers inject a custom registry.
func NewWithRegistry(root string, reg *Registry) *Manager {
	home, _ := os.UserHomeDir()
	return &Manager{reg: reg, store: NewStore(root), root: root, home: home}
}

// Registry exposes the underlying registry (read-only use by callers/tests).
func (m *Manager) Registry() *Registry { return m.reg }

// Store exposes the canonical store.
func (m *Manager) Store() *Store { return m.store }

// Detect merges the canonical store with supporting providers and classifies
// every skill (canonical, provider-only install candidate, per-provider state).
// It first migrates any legacy .agent/skills store into the canonical
// .agents/skills location (back-compat, idempotent).
func (m *Manager) Detect(root string) ([]DetectedSkill, error) {
	if _, err := m.store.MigrateLegacy(); err != nil {
		return nil, err
	}
	return Detect(m.reg, m.store, m.rootOr(root), m.home)
}

// Install installs a skill into the canonical store and applies it. nameOrPath
// is either:
//   - a filesystem path to a skill dir (containing SKILL.md) to copy in, or
//   - a bare skill name already present in the canonical store, or
//   - a bare skill name found in a supporting provider's dir (copied in from there).
//
// After the copy-in it runs Apply so the new canonical skill is symlinked into
// every supporting provider (respecting opts). It returns the canonical Skill.
//
// Unlike Detect/Apply/Status, Install takes no root override and always operates
// on m.root. This is deliberate, not an oversight: the exported signature is part
// of the internal/gateway contract (another package) and must stay stable, and
// the gateway always constructs the Manager via New(workspaceRoot), so m.root is
// already the resolved workspace root. The root parameter on the other methods is
// purely a test seam; Install reaches the same effective root through m.root.
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
// is already canonical (no copy-in needed). When the same bare name is found in
// more than one supporting provider, the tie is broken by Supporting()'s
// alphabetical provider order: the first (lexically smallest provider id) match
// wins as the install source.
func (m *Manager) resolveSource(nameOrPath string) (srcDir, name string, err error) {
	// A filesystem path to a skill dir?
	if looksLikePath(nameOrPath) {
		clean := filepath.Clean(nameOrPath)
		// Expand a leading "~" / "~/..." to the resolved home — filepath.Clean
		// keeps the literal tilde, so an API caller passing "~/skill" would
		// otherwise hit ENOENT. Only expand when home is known.
		if m.home != "" && (clean == "~" || (len(clean) > 1 && clean[0] == '~' && os.IsPathSeparator(clean[1]))) {
			clean = filepath.Join(m.home, clean[1:])
		}
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
	// Found in a supporting provider's project dir? Copy from there. Otherwise a
	// home/user-scope dir (e.g. ~/.claude/skills) -- personal skills are
	// installable by bare name too.
	for _, p := range m.reg.Supporting() {
		refs, derr := p.DetectSkills(m.root)
		if derr != nil {
			return "", "", derr
		}
		if m.home != "" {
			hrefs, herr := scanHome(p, m.home)
			if herr != nil {
				return "", "", herr
			}
			refs = append(refs, hrefs...)
		}
		for _, ref := range refs {
			if ref.Name == name {
				return ref.Path, name, nil
			}
		}
	}
	return "", "", fmt.Errorf("skills: %q not found in canonical store, any provider, or home scope", name)
}

// Apply symlinks every canonical skill into every supporting provider's skills
// dir and returns the resulting live link state per (provider, skill). It honors
// opts.Provider (limit to one provider id) and opts.Override (replace a real
// non-symlink entry with a symlink). Non-supporting providers are never touched.
func (m *Manager) Apply(root string, opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	r := m.rootOr(root)
	if _, err := m.store.MigrateLegacy(); err != nil {
		return nil, err
	}
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
//
// Asymmetry with Apply: Status fails fast on the first LiveState I/O error
// (returning nil, err), whereas Apply accumulates per-(provider,skill) failures
// and returns partial results joined with the errors. This is deliberate — a
// read-only status probe has no partial work to preserve, so a stat/readlink
// failure is reported immediately. Use Apply when you need resilient fan-out
// that continues past an individual provider/skill failure.
func (m *Manager) Status(root string, opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	r := m.rootOr(root)
	if _, err := m.store.MigrateLegacy(); err != nil {
		return nil, err
	}
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

// List returns the canonical skills in the store. It first migrates any legacy
// .agent/skills store into the canonical .agents/skills location (idempotent).
func (m *Manager) List() ([]contract.Skill, error) {
	if _, err := m.store.MigrateLegacy(); err != nil {
		return nil, err
	}
	return m.store.List()
}

// PruneDeadLinks scans each supporting provider's skills dir for graft-managed
// DANGLING symlinks — entries that ARE symlinks (lstat) whose readlink target
// resolves under <root>/.agents/skills AND whose target is now MISSING — and
// removes (os.Remove) each dangling symlink. It returns the per-(provider,skill)
// SkillDead states for the links it pruned.
//
// This is intentionally NOT gated on store.List(): it finds orphaned symlinks
// even when the canonical skill no longer exists (the case Status/Apply miss,
// since they iterate only canonical skills). It honors opts.Provider.
//
// SAFETY — it removes an entry ONLY when ALL hold:
//   - the entry is a symlink (os.Lstat reports ModeSymlink); a real dir/file is
//     NEVER removed (left in place — reported elsewhere as SkillConflict),
//   - the symlink's target resolves UNDER <root>/.agents/skills (graft-managed);
//     a symlink pointing anywhere else is left untouched,
//   - the target is MISSING (dangling); a live/valid link is never pruned.
//
// It uses os.Remove (not RemoveAll) so it can only ever unlink a single symlink,
// never recursively delete a tree.
func (m *Manager) PruneDeadLinks(root string, opts contract.SkillOpts) ([]contract.SkillStatus, error) {
	r := m.rootOr(root)
	storeDir := filepath.Join(r, agentDir, skillsDirName)

	var out []contract.SkillStatus
	var errs []error
	for _, p := range m.reg.Supporting() {
		if opts.Provider != "" && p.Name() != opts.Provider {
			continue
		}
		provDir := p.SkillDir(r)
		entries, err := os.ReadDir(provDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("%s: read skill dir %q: %w", p.Name(), provDir, err))
			continue
		}
		for _, e := range entries {
			entryPath := filepath.Join(provDir, e.Name())
			fi, lerr := os.Lstat(entryPath)
			if lerr != nil {
				errs = append(errs, fmt.Errorf("%s/%s: lstat: %w", p.Name(), e.Name(), lerr))
				continue
			}
			// SAFETY 1: only symlinks are candidates; a real dir/file is left as-is.
			if fi.Mode()&os.ModeSymlink == 0 {
				continue
			}
			// SAFETY 2: the symlink must point INTO the canonical store. Resolve the
			// readlink target relative to the provider dir when it is relative.
			tgt, rerr := os.Readlink(entryPath)
			if rerr != nil {
				errs = append(errs, fmt.Errorf("%s/%s: readlink: %w", p.Name(), e.Name(), rerr))
				continue
			}
			resolved := tgt
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(provDir, resolved)
			}
			if !underDir(filepath.Clean(resolved), filepath.Clean(storeDir)) {
				// Points elsewhere (not graft-managed) -> never touch.
				continue
			}
			// SAFETY 3: the target must be MISSING (dangling); a live link stays.
			dangling, derr := isDanglingSymlink(entryPath)
			if derr != nil {
				errs = append(errs, fmt.Errorf("%s/%s: %w", p.Name(), e.Name(), derr))
				continue
			}
			if !dangling {
				continue
			}
			// Prune the single dangling symlink (os.Remove, never RemoveAll).
			if rmErr := os.Remove(entryPath); rmErr != nil {
				errs = append(errs, fmt.Errorf("%s/%s: prune dead link: %w", p.Name(), e.Name(), rmErr))
				continue
			}
			out = append(out, contract.SkillStatus{
				Skill:    e.Name(),
				Provider: p.Name(),
				State:    contract.SkillDead,
				LinkPath: entryPath,
			})
		}
	}
	sortStatuses(out)
	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}
	return out, nil
}

// underDir reports whether path is dir itself or lies under dir. Both args are
// expected to be cleaned absolute paths.
func underDir(path, dir string) bool {
	if path == dir {
		return true
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !filepathHasDotDotPrefix(rel)
}

// filepathHasDotDotPrefix reports whether rel escapes its base (starts with
// "..", e.g. ".." or "../x"), which would mean path is NOT under dir.
func filepathHasDotDotPrefix(rel string) bool {
	return len(rel) >= 2 && rel[0] == '.' && rel[1] == '.' &&
		(len(rel) == 2 || os.IsPathSeparator(rel[2]))
}

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
