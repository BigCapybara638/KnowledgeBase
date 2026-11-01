-- +goose Up
CREATE TABLE drafts_new (id TEXT PRIMARY KEY, article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE, owner_id TEXT NOT NULL REFERENCES users(id), category_id TEXT REFERENCES categories(id) ON DELETE RESTRICT, title TEXT NOT NULL DEFAULT '', document TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1, base_article_version INTEGER NOT NULL, updated_at INTEGER NOT NULL);
INSERT INTO drafts_new SELECT * FROM drafts;
CREATE TABLE draft_media_new (draft_id TEXT NOT NULL REFERENCES drafts_new(id) ON DELETE CASCADE, media_id TEXT NOT NULL REFERENCES media(id) ON DELETE RESTRICT, PRIMARY KEY(draft_id,media_id));
INSERT INTO draft_media_new SELECT * FROM draft_media;
DROP TABLE draft_media;
DROP TABLE drafts;
ALTER TABLE drafts_new RENAME TO drafts;
ALTER TABLE draft_media_new RENAME TO draft_media;
CREATE INDEX drafts_owner ON drafts(owner_id,updated_at DESC,id);
