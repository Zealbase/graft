package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestFreshInstallMarkedInitializedAtZero locks the MED fix: with zero embedded
// migration files, a fresh DB must be marked initialized at pointer 0 (not left
// at is_initialized=0). Otherwise a later first migration would mis-classify
// this existing install as fresh and skip the migration body.
func TestFreshInstallMarkedInitializedAtZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var pointer, inited int
	if err := db.QueryRow(
		`SELECT pointer_value, is_initialized FROM schema_migration`,
	).Scan(&pointer, &inited); err != nil {
		t.Fatalf("schema_migration: %v", err)
	}
	if pointer != 0 || inited != 1 {
		t.Fatalf("got (pointer=%d, initialized=%d), want (0, 1)", pointer, inited)
	}
}

// TestFirstMigrationRunsOnInitializedInstall proves the pointer-advance path the
// MED fix preserves: an install already initialized at pointer 0 (the state Open
// leaves behind today) runs a newly-shipped first migration rather than skipping
// it as "fresh". We drive the exact classification runMigrations uses.
func TestFirstMigrationRunsOnInitializedInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graft.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// State after Open with no migration files.
	initialized, err := isInitialized(db)
	if err != nil {
		t.Fatalf("isInitialized: %v", err)
	}
	if !initialized {
		t.Fatalf("expected initialized install (pointer-advance path), got fresh")
	}

	// Simulate "first migration (1.sql) ships later" applied via the normal
	// pointer-advance path: because the install is already initialized, it is NOT
	// stamped-to-max/skipped — the body would run and the pointer advance to 1.
	if _, err := db.Exec(`ALTER TABLE workspaces ADD COLUMN note TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatalf("apply migration body: %v", err)
	}
	if err := setPointer(db, 1); err != nil {
		t.Fatalf("setPointer: %v", err)
	}

	var got string
	if err := db.QueryRow(
		`SELECT note FROM workspaces`,
	).Scan(&got); err != nil && err != sql.ErrNoRows {
		t.Fatalf("new column not present: %v", err)
	}
	p, err := getPointer(db)
	if err != nil {
		t.Fatalf("getPointer: %v", err)
	}
	if p != 1 {
		t.Fatalf("pointer = %d, want 1 after first migration", p)
	}
}
