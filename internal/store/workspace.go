package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// Workspace upserts the (root, remote, branch) identity and returns the row.
// Identity is the unique tuple; a repeat call returns the existing workspace
// unchanged (id and created_at are preserved).
func (s *sqlStore) Workspace(root, remote, branch string) (contract.Workspace, error) {
	if existing, err := s.workspaceByIdentity(root, remote, branch); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return contract.Workspace{}, err
	}

	ws := contract.Workspace{
		ID:        newID(),
		Root:      root,
		Remote:    remote,
		Branch:    branch,
		GitMode:   contract.GitTracked,
		CreatedAt: time.Now().Unix(),
	}
	_, err := s.db.Exec(
		`INSERT INTO workspaces (id, root, remote, branch, git_mode, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(root, remote, branch) DO NOTHING`,
		ws.ID, ws.Root, ws.Remote, ws.Branch, string(ws.GitMode), ws.CreatedAt,
	)
	if err != nil {
		return contract.Workspace{}, err
	}
	// Re-read to return the canonical row (handles a concurrent insert that won).
	return s.workspaceByIdentity(root, remote, branch)
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
