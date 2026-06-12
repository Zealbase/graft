package gateway

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/store"
)

// graftGitignore is written to <root>/.graft/.gitignore so the in-repo .graft/
// commits ONLY the portable store (agents/ + their .meta.json) and ignores any
// stray local/runtime artifacts (an old db, lock, or sentinel from before the
// global-db move, plus general junk).
const graftGitignore = `# graft: keep only the portable store committed.
# Runtime/local state now lives in the global XDG dir, not here.
*
!.gitignore
!agents/
!agents/**
`

// writeGraftGitignore writes (idempotently) the .graft/.gitignore. It overwrites
// only when the content differs so it is cheap on every Open.
func writeGraftGitignore(root string) error {
	path := filepath.Join(root, graftDir, ".gitignore")
	if cur, err := os.ReadFile(path); err == nil && string(cur) == graftGitignore {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(graftGitignore), 0o644)
}

// migrateLegacyRepo imports an old per-repo db (<root>/.graft/graft.db) into the
// global db at globalDB, then removes the old in-repo runtime bits (db, lock,
// .initialized). store.Migrate is a no-op when the old db is absent and
// idempotent on repeat calls. It never touches agents/ or .meta.json (the
// portable store). Must run BEFORE the global store is opened in this process so
// store.Migrate's own destination connection does not race the gate's.
func migrateLegacyRepo(root, globalDB string) error {
	oldDB := filepath.Join(root, graftDir, legacyDBName)
	present := false
	if _, err := os.Stat(oldDB); err == nil {
		present = true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := store.Migrate(globalDB, oldDB); err != nil {
		return fmt.Errorf("import legacy db: %w", err)
	}

	// Only clean up when there actually was an old db to migrate (store.Migrate
	// is a no-op otherwise). Best-effort: a delete failure must not fail the
	// gateway. agents/ and *.meta.json are never touched.
	if present {
		cleanupLegacyArtifacts(root)
	}
	return nil
}

// cleanupLegacyArtifacts removes the old in-repo db (+ its sqlite sidecars),
// lock, and .initialized sentinel after a successful migration.
func cleanupLegacyArtifacts(root string) {
	base := filepath.Join(root, graftDir)
	for _, name := range []string{
		legacyDBName,
		legacyDBName + "-wal",
		legacyDBName + "-shm",
		legacyDBName + "-journal",
		"lock",
		".initialized",
	} {
		_ = os.Remove(filepath.Join(base, name))
	}
}
