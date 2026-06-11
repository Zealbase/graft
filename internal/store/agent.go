package store

import (
	"database/sql"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// UpsertAgent creates or updates an agent row keyed by (ws_id, name), setting
// canonical_hash, and returns the agent with ID populated. A new row gets a
// generated id; an existing (ws_id, name) keeps its id and updates the hash.
func (s *sqlStore) UpsertAgent(a contract.Agent) (contract.Agent, error) {
	if a.ID == "" {
		a.ID = newID()
	}
	_, err := s.db.Exec(
		`INSERT INTO agents (id, ws_id, name, canonical_hash)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(ws_id, name) DO UPDATE SET
		   canonical_hash = excluded.canonical_hash`,
		a.ID, a.WsID, a.Name, a.CanonicalHash,
	)
	if err != nil {
		return contract.Agent{}, err
	}
	// Re-read so the returned ID is the canonical one (handles the case where
	// the row already existed under a different generated id).
	var got contract.Agent
	if err := s.db.QueryRow(
		`SELECT id, ws_id, name, canonical_hash FROM agents WHERE ws_id = ? AND name = ?`,
		a.WsID, a.Name,
	).Scan(&got.ID, &got.WsID, &got.Name, &got.CanonicalHash); err != nil {
		return contract.Agent{}, err
	}
	return got, nil
}

// UpsertProviderLink upserts one provider's on-disk mapping for an agent.
// Identity is (agent_id, provider). The referenced agent row is ensured here
// (lazily, in a transaction) as an idempotent safety net so the FK always holds
// even if the engine has not yet called UpsertAgent for it.
func (s *sqlStore) UpsertProviderLink(l contract.ProviderLink) error {
	if l.ID == "" {
		l.ID = newID()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := ensureAgentTx(tx, l.AgentID); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO provider_links (id, agent_id, provider, file_path, content_hash, commit_hash)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, provider) DO UPDATE SET
		   file_path = excluded.file_path,
		   content_hash = excluded.content_hash,
		   commit_hash = excluded.commit_hash`,
		l.ID, l.AgentID, l.Provider, l.FilePath, l.ContentHash, l.CommitHash,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ensureAgentTx inserts a placeholder agents row if one does not already exist
// for id. No-op when the row is present (canonical fields preserved).
func ensureAgentTx(tx *sql.Tx, id string) error {
	_, err := tx.Exec(
		`INSERT INTO agents (id, ws_id, name, canonical_hash)
		 VALUES (?, '', '', '')
		 ON CONFLICT(id) DO NOTHING`,
		id,
	)
	return err
}
