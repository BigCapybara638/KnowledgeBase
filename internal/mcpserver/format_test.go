package mcpserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func formattedDocument() map[string]any {
	var doc map[string]any
	if err := json.Unmarshal([]byte(`{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Настройка 🚀"}]},{"type":"paragraph","content":[{"type":"text","text":"Важно","marks":[{"type":"bold"},{"type":"textStyle","attrs":{"color":"#123456","backgroundColor":"#ffeecc"}}]},{"type":"text","text":"ссылка","marks":[{"type":"link","attrs":{"href":"https://example.com/help"}}]}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Шаг один"}]}]}]},{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"Название"}]}]},{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"Значение"}]}]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"Порт"}]}]},{"type":"tableCell","attrs":{"align":"center","backgroundColor":"#ffeecc"},"content":[{"type":"paragraph","content":[{"type":"text","text":"8081"}]}]}]}]}]}`), &doc); err != nil {
		panic(err)
	}
	return doc
}

func TestMCPCreateEditorDocument(t *testing.T) {
	a, owner, raw, srv, _ := fixture(t)
	ctx := context.Background()
	keys, _ := a.ListMCPTokens(ctx, owner)
	if err := a.SetMCPTokenWrite(ctx, owner, keys[0].ID, true); err != nil {
		t.Fatal(err)
	}
	session := connect(t, srv.URL, raw, "2025-11-25")
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "create_article" {
			b, _ := json.Marshal(tool.InputSchema)
			if !strings.Contains(string(b), `"document"`) || !strings.Contains(string(b), `"tableRow"`) {
				t.Fatal("schema does not explain editor documents")
			}
		}
	}
	result := call(t, session, "create_article", map[string]any{"title": "Форматированная статья", "document": formattedDocument()})
	draft, err := articles.New(a.Store).GetDraft(ctx, owner, domain.ID(result["draft_id"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := content.ValidateAndRender(draft.Document)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<h2>", "<strong>", "color:#123456", "https://example.com/help", "<ul>", "<table", "<th", "<td", "8081"} {
		if !strings.Contains(string(rendered.HTML), fragment) {
			t.Fatalf("lost %s: %s", fragment, rendered.HTML)
		}
	}
	if result["status"] != "draft" || draft.OwnerID != owner {
		t.Fatal("incorrect draft ownership/status")
	}
}

func TestMCPRejectsAmbiguousOrUnsupportedEditorDocuments(t *testing.T) {
	a, owner, raw, srv, _ := fixture(t)
	ctx := context.Background()
	keys, _ := a.ListMCPTokens(ctx, owner)
	if err := a.SetMCPTokenWrite(ctx, owner, keys[0].ID, true); err != nil {
		t.Fatal(err)
	}
	session := connect(t, srv.URL, raw, "")
	decode := func(raw string) any {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	for i, args := range []map[string]any{
		{"title": "Title"}, {"title": "Title", "document": nil},
		{"title": "Title", "body": "Text", "document": formattedDocument()},
		{"title": "Title", "body": "", "document": formattedDocument()},
		{"title": "Title", "body": nil, "document": formattedDocument()},
		{"title": "Title", "document": decode(`{"type":"doc","content":[{"type":"blockquote","content":[{"type":"video","attrs":{"src":"https://example.com/video.mp4"}}]}]}`)},
		{"title": "Title", "document": decode(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`)},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_article", Arguments: args})
		if err == nil && !result.IsError {
			t.Fatalf("accepted invalid input %d", i)
		}
	}
	var count int
	if err := a.Store.Read.QueryRow("SELECT count(*) FROM articles").Scan(&count); err != nil || count != 3 {
		t.Fatalf("invalid documents left placeholders: %d %v", count, err)
	}
	drafts, err := articles.New(a.Store).Drafts(ctx, owner)
	if err != nil || len(drafts) != 0 {
		t.Fatal("invalid documents left drafts")
	}
}
