# MCP Article Drafts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Let an authorized MCP agent create a complete personal article draft.

**Architecture:** Add per-key write permission, an atomic filled-draft service,
and a conditional MCP write tool. Reuse existing author identity, editor,
publication, SDK transports, session and CSRF protections.

**Tech Stack:** Existing Go 1.27.3/SQLite/Tiptap, MCP Go SDK v1.8.0, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-11-mcp-article-drafts-design.md`

## Global Constraints

- `title` up to 512 bytes; plain `body` up to 256 KiB; optional category UUID.
- Keep newline structure and Unicode; no HTML/Markdown interpretation.
- Complete new draft is atomic and private, with no published/FTS entry.
- Existing keys default to read only; owner and identity come from the key.
- Browser key permission changes require session, active owner and CSRF.
- Read tokens expose 4 tools, write tokens 5; no publication or update tools.
- Keep existing local port 8081 changes and current working tree.

## Review Focus

- Invalid/empty content and unknown categories must leave no orphan rows.
- Read-only, revoked and disabled keys must not create drafts by calling a hidden tool directly.
- Permission changes must apply to subsequent calls of an already connected client.
- New draft must retain line breaks and be editable/publishable by its owner.
- stdio must preserve write-tool annotations and not describe the server as unconditionally read only.

### Task 1: Key permissions and management

**Files:** migration `00004_mcp_write.sql`, `internal/auth/mcp_tokens.go`, auth tests;
`internal/web/mcp_tokens.go`, `internal/ui/templates/mcp_tokens.html`, web tests.

**Interfaces:** `MCPToken.CanWrite bool`, `MCPAccess{User domain.User; CanWrite bool}`;
`CreateMCPTokenWithWrite(ctx,actor,name,days,canWrite)`, `ResolveMCPAccess(ctx,raw)`,
`SetMCPTokenWrite(ctx,actor,id,canWrite)`. Existing creation/resolution APIs remain wrappers.

- [x] Write and run failing tests for defaults, explicit permission, changes and owner/expiry gates.
- [x] Implement migration and auth APIs; require active owned key for permission updates.
- [x] Write failing web tests; implement create rights selector and CSRF-protected `POST /settings/mcp/{id}/permissions`.
- [x] Run auth/web tests, verify secrets and existing read behavior remain intact.

### Task 2: Complete draft creation

**Files:** `internal/articles/create.go`, creation tests, `internal/content/plain.go`, tests.

**Interfaces:** `content.PlainDocument(text string) (json.RawMessage,error)`;
`articles.CreateDraftInput{Title string; CategoryID domain.ID; Document json.RawMessage}`;
`(*articles.Service).CreateFilledDraft(ctx,actor,input) (domain.Draft,error)`.

- [x] Write/run failing tests for line breaks/Unicode, limits, empty title and atomic invalid-category failure.
- [x] Implement safe document construction and a transaction creating article placeholder plus full draft.
- [x] Verify private ownership, no published/search result, and existing publication works afterwards.

### Task 3: MCP write tool and adapter

**Files:** `internal/mcpserver/server.go`, new `write.go`, integration tests;
`internal/mcpbridge/bridge.go`, bridge/process tests.

**Interfaces:** `create_article` returns `{draft_id,article_id,title,version,status,edit_url}`;
HTTP chooses read or write SDK server using per-request `MCPAccess`.

- [x] Write/run failing SDK tests for five tools, read-only direct rejection, exact owner and draft URL, invalid input and permission revocation.
- [x] Register write tool with additive/non-idempotent annotations; preserve existing four read tools.
- [x] Make bridge instructions reflect the advertised tools; verify actual stdio creation against temporary App data.

### Task 4: Integration and delivery

**Files:** `docs/mcp.md`, README MCP section, browser scenario, generated sqlc model,
validation report and this plan.

- [x] Document tool body format, permissions and reconnect requirement; update existing read-only claims.
- [x] Run sqlc, UI build, full Go race tests and vet, browser regression, adapter builds and production Docker build.
- [x] Request independent final review; reproduce/fix material findings and repeat affected checks.
- [x] Back up existing data, update local container, verify health/read MCP without creating test articles in the real database.
- [x] Record evidence and final usage steps. Do not change the user's external MCP config or publish sample articles.

## Execution

Native execution in the current isolated worktree. The user chose drafts;
continue the authorized feature work without another approval loop.
