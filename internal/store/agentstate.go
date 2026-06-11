package store

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SaveAgentState upserts the per-run sync state of one agent. Identity is
// (run_id, agent_id). Both run_id and agent_id must already exist (the engine
// opens the run and calls UpsertAgent first); unknown ids are rejected by the
// respective FKs with a clear foreign-key error.
func (s *sqlStore) SaveAgentState(st contract.AgentState) error {
	if st.ID == "" {
		st.ID = newID()
	}
	_, err := s.db.Exec(
		`INSERT INTO agent_states (id, run_id, agent_id, in_sync, reason)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(run_id, agent_id) DO UPDATE SET
		   in_sync = excluded.in_sync,
		   reason = excluded.reason`,
		st.ID, st.RunID, st.AgentID, boolToInt(st.InSync), st.Reason,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
