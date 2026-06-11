//go:build unix

package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// createSymlink creates a symlink at targetPath pointing to canonicalDir. The
// parent directory is created if needed. targetPath must not already exist.
func createSymlink(canonicalDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir for link %q: %w", targetPath, err)
	}
	if err := os.Symlink(canonicalDir, targetPath); err != nil {
		return fmt.Errorf("skills: symlink %q -> %q: %w", targetPath, canonicalDir, err)
	}
	return nil
}

// replaceWithSymlink removes whatever is at targetPath (a symlink, file, or real
// directory tree) and replaces it with a fresh symlink to canonicalDir.
func replaceWithSymlink(canonicalDir, targetPath string) error {
	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("skills: replace %q: %w", targetPath, err)
	}
	return createSymlink(canonicalDir, targetPath)
}
