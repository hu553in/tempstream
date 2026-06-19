-- +goose Up
CREATE TABLE IF NOT EXISTS watch_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    expires_at INTEGER,
    disabled_at INTEGER,
    note TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_watch_links_token ON watch_links (token);

CREATE INDEX IF NOT EXISTS idx_watch_links_enabled ON watch_links (enabled);

-- +goose Down
DROP TABLE IF EXISTS watch_links;
