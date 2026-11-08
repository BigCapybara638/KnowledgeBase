package web

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
)

func TestMCPPageAndKeyLifecycle(t *testing.T) {
	h, a, u := authFixture(t)
	ctx := context.Background()
	l, err := a.Login(ctx, u.Login, testutil.Password)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest("GET", "https://kb.test/settings/mcp", nil)
	get.AddCookie(&http.Cookie{Name: "kb_session", Value: l.RawToken})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://kb.test/mcp") {
		t.Fatalf("settings: %d %s", w.Code, w.Body.String())
	}
	m := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
	if len(m) != 2 {
		t.Fatal("no CSRF token")
	}
	cookies := w.Result().Cookies()
	post := func(path string, values url.Values, csrf bool) *httptest.ResponseRecorder {
		if csrf {
			values.Set("csrf_token", html.UnescapeString(m[1]))
		}
		r := httptest.NewRequest("POST", "https://kb.test"+path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://kb.test")
		r.Header.Set("Referer", "https://kb.test/settings/mcp")
		r.AddCookie(&http.Cookie{Name: "kb_session", Value: l.RawToken})
		for _, c := range cookies {
			r.AddCookie(c)
		}
		result := httptest.NewRecorder()
		h.ServeHTTP(result, r)
		return result
	}
	if got := post("/settings/mcp", url.Values{"name": {"Agent"}, "days": {"90"}}, false); got.Code != 403 {
		t.Fatalf("missing CSRF: %d", got.Code)
	}
	created := post("/settings/mcp", url.Values{"name": {"Мой агент"}, "days": {"90"}}, true)
	raw := regexp.MustCompile(`kb_mcp_[A-Za-z0-9_-]{43}`).FindString(created.Body.String())
	if created.Code != 200 || raw == "" || created.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 || strings.Contains(w.Body.String(), raw) || !strings.Contains(w.Body.String(), "Мой агент") {
		t.Fatal("secret visible again or key missing")
	}
	keys, err := a.ListMCPTokens(ctx, u.ID)
	if err != nil || len(keys) != 1 {
		t.Fatal(err)
	}
	other := testutil.User(t, a.Store, "user")
	foreign, _, err := a.CreateMCPToken(ctx, other, "Foreign", 90)
	if err != nil {
		t.Fatal(err)
	}
	if got := post("/settings/mcp/"+string(foreign.ID)+"/revoke", url.Values{}, true); got.Code != 404 {
		t.Fatalf("foreign key revoke: %d", got.Code)
	}
	if got := post("/settings/mcp/"+string(keys[0].ID)+"/revoke", url.Values{}, true); got.Code != 303 {
		t.Fatalf("revoke: %d", got.Code)
	}
	if _, err := a.ResolveMCPToken(ctx, raw); err == nil {
		t.Fatal("revoked key accepted")
	}
}

func TestMCPRouteDoesNotUseBrowserCredentials(t *testing.T) {
	h, a, u := authFixture(t)
	l, err := a.Login(context.Background(), u.Login, testutil.Password)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://kb.test/mcp", strings.NewReader(`{}`))
	r.AddCookie(&http.Cookie{Name: "kb_session", Value: l.RawToken})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" || w.Header().Get("Location") != "" {
		t.Fatalf("MCP browser bypass: %d", w.Code)
	}
	get := httptest.NewRequest("GET", "https://kb.test/settings/mcp", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 303 {
		t.Fatalf("anonymous settings page: %d", w.Code)
	}
}
