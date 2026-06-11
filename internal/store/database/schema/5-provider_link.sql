CREATE TABLE IF NOT EXISTS provider_links (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    provider     TEXT NOT NULL,
    file_path    TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    commit_hash  TEXT NOT NULL DEFAULT '',
    UNIQUE (agent_id, provider),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);
