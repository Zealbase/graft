//go:build !unix

package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// createSymlink creates a symlink at targetPath pointing to canonicalDir. On
// platforms where unprivileged symlink creation is unavailable (notably some
// Windows configurations), os.Symlink fails and createSymlink returns
// ErrSymlinkUnavailable rather than silently substituting a real directory copy.
//
// A copy fallback was deliberately removed: it left a REAL directory at the
// provider path, which LiveState classifies as SkillConflict. Worse, an
// override re-link (replaceWithSymlink) would RemoveAll then re-copy, so the
// state could never converge to linked while Link kept reporting success. By
// returning a sentinel error here we keep the contract honest — on a platform
// that cannot link, the caller sees a real error instead of a fake "linked".
func createSymlink(canonicalDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir for link %q: %w", targetPath, err)
	}
	if err := os.Symlink(canonicalDir, targetPath); err != nil {
		return fmt.Errorf("skills: link %q -> %q: %w", targetPath, canonicalDir, ErrSymlinkUnavailable)
	}
	return nil
}

// replaceWithSymlink removes whatever is at targetPath and re-creates the link.
// If the platform cannot create symlinks it returns ErrSymlinkUnavailable (and
// the target has already been removed, leaving the path absent rather than a
// stale real copy).
func replaceWithSymlink(canonicalDir, targetPath string) error {
	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("skills: replace %q: %w", targetPath, err)
	}
	return createSymlink(canonicalDir, targetPath)
}
