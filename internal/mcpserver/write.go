package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type createArticleInput struct {
	Title      string          `json:"title" jsonschema:"Non-empty article title, up to 512 UTF-8 bytes."`
	Body       *string         `json:"body,omitempty"`
	Document   json.RawMessage `json:"document,omitempty"`
	CategoryID string          `json:"category_id,omitempty" jsonschema:"Optional existing category UUID from list_categories. Can be chosen later in the editor."`
}
type createdDraft struct {
	DraftID   string `json:"draft_id"`
	ArticleID string `json:"article_id"`
	Title     string `json:"title"`
	Version   int64  `json:"version"`
	Status    string `json:"status"`
	EditURL   string `json:"edit_url"`
}

func createError(err error) error {
	if errors.Is(err, domain.ErrInvalid) {
		return errors.New("Invalid draft: provide a nonblank title (up to 512 UTF-8 bytes), exactly one of body (plain text) or document (Tiptap text/table JSON), each up to 256 KiB, and optionally an existing category UUID. Use headings 1–3 and rectangular tables without merged cells; media, nested tables and HTML nodes are unsupported.")
	}
	return safeError(err)
}

func registerWriteTool(server *mcp.Server, cfg config.Config, s *storage.Store) {
	no := false
	definition := &mcp.Tool{Name: "create_article", InputSchema: createArticleSchema, Description: "Write a new article as a personal draft. Send exactly one of plain body or native Tiptap document for formatting: headings 1–3, text marks, safe links, colors, lists, quotes, code and simple rectangular tables. No media or attachments. Does not publish or change existing articles. Returns an editor link for review and publication. Repeated calls create separate drafts.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &no, OpenWorldHint: &no}}
	mcp.AddTool(server, definition, func(ctx context.Context, _ *mcp.CallToolRequest, in createArticleInput) (*mcp.CallToolResult, createdDraft, error) {
		if !canWrite(ctx) {
			return nil, createdDraft{}, safeError(domain.ErrForbidden)
		}
		if in.CategoryID != "" {
			if _, err := uuid.Parse(in.CategoryID); err != nil {
				return nil, createdDraft{}, createError(domain.ErrInvalid)
			}
		}
		if (in.Body != nil) == (len(in.Document) != 0) {
			return nil, createdDraft{}, createError(domain.ErrInvalid)
		}
		var doc json.RawMessage
		var err error
		if in.Body != nil {
			doc, err = content.PlainDocument(*in.Body)
		} else {
			doc, err = content.AgentDocument(in.Document)
		}
		if err != nil {
			return nil, createdDraft{}, createError(err)
		}
		draft, err := articles.New(s).CreateFilledDraft(ctx, actor(ctx), articles.CreateDraftInput{Title: in.Title, CategoryID: domain.ID(in.CategoryID), Document: doc})
		if err != nil {
			return nil, createdDraft{}, createError(err)
		}
		result := createdDraft{DraftID: string(draft.ID), ArticleID: string(draft.ArticleID), Title: draft.Title, Version: int64(draft.Version), Status: "draft", EditURL: strings.TrimRight(cfg.PublicURL, "/") + "/drafts/" + string(draft.ID) + "/edit"}
		return nil, result, nil
	})
}
