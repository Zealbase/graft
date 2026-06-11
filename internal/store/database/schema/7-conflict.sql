CREATE TABLE IF NOT EXISTS conflicts (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL,
    agent_name TEXT NOT NULL DEFAULT '',
    path       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'open',
    UNIQUE (run_id, path),
    FOREIGN KEY (run_id) REFERENCES sync_runs(run_id)
);
