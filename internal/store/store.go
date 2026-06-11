// Package store is the sqlite-backed implementation of contract.Store.
// It owns the schema (internal/store/database) and all queries. Cross-table
// writes thread a *sql.Tx internally.
package store

import (
	"database/sql"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/store/database"
)

// sqlStore implements contract.Store over a single *sql.DB.
type sqlStore struct {
	db *sql.DB
}

// Compile-time assertion that sqlStore satisfies the frozen contract.
var _ contract.Store = (*sqlStore)(nil)

// Open opens (and migrates) the sqlite database at dbPath and returns a Store.
func Open(dbPath string) (contract.Store, error) {
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &sqlStore{db: db}, nil
}

// New wraps an already-open *sql.DB. Test seam — the caller owns the schema
// (use database.Open or apply the embedded schema first).
func New(db *sql.DB) contract.Store {
	return &sqlStore{db: db}
}

// Close closes the underlying database.
func (s *sqlStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
