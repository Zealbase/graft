package store

import (
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

// AgentSynced reports whether a PRIOR sync COMPLETED for (wsID, name) — defined
// as: an agents row exists AND it has at least one provider_links row. The
// provider_links requirement is the robust discriminator: applyProviders only
// records a provider link after git.Copy lands the resolved canonical, so a row
// here means at least one full sync ran to completion for this agent. An agents
// row WITHOUT any provider_links (e.g. a prior aborted run that called
// UpsertAgent in prepareBranches but never reached applyProviders) is NOT
// "synced" — treating it as such would let a deletion probe mis-classify a
// genuinely-new provider-authored agent as a deleted one (v0.0.4 verify r2
// HIGH 2). Returns false (no error) when no agents row exists.
func (s *sqlStore) AgentSynced(wsID, name string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM provider_links pl
		   JOIN agents a ON a.id = pl.agent_id
		  WHERE a.ws_id = ? AND a.name = ?`,
		wsID, name,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// UpsertProviderLink upserts one provider's on-disk mapping for an agent.
// Identity is (agent_id, provider). The agent_id must already exist (the engine
// calls UpsertAgent first); an unknown agent is rejected by the agents FK with a
// clear foreign-key error rather than being silently fabricated.
func (s *sqlStore) UpsertProviderLink(l contract.ProviderLink) error {
	if l.ID == "" {
		l.ID = newID()
	}
	_, err := s.db.Exec(
		`INSERT INTO provider_links (id, agent_id, provider, file_path, content_hash, commit_hash)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, provider) DO UPDATE SET
		   file_path = excluded.file_path,
		   content_hash = excluded.content_hash,
		   commit_hash = excluded.commit_hash`,
		l.ID, l.AgentID, l.Provider, l.FilePath, l.ContentHash, l.CommitHash,
	)
	return err
}
