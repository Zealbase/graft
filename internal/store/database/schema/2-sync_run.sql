CREATE TABLE IF NOT EXISTS sync_runs (
    run_id          TEXT PRIMARY KEY,
    ws_id           TEXT NOT NULL,
    base_branch     TEXT NOT NULL DEFAULT '',
    base_start_hash TEXT NOT NULL DEFAULT '',
    beta_branch     TEXT NOT NULL DEFAULT '',
    phase           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'running',
    started_at      INTEGER NOT NULL DEFAULT 0,
    ended_at        INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (ws_id) REFERENCES workspaces(id)
);

CREATE INDEX IF NOT EXISTS idx_sync_runs_ws_status ON sync_runs(ws_id, status);
