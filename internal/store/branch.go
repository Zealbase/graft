package store

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SaveBranch upserts a deterministic graft-created ref for a run. Identity is
// (run_id, name); a repeat call updates head_hash/kind/state and keeps the id.
func (s *sqlStore) SaveBranch(b contract.Branch) error {
	if b.ID == "" {
		b.ID = newID()
	}
	if b.Kind == "" {
		b.Kind = contract.BranchAgent
	}
	_, err := s.db.Exec(
		`INSERT INTO branches (id, run_id, name, kind, head_hash, state)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id, name) DO UPDATE SET
		   kind = excluded.kind,
		   head_hash = excluded.head_hash,
		   state = excluded.state`,
		b.ID, b.RunID, b.Name, string(b.Kind), b.HeadHash, b.State,
	)
	return err
}

// Branches lists every branch recorded for a run.
func (s *sqlStore) Branches(runID string) ([]contract.Branch, error) {
	rows, err := s.db.Query(
		`SELECT id, run_id, name, kind, head_hash, state
		 FROM branches WHERE run_id = ? ORDER BY rowid`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []contract.Branch
	for rows.Next() {
		var b contract.Branch
		var kind string
		if err := rows.Scan(&b.ID, &b.RunID, &b.Name, &kind, &b.HeadHash, &b.State); err != nil {
			return nil, err
		}
		b.Kind = contract.BranchKind(kind)
		out = append(out, b)
	}
	return out, rows.Err()
}
