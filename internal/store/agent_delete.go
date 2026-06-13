package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeleteAgent removes one agent's rows (agent_states, provider_links, agents)
// for (wsID, name), FK-safe leaf-to-root, in a single atomic transaction.
// A (wsID, name) pair with no agents row is a no-op (returns nil, no error).
// Used by the sync engine to propagate a canonical deletion
// (v0.0.4 verify task 3 / no-resurrection).
//
// Note: sync_runs, branches, and conflicts are run-scoped, not agent-scoped,
// so they are intentionally left untouched.
func (s *sqlStore) DeleteAgent(wsID, name string) error {
	ctx := context.Background()

	// Resolve the agent's internal ID. If it doesn't exist, treat as a no-op.
	var agentID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM agents WHERE ws_id = ? AND name = ?`,
		wsID, name,
	).Scan(&agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // no-op: agent was never tracked
		}
		return fmt.Errorf("store.DeleteAgent: resolve id: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store.DeleteAgent: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // superseded by Commit

	// 1. agent_states — leaf table keyed by agent_id.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agent_states WHERE agent_id = ?`,
		agentID,
	); err != nil {
		return fmt.Errorf("store.DeleteAgent: delete agent_states: %w", err)
	}

	// 2. provider_links — keyed by agent_id.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM provider_links WHERE agent_id = ?`,
		agentID,
	); err != nil {
		return fmt.Errorf("store.DeleteAgent: delete provider_links: %w", err)
	}

	// 3. agents — the root agent row itself.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agents WHERE id = ?`,
		agentID,
	); err != nil {
		return fmt.Errorf("store.DeleteAgent: delete agent: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store.DeleteAgent: commit: %w", err)
	}
	return nil
}
