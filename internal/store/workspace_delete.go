package store

import (
	"context"
	"fmt"
)

// DeleteWorkspace removes a workspace and every row that cascades from it, in
// FK-safe leaf-to-root order inside a single atomic transaction.
//
// Isolation note: the modernc/sqlite driver ignores sql.TxOptions.Isolation, so
// we pass nil. Write serialization is provided by two complementary mechanisms:
//   - database/db.go pins the pool to a single connection (SetMaxOpenConns(1)),
//     which means only one goroutine can hold the write lock at a time within a
//     process.
//   - The caller (CLI/EntryGate) holds a per-workspace flock before reaching
//     this function, serializing concurrent graft processes on the same workspace.
//
// The transaction here is purely for atomicity (all-or-nothing cascade): if any
// DELETE step fails the entire cascade is rolled back automatically.
//
// Cascade order (respects FK graph):
//
//	conflicts        → scoped via sync_runs.ws_id
//	branches         → scoped via sync_runs.ws_id
//	agent_states     → scoped via agents.ws_id  (run_id FK, but agent_id FK too)
//	sync_runs        → ws_id
//	provider_links   → scoped via agents.ws_id
//	agents           → ws_id
//	workspaces       → id
//
// Foreign keys are enforced (PRAGMA foreign_keys=ON), so violations would
// surface as errors; we delete leaf tables first for correctness and
// determinism regardless.
func (s *sqlStore) DeleteWorkspace(wsID string) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store.DeleteWorkspace: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // superseded by Commit

	// 1. conflicts — keyed by run_id → join to sync_runs scoped to wsID.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM conflicts
		 WHERE run_id IN (SELECT run_id FROM sync_runs WHERE ws_id = ?)`,
		wsID,
	); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: delete conflicts: %w", err)
	}

	// 2. branches — keyed by run_id → same join.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM branches
		 WHERE run_id IN (SELECT run_id FROM sync_runs WHERE ws_id = ?)`,
		wsID,
	); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: delete branches: %w", err)
	}

	// 3. agent_states — keyed by agent_id → join through agents scoped to wsID.
	//    (also has a run_id FK, but clearing by agent_id covers all states for
	//    this workspace's agents regardless of which run they belong to.)
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agent_states
		 WHERE agent_id IN (SELECT id FROM agents WHERE ws_id = ?)`,
		wsID,
	); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: delete agent_states: %w", err)
	}

	// 4. sync_runs — directly scoped by ws_id.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM sync_runs WHERE ws_id = ?`,
		wsID,
	); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: delete sync_runs: %w", err)
	}

	// 5. provider_links — keyed by agent_id → join through agents.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM provider_links
		 WHERE agent_id IN (SELECT id FROM agents WHERE ws_id = ?)`,
		wsID,
	); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: delete provider_links: %w", err)
	}

	// 6. agents — directly scoped by ws_id.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agents WHERE ws_id = ?`,
		wsID,
	); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: delete agents: %w", err)
	}

	// 7. workspaces — the root row.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM workspaces WHERE id = ?`,
		wsID,
	); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: delete workspace: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store.DeleteWorkspace: commit: %w", err)
	}
	return nil
}
