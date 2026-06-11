package store

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SaveAgentState upserts the per-run sync state of one agent. Identity is
// (run_id, agent_id); the referenced agent row is ensured first so the FK holds.
func (s *sqlStore) SaveAgentState(st contract.AgentState) error {
	if st.ID == "" {
		st.ID = newID()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := ensureAgentTx(tx, st.AgentID); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO agent_states (id, run_id, agent_id, in_sync, reason)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(run_id, agent_id) DO UPDATE SET
		   in_sync = excluded.in_sync,
		   reason = excluded.reason`,
		st.ID, st.RunID, st.AgentID, boolToInt(st.InSync), st.Reason,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
