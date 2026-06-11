CREATE TABLE IF NOT EXISTS workspaces (
    id         TEXT PRIMARY KEY,
    root       TEXT NOT NULL,
    remote     TEXT NOT NULL,
    branch     TEXT NOT NULL,
    git_mode   TEXT NOT NULL DEFAULT 'tracked',
    created_at INTEGER NOT NULL DEFAULT 0,
    UNIQUE (root, remote, branch)
);
