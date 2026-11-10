package articles_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

func TestArticleNeighbors(t *testing.T) {
	s, user, _ := seed(t, 60)
	ctx := context.Background()
	a := articles.New(s)
	for _, tc := range []struct{ id, previous, next domain.ID }{
		{"article-000", "", "article-001"},
		{"article-001", "article-000", "article-002"},
		{"article-049", "article-048", "article-050"},
		{"article-059", "article-058", ""},
	} {
		t.Run(string(tc.id), func(t *testing.T) {
			n, err := a.Neighbors(ctx, user, tc.id)
			if err != nil || n.Previous.ID != tc.previous || n.Next.ID != tc.next {
				t.Fatalf("neighbors: %+v, %v; want %s / %s", n, err, tc.previous, tc.next)
			}
		})
	}
	for _, q := range []string{
		"UPDATE articles SET archived=1 WHERE id='article-001'",
		"UPDATE articles SET published_at=NULL WHERE id='article-002'",
		"INSERT INTO categories(id,title) VALUES('other','Other')",
		"UPDATE articles SET category_id='other' WHERE id='article-003'",
	} {
		if _, err := s.Write.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	n, err := a.Neighbors(ctx, user, "article-000")
	if err != nil || n.Previous.ID != "" || n.Next.ID != "article-004" || n.Next.Title != "Статья 004" {
		t.Fatalf("neighbors must skip hidden and other-category articles: %+v %v", n, err)
	}
	n, err = a.Neighbors(ctx, user, "article-003")
	if err != nil || n.Previous.ID != "" || n.Next.ID != "" {
		t.Fatalf("single article must have no neighbors: %+v %v", n, err)
	}
	for _, id := range []domain.ID{"article-001", "article-002", "missing"} {
		if _, err := a.Neighbors(ctx, user, id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("hidden/missing article %s: %v", id, err)
		}
	}
	if _, err := s.Write.Exec("UPDATE users SET disabled=1 WHERE id=?", user); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Neighbors(ctx, user, "article-000"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("disabled user must not read neighbors: %v", err)
	}
}

func TestArticleNeighborsUseMenuOrder(t *testing.T) {
	s, user, category := seed(t, 3)
	for i, title := range []string{"alpha", "Alpha", "Beta"} {
		id := []string{"article-000", "article-001", "article-002"}[i]
		if _, err := s.Write.Exec("UPDATE articles SET title=? WHERE id=?", title, id); err != nil {
			t.Fatal(err)
		}
	}
	a := articles.New(s)
	menu, err := a.ListCategoryMenu(context.Background(), user, category, "", "")
	if err != nil || len(menu.Items) != 3 || menu.Items[0].ID != "article-000" || menu.Items[1].ID != "article-001" {
		t.Fatalf("menu order: %+v %v", menu, err)
	}
	n, err := a.Neighbors(context.Background(), user, "article-001")
	if err != nil || n.Previous.ID != "article-000" || n.Next.ID != "article-002" {
		t.Fatalf("neighbors must follow case-insensitive title and ID ordering: %+v %v", n, err)
	}
}
