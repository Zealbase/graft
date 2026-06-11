CREATE TABLE IF NOT EXISTS branches (
    id        TEXT PRIMARY KEY,
    run_id    TEXT NOT NULL,
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL DEFAULT 'agent',
    head_hash TEXT NOT NULL DEFAULT '',
    state     TEXT NOT NULL DEFAULT '',
    UNIQUE (run_id, name),
    FOREIGN KEY (run_id) REFERENCES sync_runs(run_id)
);
