CREATE TABLE IF NOT EXISTS agents (
    id             TEXT PRIMARY KEY,
    ws_id          TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    canonical_hash TEXT NOT NULL DEFAULT '',
    UNIQUE (ws_id, name),
    FOREIGN KEY (ws_id) REFERENCES workspaces(id)
);
