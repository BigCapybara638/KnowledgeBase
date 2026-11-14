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
)

func TestCreateDraftCategory(t *testing.T) {
	for _, tt := range []struct {
		name, path, category, wantCategory string
		wantStatus                         int
	}{
		{"selected category", "/drafts", "second", "second", http.StatusSeeOther},
		{"no category", "/drafts", "", "", http.StatusSeeOther},
		{"missing category", "/drafts", "missing", "", http.StatusBadRequest},
		{"existing article keeps category", "/articles/existing/drafts", "second", "first", http.StatusSeeOther},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, accounts, user := authFixture(t)
			if _, err := a.store.Write.Exec("INSERT INTO categories(id,title) VALUES('first','Первая'),('second','Вторая')"); err != nil {
				t.Fatal(err)
			}
			if _, err := a.store.Write.Exec(`INSERT INTO articles(id,category_id,author_id,title,published_at) VALUES('existing','first',?,'Existing',1)`, user.ID); err != nil {
				t.Fatal(err)
			}
			session, err := accounts.Login(context.Background(), user.Login, "Testing-password-123!")
			if err != nil {
				t.Fatal(err)
			}
			cookie := &http.Cookie{Name: "kb_session", Value: session.RawToken}
			get := httptest.NewRequest("GET", "https://kb.test/", nil)
			get.AddCookie(cookie)
			page := httptest.NewRecorder()
			a.ServeHTTP(page, get)
			token := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(page.Body.String())
			if page.Code != 200 || len(token) != 2 {
				t.Fatalf("cannot open home: %d %s", page.Code, page.Body.String())
			}
			form := url.Values{"csrf_token": {html.UnescapeString(token[1])}, "category_id": {tt.category}}
			request := httptest.NewRequest("POST", "https://kb.test"+tt.path, strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", "https://kb.test")
			request.AddCookie(cookie)
			for _, c := range page.Result().Cookies() {
				request.AddCookie(c)
			}
			response := httptest.NewRecorder()
			a.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			drafts, err := a.articles.Drafts(context.Background(), user.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantStatus == http.StatusSeeOther {
				if len(drafts) != 1 || string(drafts[0].CategoryID) != tt.wantCategory {
					t.Fatalf("draft must persist selected category %q: %+v", tt.wantCategory, drafts)
				}
			} else {
				var articles int
				if err := a.store.Read.QueryRow("SELECT count(*) FROM articles").Scan(&articles); err != nil {
					t.Fatal(err)
				}
				if len(drafts) != 0 || articles != 1 {
					t.Fatalf("invalid category must not leave a draft or placeholder: %d drafts, %d articles", len(drafts), articles)
				}
			}
		})
	}
}
