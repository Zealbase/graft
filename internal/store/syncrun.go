package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

const syncRunCols = `run_id, ws_id, base_branch, base_start_hash, beta_branch, phase, status, started_at, ended_at`

// OpenRun creates a new sync run in the running state and returns it.
func (s *sqlStore) OpenRun(wsID, baseBranch, startHash string) (contract.SyncRun, error) {
	run := contract.SyncRun{
		RunID:         newID(),
		WsID:          wsID,
		BaseBranch:    baseBranch,
		BaseStartHash: startHash,
		Status:        contract.RunRunning,
		StartedAt:     time.Now().Unix(),
	}
	_, err := s.db.Exec(
		`INSERT INTO sync_runs (`+syncRunCols+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.RunID, run.WsID, run.BaseBranch, run.BaseStartHash, run.BetaBranch,
		run.Phase, string(run.Status), run.StartedAt, run.EndedAt,
	)
	if err != nil {
		return contract.SyncRun{}, err
	}
	return run, nil
}

// UpdateRun persists the full run row (status, phase, beta branch, ended_at …).
func (s *sqlStore) UpdateRun(run contract.SyncRun) error {
	res, err := s.db.Exec(
		`UPDATE sync_runs SET
		   ws_id = ?, base_branch = ?, base_start_hash = ?, beta_branch = ?,
		   phase = ?, status = ?, started_at = ?, ended_at = ?
		 WHERE run_id = ?`,
		run.WsID, run.BaseBranch, run.BaseStartHash, run.BetaBranch,
		run.Phase, string(run.Status), run.StartedAt, run.EndedAt, run.RunID,
	)
	if err != nil {
		return err
	}
	// A run is always opened (OpenRun) before it is updated; 0 rows means the
	// run_id is wrong and the status change was silently lost — surface it.
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("UpdateRun: no sync_run with run_id %s", run.RunID)
	}
	return nil
}

// OpenConflictRun returns the most recent resumable (status=conflict) run for a
// workspace, or nil if there is none to resume.
func (s *sqlStore) OpenConflictRun(wsID string) (*contract.SyncRun, error) {
	row := s.db.QueryRow(
		`SELECT `+syncRunCols+`
		 FROM sync_runs
		 WHERE ws_id = ? AND status = ?
		 ORDER BY started_at DESC, rowid DESC
		 LIMIT 1`,
		wsID, string(contract.RunConflict),
	)
	run, err := scanSyncRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func scanSyncRun(row *sql.Row) (contract.SyncRun, error) {
	var run contract.SyncRun
	var status string
	if err := row.Scan(
		&run.RunID, &run.WsID, &run.BaseBranch, &run.BaseStartHash, &run.BetaBranch,
		&run.Phase, &status, &run.StartedAt, &run.EndedAt,
	); err != nil {
		return contract.SyncRun{}, err
	}
	run.Status = contract.RunStatus(status)
	return run, nil
}
