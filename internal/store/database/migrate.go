package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// runMigrations advances the DB schema using a pointer table.
//
// Fresh install: the base schema already has the final shape, so we stamp the
// pointer to the highest migration index and mark initialized without running
// any migration bodies. Existing install: each migration file with index above
// the pointer is applied, tolerating duplicate-column errors so a re-run after
// a mid-file crash is self-healing. The pointer only advances after a whole
// file applies.
func runMigrations(db *sql.DB) error {
	if err := ensureMigrationTable(db); err != nil {
		return err
	}

	initialized, err := isInitialized(db)
	if err != nil {
		return err
	}

	// The migrations dir may be absent from the embed FS (e.g. it holds only a
	// dotfile, which //go:embed skips). Treat a read failure the same as zero
	// migration files: a fresh DB is still marked initialized at pointer 0.
	var indexes []int
	if files, derr := fs.ReadDir(schemaFS, "schema/migrations"); derr == nil {
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := strings.TrimSuffix(f.Name(), ".sql")
			n, aerr := strconv.Atoi(name)
			if aerr != nil {
				continue // skip non-numeric files (e.g. a .keep placeholder)
			}
			indexes = append(indexes, n)
		}
		sort.Ints(indexes)
	}

	// No migration files yet (today's state). A fresh DB must still be marked
	// initialized — at pointer 0 — so that when the FIRST migration (1.sql) ships
	// later this existing install is NOT misread as fresh and skipped; instead it
	// runs via the normal pointer-advance path below.
	if len(indexes) == 0 {
		if !initialized {
			return stampAndInit(db, 0)
		}
		return nil
	}

	// Fresh install with migrations present: base schema is already final — stamp
	// to max and skip bodies.
	if !initialized {
		return stampAndInit(db, indexes[len(indexes)-1])
	}

	pointer, err := getPointer(db)
	if err != nil {
		return err
	}

	for _, n := range indexes {
		if n <= pointer {
			continue
		}
		path := fmt.Sprintf("schema/migrations/%d.sql", n)
		data, err := schemaFS.ReadFile(path)
		if err != nil {
			return err
		}
		for _, stmt := range strings.Split(string(data), ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				if isDuplicateColumnError(err) {
					continue
				}
				return fmt.Errorf("migration %d: %w", n, err)
			}
		}
		if err := setPointer(db, n); err != nil {
			return err
		}
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}

func ensureMigrationTable(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migration (
		pointer_value  INTEGER NOT NULL DEFAULT 0,
		is_initialized INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO schema_migration (pointer_value, is_initialized)
		SELECT 0, 0 WHERE NOT EXISTS (SELECT 1 FROM schema_migration)`)
	return err
}

func isInitialized(db *sql.DB) (bool, error) {
	var v int
	err := db.QueryRow(`SELECT is_initialized FROM schema_migration`).Scan(&v)
	return v == 1, err
}

func getPointer(db *sql.DB) (int, error) {
	var v int
	err := db.QueryRow(`SELECT pointer_value FROM schema_migration`).Scan(&v)
	return v, err
}

func setPointer(db *sql.DB, n int) error {
	_, err := db.Exec(`UPDATE schema_migration SET pointer_value = ?`, n)
	return err
}

func stampAndInit(db *sql.DB, max int) error {
	_, err := db.Exec(`UPDATE schema_migration SET pointer_value = ?, is_initialized = 1`, max)
	return err
}
