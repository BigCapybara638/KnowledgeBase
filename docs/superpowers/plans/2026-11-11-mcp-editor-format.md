# MCP editor document format implementation plan

> **For agentic workers:** Use superpowers:executing-plans; execute natively and request one independent final review.

**Goal:** Let agents create formatted personal article drafts using the native editor JSON.

**Architecture:** Keep plain body input compatible, add a mutually exclusive document input with an explicit recursive schema, and validate its text/table subset before passing it to the existing transactional draft service.

**Tech Stack:** Existing Go/MCP SDK, Tiptap, Playwright; no new dependencies.

**Spec:** `docs/superpowers/specs/2026-11-11-mcp-editor-format-design.md`

## Global constraints

Keep existing uncommitted port/draft work. No live test records or changes to keys. Format support creates private drafts only. Plain input remains literal text. JSON ≤256 KiB, depth ≤32, nodes ≤10000; no video, images, merged cells or nested tables. Execute within the current isolated worktree. No automatic commit or push for this request.

## Review focus

Test both/neither body/document and null input; reject unsupported nodes nested in other blocks; ensure malformed tables are rejected rather than silently repaired; preserve links/colors through the editor; confirm old plain-text clients and read-only permissions still work.

### Task 1: Document validation and MCP contract

Files: new `internal/content/agent_document.go`, `agent_document_test.go`, new `internal/mcpserver/format_test.go`; modify `write.go`, `server.go`; new `write_schema.go`.

- [x] Add failing tests for formatted creation, schema discovery, mutually exclusive inputs, unsafe/unsupported documents without orphan rows, and content bounds.
- [x] Observe RED, implement `content.AgentDocument(json.RawMessage) (json.RawMessage,error)` and explicit MCP schema, then observe GREEN including previous plain/permission tests.

### Task 2: Editor round trip and delivery

Files: `tests/e2e/mcp.spec.ts`, `docs/mcp.md`, validation report and this plan.

- [x] Add browser scenario for styled text, lists, links, code and table opening, saving, reloading and publishing from an MCP-created draft.
- [ ] Run race tests/vet, frontend build and relevant browser checks; independently review the change and resolve material findings.
- [ ] Record evidence, back up and update the local service on 8081; verify readiness and MCP authentication without inserting live test data.
