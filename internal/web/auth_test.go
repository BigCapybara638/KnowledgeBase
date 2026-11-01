package web

import (
	"context"
	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func authFixture(t *testing.T) (*App, *auth.Service, domain.User) {
	t.Helper()
	s := testutil.Store(t)
	id := testutil.User(t, s, "user")
	u, _ := s.User(context.Background(), id)
	a, e := auth.New(s, auth.Config{PublicURL: "https://kb.test"})
	if e != nil {
		t.Fatal(e)
	}
	h, e := New(config.Config{PublicURL: "https://kb.test", DataDir: s.DataDir}, s, a)
	if e != nil {
		t.Fatal(e)
	}
	return h, a, u
}
func loginRequest(t *testing.T, h http.Handler, u domain.User, ret, origin string) *httptest.ResponseRecorder {
	t.Helper()
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest("GET", "https://kb.test/login", nil))
	m := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(get.Body.String())
	if len(m) != 2 {
		t.Fatalf("no CSRF token in %d %s", get.Code, get.Body.String())
	}
	f := url.Values{"login": {u.Login}, "password": {testutil.Password}, "return_to": {ret}, "csrf_token": {html.UnescapeString(m[1])}}
	r := httptest.NewRequest("POST", "https://kb.test/login", strings.NewReader(f.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", origin)
	r.Header.Set("Referer", "https://kb.test/login")
	for _, c := range get.Result().Cookies() {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestRejectsExternalReturnTo(t *testing.T) {
	h, _, u := authFixture(t)
	for _, ret := range []string{"https://evil.test/", "//evil.test/", "/\\evil.test", "/%2f%2fevil.test"} {
		w := loginRequest(t, h, u, ret, "https://kb.test")
		if w.Code != 303 || w.Header().Get("Location") != "/" {
			t.Fatalf("unsafe redirect %q: %d %q", ret, w.Code, w.Header().Get("Location"))
		}
	}
}
func TestUnsafeRequestsRequireCSRF(t *testing.T) {
	h, _, _ := authFixture(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://kb.test/login", strings.NewReader("login=foo&password=bad"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("CSRF bypass %d", w.Code)
	}
}
func TestContentDeniedBeforePasswordChange(t *testing.T) {
	h, a, u := authFixture(t)
	a.Store.Write.Exec("UPDATE users SET must_change=1 WHERE id=?", u.ID)
	l, e := a.Login(context.Background(), u.Login, testutil.Password)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("GET", "https://kb.test/", nil)
	r.AddCookie(&http.Cookie{Name: "kb_session", Value: l.RawToken})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 303 || w.Header().Get("Location") != "/password" {
		t.Fatalf("password gate bypass: %d", w.Code)
	}
}
func TestForgedProxyHeadersIgnored(t *testing.T) {
	h, _, u := authFixture(t)
	w := loginRequest(t, h, u, "/", "https://evil.test")
	if w.Code != 403 {
		t.Fatalf("hostile Origin accepted: %d", w.Code)
	}
	r := httptest.NewRequest("POST", "https://kb.test/login", nil)
	r.Header.Set("X-Forwarded-Host", "evil.test")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("forged proxy headers bypassed Origin: %d", w.Code)
	}
}
