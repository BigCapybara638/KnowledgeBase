package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/mcpserver"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type keyTransport struct {
	token string
	base  http.RoundTripper
}

func (t keyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}

func connect(t *testing.T, endpoint, raw, version string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "integration", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: &http.Client{Transport: keyTransport{raw, http.DefaultTransport}}}, &mcp.ClientSessionOptions{ProtocolVersion: version})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func fixture(t *testing.T) (*auth.Service, domain.ID, string, *httptest.Server, []domain.ID) {
	t.Helper()
	ctx := context.Background()
	s := testutil.Store(t)
	actor := testutil.User(t, s, "user")
	a, err := auth.New(s, auth.Config{PublicURL: "https://kb.test"})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := a.CreateMCPToken(ctx, actor, "Integration", 90)
	if err != nil {
		t.Fatal(err)
	}
	cat := domain.NewID()
	if _, err = s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,?)", cat, "Инструкции"); err != nil {
		t.Fatal(err)
	}
	ids := []domain.ID{domain.NewID(), domain.NewID(), domain.NewID()}
	for i, id := range ids {
		var published any = 1
		archived := false
		if i == 1 {
			archived = true
		}
		if i == 2 {
			published = nil
		}
		if _, err = s.Write.Exec("INSERT INTO articles(id,category_id,author_id,last_editor_id,title,plain_text,published_at,updated_at,archived) VALUES(?,?,?,?,?,?,?,?,?)", id, cat, actor, actor, "Настройка агента", strings.Repeat("Привет 🚀 мир! ", 2000), published, 1, archived); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Write.Exec("INSERT INTO articles_fts(article_id,title,body) VALUES(?,?,?)", id, "Настройка агента", "Привет мир"); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(mcpserver.New(config.Config{PublicURL: "https://kb.test"}, s, a))
	t.Cleanup(srv.Close)
	return a, actor, raw, srv, ids
}

func call(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("tool %s: %+v", name, r.Content)
	}
	if r.StructuredContent == nil || len(r.Content) == 0 {
		t.Fatal("missing structured or text result")
	}
	var data map[string]any
	if err = json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMCPReadToolsAndLegacy(t *testing.T) {
	for _, version := range []string{"", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			_, _, raw, srv, ids := fixture(t)
			session := connect(t, srv.URL, raw, version)
			tools, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(tools.Tools) != 4 {
				t.Fatalf("tools: %d", len(tools.Tools))
			}
			for _, tool := range tools.Tools {
				if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
					t.Fatal("tool missing read-only annotation")
				}
			}
			for _, query := range []string{"Привет", ""} {
				r := call(t, session, "search_articles", map[string]any{"query": query})
				items := r["items"].([]any)
				if len(items) != 1 || items[0].(map[string]any)["id"] != string(ids[0]) {
					t.Fatal("search exposed hidden article")
				}
			}
			listed := call(t, session, "list_articles", map[string]any{})["items"].([]any)
			if len(listed) != 1 || len(listed[0].(map[string]any)["snippet"].(string)) > 1000 {
				t.Fatal("unbounded or hidden list output")
			}
			categories := call(t, session, "list_categories", map[string]any{})["items"].([]any)
			if len(categories) != 1 {
				t.Fatal("missing category")
			}
			part := call(t, session, "get_article", map[string]any{"id": ids[0], "limit": 8})
			if part["text"] != "Привет 🚀" || part["next_offset"] != float64(8) || part["url"] != "https://kb.test/articles/"+string(ids[0]) {
				t.Fatalf("chunk: %+v", part)
			}
			part = call(t, session, "get_article", map[string]any{"id": ids[0], "offset": 30000})
			if part["text"] != "" || part["next_offset"] != nil {
				t.Fatal("end-of-text chunk")
			}
			for _, id := range ids[1:] {
				r, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_article", Arguments: map[string]any{"id": id}})
				if err == nil && !r.IsError {
					t.Fatal("hidden article fetched")
				}
			}
			for name, args := range map[string]map[string]any{"list_articles": {"page": -1}, "get_article": {"id": ids[0], "limit": 20001}, "list_categories": {"page": 100001}, "search_articles": {"query": strings.Repeat("я", 257)}} {
				r, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
				if err == nil && !r.IsError {
					t.Fatalf("invalid args accepted: %s", name)
				}
			}
		})
	}
}

