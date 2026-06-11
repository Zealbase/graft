// Package database owns the singleton sqlite connection, the embedded base
// schema, and the pointer-table migration runner for graft's store.
package database

import (
	"database/sql"
	"embed"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema
var schemaFS embed.FS

// Open opens a sqlite database at path, applies the embedded base schema,
// runs migrations, and pins the pool to a single connection.
//
// WAL + busy_timeout let concurrent graft processes wait for the write lock
// instead of failing with SQLITE_BUSY. SetMaxOpenConns(1) forces every query
// to start a fresh WAL read transaction so a pooled connection never serves a
// stale snapshot that predates rows written by another process.
func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}
	if err := applySchema(conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := runMigrations(conn); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	return conn, nil
}

// OpenReadOnly opens a read-only connection that must not block writers.
// It is not pinned and not a singleton.
func OpenReadOnly(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// applySchema runs every embedded base schema file against conn in sorted
// filename order. All statements are IF NOT EXISTS so this is safe on every open.
func applySchema(conn *sql.DB) error {
	entries, err := fs.ReadDir(schemaFS, "schema")
	if err != nil {
		return err
	}
	// Sort by the numeric filename prefix (1-, 2- … 10-) so order stays correct
	// past 9 files — lexicographic sort would place "10-" before "2-".
	prefix := func(name string) int {
		if i := strings.IndexByte(name, '-'); i > 0 {
			if n, err := strconv.Atoi(name[:i]); err == nil {
				return n
			}
		}
		return 1 << 30 // unprefixed files apply last, deterministically
	}
	sort.Slice(entries, func(i, j int) bool {
		pi, pj := prefix(entries[i].Name()), prefix(entries[j].Name())
		if pi != pj {
			return pi < pj
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := schemaFS.ReadFile("schema/" + e.Name())
		if err != nil {
			return err
		}
		if _, err = conn.Exec(string(data)); err != nil {
			return err
		}
	}
	return nil
}
