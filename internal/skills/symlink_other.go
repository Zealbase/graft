//go:build !unix

package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// createSymlink attempts a real symlink at targetPath -> canonicalDir. On
// platforms where unprivileged symlink creation is unavailable (notably some
// Windows configurations), it falls back to a recursive directory COPY so the
// skill content is still present at the provider path. The copy is not a link, so
// a subsequent LiveState reports it as a conflict — which is the honest state on
// a platform that could not create the link.
func createSymlink(canonicalDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir for link %q: %w", targetPath, err)
	}
	if err := os.Symlink(canonicalDir, targetPath); err == nil {
		return nil
	}
	// Fallback: copy the canonical skill tree into place.
	if err := copyTree(canonicalDir, targetPath); err != nil {
		return fmt.Errorf("skills: symlink fallback copy %q -> %q: %w", targetPath, canonicalDir, err)
	}
	return nil
}

// replaceWithSymlink removes whatever is at targetPath and re-creates the link
// (or copy fallback).
func replaceWithSymlink(canonicalDir, targetPath string) error {
	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("skills: replace %q: %w", targetPath, err)
	}
	return createSymlink(canonicalDir, targetPath)
}
