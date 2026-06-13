//go:build !unix

package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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
//
// Only a genuine "symlinks unsupported/not permitted" failure is mapped to
// ErrSymlinkUnavailable; transient errors (ENOSPC, I/O, etc.) are returned
// unwrapped so they are not misreported as a platform-capability problem.
func createSymlink(canonicalDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir for link %q: %w", targetPath, err)
	}
	if err := os.Symlink(canonicalDir, targetPath); err != nil {
		// EEXIST despite an absent target means a concurrent linker won the race
		// (init + sync hook). If the existing link already points where we want,
		// the FS is in the desired state — treat as an idempotent success.
		if os.IsExist(err) {
			if ok, _ := isSymlinkTo(targetPath, canonicalDir); ok {
				return nil
			}
		}
		if isSymlinkCapabilityErr(err) {
			return fmt.Errorf("skills: link %q -> %q: %w", targetPath, canonicalDir, ErrSymlinkUnavailable)
		}
		return fmt.Errorf("skills: link %q -> %q: %w", targetPath, canonicalDir, err)
	}
	return nil
}

// isSymlinkCapabilityErr reports whether err indicates that symlink creation is
// fundamentally unsupported or not permitted on this platform/account (as
// opposed to a transient failure like ENOSPC or an I/O error). On Windows a
// non-privileged account without Developer Mode surfaces this as
// ERROR_PRIVILEGE_NOT_HELD, which maps to a permission error; ENOSYS covers
// platforms lacking the syscall entirely.
func isSymlinkCapabilityErr(err error) bool {
	return errors.Is(err, syscall.ENOSYS) || os.IsPermission(err)
}

// replaceWithSymlink atomically replaces whatever is at targetPath with a fresh
// symlink to canonicalDir, in a DATA-SAFE order: the new link is staged at a
// temporary sibling path first, and the pre-existing target is only destroyed
// once the link is known to be creatable. If the symlink cannot be created
// (e.g. the platform forbids it), the pre-existing target is left UNTOUCHED and
// the error is returned — never deleting the user's real content on a platform
// that then fails to recreate the link.
func replaceWithSymlink(canonicalDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir for link %q: %w", targetPath, err)
	}
	tmpLink := targetPath + ".graft-tmplink"
	// Clear any stale temp link from a previous interrupted run.
	_ = os.Remove(tmpLink)
	if err := createSymlink(canonicalDir, tmpLink); err != nil {
		// Staging failed: original target is untouched. Surface the error
		// (createSymlink already mapped capability errors appropriately).
		return err
	}
	// Staging succeeded — now it is safe to destroy the old target and move the
	// link into place.
	if err := os.RemoveAll(targetPath); err != nil {
		_ = os.Remove(tmpLink)
		return fmt.Errorf("skills: replace %q: %w", targetPath, err)
	}
	if err := os.Rename(tmpLink, targetPath); err != nil {
		_ = os.Remove(tmpLink)
		return fmt.Errorf("skills: finalize link %q: %w", targetPath, err)
	}
	return nil
}
