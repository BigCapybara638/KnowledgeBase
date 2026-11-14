package mcpserver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPCreateArticleMakesPersonalDraft(t *testing.T) {
	for _, version := range []string{"", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			a, owner, raw, srv, _ := fixture(t)
			ctx := context.Background()
			keys, err := a.ListMCPTokens(ctx, owner)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.SetMCPTokenWrite(ctx, owner, keys[0].ID, true); err != nil {
				t.Fatal(err)
			}
			session := connect(t, srv.URL, raw, version)
			tools, err := session.ListTools(ctx, nil)
			if err != nil || len(tools.Tools) != 5 {
				t.Fatalf("write tools: %v %v", tools, err)
			}
			found := false
			for _, tool := range tools.Tools {
				if tool.Name == "create_article" {
					found = true
					if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
						t.Fatal("incorrect write tool annotations")
					}
				}
			}
			if !found {
				t.Fatal("missing create_article")
			}
			body := "Инструкция агента 🚀\n\nПроверьте настройки."
			result := call(t, session, "create_article", map[string]any{"title": "Статья агента", "body": body})
			if result["status"] != "draft" || result["edit_url"] != "https://kb.test/drafts/"+result["draft_id"].(string)+"/edit" {
				t.Fatalf("draft result: %+v", result)
			}
			svc := articles.New(a.Store)
			draft, err := svc.GetDraft(ctx, owner, domain.ID(result["draft_id"].(string)))
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := content.ValidateAndRender(draft.Document)
			if err != nil || rendered.PlainText != body || draft.OwnerID != owner {
				t.Fatal("draft content or owner mismatch")
			}
			if _, err = svc.Get(ctx, owner, draft.ArticleID); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("MCP published article")
			}
			other := testutil.User(t, a.Store, "user")
			if _, err = svc.GetDraft(ctx, other, draft.ID); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("foreign draft accessible")
			}
		})
	}
}

func TestMCPWritePermissionChangesApplyToExistingClient(t *testing.T) {
	a, owner, raw, srv, _ := fixture(t)
	ctx := context.Background()
	session := connect(t, srv.URL, raw, "")
	args := map[string]any{"title": "Статья", "body": "Текст"}
	denied := func() {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_article", Arguments: args})
		if err == nil && !result.IsError {
			t.Fatal("read-only key created draft")
		}
	}
	denied()
	keys, err := a.ListMCPTokens(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.SetMCPTokenWrite(ctx, owner, keys[0].ID, true); err != nil {
		t.Fatal(err)
	}
	call(t, session, "create_article", args)
	if err = a.SetMCPTokenWrite(ctx, owner, keys[0].ID, false); err != nil {
		t.Fatal(err)
	}
	denied()
	if err = a.SetMCPTokenWrite(ctx, owner, keys[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err = a.RevokeMCPToken(ctx, owner, keys[0].ID); err != nil {
		t.Fatal(err)
	}
	denied()
	drafts, err := articles.New(a.Store).Drafts(ctx, owner)
	if err != nil || len(drafts) != 1 {
		t.Fatalf("denied requests changed data: %d %v", len(drafts), err)
	}
}

func TestMCPCreateRejectsInvalidInputWithoutOrphans(t *testing.T) {
	a, owner, raw, srv, _ := fixture(t)
	ctx := context.Background()
	keys, err := a.ListMCPTokens(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.SetMCPTokenWrite(ctx, owner, keys[0].ID, true); err != nil {
		t.Fatal(err)
	}
	session := connect(t, srv.URL, raw, "")
	for _, args := range []map[string]any{
		{"title": "", "body": "Text"}, {"title": "Valid", "body": "  "}, {"title": strings.Repeat("я", 257), "body": "Text"},
		{"title": "Valid", "body": strings.Repeat("x", (256<<10)+1)}, {"title": "Valid", "body": "Text", "category_id": "bad"},
		{"title": "Valid", "body": "Text", "category_id": domain.NewID()},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "create_article", Arguments: args})
		if err == nil && !result.IsError {
			t.Fatal("invalid draft accepted")
		}
	}
	drafts, err := articles.New(a.Store).Drafts(ctx, owner)
	if err != nil || len(drafts) != 0 {
		t.Fatal("invalid calls left drafts")
	}
	var count int
	if err = a.Store.Read.QueryRow("SELECT count(*) FROM articles").Scan(&count); err != nil || count != 3 {
		t.Fatal("invalid calls left article placeholders")
	}
}
