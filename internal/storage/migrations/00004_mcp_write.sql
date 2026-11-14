-- +goose Up
ALTER TABLE mcp_tokens ADD COLUMN can_write INTEGER NOT NULL DEFAULT 0 CHECK(can_write IN (0,1));

-- +goose Down
ALTER TABLE mcp_tokens DROP COLUMN can_write;
