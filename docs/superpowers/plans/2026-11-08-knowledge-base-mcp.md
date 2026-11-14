# Knowledge Base MCP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Give local MCP agents authenticated read access to published knowledge-base articles.

**Architecture:** Embed the official Go MCP SDK's stateless HTTP handler at `/mcp` in the existing application. Store hashed, user-owned access keys in SQLite and manage them through session/CSRF-protected pages. Provide an independent stdio-to-HTTP adapter for desktop clients.

**Tech Stack:** Existing Go, SQLite, HTML templates; official MCP Go SDK pinned to a stable version; Docker for Linux verification.

**Spec:** `docs/superpowers/specs/2026-11-08-knowledge-base-mcp-design.md`

## Global Constraints

- Reuse the current linked worktree, single application container, database and search.
- Only published, nonarchived articles; no MCP mutations or private drafts.
- SHA-256 hashes only; 32 random secret bytes; named keys of 1–80 characters.
- At most 20 active keys per user; expiry choices 30, 90, 365 days, default 90.
- HTTP uses Bearer keys; browser session cookies are not MCP credentials.
- MCP request body at most 1 MiB; operation deadline 15 seconds; no shared mutable actor.
- Article chunks count Unicode runes: default 12000, maximum 20000.
- Page sizes: articles/search 20, categories 100; input page upper bound 100000.
- Adapter has no storage imports; HTTPS except loopback development; no HTTP redirects.
- Keys never appear in URLs or logs. stdout of adapter contains MCP only.

## Review Focus

- Already connected clients after revocation: their next request must fail.
- Different users under concurrent calls: no identity crossing or key management leakage.
- Russian text at a chunk boundary: reconstruct exactly without losing Unicode characters.
- Invalid transport credentials: no HTML redirect, no cookie fallback or secret-bearing redirects.
- EOF and cancellation in the stdio adapter: shut down cleanly without hanging.

### Task 1: User-owned access keys

**Files:** create `internal/storage/migrations/00003_mcp_tokens.sql`, `internal/auth/mcp_tokens.go`, `internal/auth/mcp_tokens_test.go`; modify `internal/auth/service.go`.

**Interfaces:** `MCPToken{ID,Name,CreatedAt,ExpiresAt,RevokedAt}`; `CreateMCPToken(ctx, actor, name, days) (MCPToken,string,error)`, `ListMCPTokens(ctx,actor) ([]MCPToken,error)`, `RevokeMCPToken(ctx,actor,id) error`, `ResolveMCPToken(ctx,raw) (domain.User,error)` on `auth.Service`.

- [x] Add failing tests for issuance/hash-only storage, expiry, foreign ownership, revocation, disabled/must-change users, the 20-key limit and password changes/resets.
- [x] Run targeted tests and confirm failure before implementation.
- [x] Implement migration and methods using existing `Store.WithWrite`, `AssertActor`, and `Service.now`; revoke tokens transactionally alongside sessions on password changes/resets.
- [x] Run targeted tests; expect PASS. Check diff for unintended auth changes.

### Task 2: Embedded HTTP MCP and read tools

**Files:** create `internal/mcpserver/server.go`, `internal/mcpserver/tools.go`, `internal/mcpserver/server_test.go`; modify `go.mod`, `go.sum`, `internal/web/app.go`.

**Interfaces:** `mcpserver.New(cfg config.Config,s *storage.Store,accounts *auth.Service) http.Handler`; consumes Task 1's resolver. `web.App` intercepts exact `/mcp` before browser middleware.

- [x] Add SDK-client integration tests: discover four tools, search and fetch published articles; deny drafts/archive, missing/revoked keys, cookie fallback and untrusted Origin; validate chunk boundaries and concurrent users.
- [x] Add official SDK dependency; run tests and confirm missing implementation fails.
- [x] Implement bounded stateless HTTP handler and four tools using existing services. Preserve errors as standard MCP results; never reveal SQL errors. Return structured content plus JSON text.
- [x] Bound snippets and article list metadata so a 2 MiB article does not inflate list responses. Paginate category output with the prescribed bounds.
- [x] Run integration tests with latest SDK negotiation and a supported legacy protocol; expect PASS.

### Task 3: Browser key management

**Files:** create `internal/web/mcp_tokens.go`, `internal/web/mcp_tokens_test.go`, `internal/ui/templates/mcp_tokens.html`; modify `internal/web/app.go`, `internal/ui/templates/base.html`; add browser scenario `tests/e2e/mcp.spec.ts`.

**Interfaces:** authenticated `GET /settings/mcp`, `POST /settings/mcp`, `POST /settings/mcp/{id}/revoke`. Create response shows raw token once; listing never returns secrets.

- [x] Write failing HTTP tests for session and CSRF checks, creation, one-time display, ownership and revocation.
- [x] Implement server-rendered forms using existing UI classes and page styles; provide MCP URL, expiry choices and clear success/error messages.
- [x] Verify HTTP tests; add a Playwright scenario that creates/revokes a key and uses it against real `/mcp`.

### Task 4: Portable stdio adapter

**Files:** create `internal/mcpbridge/bridge.go`, `internal/mcpbridge/bridge_test.go`, `cmd/kb-mcp/main.go`, `cmd/kb-mcp/main_test.go`; modify `Makefile`.

**Interfaces:** bridge configuration from `KB_MCP_URL`, `KB_MCP_TOKEN`; `Run(ctx,url,token,in,out) error` forwards SDK tools and results, with no application storage imports.

- [x] Write failing tests for URL validation, authentication header, rejected redirect, actual stdio child calls, EOF/cancellation, and secret-free diagnostics.
- [x] Implement SDK client/server bridge, bounded operations, safe transport and stderr-only diagnostics.
- [x] Run integration tests; cross-compile adapter for Windows/macOS/Linux and run it on the available platform.

### Task 5: Documentation, integration and final review

**Files:** create `docs/mcp.md`; modify `README.md`, `docs/operations.md`, plan checkboxes and design status as appropriate.

- [x] Document generic HTTP and stdio configuration, user key lifecycle, chunking, text-only output and build commands, with placeholders only.
- [x] Build UI, run complete Go tests/race checks and vet in Docker, run browser scenario and production Docker build. Keep temporary data under ignored `.validation` or container temporary directories.
- [x] Request independent code review of the complete diff. Address verified issues and repeat only affected checks.
- [x] Run `git diff --check`, inspect final status and summarize observed evidence, remaining platform limitations and instructions for connecting an agent. Do not deploy or push without an explicit request.

## Execution

The user approved the integrated MCP approach and chose personal keys after discussing alternatives. Execute natively in this session, using the already authorized scope; independent review follows implementation.
