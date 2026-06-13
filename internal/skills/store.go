package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// The canonical skills store lives at <root>/.agents/skills/<name>/, and a
// directory counts as a skill only when it contains a SKILL.md marker file. The
// plural ".agents" is the agentskills.io vendor-neutral convention that gemini-cli
// and opencode read natively; the singular ".agent" is the legacy graft location
// kept only for back-compat migration (see legacyAgentDir / MigrateLegacy).
const (
	agentDir        = ".agents"
	legacyAgentDir  = ".agent"
	skillsDirName   = "skills"
	skillMarkerFile = "SKILL.md"
)

// Store is the canonical skills store rooted at a workspace. It owns reads of
// <root>/.agents/skills and copy-in installs; it performs no symlinking (that is
// symlink.go) and keeps no database (link state is live, per plan-02).
type Store struct {
	root string
}

// NewStore returns a Store for the given workspace root.
func NewStore(root string) *Store { return &Store{root: root} }

// Dir returns the canonical skills directory: <root>/.agents/skills.
func (s *Store) Dir() string {
	return filepath.Join(s.root, agentDir, skillsDirName)
}

// SkillDir returns the canonical directory for a named skill:
// <root>/.agents/skills/<name>.
func (s *Store) SkillDir(name string) string {
	return filepath.Join(s.Dir(), name)
}

// LegacyDir returns the legacy singular skills directory <root>/.agent/skills.
// It is consulted only by MigrateLegacy for back-compat with stores created
// before the plural ".agents" convention was adopted.
func (s *Store) LegacyDir() string {
	return filepath.Join(s.root, legacyAgentDir, skillsDirName)
}

// MigrateLegacy moves skills from the legacy <root>/.agent/skills store into the
// canonical <root>/.agents/skills store when the legacy dir exists. It is safe and
// idempotent: a skill already present canonically is left untouched (the legacy
// copy is skipped, not overwritten); only skills missing from the canonical store
// are moved. The legacy directory is removed once it has been drained of skills.
// It returns the names of the skills that were migrated. A missing legacy dir is
// a no-op (nil, nil).
func (s *Store) MigrateLegacy() ([]string, error) {
	legacy := s.LegacyDir()
	entries, err := os.ReadDir(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skills: migrate legacy: %w", err)
	}

	var migrated []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		src := filepath.Join(legacy, e.Name())
		if !isSkillDir(src) {
			continue
		}
		dst := s.SkillDir(e.Name())
		if isSkillDir(dst) {
			// Canonical already has it -- never clobber; leave the legacy copy in
			// place (removed below only if the dir ends up empty).
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return migrated, fmt.Errorf("skills: migrate legacy %q: %w", e.Name(), err)
		}
		// Prefer an atomic rename; fall back to copy+remove across filesystems.
		if err := os.Rename(src, dst); err != nil {
			if cerr := copyTree(src, dst); cerr != nil {
				return migrated, fmt.Errorf("skills: migrate legacy %q: %w", e.Name(), cerr)
			}
			_ = os.RemoveAll(src)
		}
		migrated = append(migrated, e.Name())
	}

	// Drop the legacy dir once drained -- but only if nothing real remains (a
	// non-skill file/dir the user put there must not be silently deleted).
	if remaining, rerr := os.ReadDir(legacy); rerr == nil && len(remaining) == 0 {
		_ = os.Remove(legacy)
		_ = os.Remove(filepath.Join(s.root, legacyAgentDir))
	}
	return migrated, nil
}

// List returns the canonical skills present in the store, sorted by name. A
// directory counts as a skill only when it contains a SKILL.md marker. A missing
// store directory yields no skills (not an error).
func (s *Store) List() ([]contract.Skill, error) {
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skills: list store: %w", err)
	}
	var out []contract.Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := s.SkillDir(e.Name())
		if !isSkillDir(dir) {
			continue
		}
		out = append(out, contract.Skill{Name: e.Name(), Dir: dir})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Has reports whether a canonical skill of the given name exists in the store.
func (s *Store) Has(name string) bool {
	return isSkillDir(s.SkillDir(name))
}

// Install copies an external/found skill directory into the canonical store when
// it is not already present. It never overwrites an existing canonical skill: if
// the named skill already exists, Install is a no-op and returns the existing
// canonical Skill (idempotent). srcDir must be a directory containing a SKILL.md.
// The skill name is derived from the source directory's base name unless name is
// given.
func (s *Store) Install(srcDir, name string) (contract.Skill, error) {
	if name == "" {
		name = filepath.Base(srcDir)
	}
	if !isSkillDir(srcDir) {
		return contract.Skill{}, fmt.Errorf("skills: %q is not a skill dir (no %s)", srcDir, skillMarkerFile)
	}

	dst := s.SkillDir(name)
	if isSkillDir(dst) {
		// Already canonical — never overwrite without explicit intent (callers that
		// want to replace should remove the canonical dir first).
		return contract.Skill{Name: name, Dir: dst}, nil
	}

	// Resolve the source through any symlink so we copy the real skill content,
	// not a link (e.g. when "installing" from a provider dir already symlinked).
	realSrc, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		realSrc = srcDir
	}

	// Atomic install: copy into a temp dir under the same parent, then rename
	// into place. A failure mid-copy never leaves a partially-populated canonical
	// skill that isSkillDir would later treat as complete.
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return contract.Skill{}, fmt.Errorf("skills: install %q: %w", name, err)
	}
	tmp, err := os.MkdirTemp(parent, ".graft-skill-*")
	if err != nil {
		return contract.Skill{}, fmt.Errorf("skills: install %q: %w", name, err)
	}
	defer os.RemoveAll(tmp) // no-op once renamed into place
	entries, err := os.ReadDir(realSrc)
	if err != nil {
		return contract.Skill{}, fmt.Errorf("skills: install %q: %w", name, err)
	}
	for _, e := range entries {
		if err := copyTree(filepath.Join(realSrc, e.Name()), filepath.Join(tmp, e.Name())); err != nil {
			return contract.Skill{}, fmt.Errorf("skills: install %q: %w", name, err)
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		return contract.Skill{}, fmt.Errorf("skills: install %q: %w", name, err)
	}
	return contract.Skill{Name: name, Dir: dst}, nil
}

// isSkillDir reports whether dir is a directory containing a SKILL.md marker.
func isSkillDir(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	mi, err := os.Stat(filepath.Join(dir, skillMarkerFile))
	return err == nil && !mi.IsDir()
}

// copyTree recursively copies the directory tree at src to dst (files, subdirs,
// and nested symlinks preserved). dst must not already exist.
func copyTree(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case fi.IsDir():
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	default:
		return copyFile(src, dst, fi.Mode().Perm())
	}
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
