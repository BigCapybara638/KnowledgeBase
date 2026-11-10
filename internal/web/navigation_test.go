package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func navigationFixture(t *testing.T, count int) (*App, func(string) *httptest.ResponseRecorder) {
	t.Helper()
	a, accounts, user := authFixture(t)
	if _, err := a.store.Write.Exec("INSERT INTO categories(id,title) VALUES('instructions','Инструкции')"); err != nil {
		t.Fatal(err)
	}
	for i := range count {
		_, err := a.store.Write.Exec(`INSERT INTO articles(id,category_id,author_id,last_editor_id,title,document,plain_text,version,published_at,updated_at) VALUES(?,'instructions',?,?,?,?,'Содержание',1,1,1)`, fmt.Sprintf("article-%03d", i), user.ID, user.ID, fmt.Sprintf("Article %03d", i), `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Содержание"}]}]}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	session, err := accounts.Login(context.Background(), user.Login, "Testing-password-123!")
	if err != nil {
		t.Fatal(err)
	}
	return a, func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "https://kb.test"+path, nil)
		r.AddCookie(&http.Cookie{Name: "kb_session", Value: session.RawToken})
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
}

func TestSidebarCurrentArticle(t *testing.T) {
	_, get := navigationFixture(t, 105)
	page := get("/articles/article-054")
	if page.Code != 200 || !strings.Contains(page.Body.String(), "current=article-054") {
		t.Fatalf("article must pass its ID to the lazy menu: %d %s", page.Code, page.Body.String())
	}
	menu := get("/categories/instructions/articles?current=article-054")
	active := regexp.MustCompile(`<a[^>]*href="/articles/article-054"[^>]*aria-current="page"`)
	if menu.Code != 200 || !active.MatchString(menu.Body.String()) {
		t.Fatalf("current article beyond first 50 must be visible and marked: %d %s", menu.Code, menu.Body.String())
	}
	if !strings.Contains(menu.Body.String(), "cursor=100") || !strings.Contains(menu.Body.String(), "current=article-054") {
		t.Fatalf("load more must preserve active article: %s", menu.Body.String())
	}
	next := get("/categories/instructions/articles?cursor=50&current=article-054")
	if !active.MatchString(next.Body.String()) {
		t.Fatal("current article lost when loading more")
	}
	home := get("/categories/instructions/articles")
	if strings.Contains(home.Body.String(), `aria-current="page"`) {
		t.Fatal("home menu must not mark an article")
	}
}

func TestArticleNavigationLinks(t *testing.T) {
	_, get := navigationFixture(t, 3)
	w := get("/articles/article-001")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, pattern := range []string{`<nav[^>]*aria-label="Навигация по статьям"`, `<a[^>]*href="/articles/article-000"[^>]*rel="prev"`, `<a[^>]*href="/articles/article-002"[^>]*rel="next"`} {
		if !regexp.MustCompile(pattern).MatchString(body) {
			t.Fatalf("missing article navigation %s: %s", pattern, body)
		}
	}
	if strings.Index(body, `aria-label="Навигация по статьям"`) < strings.Index(body, "</article>") {
		t.Fatal("navigation must follow the article")
	}
}

func TestCategoryListingRedirectsHome(t *testing.T) {
	_, get := navigationFixture(t, 21)
	for _, path := range []string{"/?category=instructions", "/?category=missing&page=2", "/?category="} {
		w := get(path)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
			t.Fatalf("old category URL %s must return home: %d %s", path, w.Code, w.Header().Get("Location"))
		}
	}
	first := get("/")
	second := get("/?page=2")
	if first.Code != 200 || second.Code != 200 || strings.Count(first.Body.String(), `class="article-card"`) != 20 || strings.Count(second.Body.String(), `class="article-card"`) != 1 {
		t.Fatalf("home pagination broken: %d / %d", first.Code, second.Code)
	}
	for _, body := range []string{first.Body.String(), second.Body.String(), get("/articles/article-000").Body.String()} {
		if strings.Contains(body, `href="/?category=`) || strings.Contains(body, "&amp;category=") || strings.Contains(body, "Сбросить категорию") {
			t.Fatal("category listing link remains")
		}
	}
}
