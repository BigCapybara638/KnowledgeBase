package mcpserver

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/categories"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/search"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type searchInput struct {
	Query string `json:"query" jsonschema:"Search words in article titles and full text; empty lists recent articles. Maximum 256 characters."`
	Page  int    `json:"page,omitempty" jsonschema:"Page number, starting at 1."`
}
type listInput struct {
	CategoryID string `json:"category_id,omitempty" jsonschema:"Optional category UUID from list_categories."`
	Page       int    `json:"page,omitempty"`
}
type articleInput struct {
	ID     string `json:"id" jsonschema:"Article UUID returned by search_articles or list_articles."`
	Offset int    `json:"offset,omitempty" jsonschema:"Zero-based Unicode character offset; continue with the returned next_offset."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Text characters to return; default 12000, maximum 20000."`
}
type categoryInput struct {
	Page int `json:"page,omitempty"`
}
type card struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
	Category string `json:"category"`
	Author   string `json:"author"`
	URL      string `json:"url"`
}
type articleList struct {
	Items   []card `json:"items"`
	Page    int    `json:"page"`
	HasNext bool   `json:"has_next"`
}
type articleText struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Category        string `json:"category"`
	Author          string `json:"author"`
	LastEditor      string `json:"last_editor"`
	Version         int64  `json:"version"`
	PublishedAt     int64  `json:"published_at"`
	UpdatedAt       int64  `json:"updated_at"`
	URL             string `json:"url"`
	Text            string `json:"text"`
	Offset          int    `json:"offset"`
	TotalCharacters int    `json:"total_characters"`
	NextOffset      *int   `json:"next_offset"`
}
type category struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Order int    `json:"order"`
}
type categoryList struct {
	Items   []category `json:"items"`
	Page    int        `json:"page"`
	HasNext bool       `json:"has_next"`
}

func tool(name, description string) *mcp.Tool {
	no := false
	return &mcp.Tool{Name: name, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &no, OpenWorldHint: &no}}
}
func page(n int) (int, error) {
	if n == 0 {
		return 1, nil
	}
	if n < 1 || n > 100000 {
		return 0, domain.ErrInvalid
	}
	return n, nil
}
func safeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrInvalid):
		return errors.New("Invalid arguments: check IDs, page, query length, offset and limit.")
	case errors.Is(err, domain.ErrNotFound):
		return errors.New("Published article not found.")
	case errors.Is(err, domain.ErrForbidden):
		return errors.New("Access denied.")
	default:
		return errors.New("Knowledge base temporarily unavailable. Try again later.")
	}
}
func snippet(s string) string {
	r := []rune(s)
	if len(r) > 240 {
		return string(r[:240]) + "…"
	}
	return s
}

func registerTools(server *mcp.Server, cfg config.Config, s *storage.Store) {
	reads := articles.New(s)
	find := search.New(s)
	cats := categories.New(s)
	url := func(id domain.ID) string { return strings.TrimRight(cfg.PublicURL, "/") + "/articles/" + string(id) }
	list := func(ctx context.Context, categoryID string, p int) (articleList, error) {
		var cat *domain.ID
		if categoryID != "" {
			if _, err := uuid.Parse(categoryID); err != nil {
				return articleList{}, domain.ErrInvalid
			}
			id := domain.ID(categoryID)
			cat = &id
		}
		items, next, err := reads.ListSummaries(ctx, actor(ctx), cat, p)
		result := articleList{Items: []card{}, Page: p, HasNext: next}
		for _, item := range items {
			result.Items = append(result.Items, card{ID: string(item.ID), Title: item.Title, Snippet: item.Snippet, Category: item.Category, Author: item.Author, URL: url(item.ID)})
		}
		return result, err
	}
	mcp.AddTool(server, tool("search_articles", "Search published knowledge-base articles by words. Returns snippets and source URLs; use get_article for the full text."), func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, articleList, error) {
		if !utf8.ValidString(in.Query) || utf8.RuneCountInString(in.Query) > 256 {
			return nil, articleList{}, safeError(domain.ErrInvalid)
		}
		p, err := page(in.Page)
		if err != nil {
			return nil, articleList{}, safeError(err)
		}
		if !strings.ContainsFunc(in.Query, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			result, err := list(ctx, "", p)
			return nil, result, safeError(err)
		}
		found, err := find.Find(ctx, actor(ctx), in.Query, p)
		result := articleList{Items: []card{}, Page: p, HasNext: found.HasNext}
		for _, item := range found.Items {
			result.Items = append(result.Items, card{ID: string(item.ID), Title: item.Title, Snippet: snippet(item.Snippet), Category: item.Category, Author: item.Author, URL: url(item.ID)})
		}
		return nil, result, safeError(err)
	})
	mcp.AddTool(server, tool("list_articles", "List recent published articles, optionally filtered by category. Returns up to 20 metadata cards per page."), func(ctx context.Context, _ *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, articleList, error) {
		p, err := page(in.Page)
		if err != nil {
			return nil, articleList{}, safeError(err)
		}
		result, err := list(ctx, in.CategoryID, p)
		return nil, result, safeError(err)
	})
	mcp.AddTool(server, tool("get_article", "Read a published article's plain text and source URL. Follow next_offset for remaining text. Rich formatting and media require opening the original website."), func(ctx context.Context, _ *mcp.CallToolRequest, in articleInput) (*mcp.CallToolResult, articleText, error) {
		if _, err := uuid.Parse(in.ID); err != nil || in.Offset < 0 || in.Limit < 0 || in.Limit > 20000 {
			return nil, articleText{}, safeError(domain.ErrInvalid)
		}
		if in.Limit == 0 {
			in.Limit = 12000
		}
		a, err := reads.Get(ctx, actor(ctx), domain.ID(in.ID))
		if err != nil {
			return nil, articleText{}, safeError(err)
		}
		text := []rune(a.PlainText)
		start := min(in.Offset, len(text))
		end := start + min(in.Limit, len(text)-start)
		result := articleText{ID: string(a.ID), Title: a.Title, Category: a.CategoryTitle, Author: a.AuthorName, LastEditor: a.EditorName, Version: int64(a.Version), PublishedAt: a.PublishedAt, UpdatedAt: a.UpdatedAt, URL: url(a.ID), Text: string(text[start:end]), Offset: in.Offset, TotalCharacters: len(text)}
		if end < len(text) {
			result.NextOffset = &end
		}
		return nil, result, nil
	})
	mcp.AddTool(server, tool("list_categories", "List knowledge-base categories, up to 100 per page. Use a category ID with list_articles."), func(ctx context.Context, _ *mcp.CallToolRequest, in categoryInput) (*mcp.CallToolResult, categoryList, error) {
		p, err := page(in.Page)
		if err != nil {
			return nil, categoryList{}, safeError(err)
		}
		items, next, err := cats.ListPage(ctx, actor(ctx), p)
		result := categoryList{Items: []category{}, Page: p, HasNext: next}
		for _, item := range items {
			result.Items = append(result.Items, category{ID: string(item.ID), Title: item.Title, Order: item.Order})
		}
		return nil, result, safeError(err)
	})
}
