package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// Workspace upserts the (root, remote, branch) identity with the given git mode
// and returns the canonical row. Identity is the unique tuple; on a repeat call
// the existing row's id and created_at are preserved. git_mode is updated to the
// passed mode EXCEPT when the stored mode is already 'internal' — internal is
// sticky once set (re-init cannot downgrade it to tracked). Other modes update
// normally (e.g. tracked→tracked, tracked→internal are both allowed).
func (s *sqlStore) Workspace(root, remote, branch string, mode contract.GitMode) (contract.Workspace, error) {
	ws := contract.Workspace{
		ID:        newID(),
		Root:      root,
		Remote:    remote,
		Branch:    branch,
		GitMode:   mode,
		CreatedAt: time.Now().Unix(),
	}
	_, err := s.db.Exec(
		`INSERT INTO workspaces (id, root, remote, branch, git_mode, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(root, remote, branch) DO UPDATE SET
		   git_mode = excluded.git_mode WHERE workspaces.git_mode != 'internal'`,
		ws.ID, ws.Root, ws.Remote, ws.Branch, string(ws.GitMode), ws.CreatedAt,
	)
	if err != nil {
		return contract.Workspace{}, err
	}
	// Re-read to return the canonical row (preserves original id/created_at on an
	// existing identity, and handles a concurrent insert that won).
	return s.workspaceByIdentity(root, remote, branch)
}

// FindWorkspace is a read-only probe for the (root, remote, branch) identity. It
// returns the existing workspace row or (nil, nil) when none exists — it never
// inserts. Used to derive "initialized?" and to gate checks without side effects.
func (s *sqlStore) FindWorkspace(root, remote, branch string) (*contract.Workspace, error) {
	ws, err := s.workspaceByIdentity(root, remote, branch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

func (s *sqlStore) workspaceByIdentity(root, remote, branch string) (contract.Workspace, error) {
	row := s.db.QueryRow(
		`SELECT id, root, remote, branch, git_mode, created_at
		 FROM workspaces WHERE root = ? AND remote = ? AND branch = ?`,
		root, remote, branch,
	)
	return scanWorkspace(row)
}

func scanWorkspace(row *sql.Row) (contract.Workspace, error) {
	var ws contract.Workspace
	var mode string
	if err := row.Scan(&ws.ID, &ws.Root, &ws.Remote, &ws.Branch, &mode, &ws.CreatedAt); err != nil {
		return contract.Workspace{}, err
	}
	ws.GitMode = contract.GitMode(mode)
	return ws, nil
}
