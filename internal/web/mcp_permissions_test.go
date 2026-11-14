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

func mcpSettingsPost(t *testing.T, h http.Handler, session, path string, values url.Values, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	get := httptest.NewRequest("GET", "https://kb.test/settings/mcp", nil)
	get.AddCookie(&http.Cookie{Name: "kb_session", Value: session})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 {
		t.Fatalf("settings page: %d", w.Code)
	}
	if csrf {
		m := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
		if len(m) != 2 {
			t.Fatal("missing CSRF")
		}
		values.Set("csrf_token", html.UnescapeString(m[1]))
	}
	r := httptest.NewRequest("POST", "https://kb.test"+path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://kb.test")
	r.Header.Set("Referer", "https://kb.test/settings/mcp")
	r.AddCookie(&http.Cookie{Name: "kb_session", Value: session})
	for _, cookie := range w.Result().Cookies() {
		r.AddCookie(cookie)
	}
	result := httptest.NewRecorder()
	h.ServeHTTP(result, r)
	return result
}

func TestMCPBrowserPermissionChangesNeedOwnerAndCSRF(t *testing.T) {
	h, a, u := authFixture(t)
	ctx := context.Background()
	l, err := a.Login(ctx, u.Login, testutil.Password)
	if err != nil {
		t.Fatal(err)
	}
	created := mcpSettingsPost(t, h, l.RawToken, "/settings/mcp", url.Values{"name": {"Writer"}, "days": {"90"}, "can_write": {"true"}}, true)
	if created.Code != 200 {
		t.Fatalf("create: %d", created.Code)
	}
	raw := regexp.MustCompile(`kb_mcp_[A-Za-z0-9_-]{43}`).FindString(created.Body.String())
	access, err := a.ResolveMCPAccess(ctx, raw)
	if err != nil || !access.CanWrite {
		t.Fatal("create rights ignored")
	}
	keys, err := a.ListMCPTokens(ctx, u.ID)
	if err != nil || len(keys) != 1 {
		t.Fatal(err)
	}
	path := "/settings/mcp/" + string(keys[0].ID) + "/permissions"
	if got := mcpSettingsPost(t, h, l.RawToken, path, url.Values{"can_write": {"false"}}, false); got.Code != 403 {
		t.Fatalf("CSRF bypass: %d", got.Code)
	}
	if got := mcpSettingsPost(t, h, l.RawToken, path, url.Values{"can_write": {"invalid"}}, true); got.Code != 400 {
		t.Fatalf("invalid permission: %d", got.Code)
	}
	if got := mcpSettingsPost(t, h, l.RawToken, path, url.Values{"can_write": {"false"}}, true); got.Code != 303 {
		t.Fatalf("permission update: %d", got.Code)
	}
	access, err = a.ResolveMCPAccess(ctx, raw)
	if err != nil || access.CanWrite {
		t.Fatal("permission stayed enabled")
	}
	other := testutil.User(t, a.Store, "user")
	foreign, _, err := a.CreateMCPToken(ctx, other, "Other", 90)
	if err != nil {
		t.Fatal(err)
	}
	if got := mcpSettingsPost(t, h, l.RawToken, "/settings/mcp/"+string(foreign.ID)+"/permissions", url.Values{"can_write": {"true"}}, true); got.Code != 404 {
		t.Fatalf("foreign permission updated: %d", got.Code)
	}
}
