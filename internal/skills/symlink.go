package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

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
		return contract.SkillWrongLink, nil
	}
	return contract.SkillConflict, nil
}

// isSymlinkTo reports whether path is a symlink resolving to want. It compares
// both the raw readlink target (resolved against path's dir if relative) and the
// fully-evaluated real paths, so a relative or absolute link both match. A
// dangling symlink (target does not exist) returns false (treated as wrong-link).
func isSymlinkTo(path, want string) (bool, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return false, fmt.Errorf("skills: readlink %q: %w", path, err)
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(path), resolved)
	}
	// Fast path: lexically equal (after cleaning).
	if filepath.Clean(resolved) == filepath.Clean(want) {
		return true, nil
	}
	// Robust path: compare evaluated real paths (handles ., .., trailing slash,
	// and intermediate symlinks). A dangling link fails EvalSymlinks -> not a
	// match -> wrong-link, which is the desired classification.
	rp, err1 := filepath.EvalSymlinks(resolved)
	wp, err2 := filepath.EvalSymlinks(want)
	if err1 == nil && err2 == nil {
		return rp == wp, nil
	}
	return false, nil
}

// IsSymlinkTo is the exported live check used by callers/tests.
func IsSymlinkTo(path, want string) (bool, error) { return isSymlinkTo(path, want) }
