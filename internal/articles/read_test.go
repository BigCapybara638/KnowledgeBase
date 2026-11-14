package articles_test

import (
	"context"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"testing"
)

func seed(t *testing.T, n int) (*storage.Store, domain.ID, domain.ID) {
	t.Helper()
	s := testutil.Store(t)
	u := testutil.User(t, s, "user")
	c := domain.NewID()
	if _, e := s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,?)", c, "Инструкции"); e != nil {
		t.Fatal(e)
	}
	for i := range n {
		_, e := s.Write.Exec("INSERT INTO articles(id,category_id,author_id,last_editor_id,title,plain_text,version,published_at,updated_at) VALUES(?,?,?,?,?,?,1,1,?)", fmt.Sprintf("article-%03d", i), c, u, u, fmt.Sprintf("Статья %03d", i), "Содержание", i)
		if e != nil {
			t.Fatal(e)
		}
	}
	return s, u, c
}
func TestRenameKeepsArticleURL(t *testing.T) {
	s, u, _ := seed(t, 1)
	a := articles.New(s)
	first, e := a.Get(context.Background(), u, "article-000")
	if e != nil {
		t.Fatal(e)
	}
	s.Write.Exec("UPDATE articles SET title='Другое название' WHERE id='article-000'")
	second, e := a.Get(context.Background(), u, first.ID)
	if e != nil || second.ID != first.ID || second.Title != "Другое название" {
		t.Fatalf("unstable article: %+v %v", second, e)
	}
}
func TestCardsExcludeUnpublishedAndArchived(t *testing.T) {
	s, u, _ := seed(t, 3)
	s.Write.Exec("UPDATE articles SET published_at=NULL WHERE id='article-001'")
	s.Write.Exec("UPDATE articles SET archived=1 WHERE id='article-002'")
	p, e := articles.New(s).List(context.Background(), u, nil, 1)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 1 || p.Items[0].ID != "article-000" {
		t.Fatal(p)
	}
}
func TestCategoryFilterAndStablePagination(t *testing.T) {
	s, u, c := seed(t, 25)
	a := articles.New(s)
	one, e := a.List(context.Background(), u, &c, 1)
	if e != nil {
		t.Fatal(e)
	}
	two, e := a.List(context.Background(), u, &c, 2)
	if e != nil {
		t.Fatal(e)
	}
	if len(one.Items) != 20 || !one.HasNext || len(two.Items) != 5 || two.HasNext {
		t.Fatal(one, two)
	}
	seen := map[domain.ID]bool{}
	for _, i := range append(one.Items, two.Items...) {
		if seen[i.ID] {
			t.Fatal("duplicate page row")
		}
		seen[i.ID] = true
	}
	other := domain.NewID()
	p, e := a.List(context.Background(), u, &other, 1)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
}
func TestSidebarPagesBy50(t *testing.T) {
	s, u, c := seed(t, 55)
	a := articles.New(s)
	p, e := a.ListCategoryMenu(context.Background(), u, c, "", "")
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Items) != 50 || p.Next == "" {
		t.Fatal(p)
	}
	next, e := a.ListCategoryMenu(context.Background(), u, c, p.Next, "")
	if e != nil || len(next.Items) != 5 || next.Next != "" {
		t.Fatal(next, e)
	}
}
