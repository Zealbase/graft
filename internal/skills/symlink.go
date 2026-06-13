package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// ErrSymlinkUnavailable is returned by createSymlink/replaceWithSymlink on
// platforms where unprivileged symlink creation is not possible (notably some
// Windows configurations). Link surfaces it instead of falsely reporting a skill
// as linked: on such a platform we cannot honor the symlink contract, so the
// caller must see an honest error rather than a "linked" state that LiveState
// would forever report as SkillConflict. The unix build never returns it.
var ErrSymlinkUnavailable = errors.New("skills: symlinks are unavailable on this platform")

// Link reconciles a single provider link path against a canonical skill dir,
// implementing the plan-02 state machine EXACTLY and idempotently:
//
//   - target absent              -> create symlink            -> linked
//   - symlink already correct    -> no-op                     -> linked
//   - symlink wrong/dangling     -> re-link (replace symlink) -> linked
//   - real dir/file present       -> conflict (unless override) -> conflict|linked
//
// canonicalDir is the absolute path of <root>/.agents/skills/<name>; targetPath is
// the absolute path of <provDir>/<name> where the symlink should live. When a
// real (non-symlink) entry blocks the target, Link returns SkillConflict unless
// override is set, in which case the real entry is removed and replaced with the
// symlink (-> linked).
func Link(canonicalDir, targetPath string, override bool) (contract.SkillLinkState, error) {
	fi, err := os.Lstat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Absent -> create.
			if err := createSymlink(canonicalDir, targetPath); err != nil {
				return "", err
			}
			return contract.SkillLinked, nil
		}
		return "", fmt.Errorf("skills: lstat %q: %w", targetPath, err)
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		// Existing symlink: correct -> no-op; wrong/dangling -> re-link.
		ok, lerr := isSymlinkTo(targetPath, canonicalDir)
		if lerr != nil {
			return "", lerr
		}
		if ok {
			return contract.SkillLinked, nil
		}
		if err := replaceWithSymlink(canonicalDir, targetPath); err != nil {
			return "", err
		}
		return contract.SkillLinked, nil
	}

	// A real file/dir is present.
	if !override {
		return contract.SkillConflict, nil
	}
	if err := replaceWithSymlink(canonicalDir, targetPath); err != nil {
		return "", err
	}
	return contract.SkillLinked, nil
}

// LiveState computes (without mutating) the live link state of targetPath
// relative to canonicalDir, per the plan-02 live-check flowchart.
func LiveState(canonicalDir, targetPath string) (contract.SkillLinkState, error) {
	fi, err := os.Lstat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return contract.SkillMissing, nil
		}
		return "", fmt.Errorf("skills: lstat %q: %w", targetPath, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		ok, lerr := isSymlinkTo(targetPath, canonicalDir)
		if lerr != nil {
			return "", lerr
		}
		if ok {
			return contract.SkillLinked, nil
		}
		// Not a (live) match to the canonical dir. Distinguish a dangling link
		// (symlink whose target is missing) from a wrong-link (symlink to some
		// other existing target): a dangling link is SkillDead so sync can prune
		// it, even when the canonical skill no longer exists.
		if dangling, derr := isDanglingSymlink(targetPath); derr != nil {
			return "", derr
		} else if dangling {
			return contract.SkillDead, nil
		}
		return contract.SkillWrongLink, nil
	}
	return contract.SkillConflict, nil
}

// isDanglingSymlink reports whether path is a symlink whose target does not
// exist (a broken/dead link). path is assumed to already be a symlink (callers
// check Lstat mode first). A Stat (which follows the link) failing with
// not-exist means the target is missing -> dangling.
func isDanglingSymlink(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("skills: stat %q: %w", path, err)
	}
	return false, nil
}

// isSymlinkTo reports whether path is a symlink resolving to want AND whose
// target actually exists. It compares both the raw readlink target (resolved
// against path's dir if relative) and the fully-evaluated real paths, so a
// relative or absolute link both match. A dangling symlink (target does not
// exist) returns false even when it lexically points at want — a dead link is
// NOT "linked"; callers reclassify it (SkillDead). This is the fix for the
// fast-path that previously accepted a lexical match without verifying the
// target existed.
func isSymlinkTo(path, want string) (bool, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return false, fmt.Errorf("skills: readlink %q: %w", path, err)
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(path), resolved)
	}
	// Fast path: lexically equal (after cleaning) AND the target exists. Stat
	// (follows the link) succeeding proves the target is present; a dangling link
	// fails here so it is never classified linked.
	if filepath.Clean(resolved) == filepath.Clean(want) {
		if _, serr := os.Stat(path); serr == nil {
			return true, nil
		}
		// Lexical match but target missing -> dangling, not linked.
		return false, nil
	}
	// Robust path: compare evaluated real paths (handles ., .., trailing slash,
	// and intermediate symlinks). A dangling link fails EvalSymlinks -> not a
	// match, which is the desired classification.
	rp, err1 := filepath.EvalSymlinks(resolved)
	wp, err2 := filepath.EvalSymlinks(want)
	if err1 == nil && err2 == nil {
		return rp == wp, nil
	}
	return false, nil
}

// IsSymlinkTo is the exported live check used by callers/tests.
func IsSymlinkTo(path, want string) (bool, error) { return isSymlinkTo(path, want) }
