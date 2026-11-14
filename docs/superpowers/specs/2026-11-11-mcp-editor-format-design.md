# MCP editor document format

The user selected the editor's document format and excluded video and complex features. Extend `create_article` to accept exactly one of the existing plain `body` or a `document` object in the editor's Tiptap JSON format. Keep personal draft ownership, write permission, publication workflow, and existing clients unchanged.

Support paragraphs, headings 1–3, hard breaks, bold/italic/underline/strike/code, safe links, text colors/backgrounds, bullet/numbered lists, blockquotes, code blocks, horizontal rules, and rectangular tables with header cells and basic cell formatting. Exclude media, attachments, arbitrary HTML/custom nodes, merged cells, and nested tables. Validate structure before creating any database rows; do not silently remove unsupported content. Require nonblank text. Limit document JSON to 256 KiB, nesting to 32 and nodes to 10000, reusing the existing renderer's safety checks.

Advertise the recursive document structure in the MCP input schema and explain supported attributes in the tool and documentation. New agents discover it through `tools/list`; existing plain-text calls still work. No schema migration or new dependencies. Verify the HTTP tool, permissions, unsafe/unsupported inputs, and editor save/publish round trip on temporary data before updating the local service on 8081.
