package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"github.com/BigCapybara638/KnowledgeBase/internal/web"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("KB_MCP_TEST_PROCESS") != "1" {
		return
	}
	main()
	os.Exit(0)
}

func TestAdapterProcessAgainstApplication(t *testing.T) {
	for _, writable := range []bool{false, true} {
		t.Run(map[bool]string{false: "reader", true: "writer"}[writable], func(t *testing.T) { testAdapterProcess(t, writable) })
	}
}

func testAdapterProcess(t *testing.T, writable bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := testutil.Store(t)
	id := testutil.User(t, s, "user")
	a, err := auth.New(s, auth.Config{PublicURL: "https://kb.test"})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := a.CreateMCPTokenWithWrite(ctx, id, "Process", 90, writable)
	if err != nil {
		t.Fatal(err)
	}
	app, err := web.New(config.Config{PublicURL: "https://kb.test", DataDir: s.DataDir}, s, a)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app)
	defer srv.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, executable, "-test.run=^TestMCPHelperProcess$")
	command.Env = append(os.Environ(), "KB_MCP_TEST_PROCESS=1", "KB_MCP_URL="+srv.URL+"/mcp", "KB_MCP_TOKEN="+raw)
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "process-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("process connect: %v %s", err, stderr.String())
	}
	tools, err := session.ListTools(ctx, nil)
	want := 4
	if writable {
		want = 5
	}
	if err != nil || len(tools.Tools) != want {
		t.Fatalf("process tools: %v %v", tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_articles", Arguments: map[string]any{}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("process call: %+v %v", result, err)
	}
	if writable {
		for _, tool := range tools.Tools {
			if tool.Name == "create_article" && (tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint) {
				t.Fatal("bridge lost write annotations")
			}
		}
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "create_article", Arguments: map[string]any{"title": "Статья через stdio", "body": "Личный черновик 🚀"}})
		if err != nil || result.IsError {
			t.Fatalf("stdio create: %+v %v", result, err)
		}
		var created struct {
			DraftID string `json:"draft_id"`
		}
		if err = json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &created); err != nil {
			t.Fatal(err)
		}
		draft, err := articles.New(s).GetDraft(ctx, id, domain.ID(created.DraftID))
		if err != nil || draft.OwnerID != id || draft.Title != "Статья через stdio" {
			t.Fatalf("stdio draft: %+v %v", draft, err)
		}
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "create_article", Arguments: map[string]any{
			"title": "Оформление через stdio", "document": json.RawMessage(`{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Заголовок"}]},{"type":"paragraph","content":[{"type":"text","text":"Выделение","marks":[{"type":"bold"}]}]}]}`),
		}})
		if err != nil || result.IsError {
			t.Fatalf("stdio formatted create: %+v %v", result, err)
		}
		if err = json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &created); err != nil {
			t.Fatal(err)
		}
		draft, err = articles.New(s).GetDraft(ctx, id, domain.ID(created.DraftID))
		if err != nil {
			t.Fatal(err)
		}
		rendered, err := content.ValidateAndRender(draft.Document)
		if err != nil || !strings.Contains(string(rendered.HTML), "<h2>Заголовок</h2>") || !strings.Contains(string(rendered.HTML), "<strong>Выделение</strong>") {
			t.Fatal("stdio lost formatting", err)
		}
	}
	keys, err := a.ListMCPTokens(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.RevokeMCPToken(ctx, id, keys[0].ID); err != nil {
		t.Fatal(err)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "list_articles", Arguments: map[string]any{}})
	if err == nil && !result.IsError {
		t.Fatal("adapter bypassed key revocation")
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), raw) {
		t.Fatal("process logged secret")
	}
}
