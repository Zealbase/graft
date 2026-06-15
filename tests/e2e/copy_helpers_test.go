package e2e

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// copyTree performs a deep copy of src into dst, preserving directory structure.
// Symlinks are copied AS-IS (not dereferenced): a symlink at src/a/b is
// re-created at dst/a/b with the SAME target string (absolute or relative). This
// is critical for skill tests where an absolute symlink originally pointing into
// src will become dangling (or wrong) in dst, exactly mimicking what happens when
// a repo is copied to a new machine / new path with `cp -r` or `rsync --no-l`.
//
// copyTree does NOT copy the xdg-data subtree (if src was previously used as a
// graft workspace, its xdg-data holds the global DB). Excluding it gives rootB a
// fresh, empty global DB — exactly the "fresh machine" scenario.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		// Skip the xdg-data subtree: the copy represents a fresh machine with
		// no prior global graft DB.
		if rel == "xdg-data" || len(rel) > 8 && rel[:9] == "xdg-data/" {
			return filepath.SkipDir
		}
		// Skip the home subtree: synthetic HOME isolation dir from the harness.
		if rel == "home" || len(rel) > 4 && rel[:5] == "home/" {
			return filepath.SkipDir
		}
		dstPath := filepath.Join(dst, rel)

		// Handle symlinks: copy the link itself (not the target).
		info, lerr := os.Lstat(path)
		if lerr != nil {
			return lerr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, rerr2 := os.Readlink(path)
			if rerr2 != nil {
				return rerr2
			}
			return os.Symlink(target, dstPath)
		}

		if d.IsDir() {
			return os.MkdirAll(dstPath, info.Mode().Perm())
		}
		return copyFile(dstPath, path, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copyTree %s -> %s: %v", src, dst, err)
	}
}

// copyFile copies src to dst preserving mode.
func copyFile(dst, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// copyTreeWithXDG copies rootA to rootB but also copies xdg-data so the clone
// inherits the global DB state. This simulates a copy that also migrated the
// user's data directory (not the common case, but used by one variant test).
func copyTreeWithXDG(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		// Skip the home subtree.
		if rel == "home" || len(rel) > 4 && rel[:5] == "home/" {
			return filepath.SkipDir
		}
		dstPath := filepath.Join(dst, rel)

		info, lerr := os.Lstat(path)
		if lerr != nil {
			return lerr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, rerr2 := os.Readlink(path)
			if rerr2 != nil {
				return rerr2
			}
			return os.Symlink(target, dstPath)
		}
		if d.IsDir() {
			return os.MkdirAll(dstPath, info.Mode().Perm())
		}
		return copyFile(dstPath, path, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copyTreeWithXDG %s -> %s: %v", src, dst, err)
	}
}

// readFileBytes reads a path relative to dir and returns raw bytes. Used by
// provider-mutation assertions that compare byte content before/after.
func readFileBytes(t *testing.T, dir, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("readFileBytes %s: %v", rel, err)
	}
	return b
}
