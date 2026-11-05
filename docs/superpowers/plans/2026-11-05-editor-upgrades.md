# Knowledge base editor upgrades

**Goal:** Apply the requested Axentris branding, dark appearance and richer article editing in feat/knowledge-base, then open a PR into main.

**Architecture:** Keep Go/SQLite and Tiptap. Extend the structured document schema on both client and server, keeping publication validation. Import DOCX locally with bounded ZIP/XML parsing; sanitize through Tiptap and the server renderer. Self-host Inter. Video uses HTTP(S) file links rendered as native video; Word embedded objects without playable sources are reported.

**Execution:** User authorized autonomous implementation, followed by a final push and PR. Use the existing isolated checkout, no additional worktree.

- [x] Appearance: dark palette, local Inter, Axentris image, expanded sidebar, equal field heights; build and browser smoke checks.
- [x] Passwords: regression test for eight Unicode characters, reject seven and >72 bytes; implement server and form limits; run auth tests.
- [x] Rich document: failing rendering/security tests for text/background color, table widths/heights and videos; implement validated attributes and rendering; run content tests.
- [x] Editor: matching extensions and controls, sticky toolbar, merge/split cells and dimensions; DOCX upload/drop with styled text, tables and images; paste retains supported HTML formatting.
- [x] Validation: real DOCX fixture, publish/reopen browser checks, all Go tests and frontend build; independent review and document limitations. Delivery: commit/push feat/knowledge-base and request PR into main.

## Review focus
Untrusted CSS and URLs must not become executable HTML. DOCX archive and expanded size must be bounded. Imported images must retain draft authorization. Toolbar must remain visible during scrolling. Formatting must survive publication and reopening; unsupported Word media must not silently disappear.
