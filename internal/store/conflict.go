package store

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SaveConflict records (or refreshes) an unresolved merge for a run. Identity is
// (run_id, path). The contract.Conflict carries Path + Agent (agent name); the
// row's status defaults to "open" and is left untouched on a repeat save so a
// later resolution state set elsewhere is not clobbered.
func (s *sqlStore) SaveConflict(runID string, c contract.Conflict) error {
	_, err := s.db.Exec(
		`INSERT INTO conflicts (id, run_id, agent_name, path, status)
		 VALUES (?, ?, ?, ?, 'open')
		 ON CONFLICT(run_id, path) DO UPDATE SET
		   agent_name = excluded.agent_name`,
		newID(), runID, c.Agent, c.Path,
	)
	return err
}

// ResolveConflicts marks every still-open conflict for a run as resolved.
func (s *sqlStore) ResolveConflicts(runID string) error {
	_, err := s.db.Exec(
		`UPDATE conflicts SET status = 'resolved' WHERE run_id = ? AND status = 'open'`,
		runID,
	)
	return err
}
