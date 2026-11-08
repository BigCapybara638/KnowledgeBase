-- +goose Up
CREATE TABLE mcp_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    revoked_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX mcp_tokens_user ON mcp_tokens(user_id, created_at DESC);

-- +goose Down
DROP TABLE mcp_tokens;
