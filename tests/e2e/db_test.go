package e2e

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// openDB opens the global graft.db read-only. The binary stores the db at
// $XDG_DATA_HOME/graft/graft.db (moved out of the repo by plan-revise). The
// harness sets XDG_DATA_HOME=<dir>/xdg-data for every subprocess, so the
// global db for a test rooted at dir lives at that path.
func openDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	path := filepath.Join(dir, "xdg-data", "graft", "graft.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db ro: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}

// queryInt runs a scalar-int query and returns the value.
func queryInt(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

// queryString runs a scalar-string query and returns the value.
func queryString(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s string
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return s
}

// providerLinkHashes returns provider -> content_hash for one agent name.
func providerLinkHashes(t *testing.T, db *sql.DB, agentName string) map[string]string {
	t.Helper()
	rows, err := db.Query(`
		SELECT pl.provider, pl.content_hash
		FROM provider_links pl
		JOIN agents a ON a.id = pl.agent_id
		WHERE a.name = ?`, agentName)
	if err != nil {
		t.Fatalf("query provider_links: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			t.Fatalf("scan provider_link: %v", err)
		}
		out[p] = h
	}
	return out
}
