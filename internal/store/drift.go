package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Drift reports whether the named agent in a workspace is out of sync with its
// canonical definition. An agent is drifted when any provider link's content
// hash differs from the agent's canonical hash. The returned reason names the
// first diverging provider (or explains why drift could not be determined).
func (s *sqlStore) Drift(wsID, name string) (bool, string, error) {
	var agentID, canonicalHash string
	err := s.db.QueryRow(
		`SELECT id, canonical_hash FROM agents WHERE ws_id = ? AND name = ?`,
		wsID, name,
	).Scan(&agentID, &canonicalHash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "agent not tracked", nil
	}
	if err != nil {
		return false, "", err
	}

	rows, err := s.db.Query(
		`SELECT provider, content_hash FROM provider_links WHERE agent_id = ?`,
		agentID,
	)
	if err != nil {
		return false, "", err
	}
	defer rows.Close()

	any := false
	for rows.Next() {
		var provider, contentHash string
		if err := rows.Scan(&provider, &contentHash); err != nil {
			return false, "", err
		}
		any = true
		if contentHash != canonicalHash {
			return true, fmt.Sprintf("%s differs from canonical", provider), nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, "", err
	}
	if !any {
		return false, "no provider links", nil
	}
	return false, "", nil
}
