-- +goose Up
CREATE TABLE users (id TEXT PRIMARY KEY, login TEXT NOT NULL UNIQUE, name TEXT NOT NULL, password_hash BLOB NOT NULL, role TEXT NOT NULL CHECK(role IN ('admin','user')), disabled INTEGER NOT NULL DEFAULT 0, must_change INTEGER NOT NULL DEFAULT 1);
CREATE TABLE sessions (token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at INTEGER NOT NULL);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE TABLE categories (id TEXT PRIMARY KEY, title TEXT NOT NULL UNIQUE, sort_order INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1);
CREATE TABLE articles (id TEXT PRIMARY KEY, category_id TEXT REFERENCES categories(id) ON DELETE RESTRICT, author_id TEXT NOT NULL REFERENCES users(id), last_editor_id TEXT REFERENCES users(id), title TEXT NOT NULL DEFAULT '', document TEXT NOT NULL DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}', plain_text TEXT NOT NULL DEFAULT '', version INTEGER NOT NULL DEFAULT 0, published_at INTEGER, updated_at INTEGER NOT NULL DEFAULT 0, archived INTEGER NOT NULL DEFAULT 0);
CREATE INDEX articles_browse ON articles(archived,published_at,updated_at DESC,id);
CREATE TABLE drafts (id TEXT PRIMARY KEY, article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE, owner_id TEXT NOT NULL REFERENCES users(id), category_id TEXT REFERENCES categories(id) ON DELETE RESTRICT, title TEXT NOT NULL DEFAULT '', document TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1, base_article_version INTEGER NOT NULL, updated_at INTEGER NOT NULL, UNIQUE(article_id,owner_id));
CREATE TABLE media (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL REFERENCES users(id), mime TEXT NOT NULL, size INTEGER NOT NULL, width INTEGER NOT NULL, height INTEGER NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE draft_media (draft_id TEXT NOT NULL REFERENCES drafts(id) ON DELETE CASCADE, media_id TEXT NOT NULL REFERENCES media(id) ON DELETE RESTRICT, PRIMARY KEY(draft_id,media_id));
CREATE TABLE article_media (article_id TEXT NOT NULL REFERENCES articles(id) ON DELETE CASCADE, media_id TEXT NOT NULL REFERENCES media(id) ON DELETE RESTRICT, PRIMARY KEY(article_id,media_id));
CREATE VIRTUAL TABLE articles_fts USING fts5(title,body,article_id UNINDEXED,tokenize='unicode61');
CREATE TABLE login_limits (login TEXT PRIMARY KEY, failures INTEGER NOT NULL, window_start INTEGER NOT NULL);
-- +goose Down
DROP TABLE login_limits;
DROP TABLE articles_fts;
DROP TABLE article_media;
DROP TABLE draft_media;
DROP TABLE media;
DROP TABLE drafts;
DROP TABLE articles;
DROP TABLE categories;
DROP TABLE sessions;
DROP TABLE users;