func TestMCPAuthOriginAndRevocation(t *testing.T) {
	a, actor, raw, srv, _ := fixture(t)
	for _, tc := range []struct {
		token, origin string
		want          int
	}{{"", "", 401}, {"bad", "", 401}, {raw, "https://evil.test", 403}} {
		r, _ := http.NewRequest("POST", srv.URL, strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		r.AddCookie(&http.Cookie{Name: "kb_session", Value: raw})
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want || resp.Header.Get("Location") != "" {
			t.Fatalf("auth: %d", resp.StatusCode)
		}
	}
	session := connect(t, srv.URL, raw, "")
	keys, err := a.ListMCPTokens(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.RevokeMCPToken(context.Background(), actor, keys[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = session.ListTools(context.Background(), nil); err == nil {
		t.Fatal("existing client survived revocation")
	}
}

func TestMCPRejectsOversizedWhitespaceQuery(t *testing.T) {
	_, _, raw, srv, _ := fixture(t)
	session := connect(t, srv.URL, raw, "")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_articles", Arguments: map[string]any{"query": strings.Repeat(" ", 257)}})
	if err == nil && !result.IsError {
		t.Fatal("query length check bypassed by whitespace")
	}
}

func TestMCPQueriesWithoutWordsUseRecentArticleCards(t *testing.T) {
	_, _, raw, srv, _ := fixture(t)
	session := connect(t, srv.URL, raw, "")
	recent := call(t, session, "search_articles", map[string]any{"query": ""})
	for _, query := range []string{"!!!", "🚀", " — "} {
		got := call(t, session, "search_articles", map[string]any{"query": query})
		if !reflect.DeepEqual(got, recent) {
			t.Fatalf("query %q should return the same recent-article cards as an empty query", query)
		}
	}
}

func TestMCPUnicodeTextReconstructsAcrossChunks(t *testing.T) {
	_, _, raw, srv, ids := fixture(t)
	session := connect(t, srv.URL, raw, "")
	var text strings.Builder
	offset := 0
	for count := 0; count < 10; count++ {
		part := call(t, session, "get_article", map[string]any{"id": ids[0], "offset": offset, "limit": 12345})
		text.WriteString(part["text"].(string))
		next := part["next_offset"]
		if next == nil {
			break
		}
		if int(next.(float64)) <= offset {
			t.Fatal("chunk cursor did not advance")
		}
		offset = int(next.(float64))
	}
	if text.String() != strings.Repeat("Привет 🚀 мир! ", 2000) {
		t.Fatal("chunked text lost or duplicated Unicode characters")
	}
}

func TestMCPConcurrentClientsKeepIndependentKeys(t *testing.T) {
	a, actor, raw, srv, ids := fixture(t)
	other := testutil.User(t, a.Store, "user")
	_, otherRaw, err := a.CreateMCPToken(context.Background(), other, "Other", 90)
	if err != nil {
		t.Fatal(err)
	}
	sessions := []*mcp.ClientSession{connect(t, srv.URL, raw, ""), connect(t, srv.URL, otherRaw, "")}
	run := func(revoked bool) {
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for index, session := range sessions {
			wg.Go(func() {
				for range 10 {
					result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_article", Arguments: map[string]any{"id": ids[0], "limit": 8}})
					denied := err != nil || (result != nil && result.IsError)
					if denied != (revoked && index == 0) {
						errs <- fmt.Errorf("client %d denied=%v revoked=%v", index, denied, revoked)
						return
					}
				}
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
	}
	run(false)
	keys, err := a.ListMCPTokens(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.RevokeMCPToken(context.Background(), actor, keys[0].ID); err != nil {
		t.Fatal(err)
	}
	run(true)
}
