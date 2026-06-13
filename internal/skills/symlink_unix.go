//go:build unix

package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// createSymlink creates a symlink at targetPath pointing to canonicalDir. The
// parent directory is created if needed. targetPath must not already exist.
//
// If a concurrent linker (init + sync hook racing on the same absent target)
// wins and os.Symlink returns EEXIST, but the existing link already points at
// canonicalDir, the filesystem is already in the desired state and createSymlink
// returns nil (idempotent). Only a correct pre-existing link is swallowed.
func createSymlink(canonicalDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir for link %q: %w", targetPath, err)
	}
	if err := os.Symlink(canonicalDir, targetPath); err != nil {
		if os.IsExist(err) {
			if ok, _ := isSymlinkTo(targetPath, canonicalDir); ok {
				return nil
			}
		}
		return fmt.Errorf("skills: symlink %q -> %q: %w", targetPath, canonicalDir, err)
	}
	return nil
}

// replaceWithSymlink replaces whatever is at targetPath (a symlink, file, or
// real directory tree) with a fresh symlink to canonicalDir, staging the new
// link at a temporary sibling first so the pre-existing target is only destroyed
// once the link is known to be creatable. If the symlink cannot be created the
// pre-existing target is left untouched.
func replaceWithSymlink(canonicalDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir for link %q: %w", targetPath, err)
	}
	tmpLink := targetPath + ".graft-tmplink"
	_ = os.Remove(tmpLink)
	if err := os.Symlink(canonicalDir, tmpLink); err != nil {
		return fmt.Errorf("skills: stage link %q -> %q: %w", tmpLink, canonicalDir, err)
	}
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
