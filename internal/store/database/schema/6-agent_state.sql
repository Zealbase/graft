CREATE TABLE IF NOT EXISTS agent_states (
    id       TEXT PRIMARY KEY,
    run_id   TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    in_sync  INTEGER NOT NULL DEFAULT 0,
    reason   TEXT NOT NULL DEFAULT '',
    UNIQUE (run_id, agent_id),
    FOREIGN KEY (run_id) REFERENCES sync_runs(run_id),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);
